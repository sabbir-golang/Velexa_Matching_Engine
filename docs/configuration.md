# Configuration

This page lists everything that configures the matching engine at runtime: every environment variable the code reads, every data and config file it loads or writes, the directory layout those defaults assume, and the security issues to fix before running it anywhere shared.

The engine has no command-line flags and no config file of its own. Everything comes from environment variables read through `envOr` in `config.go` (which treats an empty or whitespace-only value as unset) or, for a few older settings, `os.Getenv`. The process does not load an env file itself; whatever starts it must export the variables. The repo's `.gitignore` reserves `me.env` for a local env file.

## Environment variables

Defaults below are the values compiled into the code. "Deployment-specific default" means the code ships with a real host or IP baked in; set the variable explicitly instead of relying on it.

<!-- AUTO:env-vars -->
| Variable | Default | Purpose | Read in |
|---|---|---|---|
| `EXCHANGE_INSTRUMENT_URL` | deployment-specific default, see `loader.go` | URL of the exchange security master. Second choice for the instrument universe at boot and on reload (see [Instrument universe](architecture.md#instrument-universe)). | `loader.go` (`instrumentURL`) |
| `FIXS_ALLOW_FILE` | `/opt/velexa-sim-me/fix_allow.json` | Source-IP allowlist for the external FIX gateway on port 5443, edited from the admin console. | `admin.go` (`allowPath`), `regen-fixs-gateway.sh` |
| `FIX_PORT` | `5001` | TCP port of the FIX acceptor. | `main.go` |
| `FIX_SENDER` | `DSEME` | SenderCompID of the FIX acceptor; also printed as the TargetCompID on broker onboarding sheets. | `main.go`, `admin.go` (`meEndpoints`) |
| `ITCH_PORT` | `5501` | TCP port of the SoupBinTCP ITCH market-data feed. | `itchfeed.go` |
| `LIVE_BROKERS_FILE` | unset; falls back to `/opt/velexa-sim-me/brokers.tsv` | Broker list used by the live load test. If the file is missing or empty, three built-in brokers are used. | `live.go` (`loadBrokersFile`) |
| `ME_ADMIN_PASS` | built-in default (see [Security notes](#security-notes)) | Admin console password. | `admin.go` (`adminPass`) |
| `ME_ADMIN_SECRET` | built-in default derived from the admin password | HMAC key that signs the `me_admin` session cookie. | `admin.go` (`adminSecret`) |
| `ME_ADMIN_USER` | `admin` | Admin console username. | `admin.go` (`adminUser`) |
| `ME_ADVERTISE_HOST` | `127.0.0.1` | Host the OMS FIX initiator is told to dial to reach this engine's FIX acceptor, used when the live load test registers broker sessions. | `config.go` (`meAdvertiseHost`) |
| `ME_API_LATENCY_URL` | empty: derived from `ME_API_URL` by replacing `/api/internal/order` with `/api/internal/latency` | OMS API latency endpoint read by `/api/live/latency`. | `config.go` (`meAPILatencyURL`) |
| `ME_API_URL` | `http://127.0.0.1:8007/api/internal/order` | OMS API internal order endpoint that the live load test posts orders to. | `config.go` (`meAPIURL`) |
| `ME_AUDIT_FILE` | `/opt/velexa-sim-me/admin_audit.jsonl` | Append-only admin audit log. | `admin.go` (`auditPath`) |
| `ME_AUDIT_URL` | `http://127.0.0.1:8092` | Base URL of the external audit service; `/api/live/cleanup` posts to `<url>/purge`. | `config.go` (`meAuditURL`) |
| `ME_AUTO_LIST` | `on` | Whether an order for an unknown but well-formed symbol opens a new book instead of being rejected. `off`, `0`, `false` or `no` disable it. | `instruments.go` (`autoListEnabled`) |
| `ME_BROKER_META_FILE` | `/opt/velexa-sim-me/broker_meta.json` | Broker metadata store (company, SenderCompID, linked ITCH user). | `brokermeta.go` (`loadBrokerMeta`) |
| `ME_DB_HOST` | `127.0.0.1` | Host of the engine's own Postgres (database `velexa_sim`). Also the default for `ME_OMS_HOST`. | `config.go` (`meDBHost`) |
| `ME_DB_PASS` | built-in default (see [Security notes](#security-notes)) | Password for the engine's own Postgres. | `config.go` (`meDSN`) |
| `ME_DB_PORT` | `5432` | Port of the engine's own Postgres. | `config.go` (`meDSN`) |
| `ME_DB_USER` | `postgres` | User for the engine's own Postgres. | `config.go` (`meDSN`) |
| `ME_FIXS_PORT` | `5443` | External FIX gateway port printed on onboarding sheets. Display only: the engine does not listen on it (nginx does, see [operations](operations.md#external-fix-gateway-port-5443)). | `admin.go` (`meEndpoints`) |
| `ME_FIX_APPLVER` | `9` | `DefaultApplVerID` when the BeginString is `FIXT.1.1` (`9` = FIX 5.0 SP2). | `fixacceptor.go` (`StartFixAcceptor`) |
| `ME_FIX_AUTH` | `off` | `on` makes the FIX acceptor check every Logon against `fix_brokers.json`. | `fixacceptor.go` (`loadFixAuth`) |
| `ME_FIX_AUTH_FILE` | `/opt/velexa-sim-me/fix_brokers.json` | FIX logon credential store (CompID → password). | `fixacceptor.go` (`loadFixAuth`) |
| `ME_FIX_BEGIN` | `FIXT.1.1` | FIX BeginString of the acceptor session. The code comment names `FIX.4.4` as the rollback value. | `fixacceptor.go` (`StartFixAcceptor`) |
| `ME_FIX_DD` | `on` | `off` turns off FIX 5.0 SP2 data-dictionary validation. Only applies when the BeginString is `FIXT.1.1`. | `fixacceptor.go` (`StartFixAcceptor`) |
| `ME_FIX_LATENCY_URL` | empty: derived from `ME_FIX_WS` (`ws://` → `http://`, `wss://` → `https://`, `/ws/auth` → `/api/latency`) | FIX initiator latency endpoint read by `/api/live/latency`. | `config.go` (`meFixLatencyURL`) |
| `ME_FIX_MSGLOG_FILE` | `/opt/velexa-sim-me/fix_messages.log` | Append-only FIX message log. | `fixmsglog.go` (`initMsgLog`) |
| `ME_FIX_SPEC_DIR` | `/opt/velexa-sim-me/spec` | Directory holding `FIXT11.xml` and `FIX50SP2.xml`, the QuickFIX data dictionaries. | `fixacceptor.go` (`StartFixAcceptor`) |
| `ME_FIX_WS` | `ws://127.0.0.1:8003/ws/auth` | Control WebSocket of the OMS FIX initiator. The live load test sends one logon frame per broker here. | `config.go` (`meFixAuthWS`) |
| `ME_INSTRUMENT_FILE` | `/opt/velexa-sim-me/instruments.json` | Operator-supplied security master; first choice for the instrument universe. Admin imports are written here. | `loader.go` (`instrumentFile`) |
| `ME_INT_IP` | deployment-specific default, see `admin.go` | Internal (on-platform) FIX IP printed on onboarding sheets. | `admin.go` (`meEndpoints`) |
| `ME_INT_PORT` | `5001` | Internal FIX port printed on onboarding sheets. Display only. | `admin.go` (`meEndpoints`) |
| `ME_ITCH_AUTH` | `off` | `on` makes the ITCH feed check SoupBinTCP logins against `itch_brokers.json`. | `itchauth.go` (`loadItchAuth`) |
| `ME_ITCH_AUTH_FILE` | `/opt/velexa-sim-me/itch_brokers.json` | ITCH feed credential store (username → password). | `itchauth.go` (`loadItchAuth`) |
| `ME_ITCH_IP` | value of `ME_PUB_IP` | ITCH feed IP printed on onboarding sheets. | `admin.go` (`meEndpoints`) |
| `ME_ITCH_PORT` | `5501` | ITCH feed port printed on onboarding sheets. Display only: the listener uses `ITCH_PORT`. | `admin.go` (`meEndpoints`) |
| `ME_OMS_HOST` | value of `ME_DB_HOST` | Host of the OMS Postgres (broker tenant databases and the trade master), used by the live load test and cleanup. | `config.go` (`omsDBHost`) |
| `ME_OMS_PASS` | built-in default (see [Security notes](#security-notes)) | Password for the OMS Postgres. | `config.go` (`omsDSN`) |
| `ME_OMS_PORT` | `5432` | Port of the OMS Postgres. | `config.go` (`omsDSN`) |
| `ME_OMS_USER` | `postgres` | User for the OMS Postgres. | `config.go` (`omsDSN`) |
| `ME_PUB_HOST` | deployment-specific default, see `admin.go` | Public hostname printed on onboarding sheets (Host / SNI). | `admin.go` (`meEndpoints`) |
| `ME_PUB_IP` | deployment-specific default, see `admin.go` | Public IP printed on onboarding sheets for the external FIX gateway; also the default for `ME_ITCH_IP`. | `admin.go` (`meEndpoints`) |
| `PORT` | `8010` | HTTP listen port (web UI, JSON API, WebSockets, admin console). | `main.go` |
<!-- /AUTO:env-vars -->

### Variables that only change what the onboarding sheet says

`ME_PUB_HOST`, `ME_PUB_IP`, `ME_FIXS_PORT`, `ME_INT_IP`, `ME_INT_PORT`, `ME_ITCH_IP` and `ME_ITCH_PORT` are read only by `meEndpoints` in `admin.go`. They fill in the connection details on the broker detail view and the printable onboarding sheet. They don't open or move any listener. To move the real listeners, set `PORT`, `FIX_PORT` and `ITCH_PORT`.

### Minimal single-box setup

With no variables set, the engine runs standalone on one machine:

- HTTP on `:8010`, FIX on `:5001`, ITCH on `:5501`.
- FIX and ITCH authentication are off.
- It looks for its data files under `/opt/velexa-sim-me/`. Missing credential, metadata and allowlist files just start out empty. A missing instrument master falls back to the exchange URL, then to the 400 built-in instruments.
- The FIX acceptor needs `FIXT11.xml` and `FIX50SP2.xml` in `ME_FIX_SPEC_DIR`, unless `ME_FIX_DD=off` or `ME_FIX_BEGIN` is not `FIXT.1.1`. If they are missing, the log shows `[FIX] acceptor start failed: problem parsing XML datadictionary …`. HTTP and ITCH keep running, but there is no FIX listener.
- Benchmark persistence and the live load test need Postgres and the OMS services on `127.0.0.1`. Without them, those features fail but trading still works.

## Data and config files

<!-- AUTO:config-files -->
| File | Env override | Format | Purpose |
|---|---|---|---|
| `instruments.json` | `ME_INSTRUMENT_FILE` | JSON `{"instruments":[…]}`, a bare JSON array, or CSV/TSV with a header row | Security master; first source of the instrument universe. Written atomically by the admin import. Not in the repo. |
| `fix_brokers.json` | `ME_FIX_AUTH_FILE` | JSON object: SenderCompID → password | FIX logon credentials. Rewritten (mode `0600`) when brokers are added or revoked in the admin console. |
| `itch_brokers.json` | `ME_ITCH_AUTH_FILE` | JSON object: username → password | ITCH feed credentials. Rewritten (mode `0600`) from the admin console. |
| `broker_meta.json` | `ME_BROKER_META_FILE` | JSON object: SenderCompID → `{company, sender, itch_user, contact, created}` | Links each broker's FIX CompID to its ITCH user and company for the admin console. Holds no passwords. |
| `fix_allow.json` | `FIXS_ALLOW_FILE` | JSON object: IP address → label | Source-IP allowlist for the external FIX gateway. Read by `regen-fixs-gateway.sh`. |
| `brokers.tsv` | `LIVE_BROKERS_FILE` | Tab-separated, no header. Columns: BO-ID prefix, OMS subdomain, OMS database name, SenderCompID; further columns are ignored | Broker set for the live load test and the broker picker on `/bench`. |
| `admin_audit.jsonl` | `ME_AUDIT_FILE` | JSON lines: `{time, actor, action, target}` | Admin audit trail. The last 300 entries are reloaded at startup. Git-ignored. |
| `fix_messages.log` | `ME_FIX_MSGLOG_FILE` | Text lines: `YYYY-MM-DD HH:MM:SS.mmm DIR SESSION RAW`, with FIX SOH shown as `\|` | Every FIX message in and out except heartbeats. The last 4000 lines are reloaded at startup. Git-ignored. |
| `spec/FIXT11.xml`, `spec/FIX50SP2.xml` | `ME_FIX_SPEC_DIR` | QuickFIX XML data dictionaries | Transport and application dictionaries for FIX 5.0 SP2 validation. |
<!-- /AUTO:config-files -->

### Format examples

These examples are made up; they are not real entries.

`instruments.json`, in the exchange payload shape (`masterInstr` in `loader.go`):

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

Rows with `is_index: true`, or with `status` `halted` or `delisted`, are skipped, as are symbols that aren't valid tickers (see [Instrument universe](architecture.md#instrument-universe)). A CSV or TSV master needs a header row with a `symbol`, `ticker` or `trading_code` column. Other columns are matched loosely: case, spaces and underscores are ignored, so `Prev Close`, `prev_close` and `PrevClose` all work. `parseDelimited` in `loader.go` lists every accepted alias.

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

## Deployment directory layout

Every file default points into one directory:

```text
/opt/velexa-sim-me/
├── instruments.json        # security master (optional; written by admin import)
├── fix_brokers.json        # FIX logon credentials
├── itch_brokers.json       # ITCH feed credentials
├── broker_meta.json        # broker ↔ ITCH user linkage
├── fix_allow.json          # external FIX gateway IP allowlist
├── brokers.tsv             # live load-test broker set
├── spec/
│   ├── FIXT11.xml
│   └── FIX50SP2.xml
├── admin_audit.jsonl       # written at runtime
└── fix_messages.log        # written at runtime
```

`regen-fixs-gateway.sh` says to run it as `~/matching-engine/regen-fixs-gateway.sh`. The binary and scripts can live elsewhere, as long as the data paths resolve. Each path can be moved with the env override listed in the table above.

## Security notes

Fix these before exposing the engine beyond a trusted network.

- **Hardcoded credential defaults.** `ME_ADMIN_PASS` (`admin.go`), `ME_DB_PASS` and `ME_OMS_PASS` (`config.go`) fall back to passwords compiled into the binary, and `ME_ADMIN_SECRET` derives from the admin password. All four must be overridden in production. Because they are in the source, treat the defaults as public.
- **Hardcoded internal API token.** The live load test sends a fixed `X-Internal-Token` header to the OMS API, compiled into `live.go` (`NewLiveBench`). There is no variable to override it.
- **Credential stores are committed to git.** `fix_brokers.json` and `itch_brokers.json` are tracked in the repository and hold broker passwords. `broker_meta.json`, `fix_allow.json` and `brokers.tsv` are tracked too and contain real broker and network details.
- **Authentication is off by default.** `ME_FIX_AUTH` and `ME_ITCH_AUTH` both default to `off`, so any FIX CompID can log on and any ITCH client can subscribe.
- **ITCH login check can be skipped.** With `ME_ITCH_AUTH=on`, `readLogin` in `itchfeed.go` checks credentials only when the login body is at least 16 bytes long; a shorter first packet is accepted without a check.
- **Most HTTP control endpoints are unauthenticated.** Only `/admin/api/*` and `/admin` need a login. `/api/order`, the benchmark and live-test controls, the session controls and `/api/live/cleanup` are open to anyone who can reach `PORT`. `/api/live/cleanup` truncates order tables in every broker database on the OMS Postgres (see [http-api](http-api.md#live-load-test)).
- **The broker detail API returns passwords.** `/admin/api/broker/detail` returns a broker's FIX and ITCH passwords in plain JSON (behind the admin login).
