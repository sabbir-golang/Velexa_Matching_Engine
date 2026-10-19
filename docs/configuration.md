# Configuration

This page lists everything that configures the matching engine at runtime: every environment variable the code reads, every data and config file it loads or writes, the directory layout those defaults assume, and the security issues to fix before running it anywhere shared.

The engine has no command-line flags. Everything comes from environment variables read through `EnvOr` in `config/config.go` (which treats an empty or whitespace-only value as unset) or, for a few settings, `os.Getenv`.

**Env file.** At start-up the engine loads an env file (`config/envfile.go`): `ME_ENV_FILE` if set, otherwise `me.env` in the working directory, otherwise `/opt/velexa-sim-me/me.env` on a server. Each line is `KEY=VALUE`; blank lines and `#` comments are skipped, `export ` and quotes are allowed. **A variable already set in the environment wins over the file.** The start-up log says which file was loaded (`[config] loaded N settings from me.env …`). `me.env` is git-ignored.

## Environment variables

Defaults below are the values compiled into the code. "Deployment-specific default" means the code ships with a real host or IP baked in; set the variable explicitly instead of relying on it.

<!-- AUTO:env-vars -->
| Variable | Default | Purpose | Read in |
|---|---|---|---|
| `EXCHANGE_INSTRUMENT_URL` | unset (source skipped) | URL of the exchange security master. Second choice for the instrument universe at boot and on reload (see [Instrument universe](architecture.md#instrument-universe)). Unset, the engine goes straight to the built-in list instead of waiting for an exchange. | `engine/loader.go` (`InstrumentURL`) |
| `FIXS_ALLOW_FILE` | `/opt/velexa-sim-me/fix_allow.json` | Source-IP allowlist for the external FIX gateway on port 5443, edited from the admin console. | `rest/handlers/broker/helpers.go` (`allowPath`), `deploy/regen-fixs-gateway.sh` |
| `FIX_PORT` | `5001` | TCP port of the FIX acceptor. | `Cmd/serve.go` |
| `FIX_SENDER` | `DSEME` | SenderCompID of the FIX acceptor; also printed as the TargetCompID on broker onboarding sheets. | `Cmd/serve.go`, `rest/handlers/broker/helpers.go` (`meEndpoints`) |
| `ITCH_PORT` | `5501` | TCP port of the SoupBinTCP ITCH market-data feed. | `itchgw/market.go` |
| `ME_STATE_DIR` | `/opt/velexa-sim-me` if it exists, else `./data` | Folder for every data file (accounts, broker credentials, allowlist, audit trail, message log, orderbook ids, `session.key`). Created if missing. Each file's own variable still wins. | `config/config.go` (`StateDir`) |
| `ME_DB_URL` | unset (files) | PostgreSQL URL for console accounts and the audit trail, for example `postgres://velexa_me:PASSWORD@127.0.0.1:5435/velexa_me?sslmode=disable`. Unset, they stay in `users.json` and `admin_audit.jsonl`. When set, the engine creates the database if it is missing, applies the schema migrations, and refuses to start if it cannot reach it. `make db` sets it up on this PC. See [Database](#database). | `config/database.go`, `database/db` |
| `ME_DB_MAX_CONNS` | `10` | Size of the PostgreSQL connection pool. | `config/database.go` |
| `ME_USERS_FILE` | `<state dir>/users.json` | Console accounts and roles (PBKDF2 password hashes, mode 0600) when no database is configured. With `ME_DB_URL`, imported into the database once on first start, then renamed `.imported-…`. | `database/users/backend_file.go` |
| `ME_ORDERLOG_DIR` | `<state dir>/orders` | The order report without a database: one `orders-YYYY-MM-DD.json` per day. With `ME_DB_URL` it is the `order_log` table. | `Cmd/serve.go` (`openStores`) |
| `ME_REPORT_FILE` | `<state dir>/daily_report.json` | Daily report figures when no database is configured (the last 400 days). With `ME_DB_URL` they are in the `daily_stats` table. | `Cmd/serve.go` (`openReports`) |
| `ME_RATE_LIMIT` | `off` | `on` turns on the sign-in lockout (5 failures lock an IP or a username for 15 minutes, also on the change-password form) and the sign-up quota (5 attempts per IP per hour). | `config/admin.go` (`RateLimit`) |
| `ME_REGISTRATION` | `on` | Set `off` to close self-registration on the welcome page. | `config/admin.go` (`RegistrationOpen`) |
| `ME_OBID_FILE` | `<state dir>/orderbook_ids.json` | Permanent symbol → ITCH orderbook id map. Ids never change once handed out, so adding or sorting instruments cannot shift other symbols' ids. | `Cmd/serve.go`, `engine/obids.go` |
| `ME_ITCH_MODE` | journal-driven feed | `legacy` brings back the old market feed that re-publishes the depth as synthetic orders. | `itchgw/market.go` |
| `ME_ITCH_INDEX_PORT` | `5502` | DSE-ITCH index feed (`Z` values). `off` disables it. | `itchgw/index.go` |
| `ME_ITCH_NEWS_PORT` | `5503` | DSE-ITCH news feed (`N` announcements). `off` disables it. | `itchgw/news.go` |
| `ME_JOURNAL_CAP` | `400000` | Market events kept in memory; an ITCH client can resume (log in again with its next sequence) while its missed events are still held. | `engine/events.go` |
| `ME_SESSION_AUTO` | `on` | `off` starts with the session clock off: phases change only from the admin console or `/api/session/*`. | `engine/schedule.go` |
| `ME_SESSION_SCHEDULE` | `09:30,10:00,14:30,14:40,15:00` | Dhaka start times of pre-open, continuous trading, close, post-close and the end of the day (Sunday to Thursday). | `engine/schedule.go` |
| `ME_ENV_FILE` | `me.env` (then `/opt/velexa-sim-me/me.env` on a server) | Env file loaded at start-up; the environment wins over it. When none exists, the engine writes one there from `me.env.example` with a random `ME_ADMIN_PASS` and `ME_ADMIN_SECRET` (mode 0600). `ME_ENV_GENERATE=off` stops that. | `config/envfile.go` |
| `ME_IN_CONTAINER` | detected | `1` tells the engine it runs in a container (also detected from `/.dockerenv`); it then warns when `ME_PUB_IP` is missing. Set by the Docker image. | `config/version.go` |
| `ME_ADMIN_PASS` | unset | Password of the environment admin. Seeds the first `admin` account and works as a break-glass admin only while `users.json` has no enabled admin. Unset (or the old published default) turns the environment admin off; the first admin then gets a random password written to `initial-admin-password`. | `config/admin.go` (`AdminPass`) |
| `ME_ADMIN_SECRET` | random key in `<state dir>/session.key` | HMAC key that signs console session cookies. Used only when at least 32 bytes long; otherwise a random key is generated once and kept in `session.key`. Changing it signs everyone out. | `config/admin.go` (`SessionKey`) |
| `ME_ADMIN_USER` | `admin` | Username of the environment admin; also reserved against self-registration. | `config/admin.go` (`AdminUser`) |
| `ME_COOKIE_SECURE` | `off` | `on` when users reach the console over HTTPS through a proxy: session and CSRF cookies get `Secure` (the session cookie is then named `__Host-me_admin`) and responses carry HSTS. Leave off on plain HTTP or the browser drops the cookie. | `config/admin.go` (`CookieSecure`) |
| `ME_TRUSTED_PROXIES` | `127.0.0.1,::1` | Comma-separated IPs or CIDRs of reverse proxies whose `X-Forwarded-For`, `X-Real-IP`, `CF-Connecting-IP` and `X-Forwarded-Proto` headers are believed. From any other peer these headers are ignored, so a client cannot pick its own address to dodge the sign-in lockout. | `config/admin.go` (`TrustedProxies`), `rest/middleware/session.go` (`ClientIP`) |
| `ME_AUDIT_FILE` | `<state dir>/admin_audit.jsonl` | Audit trail when no database is configured; with `ME_DB_URL`, only a fallback for entries the database could not take (imported at the next start). | `database/audit/audit.go` (`Path`) |
| `ME_AUTO_LIST` | `on` | Whether an order for an unknown but well-formed symbol opens a new book instead of being rejected. `off`, `0`, `false` or `no` disable it. | `engine/instruments.go` (`autoListEnabled`) |
| `ME_BROKER_META_FILE` | `/opt/velexa-sim-me/broker_meta.json` | Broker metadata store (company, SenderCompID, linked ITCH user). | `brokers/meta.go` (`loadBrokerMeta`) |
| `ME_FIXS_PORT` | unset | Port of an external TLS FIX gateway (nginx, see [operations](operations.md#external-fix-gateway-port-5443)). When set, onboarding sheets send brokers there with TLS instructions and the dashboard checks it; unset, sheets show the FIX acceptor itself. | `rest/handlers/broker/helpers.go` (`meEndpoints`), `rest/handlers/admin/status.go` |
| `ME_FIX_APPLVER` | `9` | `DefaultApplVerID` when the BeginString is `FIXT.1.1` (`9` = FIX 5.0 SP2). | `fixgw/fixacceptor.go` (`StartFixAcceptor`) |
| `ME_FIX_AUTH` | unset (auto) | Unset: FIX logons are checked against `fix_brokers.json` once at least one broker is registered (a fresh install accepts any CompID until then). `on`: always checked. `off`: never. | `database/brokers/fixauth.go` (`LoadFix`) |
| `ME_FIX_AUTH_FILE` | `/opt/velexa-sim-me/fix_brokers.json` | FIX logon credential store (CompID → password). | `fixgw/fixacceptor.go` (`loadFixAuth`) |
| `ME_FIX_BEGIN` | `FIXT.1.1` | FIX BeginString of the acceptor session. The code comment names `FIX.4.4` as the rollback value. | `fixgw/fixacceptor.go` (`StartFixAcceptor`) |
| `ME_FIX_DD` | `on` | `off` turns off FIX 5.0 SP2 data-dictionary validation. Only applies when the BeginString is `FIXT.1.1`. | `fixgw/fixacceptor.go` (`StartFixAcceptor`) |
| `ME_FIX_MSGLOG_FILE` | `/opt/velexa-sim-me/fix_messages.log` | Append-only FIX message log. | `fixgw/msglog.go` (`initMsgLog`) |
| `ME_FIX_MSGLOG_MAX_MB` | `256` | The FIX message log is rotated when it would grow past this size (MB). Only the last 4 MB are read at start-up. | `fixgw/msglog.go` (`initMsgLog`) |
| `ME_FIX_MSGLOG_KEEP` | `5` | Rotated message logs kept (`fix_messages.log.1` … `.5`); `0` keeps none. | `fixgw/msglog.go` |
| `ME_FIX_SPEC_DIR` | `/opt/velexa-sim-me/spec` | Directory holding `FIXT11.xml` and `FIX50SP2.xml`, the QuickFIX data dictionaries. | `fixgw/fixacceptor.go` (`StartFixAcceptor`) |
| `ME_INSTRUMENT_FILE` | `/opt/velexa-sim-me/instruments.json` | Operator-supplied security master; first choice for the instrument universe. Admin imports are written here. An explicitly empty list (`{"instruments": []}`, as saved when every instrument is removed) starts the engine with no instruments instead of the built-in list. | `engine/loader.go` (`instrumentFile`) |
| `ME_INT_IP` | this PC's LAN IP | Internal (on-platform) FIX IP printed on onboarding sheets; shown only when it differs from the external endpoint. | `rest/handlers/broker/helpers.go` (`meEndpoints`) |
| `ME_INT_PORT` | value of `FIX_PORT` | Internal FIX port printed on onboarding sheets. Display only. | `rest/handlers/broker/helpers.go` (`meEndpoints`) |
| `ME_ITCH_AUTH` | `off` | `on` makes the ITCH feed check SoupBinTCP logins against `itch_brokers.json`. | `brokers/itchauth.go` (`loadItchAuth`) |
| `ME_ITCH_AUTH_FILE` | `/opt/velexa-sim-me/itch_brokers.json` | ITCH feed credential store (username → password). | `brokers/itchauth.go` (`loadItchAuth`) |
| `ME_ITCH_IP` | `ME_PUB_IP`, else this PC's LAN IP | ITCH feed IP printed on onboarding sheets. | `rest/handlers/broker/helpers.go` (`meEndpoints`) |
| `ME_ITCH_PORT` | value of `ITCH_PORT` | ITCH feed port printed on onboarding sheets. Display only. | `rest/handlers/broker/helpers.go` (`meEndpoints`) |
| `ME_PUB_HOST` | this PC's LAN IP | Host / SNI printed on onboarding sheets. Set it to a DNS name only if brokers should connect by name. | `rest/handlers/broker/helpers.go` (`meEndpoints`) |
| `ME_PUB_IP` | this PC's LAN IP | Public IP printed on onboarding sheets for the external FIX endpoint; also the default for `ME_ITCH_IP`. | `rest/handlers/broker/helpers.go` (`meEndpoints`) |
| `ME_PUB_FIX_PORT` | value of `FIX_PORT` | FIX port printed on onboarding sheets when brokers reach the acceptor on another port (a container whose 5001 is published as another host port). Display only. | `rest/handlers/broker/helpers.go` (`meEndpoints`) |
| `PORT` | `8010` | HTTP listen port (web UI, JSON API, WebSockets, admin console). | `Cmd/serve.go` |
<!-- /AUTO:env-vars -->

### Variables that only change what the onboarding sheet says

`ME_PUB_HOST`, `ME_PUB_IP`, `ME_FIXS_PORT`, `ME_INT_IP`, `ME_INT_PORT`, `ME_ITCH_IP` and `ME_ITCH_PORT` only change the connection details on the broker detail view and the printable onboarding sheet (`meEndpoints` in `rest/handlers/broker/helpers.go`); they don't open or move any listener. Left unset, they follow the machine the engine runs on: its LAN IP address (the interface of the default route, also used as Host / SNI) and the real `FIX_PORT` / `ITCH_PORT`, so a fresh install on any PC hands out working addresses. Set them when brokers reach the engine through a public IP, NAT or a proxy. To move the real listeners, set `PORT`, `FIX_PORT` and `ITCH_PORT`.

### Minimal single-box setup

With no variables set, the engine runs standalone on one machine:

- HTTP on `:8010`, FIX on `:5001`, ITCH on `:5501`.
- FIX and ITCH authentication are off.
- It looks for its data files under `/opt/velexa-sim-me/`. Missing credential, metadata and allowlist files just start out empty. A missing instrument master falls back to the exchange URL, then to the 400 built-in instruments.
- The FIX acceptor needs `FIXT11.xml` and `FIX50SP2.xml` in `ME_FIX_SPEC_DIR`, unless `ME_FIX_DD=off` or `ME_FIX_BEGIN` is not `FIXT.1.1`. If they are missing, the log shows `[FIX] acceptor start failed: problem parsing XML datadictionary …`. HTTP and ITCH keep running, but there is no FIX listener.

## Data and config files

<!-- AUTO:config-files -->
| File | Env override | Format | Purpose |
|---|---|---|---|
| `instruments.json` | `ME_INSTRUMENT_FILE` | JSON `{"instruments":[…]}`, a bare JSON array, or CSV/TSV with a header row | Security master; first source of the instrument universe. Written atomically by the admin import. Not in the repo. |
| `fix_brokers.json` | `ME_FIX_AUTH_FILE` | JSON object: SenderCompID → password | FIX logon credentials. Rewritten (mode `0600`) when brokers are added or revoked in the admin console. |
| `itch_brokers.json` | `ME_ITCH_AUTH_FILE` | JSON object: username → password | ITCH feed credentials. Rewritten (mode `0600`) from the admin console. |
| `broker_meta.json` | `ME_BROKER_META_FILE` | JSON object: SenderCompID → `{company, sender, itch_user, contact, created}` | Links each broker's FIX CompID to its ITCH user and company for the admin console. Holds no passwords. |
| `fix_allow.json` | `FIXS_ALLOW_FILE` | JSON object: IP address → label | Source-IP allowlist for the external FIX gateway. Written mode `0600`; read by `deploy/regen-fixs-gateway.sh` (run with sudo), which always adds loopback and this host (`FIXS_HOST_IP`, default the first address of `hostname -I`). Not tracked in git; see `deploy/state/fix_allow.json.example`. |
| `users.json` | `ME_USERS_FILE` | JSON array of accounts | Console accounts (role, profile, PBKDF2-SHA256 hash, session token), used **only without a database**. Mode `0600`. Git-ignored. |
| `session.key` | `ME_ADMIN_SECRET` | 32 random bytes | Key that signs session cookies, created on first start. Mode `0600`. Deleting it signs everyone out. Git-ignored. |
| `initial-admin-password` | — | `username password` | Written next to `users.json` only when the first admin was created with a random password (no `ME_ADMIN_PASS`). Mode `0600`. Delete it after signing in. |
| `admin_audit.jsonl` | `ME_AUDIT_FILE` | JSON lines: `{time, actor, action, target}` | Audit trail without a database (the newest 300 entries are kept in memory); with a database, only a fallback for failed inserts. Mode `0600`. Git-ignored. |
| `fix_messages.log` | `ME_FIX_MSGLOG_FILE` | Text lines: `YYYY-MM-DD HH:MM:SS.mmm DIR SESSION RAW`, with FIX SOH shown as `\|` | Every FIX message in and out except heartbeats. Logon passwords (tags 554 and 925) are replaced with `***` before anything is stored; an older log is scrubbed once at startup. Mode `0600`. The last 4000 lines are reloaded at startup. Git-ignored. |
| `spec/FIXT11.xml`, `spec/FIX50SP2.xml` | `ME_FIX_SPEC_DIR` | QuickFIX XML data dictionaries | Transport and application dictionaries for FIX 5.0 SP2 validation. |
<!-- /AUTO:config-files -->

### Format examples

These examples are made up; they are not real entries.

`instruments.json`, in the exchange payload shape (`masterInstr` in `engine/loader.go`):

```json
{
  "count": 2,
  "instruments": [
    {"symbol": "ACME", "isin": "BD0000000001", "name": "Acme Ltd.", "board": "MAIN",
     "prev_close": 73.30, "tick": 0.10, "is_index": false, "status": "active"},
    {"symbol": "DSEX", "name": "DSE Broad Index", "is_index": true}
  ]
}
```

Rows with `is_index: true`, or with `status` `halted` or `delisted`, are skipped, as are symbols that aren't valid tickers (see [Instrument universe](architecture.md#instrument-universe)). A CSV or TSV master needs a header row with a `symbol`, `ticker` or `trading_code` column. Other columns are matched loosely: case, spaces and underscores are ignored, so `Prev Close`, `prev_close` and `PrevClose` all work. `parseDelimited` in `engine/loader.go` lists every accepted alias.

`fix_brokers.json` and `itch_brokers.json`:

```json
{ "EXAMPLEBRKR": "FIX-0123456789ab" }
```

`broker_meta.json`:

```json
{
  "EXAMPLEBRKR": {"company": "Example Securities Ltd.", "sender": "EXAMPLEBRKR",
                  "itch_user": "EXAMPL", "contact": "ops@example.com", "created": "2026-01-31 09:00"}
}
```

`fix_allow.json`:

```json
{ "192.0.2.10": "Example Securities OMS" }
```

`brokers.tsv`, one broker per line with tab separators:

```text
99	example	terminal_example_db	EXAMPLEBRKR	DSE-999	Example Securities Ltd.
```

## Database

With `ME_DB_URL` set, console accounts and the audit trail live in PostgreSQL. Broker credentials, the allowlist, order-book IDs and the FIX message log stay in files.

| Table | Holds |
|---|---|
| `users` | One row per console account: `username_key` (lower-case, primary key), `username`, `role` (`admin`/`user`), `disabled`, `password_hash` (PBKDF2-SHA256), `session_ver` (changing it signs the account out everywhere), profile (`full_name`, `email` — unique when set, `phone`, `gender`, `country`), `created_at`, `last_login_at`, `updated_at`. |
| `audit_log` | One row per audited action: `id`, `at`, `actor` (`name (ip)`), `action`, `target`. Indexed by time and by actor. |
| `daily_stats` | Daily report figures: one row per trading day (Dhaka date) and order source (`FIX:<CompID>`, `USER:<name>`, `*` for the exchange) with `buy_orders`, `sell_orders`, `accepted`, `rejected`, `cancelled`, `amended`, `expired`, `trades`, `volume`, `turnover`. Saved every 5 s and at shutdown; today's figures are carried on after a restart. |
| `order_log` | The order report: one row per order and day with who sent it (`actor`), OrderID, ClOrdID, client, BO account, trader, board, symbol, side, type, validity, quantity, price, filled, average price, status and reject reason. Updated as the order changes; saved every second and at shutdown. On restart the engine continues order numbers after the day's highest, so an OrderID is never handed out twice. |
| `schema_migrations` | Which schema migrations (`internal/database/db/migrations/*.sql`) have been applied. |

- **Start-up.** The engine connects (retrying for 30 s), creates the database if it doesn't exist (the user needs `CREATEDB`, otherwise create it by hand), applies new migrations under an advisory lock, and loads the accounts. If any of that fails it stops with the reason instead of falling back to files, so the data never splits.
- **Moving from files.** On the first start with a database, an existing `users.json` is imported into the empty `users` table — password hashes and session tokens included, so nobody has to sign in again — and `admin_audit.jsonl` into `audit_log`. Each file is then renamed `….imported-<date>`.
- **Writes.** The engine keeps the accounts in memory for the per-request session check and writes every change to the database before it counts; if the write fails, the change is undone and the console shows "the change could not be saved". An audit entry the database cannot take is written to `admin_audit.jsonl` and imported at the next start.
- **One engine per database.** Edits made directly in the database are seen after the engine restarts; always change accounts through the console.
- **Secrets.** The database password is only in `me.env` (mode 0600). Use `sslmode=require` (or `verify-full`) when the database is on another machine.

## Deployment directory layout

Every file default points into one directory:

```text
/opt/velexa-sim-me/
├── instruments.json        # security master (optional; written by admin import)
├── fix_brokers.json        # FIX logon credentials
├── itch_brokers.json       # ITCH feed credentials
├── broker_meta.json        # broker ↔ ITCH user linkage
├── fix_allow.json          # external FIX gateway IP allowlist
├── users.json              # console accounts (written at runtime)
├── session.key             # session-cookie signing key (written at runtime)
├── spec/
│   ├── FIXT11.xml
│   └── FIX50SP2.xml
├── admin_audit.jsonl       # written at runtime
└── fix_messages.log        # written at runtime
```

`deploy/regen-fixs-gateway.sh` says to run it as `~/matching-engine/deploy/regen-fixs-gateway.sh`. The binary and scripts can live elsewhere, as long as the data paths resolve. Each path can be moved with the env override listed in the table above.

## Security notes

What protects the console and the gateways, and what you still have to do yourself.

- **Sign-in.** Passwords are stored as salted PBKDF2-SHA256 hashes (600 000 iterations; older hashes are upgraded at the next sign-in). Rules: 10–128 characters, not containing the username, and at least three of lower-case, upper-case, digits and symbols. With `ME_RATE_LIMIT=on` (off by default for now), failed sign-ins are counted per client IP and per username: 5 failures lock that IP or that account for 15 minutes; the same lock covers the "change my password" form. Without it, passwords can be guessed without limit — turn it on before exposing the console. An unknown username costs the same time and gets the same answer as a wrong password.
- **Sessions.** The session cookie is `HttpOnly`, `SameSite=Strict`, signed with `session.key`, and carries the account's session token. It lasts at most 8 hours and ends after 30 minutes without use. Signing out, changing or resetting a password, disabling or deleting an account changes the token, which ends every session of that account at once.
- **Environment admin.** `ME_ADMIN_USER` / `ME_ADMIN_PASS` seed the first admin and act as a break-glass admin only while `users.json` has no enabled admin. The old built-in default password is refused: an account still holding it is re-keyed to `ME_ADMIN_PASS` at startup (or disabled if that is unset). To recover a lost admin password: stop the engine, remove that account from `users.json`, set `ME_ADMIN_PASS`, start again.
- **Self-registration.** With `ME_RATE_LIMIT=on`, every sign-up attempt counts against a quota of 5 per IP per hour. A taken username, a taken email and a reserved name (`admin`, `root`, `administrator`, the environment admin's name …) all get the same answer.
- **CSRF and methods.** Every POST needs the browser's CSRF token (`me_csrf` cookie echoed in the `X-CSRF-Token` header or a `csrf` form field) and must not come from another site (`Sec-Fetch-Site` / `Origin`). Every action that changes state answers only POST.
- **Browser protections.** Every response carries `Content-Security-Policy` (scripts, styles, frames and form targets limited to this origin; Google Fonts allowed), `X-Frame-Options: SAMEORIGIN`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin` and `Permissions-Policy`; console pages are `Cache-Control: no-store`; HSTS is sent over HTTPS. The console pages use inline scripts and event handlers, so the CSP allows `'unsafe-inline'`; values shown in the console are escaped before they reach the page, and FIX logons with a CompID other than 1–20 letters, digits, `.`, `-`, `_` are refused.
- **Order entry and session control.** `/api/order` needs a signed-in console user and records them as the order's actor (`USER:<name>`). `/api/session/preopen` and `/api/session/open` need an admin and POST; `GET /api/session` stays public. Market data (`/api/market`, `/api/book`, `/events`, the WebSocket feeds) is public.
- **Behind a reverse proxy.** Set `ME_TRUSTED_PROXIES` to the proxy's address so client IPs are right, and `ME_COOKIE_SECURE=on` when users come in over HTTPS. See [operations](operations.md).
- **FIX and ITCH credentials.** With `ME_FIX_AUTH` unset, FIX logons are checked once the first broker is registered — until then anyone who can reach `FIX_PORT` can log on, so register a broker (or set `ME_FIX_AUTH=on`) before exposing the port. `ME_ITCH_AUTH` still defaults to `off`; turn it on once every ITCH client has a login in `itch_brokers.json`. Passwords are compared in constant time. `/admin/api/broker/detail` returns a broker's FIX and ITCH passwords to admins and to the broker's owner.
- **ITCH login check.** With `ME_ITCH_AUTH=on`, every login (market, index and news feeds) is checked against `itch_brokers.json`; a short or malformed login packet has empty credentials and is refused with a LoginReject. The legacy feed (`ME_ITCH_MODE=legacy`) still skips the check for a login body shorter than 16 bytes.
- **Credentials in git history.** `deploy/state/fix_brokers.json`, `itch_brokers.json`, `broker_meta.json` and `fix_allow.json` are no longer tracked (`*.example` files show the format), but older commits still contain them. Rotate every password that was in them, and rewrite history (for example with `git filter-repo`) if the repository is shared.
