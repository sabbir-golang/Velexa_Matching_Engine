# Development

This page is for developers changing the engine's code: how the source is laid out, how to build and test it, the conventions the code follows, and step-by-step notes for common changes. For how the engine behaves at runtime, read [architecture](architecture.md) first.

## Build and test

You need Go 1.25 or newer (from `go.mod`). Run everything from the repo root: the engine finds `spec/` and writes `data/` relative to it.

```bash
make build            # builds bin/velexa-sim-me (git-ignored)
make run              # go run .
make vet
make test             # unit tests plus the FIX conformance suite (about 40 s)
```

## Project layout

```
main.go                entry point: a thin main that calls cmd.Serve()
Cmd/serve.go           wiring: builds the engine, starts the gateways, serves HTTP
internal/
  rest/                HTTP layer
    server.go            Server: builds the handlers, registers their routes on one mux
    middleware/          Manager and Middlewares: role checks (Viewer, Admin), sessions
    handlers/<feature>/  handler.go, routes.go and one file per endpoint:
                         public, control, bench, live, register, account, instrument, broker, admin
    deps/                the engine and services every handler shares
  database/            file-backed stores
    users/               console accounts and roles (users.json, PBKDF2 hashes)
    brokers/             FIX/ITCH logon allowlists and broker metadata
    audit/               credential audit log
  engine/              matching-engine core: orders, books, the matching loop, instrument universe, bots
  fixgw/               FIX 5.0 SP2 acceptor and the FIX message log
  itchgw/              DSE-ITCH SoupBinTCP feed and the WebSocket gateway for the trader SPA
  loadtest/            synthetic benchmark, live OMS load test, risk checks, Postgres writer
  utils/               JSON reply helpers shared by the handlers
  latency/             lock-free latency recorder shared by the FIX acceptor and the load tests
  config/              environment settings and where the data files live
  web/                 HTML pages: trade terminal, welcome page, admin console, benchmark page
  refdata/             DSE reference tables (asset class, category, sector, ISIN per ticker)
spec/                  QuickFIX data dictionaries (FIXT11.xml, FIX50SP2.xml)
docs/  tools/          documentation and the conformance report; helper scripts
deploy/                nginx patch, gateway script and sample state files for a server
Makefile               build / run / test shortcuts
```

Packages depend on each other in one direction only (the compiler enforces it, since Go forbids import cycles):

```
Cmd ──► rest ──► rest/handlers ──► rest/middleware, rest/deps, database/*, utils, web
 │                  │
 └──────────────────┴──► fixgw, itchgw, loadtest ──► engine ──► config, refdata
fixgw, itchgw, loadtest ──► engine, brokers, config      (loadtest and fixgw also use latency)
```

`engine` knows nothing about FIX, ITCH or HTTP. Process-wide state still exists, but each piece lives in the package that owns it and is exported from there: `fixgw.Active` (the running FIX acceptor), `fixgw.MsgLog`, `brokers.Fix`, `brokers.Itch` and `brokers.Meta` (in `database/brokers`) (the credential and metadata stores), `latency.Fix`, and the instrument metadata in `engine` (`MetaFor`, `SetStatus`).

Direct dependencies:

| Module | Version | Used for |
|---|---|---|
| `github.com/quickfixgo/quickfix` | v0.9.6 | FIX acceptor, sessions, data-dictionary validation |
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket market streams, and the live test's link to the FIX initiator |
| `github.com/jackc/pgx/v5` | v5.10.0 | Postgres: benchmark persistence and the live-test OMS queries |

To run a test copy next to a real deployment, move all three ports and point every data file at a scratch directory, so nothing under `/opt/velexa-sim-me` gets written:

```bash
PORT=18010 FIX_PORT=15001 ITCH_PORT=15501 ME_FIX_SPEC_DIR=$PWD/spec \
ME_INSTRUMENT_FILE=/tmp/me/instruments.json ME_FIX_MSGLOG_FILE=/tmp/me/fix.log \
ME_AUDIT_FILE=/tmp/me/audit.jsonl ME_FIX_AUTH_FILE=/tmp/me/fix_brokers.json \
ME_ITCH_AUTH_FILE=/tmp/me/itch_brokers.json ME_BROKER_META_FILE=/tmp/me/broker_meta.json \
FIXS_ALLOW_FILE=/tmp/me/fix_allow.json EXCHANGE_INSTRUMENT_URL=http://127.0.0.1:1/ \
go run .
```

