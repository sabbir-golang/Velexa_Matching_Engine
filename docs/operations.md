# Operations

This page is for whoever deploys and runs the engine. It covers building and starting it, the ports it uses, the nginx front ends (the HTTPS reverse proxy and the external FIX gateway), onboarding brokers from the admin console, the two load tests, logs and health checks, and troubleshooting. For individual settings, see [configuration](configuration.md).

## Build and run

```bash
make build               # produces bin/velexa-sim-me (go build -o bin/velexa-sim-me .)
./bin/velexa-sim-me
```

The engine reads only environment variables (see [configuration](configuration.md#environment-variables)). If you keep them in an env file such as the git-ignored `me.env`, export it into the shell before starting:

```bash
set -a; . ./me.env; set +a
./bin/velexa-sim-me 2>&1 | tee -a logs/me.log
```

The repo contains no systemd unit, Dockerfile or process supervisor; set up your own.

A healthy start logs lines like these:

```text
[universe] loaded 350 instruments from /opt/velexa-sim-me/instruments.json
[FIX] logon auth enabled=true registered_brokers=12
[FIX] acceptor listening on :5001  FIXT.1.1 SenderCompID=DSEME (DynamicSessions)
[ITCH] feed auth enabled=true registered_users=12
[ITCH] SoupBinTCP market feed listening on :5501 (350 instruments)
Velexa Simulator Matching Engine on :8010  (350 instruments from file:…, auto-list=true, phase=Continuous Trading)
```

Restarting clears all books, orders and trades (see [Persistence](architecture.md#persistence)). FIX sequence numbers reset on every logon, so brokers can reconnect straight away.

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

The admin login cookie is marked `Secure` when nginx sends `X-Forwarded-Proto: https`.

### External FIX gateway (port 5443)

Outside brokers connect to port 5443, where an nginx `stream` server forwards raw TCP to the acceptor on `127.0.0.1:5001`. `deploy/regen-fixs-gateway.sh` generates `/etc/nginx/stream-enabled/fixs-gateway.conf` from the IP allowlist:

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

Open `http://<host>:8010/admin` (or the proxied URL) and sign in with `ME_ADMIN_USER` / `ME_ADMIN_PASS` (the first `admin`) or any account created on the **Users** page. Roles are `admin` (full access) and `user` (read-only; FIX passwords are masked in the message log). Sessions last 8 hours; disabling a user or changing a role applies on their next request. The last enabled admin cannot be deleted, disabled or demoted. After 5 wrong passwords, that client IP is locked out for 5 minutes.

| View | What you can do |
|---|---|
| Dashboard | Engine phase, counters, uptime, whether FIX/ITCH auth is on, listener checks, instrument universe source. Refreshes every 3 s. |
| Trade Terminal | The `/` trade page embedded: market watch, order book, tape and manual order entry. |
| FIX Sessions | Connected brokers with logon time and sequence numbers. Force a disconnect or reset sequence numbers. |
| Simulator | The `/bench` page embedded. Also pause or resume the bots and switch between Pre-Open and open. |
| Broker Credentials | Issue, view and revoke broker FIX and ITCH credentials, and print onboarding sheets. |
| External OMS Access | Edit the port 5443 IP allowlist. |
| Message Log | Search FIX messages by broker and ClOrdID. |
| Diagnostics | Auth state, logon/logout/reject events, and the audit log. |

Every credential change, allowlist change, session control and instrument reload or import goes into the audit log (`ME_AUDIT_FILE`).

### Onboarding a broker

1. Under **Broker Credentials**, enter the broker's SenderCompID. Company, contact, FIX password, ITCH username and ITCH password are optional; blank passwords are generated, and the ITCH username defaults to the first 6 characters of the SenderCompID.
2. The engine writes `fix_brokers.json`, `itch_brokers.json` and `broker_meta.json` straight away. No restart is needed.
3. Print or download the onboarding sheet. It fills in `FIX_SENDER` as the TargetCompID, `FIXT.1.1` / ApplVerID 9, and the external, internal and ITCH endpoints from the `ME_PUB_*`, `ME_INT_*` and `ME_ITCH_*` variables. Set those variables to the real addresses before issuing any sheets.
4. If the broker connects from outside, add its OMS IP under **External OMS Access** and run `deploy/regen-fixs-gateway.sh`.
5. Credentials are only checked if `ME_FIX_AUTH=on` and `ME_ITCH_AUTH=on` are set.

Revoking a broker removes its credentials and metadata. It doesn't disconnect a live session; use **FIX Sessions → disconnect** for that.

### Managing instruments

The dashboard and `/admin/api/instruments` show where the instrument universe came from and which symbols had to be auto-listed. To refresh it without restarting:

- **Reload** reads `ME_INSTRUMENT_FILE` and then `EXCHANGE_INSTRUMENT_URL` again.
- **Import** uploads a master as JSON or CSV/TSV. It's saved to `ME_INSTRUMENT_FILE` only if it has at least as many instruments as are already listed, so a partial file can't shrink the universe on the next restart.

```bash
curl -s -b jar -X POST localhost:8010/admin/api/instruments/reload
```

See [http-api](http-api.md#logging-in) for creating the cookie jar.

## Load testing

### Synthetic pipeline benchmark

The synthetic benchmark measures the engine's own pipeline without the live market: order generation, then RMS checks (`loadtest/rms.go`) on a worker pool, then a single matching goroutine on books seeded with 300 levels per side, then optionally Postgres persistence. Its books are separate from the live ones, so trading isn't affected. The reported target is 8000 trades/sec.

**Prerequisites for persistence:** Postgres at `ME_DB_HOST:ME_DB_PORT` with a `velexa_sim` database containing the `sim_orders` and `sim_execs` tables. The columns are listed in [architecture](architecture.md#persistence). This repo doesn't create them. Without Postgres, run with `"persist": false`.

**Running it:** on `/bench`, choose the **Benchmark (synthetic)** mode, or run:

```bash
curl -s -X POST localhost:8010/api/bench/start -d '{"rate":8000,"total":200000,"rms":true,"persist":false}'
curl -s localhost:8010/api/bench/stats
curl -s -OJ localhost:8010/api/bench/summary.csv
```

With `persist: true`, each start truncates `sim_orders` and `sim_execs`. `/api/bench/orders.csv` exports the stored rows.

### Live load test

The live test sends real orders through the full external pipeline: OMS API (with its RMS) → Kafka/FIX initiator → this engine → broker blotter. It needs the whole OMS stack:

- **OMS API.** `ME_API_URL` accepts `POST` with the header `X-Internal-Token`. The token is compiled into `loadtest/live.go`.
- **FIX initiator.** Its control WebSocket is at `ME_FIX_WS`.
- **Session registration.** At start, the test sends a `logon` frame per broker to the FIX initiator. The frame contains the SenderCompID, the target `DSEME`, `FIXT.1.1`, host `ME_ADVERTISE_HOST`, port `5001` and the broker's FIX password if one is registered. The target and port are fixed in code; they don't follow `FIX_SENDER` or `FIX_PORT`.
- **Broker databases.** The OMS Postgres (`ME_OMS_*`) holds each broker's database. The test reads active traders (`broker_house_trader`, `broker_house_tradermarketsegmentauthorization`) and investors (`broker_house_investor`) from it. If there are none, it uses made-up IDs (`T01`–`T20`, `INV00001`…).
- **Broker list.** Comes from `brokers.tsv` (`LIVE_BROKERS_FILE`). If that file is missing or empty, three built-in brokers are used.
- **Sell orders.** Sells only use `GP`, `SQURPHARMA`, `BEXIMCO`, `ROBI` and `BATBC`, so investors must hold positions in those.

Orders are limit orders 2% through the last price, rounded to the instrument's tick, with client order IDs `LIVE-<subdomain>-<runID>-<seq>`. Start and watch the test from the **Live — real OMS orders** mode on `/bench` (the default), or through `/api/live/*` (see [http-api](http-api.md#live-load-test)).

> **Cleanup wipes data.** The **Clean test data** button on `/bench` and `/api/live/cleanup` truncate the order tables and reset intraday cash and positions in **every** `terminal_*` / `tenant_*` database on the OMS Postgres. They also truncate `client_order_routes` and the benchmark tables, and purge the audit service. They don't limit themselves to `LIVE-*` rows, and the endpoint needs no login. Never point the engine at an OMS Postgres that holds real data.

## Logs and files

| What | Where | Notes |
|---|---|---|
| Process log | stderr | Go `log` lines prefixed `[FIX]`, `[ITCH]`, `[universe]`, `[bench]`. Redirect it yourself. `*.log` files are git-ignored. |
| FIX message log | `ME_FIX_MSGLOG_FILE` | Every message except heartbeats. Never rotated or trimmed by the engine; `.gitignore` notes it once grew to 9.6 GB. The engine keeps the file open in append mode, so if you rotate it externally, use copy-and-truncate. |
| Admin audit log | `ME_AUDIT_FILE` | One JSON object per line. |
| nginx | `/var/log/nginx/matching-engine.*.log`, `/var/log/nginx/fixs-gateway.*.log` | From the nginx configs above. |

## Health checks

| Check | Healthy result |
|---|---|
| `curl -s localhost:8010/healthz` | `ok` |
| `curl -s localhost:8010/health` | `"ok":true`, with the current `phase` |
| `curl -s localhost:8010/api/status` | `universe.source` is your master file or the exchange, not `built-in DSE reference list` |
| Admin dashboard → listeners | FIX :5001, FIXS :5443, ITCH :5501 and Gateway :8010 all open |

The dashboard checks fixed ports on `127.0.0.1`: 5001, 5443, 5501 and 8010. If you change `PORT`, `FIX_PORT` or `ITCH_PORT`, those checks show the wrong result.

## Troubleshooting

| Symptom | Likely cause and fix |
|---|---|
| Orders rejected as `unknown symbol … (built-in DSE reference list …)` | No master was loaded. Provide `ME_INSTRUMENT_FILE` or a reachable `EXCHANGE_INSTRUMENT_URL`, or leave `ME_AUTO_LIST` on. Reload from the admin console. |
| Startup stalls for about 12 s at `[universe]` | `EXCHANGE_INSTRUMENT_URL` can't be reached; its compiled-in default is a deployment-specific address. Set it to a reachable URL, or provide an instrument file. |
| `[FIX] acceptor start failed: problem parsing XML datadictionary` | `ME_FIX_SPEC_DIR` doesn't contain `FIXT11.xml` and `FIX50SP2.xml`. Point it at the repo's `spec/`, or set `ME_FIX_DD=off`. |
| Broker logon refused | Look in Diagnostics or the log for `logon REJECTED`. The reason says whether the broker isn't registered, the password is wrong, or 553 doesn't match the CompID. |
| FIX orders rejected `off-tick` or `price band` | The price isn't on the instrument's tick, or is outside the daily band (see [Pre-trade checks](architecture.md#pre-trade-checks-fix-orders-only)). An import with the real tick and previous close fixes auto-listed symbols. |
| Market orders rejected during the auction | Pre-Open accepts only limit orders. Open the market first. |
| External broker can't connect on 5443 | Its IP isn't allowed, or `deploy/regen-fixs-gateway.sh` wasn't run after editing the allowlist. |
| Live test sends orders but none reach the engine | The FIX initiator dials `ME_ADVERTISE_HOST:5001` with target `DSEME`. Check `ME_ADVERTISE_HOST`, and note that a changed `FIX_PORT` or `FIX_SENDER` isn't used here. |
| Benchmark `persisted` count goes up but the tables stay empty | Postgres write errors are ignored, and the counter goes up anyway. Check the `ME_DB_*` settings and that the tables exist. |
| `/api/bench/orders.csv` returns 500 | The Postgres pool couldn't be created, or the query failed; the error text is in the body. |
| Browser shows no live updates through nginx | Proxy buffering is on. The snippet in `deploy/nginx-443.patch` turns it off. |
