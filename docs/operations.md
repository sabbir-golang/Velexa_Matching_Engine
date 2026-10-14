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
Velexa Matching Engine on :8010  (350 instruments from file:…, auto-list=true, phase=Continuous Trading)
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
| Trade Terminal | The `/terminal` trade page embedded: market watch, order book, tape and manual order entry. |
| FIX Sessions | Connected brokers with logon time and sequence numbers. Force a disconnect or reset sequence numbers. |
| Market Session | The session phase and the DSE clock. Force Pre-Open, Open, Close, Post-Close or End of day (switches the clock to manual), or switch back to the schedule. |
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

## Logs and files

| What | Where | Notes |
|---|---|---|
| Process log | stderr | Go `log` lines prefixed `[FIX]`, `[ITCH]`, `[universe]`, `[session]`. Redirect it yourself. `*.log` files are git-ignored. |
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
| Every order rejected `(293): Orders are not being accepted at this time` | The market is closed: outside the DSE schedule (`ME_SESSION_SCHEDULE`, Dhaka time, Sunday–Thursday) or after End of day. Open it from the admin **Market Session** page, or start with `ME_SESSION_AUTO=off`. |
| Books are empty and prices don't move | Expected: there are no simulated traders. Prices move only when real orders trade. |
| External broker can't connect on 5443 | Its IP isn't allowed, or `deploy/regen-fixs-gateway.sh` wasn't run after editing the allowlist. |
| Browser shows no live updates through nginx | Proxy buffering is on. The snippet in `deploy/nginx-443.patch` turns it off. |