Setting `EXCHANGE_INSTRUMENT_URL` to a closed local port makes the engine fall back to the built-in instruments at once, instead of waiting for the 12-second timeout on the default address.

## Source files

<!-- AUTO:source-index -->
| File (under `internal/`) | Responsibility |
|---|---|
| `Cmd/serve.go` (repo root) | `Serve()`: process startup — engine, bots, FIX acceptor, stores, ITCH gateway and feed, benchmark runners, then the HTTP server. Called from `main.go`. |
| `rest/server.go` | `Server`: builds one `Handler` per feature, registers every group's routes on one `http.ServeMux` (`Routes`), and serves it (`Start`). |
| `rest/deps/deps.go` | `Deps`: the engine, bus, ITCH gateway, benchmark runners, Postgres pool and user store shared by every handler. |
| `rest/middleware/manager.go` | `Manager`: `With` wraps one route in middlewares, `Use`/`WrapMux` apply global ones (none are registered today). |
| `rest/middleware/middlewares.go`, `roles.go` | `Middlewares` (holds the user store): `Viewer` and `Admin` route middlewares built on `RequireRole`; `CurrentUser`, `RequireAdminRole`, `ActorOf`, `Authenticate`. |
| `rest/middleware/session.go` | Signed session cookie (`SignSession`, `VerifySession`, `SetSessionCookie`), `SessionUser`, `HomeFor`, `ClientIP`. |
| `rest/handlers/<feature>/handler.go` | `Handler` (shared `Deps` plus the middlewares) and `NewHandler`, one per feature package. |
| `rest/handlers/<feature>/routes.go` | `RegRoutes(mux, manager)`: every URL of the feature, with `manager.With(handler, middleware)`. This is where the access rule of each URL is set. |
| `rest/handlers/public/` | `index.go` (Welcome page, Trade Terminal), `health.go`, `market.go` (`/api/status`, `/api/market`, `/api/book`, `/api/loadtest`), `order.go`, `events.go` (SSE stream and WebSocket upgrade); ITCH WebSocket routes in `routes.go`. |
| `rest/handlers/control/` | `bots.go` (`/api/bots*`) and `session.go` (`/api/session*`). |
| `rest/handlers/bench/`, `live/` | Benchmark API and `/bench` page; live load-test API. |
| `rest/handlers/register/register.go` | `POST /register`: self-registration of `user` accounts with a profile, and the per-IP quota. |
| `rest/handlers/account/` | `me.go` (own profile and password), `users.go` (the Users page API), `helpers.go`. |
| `rest/handlers/admin/` | `login.go` (sign-in, sign-out), `dashboard.go` (`/admin`, `/user`), `status.go`, `simulator.go`, `session.go`, `fixlog.go`, `throttle.go` (login brute-force throttle). |
| `rest/handlers/broker/` | `list.go`, `detail.go`, `add.go`, `revoke.go`, `sheet.go` (broker credentials and onboarding sheet), `allowlist.go` (OMS IP allowlist), `helpers.go`, `sheet_html.go`. |
| `rest/handlers/instrument/` | `list.go`, `save.go` (add / edit), `halt.go` (halt / delist), `export.go`, `import.go` (reload / import), `rows.go` (directory rows and the DSE-style XML/CSV builders). |
| `database/users/users.go` | Console account store: roles `admin`/`user`, profile, password hashing, last-admin guard, `users.json`. |
| `database/audit/audit.go` | Credential audit log: in-memory ring plus append-only file. |
| `utils/respond.go` | `WriteJSON` and `AtoiDefault`, shared by the handlers. |
| `config/admin.go` | Console credentials from the environment and the registration switch. |
| `engine/engine.go` | Core types (`Order`, `ExecReport`, `Trade`, `MarketData`) and the `Engine`: matching loop, price-time matching, stops, icebergs, cancel/replace, sessions and the opening auction, pre-trade checks, index, snapshots, `LoadTest`. |
| `engine/book.go` | Limit order book: price levels keyed by integer hundredths, sorted price lists, FIFO queues, depth, FOK check (`CanFill`), GTD expiry, removal. |
| `engine/bots.go` | Market-maker and noise-trader bots, the pause flag (`BotsPaused`), and the tick ladder the bots quote on. |
| `engine/bus.go` | Fan-out of SSE messages to subscribers; drops messages for slow clients. |
| `engine/loader.go` | Instrument universe loading: source priority, JSON/CSV/TSV master parsing, per-symbol metadata (`InstrMeta`, `MetaFor`), symbol validation, default tick. |
| `engine/instruments.go` | Changing the universe at runtime: `ListInstruments`, auto-listing, the universe status shown in the console. |
| `engine/instref.go` | Fills in asset class, category, sector and board from `internal/refdata` and a deterministic classifier for synthetic tickers. |
| `engine/universe_data.go` | The built-in list of 430 DSE instruments, used when no master is available. |
| `fixgw/fixacceptor.go` | FIX acceptor: session config, NewOrderSingle/Cancel/Replace handlers, ExecutionReport and OrderCancelReject building, session controls. |
| `fixgw/msglog.go` | quickfix log that captures FIX messages to an in-memory log and an append-only file. |
| `itchgw/feed.go` | SoupBinTCP DSE-ITCH server: login, heartbeats, start-of-day directory, trade and depth publishing, binary message builders. |
| `itchgw/gateway.go` | WebSocket gateway for the trader SPA: `/market`, `/marketdepth`, `/index`, and the `/events` time-and-sales/news socket. |
| `database/brokers/fixauth.go` | FIX logon allowlist (`fix_brokers.json`): CompID to password, enforced when `ME_FIX_AUTH=on`. |
| `database/brokers/itchauth.go` | ITCH feed credential store (`itch_brokers.json`). |
| `database/brokers/meta.go` | Broker metadata store (`broker_meta.json`): links each broker FIX CompID to its ITCH user and company. |
| `loadtest/bench.go` | Synthetic pipeline benchmark (`Bench`): order generator, RMS worker pool, single-goroutine matcher on seeded books. |
| `loadtest/bench_export.go` | CSV exports: the benchmark summary and the persisted `sim_orders` rows. |
| `loadtest/live.go` | Live load test (`LiveBench`): broker list, OMS test data, order generation through the OMS API, fill counting, and the destructive `Cleanup`. |
| `loadtest/pipeline_latency.go` | Collects per-stage latency from the OMS API and FIX initiator, plus the engine own FIX timing, for `/api/live/latency`. |
| `loadtest/persist.go` | Batched Postgres `COPY` writer for benchmark orders and executions. |
| `loadtest/rms.go` | `RiskEngine`: pre-trade risk checks used by the synthetic benchmark. |
| `latency/stat.go` | `Stat`: lock-free sample ring with percentiles. |
| `latency/fix.go` | `Fix`: the FIX acceptor own order-to-ExecReport timer. |
| `config/config.go` | `EnvOr`, data-file locations (`StateFile`, `SpecDir`) and the database and service endpoint settings. |
| `web/terminal.go` | The `/` trade terminal page. |
| `web/admin.go` | The admin login page and the console. |
| `web/bench.go` | The `/bench` page. |
| `refdata/dse.go` | DSE reference table: asset class, category, sector and ISIN per ticker. |
<!-- /AUTO:source-index -->

