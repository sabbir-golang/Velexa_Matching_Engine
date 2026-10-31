# Operations

This page is for whoever deploys and runs the engine. It covers building and starting it, the ports it uses, the nginx front ends (the HTTPS reverse proxy and the external FIX gateway), onboarding brokers from the admin console, logs and health checks, and troubleshooting. Testing is described in [testing](testing.md). For individual settings, see [configuration](configuration.md).

To run the engine with Docker (the recommended way on a new PC or server), see [Running with Docker](docker.md). The rest of this page also applies there, except where it names files and commands that Docker replaces.

## Build and run

```bash
make build               # produces bin/velexa-sim-me (go build -o bin/velexa-sim-me .)
./bin/velexa-sim-me
```

The engine reads environment variables (see [configuration](configuration.md#environment-variables)) and, at start-up, the env file `me.env` in the working directory (or `ME_ENV_FILE`). So `go run .` or `make run` in the repo picks up `me.env` directly; variables exported in the shell still win over the file:

```bash
mkdir -p logs && ./bin/velexa-sim-me 2>&1 | tee -a logs/me.log      # uses ./me.env (written on the first run)
PORT=18010 ./bin/velexa-sim-me                      # one-off override of a me.env value
```

The repo contains no systemd unit, Dockerfile or process supervisor; set up your own.

A healthy start logs lines like these:

```text
[universe] loaded 350 instruments from /opt/velexa-sim-me/instruments.json
[FIX] logon auth enabled=true registered_brokers=12
[FIX] acceptor listening on :5001  FIXT.1.1 SenderCompID=DSEME (DynamicSessions)
[ITCH] feed auth enabled=true registered_users=12
[ITCH] SoupBinTCP market feed listening on :5501 (350 instruments)
Velexa Matching Engine on :8010  (350 instruments from file:…, auto-list=true, phase=Continuous Trading)
```

A restart keeps the market (see [What survives a restart](#what-survives-a-restart)). FIX sequence numbers reset on every logon, so brokers can reconnect straight away.

## What survives a restart

The live market is saved every second and once more when the engine stops, and put back when it starts. The start-up log says what came back:

```text
[state] restored the market saved 2026-10-19 15:02:11 (400ms ago): 37 resting orders, 4 stops, 11 instruments, 812 trades on the tape, last trade #812, phase Continuous Trading
[state] 37 restored FIX orders report to their brokers again
```

| Kept | Notes |
|---|---|
| Every resting order, in its queue position | Time priority is unchanged; filled quantity, iceberg reserve, validity, client, account and the order's OrderID (37) are kept. Brokers can cancel and amend them by ClOrdID as before, and get their fills. |
| Parked stop orders | Still trigger on the last price. |
| Last price, open, high, low, volume, trade count | Bid and ask follow from the restored book. |
| The trade tape, and every trade ever | The tape holds the latest 500; the trade log (`trade_log` table or `trades/`) holds all of them with both sides. |
| Order and trade numbers | Continue; a trade number (880, ITCH match number) or OrderID is never handed out twice. |
| The session phase and the clock setting | A restart during Pre-Open stays in Pre-Open. If the engine was down over the close, the start runs that day's close first: closing prices, expiry of day orders (reported to the brokers), and the next day opens from the new reference prices. |
| Reports for brokers that are logged off | Kept in order and sent when the broker logs on again, for up to 7 days. |

- **How much can be lost.** A clean stop (`docker compose stop`, `make docker-up`, an Air reload, `SIGTERM`) loses nothing. A power cut or `kill -9` can lose at most the last second.
- **The dashboard** shows "market saved" next to the uptime, or how long ago the last save was if saving stalls (for example the database is down; it is retried every second). The status API has `state_saved`, `state_age`, `state_pending_trades` and, for admins, `fix_queued` per broker.
- **Starting empty on purpose.** Stop the engine and delete the `market` row (`DELETE FROM engine_state WHERE name = 'market'`) or `engine_state.json`. `ME_MARKET_STATE=off` neither saves nor restores.
- **An unreadable saved market** is set aside (`engine_state.json.bad-<time>`, or the row `market.bad-<time>`) and the engine starts empty, with a log line starting `[state] !!!`. If the database can't be read at all, the engine does not start, so an empty market never overwrites a saved one.
- **Instruments that are no longer listed**: their saved orders are dropped, and the log says how many.
- **Orders the order log shows as working but that are not on the restored book** (the market could not be restored, or `ME_MARKET_STATE=off`) are marked Cancelled with the reason "not on the book after an engine restart", with a `[orders]` log line each, so the Order Report and dashboard never show an order working that cannot trade.

## Ports

| Port | Owner | Purpose |
|---|---|---|
| 8010 (`PORT`) | engine | Web UI, JSON API, WebSocket streams, SSE, admin console |
| 5001 (`FIX_PORT`) | engine | FIX acceptor |
| 5501 (`ITCH_PORT`) | engine | SoupBinTCP ITCH feed |
| 80 / 443 | nginx | Reverse proxy to 8010 (`deploy/nginx-443.patch`) |
| 5443 | nginx | External FIX gateway, TCP proxy to 5001 (`deploy/regen-fixs-gateway.sh`) |

## nginx front ends

### HTTPS reverse proxy (`deploy/nginx-443.patch`)

`deploy/nginx-443.patch` is a unified diff against `/etc/nginx/sites-available/matching-engine`. It also creates `/etc/nginx/snippets/matching-engine-proxy.conf`. It does three things:

- Moves the proxy `location /` block into that snippet, which both the port 80 and port 443 servers include. The block:
  - proxies to `127.0.0.1:8010` over HTTP/1.1
  - passes the WebSocket `Upgrade` and `Connection` headers
  - sets `Host`, `X-Real-IP`, `X-Forwarded-For` and `X-Forwarded-Proto`
  - turns off buffering and caching, which SSE and the live feeds need
  - sets 24-hour read and send timeouts
- Adds a `listen 443 ssl http2 default_server` server that uses a Cloudflare Origin Certificate at `/etc/ssl/velexa/origin.pem` and `/etc/ssl/velexa/origin.key`, with TLS 1.2 and 1.3.
- Includes a commented-out Cloudflare Authenticated Origin Pulls block. Its own comment says to enable it, once the CA file has been downloaded, so that port 443 can't be reached without going through Cloudflare.

On purpose, there is no redirect from port 80 to 443, so Cloudflare works in both Flexible and Full SSL modes.

The snippet uses `$connection_upgrade`, which the patch doesn't define. Your nginx config must already have that `map`. To apply the patch:

```bash
cd / && sudo patch -p1 --dry-run < /path/to/nginx-443.patch   # check first
cd / && sudo patch -p1 < /path/to/nginx-443.patch
sudo nginx -t && sudo systemctl reload nginx
```

Tell the engine about the proxy, or sign-in lockouts and the audit log see nginx's address instead of the client's:

- `ME_TRUSTED_PROXIES` — nginx's address as the engine sees it (the default `127.0.0.1,::1` fits nginx on the same box). Behind Cloudflare, add Cloudflare's published IP ranges too (https://www.cloudflare.com/ips/), comma-separated; the engine then takes the right-most `X-Forwarded-For` hop that is not a trusted proxy as the client. Forwarding headers from any other peer are ignored.
- `ME_COOKIE_SECURE=on` — when users reach the console over HTTPS. Session and CSRF cookies get `Secure` (the session cookie becomes `__Host-me_admin`) and responses carry HSTS. Without it, the engine still marks cookies `Secure` when a trusted proxy sends `X-Forwarded-Proto: https`.
- Keep `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;` as in the snippet; do not let nginx pass a client's `X-Forwarded-For` through any other way.

### External FIX gateway (port 5443)

Set `ME_FIXS_PORT=5443` in the engine's `me.env` when this gateway runs, so broker sheets point brokers at it (with TLS instructions) and the dashboard checks it. `deploy/regen-fixs-gateway.sh` always allows loopback and this host (`FIXS_HOST_IP`, default the first address from `hostname -I`). Outside brokers connect to port 5443, where an nginx `stream` server forwards raw TCP to the acceptor on `127.0.0.1:5001`. `deploy/regen-fixs-gateway.sh` generates `/etc/nginx/stream-enabled/fixs-gateway.conf` from the IP allowlist:

- Each IP in `fix_allow.json` (`FIXS_ALLOW_FILE`) gets an `allow` line, followed by `deny all`. Loopback and one host address written into the script are always allowed.
- An **empty allowlist means `allow all`**, matching what the admin console says.
- Connections are logged to `/var/log/nginx/fixs-gateway.access.log` and `/var/log/nginx/fixs-gateway.error.log`.
- The gateway only filters by IP. Every FIX session still has to authenticate with its CompID and password (tag 554) when `ME_FIX_AUTH=on`.

The script needs `python3`, root, and an nginx config that loads `/etc/nginx/stream-enabled/` inside a `stream {}` block and defines a log format called `basic`. Neither of those nginx settings is in this repo.

**Changing the allowlist:**

1. Add or remove the IP in the admin console under **External OMS Access**. This only writes `fix_allow.json`.
2. Apply it to nginx:

   ```bash
   sudo ~/matching-engine/regen-fixs-gateway.sh
   ```

   The script runs `nginx -t`, reloads nginx and prints the active `allow` and `deny` lines.

The generated config is plain TCP; it does no TLS. The broker onboarding sheet, however, labels this endpoint "FIXS external (TLS)".

## Admin console

Open `http://<host>:8010/admin` (or the proxied URL) and sign in with `ME_ADMIN_USER` / `ME_ADMIN_PASS` (the first `admin`) or any account created on the **Users** page. Roles are `admin` (the whole exchange) and `user` (a private workspace: only the brokers they register, with their credentials, FIX sessions, message log and dashboard figures; never other users' data). Anyone can register on the welcome page and starts with an empty workspace. Sessions last at most 8 hours and end after 30 minutes without use; **Sign out** ends every session of the account. Disabling, deleting or resetting the password of a user signs them out at once; a role change applies on their next request. The last enabled admin cannot be deleted, disabled or demoted. With `ME_RATE_LIMIT=on` (off by default), 5 wrong passwords lock that client IP — and, separately, that username — for 15 minutes. Passwords need 10–128 characters and must mix at least 3 of lower-case, upper-case, digits and symbols. If `ME_ADMIN_PASS` is not set, the first start creates the `admin` account with a random password and writes it to `initial-admin-password` in the state directory.

| View | What you can do |
|---|---|
| Welcome | The first page after sign-in. A greeting with the Dhaka date, the market phase with a live clock and what comes next ("closes in 2h 10m", or "clock manual"), and four cards: engine, FIX sessions logged on out of those registered, trades and turnover today (against the previous trading day). **Needs attention** lists what is wrong now: registered FIX brokers that are not logged on, and a high reject rate today; for admins also listeners that are down, the market not saved for over 30 s, halted instruments, reports waiting for a logged-off broker and a manual clock. Each can be dismissed for the browser session (it comes back if it changes) or opened. **Quick actions** jump to the pages your role has; **Recent activity** shows the latest audit entries (a user sees their own). |
| Dashboard | Engine phase, counters, uptime, whether FIX/ITCH auth is on, listener checks, instrument universe source. Refreshes every 3 s. **Daily report**: orders received (buy/sell), accepted, rejected (with the reject rate), cancelled, amended, expired, trades executed, volume and turnover for any date — today's live, earlier days from the saved history — with Download CSV and Print. Admins see the whole exchange, users only their own brokers. |
| Trade Terminal | The `/terminal` trade page embedded: market watch, order book, tape and manual order entry. |
| Order Report | Every order of a day — time, broker, OrderID, client, symbol, side, type, quantity, price, filled, status, board; click a row for the BO account, ClOrdID, trader, validity, average price and reject reason. Filter by broker, symbol, side, status or an ID; download **Excel** (IDs kept as text) or **CSV**. Admins see every broker, users only their own. |
| FIX Sessions | Connected brokers with logon time and sequence numbers. Force a disconnect or reset sequence numbers. |
| Market Session | The session phase and the DSE clock. Force Pre-Open, Open, Close, Post-Close or End of day (switches the clock to manual), or switch back to the schedule. |
| Broker Credentials | Issue, view and revoke broker FIX and ITCH credentials, and print onboarding sheets. |
| External OMS Access | Edit the port 5443 IP allowlist. |
| Simulator | Market bots that trade through your OMS, and a FIX benchmark (admins); see [Simulator](#simulator). |
| Message Log | Search FIX messages by broker and ClOrdID. |
| Diagnostics | Auth state, logon/logout/reject events, and the audit log. |

Every credential change, allowlist change, session control and instrument reload or import goes into the audit log (`ME_AUDIT_FILE`).

### Onboarding a broker

1. Under **Broker Credentials**, enter the broker's SenderCompID. Company, contact, FIX password, ITCH username and ITCH password are optional; blank passwords are generated, and the ITCH username defaults to the first 6 characters of the SenderCompID.
2. The engine writes `fix_brokers.json`, `itch_brokers.json` and `broker_meta.json` straight away. No restart is needed.
3. Print or download the onboarding sheet. It fills in `FIX_SENDER` as the TargetCompID, the FIX version (`ME_FIX_BEGIN` / `ME_FIX_APPLVER`), and the endpoints: this PC's LAN IP and the real FIX/ITCH ports, unless the `ME_PUB_*`, `ME_INT_*`, `ME_ITCH_*` or `ME_FIXS_PORT` variables say otherwise. Set those when brokers connect through a public IP, NAT or the TLS gateway.
4. If the broker connects from outside, add its OMS IP under **External OMS Access** and run `deploy/regen-fixs-gateway.sh`.
5. Once the first broker exists, FIX logons are checked against `fix_brokers.json` (`ME_FIX_AUTH=on` checks even with no brokers, `off` never checks). ITCH logins are checked only with `ME_ITCH_AUTH=on`.

Revoking a broker removes its credentials and metadata. It doesn't disconnect a live session; use **FIX Sessions → disconnect** for that.

### Managing instruments

The dashboard and `/admin/api/instruments` show where the instrument universe came from and which symbols had to be auto-listed. To refresh it without restarting:

- **Reload** reads `ME_INSTRUMENT_FILE` and then `EXCHANGE_INSTRUMENT_URL` again.
- **Import** (Instruments page, next to Export) reads a DSE end-of-day ticker file (XML, as the DSE publishes it), a CSV/TSV with a header row (**Download a CSV template** in the dialog), or a JSON master. Choose **Add new + update listed**, **Add new only** or **Update listed only**; the dialog previews what is new, what changes (reference prices moving more than 20% are flagged, because the daily band moves with them) and what is skipped and why, before anything is applied. A blank field keeps the listed value, so the DSE file (no names, lots or ticks) never wipes them. Instruments not in the file stay listed, and the result is saved to `ME_INSTRUMENT_FILE`.

```bash
curl -s -b jar -X POST localhost:8010/admin/api/instruments/reload
```

See [http-api](http-api.md#logging-in) for creating the cookie jar.

### Simulator

The **Simulator** page (admins) sends test traffic to the engine. Nothing runs until you start it.

- **Market bots** trade *through a broker's OMS*, so their orders show in the OMS's Blotter, Execution, Order Summary, Positions and Purchase Power, like a dealer's. An OMS lists only orders placed through it, as on the real exchange where a broker's blotter never shows other brokers' orders, so the bots sign in to the OMS rather than to the engine.
  - **Sign-in.** Enter the OMS address traders use (e.g. `https://jbl.bdtaskoms.com`; the tenant is its first label, and the API is `https://api.<domain>`) and an OMS **dealer** login without a one-time code (MFA). With **Remember** ticked (the default), a sign-in the OMS accepts is saved on the engine in `sim_oms.json` in the data folder (mode 0600, like the broker credential files), so the fields are filled in next time and Start works with the password left blank; the page shows the saved login and when it was saved, never the password, and **Forget** deletes it. A blank password only uses the saved one for the same login on the same OMS. The password is never logged or audited. The engine must be able to reach the OMS API over HTTPS.
  - **Clients.** They trade for the first 4, 8, 12 or 20 of the dealer's clients, or for client codes you list (*My list*); a client that is not in the dealer's list goes in as `CODE/BO-ID`.
  - **Orders.** At 30–600 orders a minute they place, amend and cancel limit orders on the dealer's OMS order channel, exactly as the OMS web front end sends them: a few ticks around a moving fair price, or an IOC order at the best price, inside the daily band and on the tick. The orders pass the OMS risk checks and reach the engine through the broker's FIX session, so they trade with each other and with anyone else in the book. A buy is worth at most **Largest buy** (default Tk 20,000) and needs the client's cash; sells only use shares the OMS's position stream says the client holds (a client whose stream does not open only buys). Each client keeps at most 6 orders resting; the oldest is cancelled. They trade only in Continuous Trading.
  - **Pause** stops sending; their orders stay in the book. **Stop** also leaves their orders in the book, and the bots stay signed in and follow the OMS's reports, so the depth and the Blotter stay as they were. **Clean Data** (once stopped) cancels their orders through the OMS, so the Blotter shows them cancelled, and signs out; it also cancels, in the engine, any OMS-bot order no run follows any more (an *orphan*, counted on the card: ClOrdID `SON…` or `SOA…`), sending the cancel report under the order's own ClOrdID so the OMS marks it cancelled and frees its cash, and any order the old engine-only market bots (`SIMBOT…`) left; When the OMS does not confirm every cancel, the bots stay signed in so Clean Data can be tried again. **Start** cancels a stopped run's orders through the OMS before trading again, rather than doubling the book. A sign-in cannot outlive the engine, so an engine shutdown cancels their orders through the OMS and signs out.
  - Their orders are the broker's own, so they stay in the order report. OMS refusals (cash, holdings, limits) appear under Reject reasons as `OMS: …`, and the dealer's open OMS screens see the bots' acknowledgements and fills.
- **Benchmark** logs on to the engine's FIX gateway as `SIMBENCH1` and `SIMBENCH2` and sends orders at a set rate or as fast as the gateway answers (MAX, at most 1,000 waiting) for 5 s to 2 min. *Trades*: SIMBENCH1 sells and SIMBENCH2 buys it with an IOC order, so every pair trades; pick a share the market bots are not trading. *Orders only*: IOC buys at the bottom of the band, accepted and cancelled. It shows orders per second, sent / acknowledged / trades / rejected, and latency (avg, p50, p95, p99) from sending an order to its acknowledgement and to its first fill. At MAX the latency includes the queue of waiting orders; a set rate shows the engine's own.
- **Live order tape** shows the latest bot orders and fills (market-bot rows are marked `OMS <client>`; the benchmark is sampled), and **Reject reasons** the commonest refusals.
- **Clean test data** stops everything, cancels every simulator order still in the book (the market bots' through the OMS, signing them out) and removes the benchmark's and old engine-only bots' rows from the order report (every day).

Bot orders are real orders: they trade, move prices, go out on the ITCH feed (connected OMS screens see them) and are counted in the trade log and the daily report. Cleaning does not undo trades. The benchmark's logons exist only in memory and are never listed under Broker Credentials; its sessions are not written to the message log, and reports for a session that has logged out are dropped instead of being kept for it. A benchmark order still in the book when the engine restarts is cancelled at start-up. Start, stop and clean are in the audit trail (`SIM …`).

## Database

Console accounts and the audit trail go to PostgreSQL when `ME_DB_URL` is set (see [configuration](configuration.md#database)); otherwise they stay in files and none of this applies.

**On this PC:**

```bash
make db          # creates/starts the container velexa_me_pg (postgres:16-alpine, 127.0.0.1:5435,
                 # volume velexa_me_pg_data, random password) and adds ME_DB_URL to me.env
# then restart the engine: it imports users.json and admin_audit.jsonl once and logs
#   [db] connected to velexa_me@127.0.0.1:5435/velexa_me (schema v1): N accounts
make db-psql     # SQL shell:  select username, role, disabled, last_login_at from users;
make db-backup   # pg_dump to backups/velexa_me-YYYYMMDD-HHMM.sql.gz
```

The container restarts with Docker (`--restart unless-stopped`). Running `make db` again only starts it; the password never changes. To start over, remove the `ME_DB_URL` line from `me.env` and run `docker rm -f velexa_me_pg && docker volume rm velexa_me_pg_data`.

**Restore a backup** (stop the engine first):

```bash
gunzip -c backups/velexa_me-20261017-1200.sql.gz | docker exec -i velexa_me_pg psql -U velexa_me -d velexa_me
```

To restore into a fresh database, drop and recreate `velexa_me` first.

**On a server,** point `ME_DB_URL` at that server's PostgreSQL with a user that owns one database for the engine, for example `postgres://velexa_me:…@db-host:5432/velexa_me?sslmode=require`. Run one engine per database, and back it up like any other production database.

## Logs and files

| What | Where | Notes |
|---|---|---|
| Process log | stderr | Go `log` lines prefixed `[FIX]`, `[ITCH]`, `[universe]`, `[session]`. Redirect it yourself. `*.log` files are git-ignored. |
| FIX message log | `ME_FIX_MSGLOG_FILE` | Every message except heartbeats. Never rotated or trimmed by the engine; `.gitignore` notes it once grew to 9.6 GB. The engine keeps the file open in append mode, so if you rotate it externally, use copy-and-truncate. |
| Admin audit log | `audit_log` table, or `ME_AUDIT_FILE` without a database | One JSON object per line in the file. With a database the file only appears if an insert failed; it is imported at the next start. |
| nginx | `/var/log/nginx/matching-engine.*.log`, `/var/log/nginx/fixs-gateway.*.log` | From the nginx configs above. |

## Health checks

| Check | Healthy result |
|---|---|
| `curl -s localhost:8010/healthz` | `ok` |
| `curl -s localhost:8010/health` | `"ok":true`, with the current `phase` |
| `curl -s localhost:8010/api/status` | `universe.source` is your master file or the exchange, not `built-in DSE reference list` |
| Admin dashboard → listeners | FIX, ITCH and Gateway open (and FIXS when `ME_FIXS_PORT` is set) |

The dashboard checks the ports the engine actually uses on `127.0.0.1` (`FIX_PORT`, `ITCH_PORT`, `PORT`), plus `ME_FIXS_PORT` when a TLS FIX gateway is configured.

## Troubleshooting

| Symptom | Likely cause and fix |
|---|---|
| Orders rejected as `unknown symbol … (built-in DSE reference list …)` | No master was loaded. Provide `ME_INSTRUMENT_FILE` or a reachable `EXCHANGE_INSTRUMENT_URL`, or leave `ME_AUTO_LIST` on. Reload from the admin console. |
| Startup stalls for about 12 s at `[universe]` | `EXCHANGE_INSTRUMENT_URL` can't be reached; its compiled-in default is a deployment-specific address. Set it to a reachable URL, or provide an instrument file. |
| `[FIX] acceptor start failed: problem parsing XML datadictionary` | `ME_FIX_SPEC_DIR` doesn't contain `FIXT11.xml` and `FIX50SP2.xml`. Point it at the repo's `spec/`, or set `ME_FIX_DD=off`. |
| Broker logon refused | Look in Diagnostics or the log for `logon REJECTED`. The reason says whether the broker isn't registered, the password is wrong, or 553 doesn't match the CompID. |
| FIX orders rejected `off-tick` or `price band` | The price isn't on the instrument's tick, or is outside the daily band (see [Pre-trade checks](architecture.md#pre-trade-checks-fix-orders-only)). An import with the real tick and previous close fixes auto-listed symbols. |
| Market orders rejected during the auction | Pre-Open accepts only limit orders. Open the market first. |
| Every order rejected `(293): Orders are not being accepted at this time` | The market is closed: outside the DSE schedule (`ME_SESSION_SCHEDULE`, Dhaka time, Sunday–Thursday) or after End of day. Open it from the admin **Market Session** page, or start with `ME_SESSION_AUTO=off`. |
| Books are empty and prices don't move | Expected: there are no simulated traders. Prices move only when real orders trade. |
| External broker can't connect on 5443 | Its IP isn't allowed, or `deploy/regen-fixs-gateway.sh` wasn't run after editing the allowlist. |
| Browser shows no live updates through nginx | Proxy buffering is on. The snippet in `deploy/nginx-443.patch` turns it off. |
| Engine exits with `[db] cannot connect to PostgreSQL …` | `ME_DB_URL` is set but the database isn't reachable: `docker start velexa_me_pg` (or `make db`), check the host, port and password in `me.env`. The engine refuses to run on files instead, so the data never splits. |
| Console shows "the change could not be saved" | The database refused or lost a write; the change was undone. See the `[users]` log line for the database error. |
