# Configuration

This page lists everything that configures the matching engine at runtime: every environment variable the code reads, every data and config file it loads or writes, the directory layout those defaults assume, and the security issues to fix before running it anywhere shared.

The engine has no command-line flags and no config file of its own. Everything comes from environment variables read through `envOr` in `config/config.go` (which treats an empty or whitespace-only value as unset) or, for a few older settings, `os.Getenv`. The process does not load an env file itself; whatever starts it must export the variables. The repo's `.gitignore` reserves `me.env` for a local env file.

## Environment variables

Defaults below are the values compiled into the code. "Deployment-specific default" means the code ships with a real host or IP baked in; set the variable explicitly instead of relying on it.

<!-- AUTO:env-vars -->
| Variable | Default | Purpose | Read in |
|---|---|---|---|
| `EXCHANGE_INSTRUMENT_URL` | deployment-specific default, see `engine/loader.go` | URL of the exchange security master. Second choice for the instrument universe at boot and on reload (see [Instrument universe](architecture.md#instrument-universe)). | `engine/loader.go` (`instrumentURL`) |
| `FIXS_ALLOW_FILE` | `/opt/velexa-sim-me/fix_allow.json` | Source-IP allowlist for the external FIX gateway on port 5443, edited from the admin console. | `rest/handlers/broker/broker_helpers.go` (`allowPath`), `deploy/regen-fixs-gateway.sh` |
| `FIX_PORT` | `5001` | TCP port of the FIX acceptor. | `Cmd/serve.go` |
| `FIX_SENDER` | `DSEME` | SenderCompID of the FIX acceptor; also printed as the TargetCompID on broker onboarding sheets. | `Cmd/serve.go`, `rest/handlers/broker/broker_helpers.go` (`meEndpoints`) |
| `ITCH_PORT` | `5501` | TCP port of the SoupBinTCP ITCH market-data feed. | `itchgw/market.go` |
| `ME_USERS_FILE` | `<state dir>/users.json` | Console accounts and roles (PBKDF2 password hashes, mode 0600). | `database/users/users.go` (`Path`) |
| `ME_REGISTRATION` | `on` | Set `off` to close self-registration on the welcome page. | `config/admin.go` (`RegistrationOpen`) |
| `ME_OBID_FILE` | `<state dir>/orderbook_ids.json` | Permanent symbol → ITCH orderbook id map. Ids never change once handed out, so adding or sorting instruments cannot shift other symbols' ids. | `Cmd/serve.go`, `engine/obids.go` |
| `ME_ITCH_MODE` | journal-driven feed | `legacy` brings back the old market feed that re-publishes the depth as synthetic orders. | `itchgw/market.go` |
| `ME_ITCH_INDEX_PORT` | `5502` | DSE-ITCH index feed (`Z` values). `off` disables it. | `itchgw/index.go` |
| `ME_ITCH_NEWS_PORT` | `5503` | DSE-ITCH news feed (`N` announcements). `off` disables it. | `itchgw/news.go` |
| `ME_JOURNAL_CAP` | `400000` | Market events kept in memory; an ITCH client can resume (log in again with its next sequence) while its missed events are still held. | `engine/events.go` |
| `ME_SESSION_AUTO` | `on` | `off` starts with the session clock off: phases change only from the admin console or `/api/session/*`. | `engine/schedule.go` |
| `ME_SESSION_SCHEDULE` | `09:30,10:00,14:30,14:40,15:00` | Dhaka start times of pre-open, continuous trading, close, post-close and the end of the day (Sunday to Thursday). | `engine/schedule.go` |
| `ME_ADMIN_PASS` | built-in default (see [Security notes](#security-notes)) | Admin console password. | `config/admin.go` (`AdminPass`) |
| `ME_ADMIN_SECRET` | built-in default derived from the admin password | HMAC key that signs the `me_admin` session cookie. | `rest/handlers/broker/broker_helpers.go` (`adminSecret`) |
| `ME_ADMIN_USER` | `admin` | Admin console username. | `config/admin.go` (`AdminUser`) |
| `ME_AUDIT_FILE` | `/opt/velexa-sim-me/admin_audit.jsonl` | Append-only admin audit log. | `database/audit/audit.go` (`Path`) |
| `ME_AUTO_LIST` | `on` | Whether an order for an unknown but well-formed symbol opens a new book instead of being rejected. `off`, `0`, `false` or `no` disable it. | `engine/instruments.go` (`autoListEnabled`) |
| `ME_BROKER_META_FILE` | `/opt/velexa-sim-me/broker_meta.json` | Broker metadata store (company, SenderCompID, linked ITCH user). | `brokers/meta.go` (`loadBrokerMeta`) |
| `ME_FIXS_PORT` | `5443` | External FIX gateway port printed on onboarding sheets. Display only: the engine does not listen on it (nginx does, see [operations](operations.md#external-fix-gateway-port-5443)). | `rest/handlers/broker/broker_helpers.go` (`meEndpoints`) |
| `ME_FIX_APPLVER` | `9` | `DefaultApplVerID` when the BeginString is `FIXT.1.1` (`9` = FIX 5.0 SP2). | `fixgw/fixacceptor.go` (`StartFixAcceptor`) |
| `ME_FIX_AUTH` | `off` | `on` makes the FIX acceptor check every Logon against `fix_brokers.json`. | `fixgw/fixacceptor.go` (`loadFixAuth`) |
| `ME_FIX_AUTH_FILE` | `/opt/velexa-sim-me/fix_brokers.json` | FIX logon credential store (CompID → password). | `fixgw/fixacceptor.go` (`loadFixAuth`) |
| `ME_FIX_BEGIN` | `FIXT.1.1` | FIX BeginString of the acceptor session. The code comment names `FIX.4.4` as the rollback value. | `fixgw/fixacceptor.go` (`StartFixAcceptor`) |
| `ME_FIX_DD` | `on` | `off` turns off FIX 5.0 SP2 data-dictionary validation. Only applies when the BeginString is `FIXT.1.1`. | `fixgw/fixacceptor.go` (`StartFixAcceptor`) |
| `ME_FIX_MSGLOG_FILE` | `/opt/velexa-sim-me/fix_messages.log` | Append-only FIX message log. | `fixgw/msglog.go` (`initMsgLog`) |
| `ME_FIX_SPEC_DIR` | `/opt/velexa-sim-me/spec` | Directory holding `FIXT11.xml` and `FIX50SP2.xml`, the QuickFIX data dictionaries. | `fixgw/fixacceptor.go` (`StartFixAcceptor`) |
| `ME_INSTRUMENT_FILE` | `/opt/velexa-sim-me/instruments.json` | Operator-supplied security master; first choice for the instrument universe. Admin imports are written here. | `engine/loader.go` (`instrumentFile`) |
| `ME_INT_IP` | deployment-specific default, see `rest/handlers/broker/broker_helpers.go` | Internal (on-platform) FIX IP printed on onboarding sheets. | `rest/handlers/broker/broker_helpers.go` (`meEndpoints`) |
| `ME_INT_PORT` | `5001` | Internal FIX port printed on onboarding sheets. Display only. | `rest/handlers/broker/broker_helpers.go` (`meEndpoints`) |
| `ME_ITCH_AUTH` | `off` | `on` makes the ITCH feed check SoupBinTCP logins against `itch_brokers.json`. | `brokers/itchauth.go` (`loadItchAuth`) |
| `ME_ITCH_AUTH_FILE` | `/opt/velexa-sim-me/itch_brokers.json` | ITCH feed credential store (username → password). | `brokers/itchauth.go` (`loadItchAuth`) |
| `ME_ITCH_IP` | value of `ME_PUB_IP` | ITCH feed IP printed on onboarding sheets. | `rest/handlers/broker/broker_helpers.go` (`meEndpoints`) |
| `ME_ITCH_PORT` | `5501` | ITCH feed port printed on onboarding sheets. Display only: the listener uses `ITCH_PORT`. | `rest/handlers/broker/broker_helpers.go` (`meEndpoints`) |
| `ME_PUB_HOST` | deployment-specific default, see `rest/handlers/broker/broker_helpers.go` | Public hostname printed on onboarding sheets (Host / SNI). | `rest/handlers/broker/broker_helpers.go` (`meEndpoints`) |
| `ME_PUB_IP` | deployment-specific default, see `rest/handlers/broker/broker_helpers.go` | Public IP printed on onboarding sheets for the external FIX gateway; also the default for `ME_ITCH_IP`. | `rest/handlers/broker/broker_helpers.go` (`meEndpoints`) |
| `PORT` | `8010` | HTTP listen port (web UI, JSON API, WebSockets, admin console). | `Cmd/serve.go` |
<!-- /AUTO:env-vars -->

### Variables that only change what the onboarding sheet says

`ME_PUB_HOST`, `ME_PUB_IP`, `ME_FIXS_PORT`, `ME_INT_IP`, `ME_INT_PORT`, `ME_ITCH_IP` and `ME_ITCH_PORT` are read only by `meEndpoints` in `rest/handlers/broker/broker_helpers.go`. They fill in the connection details on the broker detail view and the printable onboarding sheet. They don't open or move any listener. To move the real listeners, set `PORT`, `FIX_PORT` and `ITCH_PORT`.

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
| `fix_allow.json` | `FIXS_ALLOW_FILE` | JSON object: IP address → label | Source-IP allowlist for the external FIX gateway. Read by `deploy/regen-fixs-gateway.sh`. |
| `admin_audit.jsonl` | `ME_AUDIT_FILE` | JSON lines: `{time, actor, action, target}` | Admin audit trail. The last 300 entries are reloaded at startup. Git-ignored. |
| `fix_messages.log` | `ME_FIX_MSGLOG_FILE` | Text lines: `YYYY-MM-DD HH:MM:SS.mmm DIR SESSION RAW`, with FIX SOH shown as `\|` | Every FIX message in and out except heartbeats. The last 4000 lines are reloaded at startup. Git-ignored. |
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

## Deployment directory layout

Every file default points into one directory:

```text
/opt/velexa-sim-me/
├── instruments.json        # security master (optional; written by admin import)
├── fix_brokers.json        # FIX logon credentials
├── itch_brokers.json       # ITCH feed credentials
├── broker_meta.json        # broker ↔ ITCH user linkage
├── fix_allow.json          # external FIX gateway IP allowlist
├── spec/
│   ├── FIXT11.xml
│   └── FIX50SP2.xml
├── admin_audit.jsonl       # written at runtime
└── fix_messages.log        # written at runtime
```

`deploy/regen-fixs-gateway.sh` says to run it as `~/matching-engine/deploy/regen-fixs-gateway.sh`. The binary and scripts can live elsewhere, as long as the data paths resolve. Each path can be moved with the env override listed in the table above.

## Security notes

Fix these before exposing the engine beyond a trusted network.

- **Console roles.** `ME_ADMIN_USER` / `ME_ADMIN_PASS` seed the first `admin` account in `users.json` and stay valid as a break-glass admin even if that file is lost. Further accounts are managed on the console **Users** page. Roles: `admin` (everything) and `user` (read-only). The public endpoints (`/api/order`, `/api/bots/*`, `/api/session/*`, the Trade Terminal) are not role-protected.
- **Hardcoded credential defaults.** `ME_ADMIN_PASS` (`config/admin.go`) falls back to a password compiled into the binary, and `ME_ADMIN_SECRET` derives from the admin password. Both must be overridden in production. Because they are in the source, treat the defaults as public.
- **Credential stores are committed to git.** `fix_brokers.json` and `itch_brokers.json` are tracked in the repository and hold broker passwords. `broker_meta.json`, `fix_allow.json` and `brokers.tsv` are tracked too and contain real broker and network details.
- **Authentication is off by default.** `ME_FIX_AUTH` and `ME_ITCH_AUTH` both default to `off`, so any FIX CompID can log on and any ITCH client can subscribe.
- **ITCH login check.** With `ME_ITCH_AUTH=on`, every login (market, index and news feeds) is checked against `itch_brokers.json`; a short or malformed login packet has empty credentials and is refused with a LoginReject. The legacy feed (`ME_ITCH_MODE=legacy`) still skips the check for a login body shorter than 16 bytes.
- **Most HTTP control endpoints are unauthenticated.** Only `/admin/api/*` and `/admin` need a login. `/api/order` and the session controls (`/api/session/*`) are open to anyone who can reach `PORT`.
- **The broker detail API returns passwords.** `/admin/api/broker/detail` returns a broker's FIX and ITCH passwords in plain JSON (behind the admin login).