Other files in the repo:

| File | Purpose |
|---|---|
| `spec/FIXT11.xml`, `spec/FIX50SP2.xml` | QuickFIX data dictionaries (see `ME_FIX_SPEC_DIR`). |
| `deploy/regen-fixs-gateway.sh` | Generates the nginx port 5443 allowlist config and reloads nginx. |
| `deploy/nginx-443.patch` | nginx HTTPS reverse-proxy change (see [operations](operations.md#https-reverse-proxy-nginx-443patch)). |
| `deploy/state/*.json`, `brokers.tsv` | Sample data files; see [configuration](configuration.md#data-and-config-files). |
| `.claude/skills/update-docs/` | The skill that keeps these docs in sync with the code. |

## Conventions

- **Configuration.** Read every setting with `config.EnvOr("NAME", "default")`, usually through a small accessor function as in `internal/config` and `internal/engine/loader.go`. Don't add flags or a config file.
- **Engine state.** Only the matching goroutine changes books, resting orders and the phase. Code outside it sends a function down `e.ctrl` and waits for the result, as `Cancel`, `Replace`, `SetPhase` and `ListInstruments` do. Readers take `e.mu.RLock()`. The instrument tables are copy-on-write; never change them in place.
- **Rejects always carry a reason.** Every `REJECTED` report sets `Reason`, which goes out in FIX tag 58. `TestRejectsCarryAReason` and `TestFixValidateNewReasons` enforce this.
- **HTTP responses.** Use `utils.WriteJSON`. Action endpoints return `{"ok": true, …}` or `{"ok": false, "error": "…"}`.
- **Admin endpoints.** Add the handler to the matching package under `rest/handlers/` (`admin`, `broker`, `account` or `instrument`), list it in that package's `routes.go` with `h.middlewares.Admin` (or `Viewer` for read-only pages), and call `audit.Record(middleware.ActorOf(r), …)` for anything that changes state.
- **FIX tags** are numeric `quickfix.Tag` constants at the top of `internal/fixgw/fixacceptor.go`. The code doesn't use quickfix's generated message packages.
- **Logging.** Use the standard `log` package with a subsystem prefix: `[FIX]`, `[ITCH]`, `[universe]`, `[bench]`.
- **Web pages** are HTML strings in Go constants in `internal/web` (`terminal.go`, `bench.go`, `admin.go`). There's no frontend build step and there are no static files.
- **Theme.** Every page has a dark / light button (sun and moon icon). The choice is kept in the browser (`localStorage` key `me-theme`) and shared by the console, the embedded Trade Terminal and the benchmark page. Page styles live in `internal/web/theme.go` (`*LightCSS` overrides on `data-theme="light"`); dark is the default.

## Common changes

### Add an environment variable

1. Read it with `envOr` next to related settings, through an accessor function.
2. Run `/update-docs`. The variable then shows up in the [configuration](configuration.md#environment-variables) table, and `--check` fails until it's documented.

### Add an HTTP route

1. Write the handler as a method on `Handler` in the matching package under `rest/handlers/` (public, control, bench, live, register, account, instrument, broker or admin), then add a `mux.Handle(...)` line to that package's `routes.go`: `manager.With(http.HandlerFunc(h.Name), h.middlewares.Admin)` (or `.Viewer`, or no middleware for a public URL). `TestRouteTableIsComplete` lists every URL, so add the new one to `wantRoutes` in `rest/routes_test.go` too.
2. If the route should accept only one method, check `r.Method` as `/api/order` does.
3. Read or change engine state as described under **Engine state** in [Conventions](#conventions).
4. Run `/update-docs` to add it to the [route index](http-api.md#route-index).

### Handle a new FIX message type

1. Add any new tag constants to the `const` block in `internal/fixgw/fixacceptor.go`.
2. Add a `case` to the `switch` in `(*FixAcceptor).FromApp`.
3. Write the handler. Do engine work through the `Engine` methods, never by touching books directly.
4. Reply with `fa.send` (ExecutionReport) or build a message with `quickfix.NewMessage` and send it with `quickfix.SendToTarget`, as `sendCxlReject` does.
5. With data-dictionary validation on (the default for FIXT.1.1), inbound messages must be valid against `spec/FIX50SP2.xml`.
6. Add a test next to `TestFixValidateNewReasons` for any new validation.

### Add an order type or time-in-force

1. Add the constant in `internal/engine/engine.go`.
2. Map the FIX value in `handleNewOrder` (OrdType) or `mapTIF` (TimeInForce).
3. Handle it in `match` and, if it rests, in `TIF.resting`.
4. Update the sweeps in `internal/engine/book.go` if it needs expiry-style removal.

### Add or change instruments

Instruments are data, not code. Supply a master through `ME_INSTRUMENT_FILE`, `EXCHANGE_INSTRUMENT_URL` or the admin import (see [Instrument universe](architecture.md#instrument-universe)). Only edit the built-in `universe` list in `universe_data.go` to change the fallback used when no master is available.

## Keeping these docs current

The docs in `docs/` and `README.md` are maintained by the `/update-docs` Claude Code skill (`.claude/skills/update-docs/`). Each time it runs, it:
- compares source hashes against `docs/.sync-manifest`
- updates only the sections affected by changed files
- rebuilds the tables between `<!-- AUTO:… -->` markers
- checks that every env var and route is documented

You can run its checker directly:

```bash
bash .claude/skills/update-docs/scripts/inventory.sh --check
```

You can edit the text outside the AUTO tables freely; the skill keeps your wording.
