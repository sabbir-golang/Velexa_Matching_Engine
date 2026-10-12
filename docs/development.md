# Development

This page is for developers changing the engine's code: how the source is laid out, how to build and test it, the conventions the code follows, and step-by-step notes for common changes. For how the engine behaves at runtime, read [architecture](architecture.md) first.

## Build and test

You need Go 1.25 or newer (from `go.mod`). Run everything from the repo root: the engine finds `spec/` and writes `data/` relative to it.

```bash
make build            # builds bin/velexa-sim-me (git-ignored)
make run              # go run ./cmd/matching-engine
make vet
make test             # unit tests plus the FIX conformance suite (about 40 s)
```

## Project layout

```
cmd/matching-engine/   entry point: a thin main that calls me.Run()
internal/me/           the engine: matching, FIX acceptor, ITCH feed/gateway, admin console,
                       instrument loading, load tests, embedded web pages, tests
internal/refdata/      DSE reference tables (asset class, category, sector, ISIN per ticker)
spec/                  QuickFIX data dictionaries (FIXT11.xml, FIX50SP2.xml)
docs/                  documentation and the DSE conformance report
tools/                 helper scripts (conformance_report.py)
deploy/                nginx patch, gateway script and sample state files for a server
Makefile               build / run / test shortcuts
```

`internal/me` is one package on purpose: the engine, gateways and admin API share state through a few process-wide tables (the instrument metadata, the broker allowlists, the message log). Splitting it further would mean exporting and threading those through every call, which changes code rather than moving it. The source-file table below lists the files in `internal/me/` (and `internal/refdata/`).

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
go run ./cmd/matching-engine
```

Setting `EXCHANGE_INSTRUMENT_URL` to a closed local port makes the engine fall back to the built-in instruments at once, instead of waiting for the 12-second timeout on the default address.

## Source files

<!-- AUTO:source-index -->
| File | Responsibility |
|---|---|
| `admin.go` | Admin console: login, cookie auth (`requireAdmin`), login throttling, audit log, `/admin/api/*` handlers, IP allowlist file, onboarding endpoint values (`meEndpoints`). |
| `admin_html.go` | Embedded HTML for the admin login page, the console, and the printable broker onboarding sheet (`brokerSheetHTML`). |
| `bench.go` | Synthetic pipeline benchmark (`Bench`): order generator, RMS worker pool, single-goroutine matcher on seeded books, and the latency `Stat` type (lock-free sample ring with percentiles). |
| `bench_export.go` | CSV exports: the benchmark summary and the persisted `sim_orders` rows. |
| `benchweb.go` | Embedded HTML for the `/bench` page. |
| `book.go` | Limit order book: price levels keyed by integer hundredths, sorted price lists, FIFO queues, depth, FOK check (`canFill`), GTD expiry, removal. |
| `bots.go` | Market-maker and noise-trader bots, the pause flag, and the bots' tick ladder (`tickSize`, `roundTick`). |
| `brokermeta.go` | Broker metadata store (`broker_meta.json`): links each broker's FIX CompID to its ITCH user and company. |
| `bus.go` | Fan-out of SSE messages to subscribers; drops messages for slow clients. |
| `config.go` | `envOr` and the database and service endpoint settings (`meDSN`, `omsDSN`, OMS API, FIX initiator, audit URLs). |
| `engine.go` | Core types (`Order`, `ExecReport`, `Trade`, `MarketData`) and the `Engine`: matching loop, price-time matching, stops, icebergs, cancel/replace, sessions and the opening auction, pre-trade checks, index, snapshots, `LoadTest`. |
| `fixacceptor.go` | FIX acceptor: session config, Logon authentication (`fixAuth`), NewOrderSingle/Cancel/Replace handlers, ExecutionReport and OrderCancelReject building, session controls. |
| `fixmsglog.go` | quickfix log that captures FIX messages to an in-memory log and an append-only file. |
| `instruments.go` | Changing the universe at runtime: `ListInstruments`, auto-listing, the admin instrument endpoints, atomic save of the master file. |
| `instruments_test.go` | Tests for master parsing, source priority, listing, order book IDs, auto-listing and reject reasons. |
| `itchauth.go` | ITCH feed credential store (`itch_brokers.json`). |
| `itchfeed.go` | SoupBinTCP DSE-ITCH server: login, heartbeats, start-of-day directory, trade and depth publishing, binary message builders. |
| `itchgw.go` | WebSocket gateway for the trader SPA: `/market`, `/marketdepth`, `/index`, and the `/events` time-and-sales/news socket. |
| `latencyproxy.go` | Collects per-stage latency from the OMS API and FIX initiator, plus this engine's own FIX timing (`meFixLatency`), for `/api/live/latency`. |
| `live.go` | Live load test (`LiveBench`): broker list, OMS test data, order generation through the OMS API, fill counting, and the destructive `Cleanup`. |
| `loader.go` | Instrument universe loading: source priority, JSON/CSV/TSV master parsing, per-symbol metadata (`instrMeta`), symbol validation, default tick. |
| `run.go` | Built-in 400-instrument fallback list, process startup, and the public HTTP routes. |
| `persist.go` | Batched Postgres `COPY` writer for benchmark orders and executions. |
| `rms.go` | `RiskEngine`: pre-trade risk checks used by the synthetic benchmark. |
| `web.go` | Embedded HTML for the `/` trade terminal page. |
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

- **Configuration.** Read every setting with `envOr("NAME", "default")`, usually through a small accessor function as in `config.go` and `loader.go`. Don't add flags or a config file.
- **Engine state.** Only the matching goroutine changes books, resting orders and the phase. Code outside it sends a function down `e.ctrl` and waits for the result, as `Cancel`, `Replace`, `SetPhase` and `ListInstruments` do. Readers take `e.mu.RLock()`. The instrument tables are copy-on-write; never change them in place.
- **Rejects always carry a reason.** Every `REJECTED` report sets `Reason`, which goes out in FIX tag 58. `TestRejectsCarryAReason` and `TestFixValidateNewReasons` enforce this.
- **HTTP responses.** Use `writeJSON`. Action endpoints return `{"ok": true, …}` or `{"ok": false, "error": "…"}`.
- **Admin endpoints.** Register them in `registerAdmin` (or `registerInstrumentAdmin`), wrap them in `requireAdmin`, and call `recordAudit` for anything that changes state.
- **FIX tags** are numeric `quickfix.Tag` constants at the top of `fixacceptor.go`. The code doesn't use quickfix's generated message packages.
- **Logging.** Use the standard `log` package with a subsystem prefix: `[FIX]`, `[ITCH]`, `[universe]`, `[bench]`.
- **Web pages** are HTML strings in Go constants (`web.go`, `benchweb.go`, `admin_html.go`). There's no frontend build step and there are no static files.

## Common changes

### Add an environment variable

1. Read it with `envOr` next to related settings, through an accessor function.
2. Run `/update-docs`. The variable then shows up in the [configuration](configuration.md#environment-variables) table, and `--check` fails until it's documented.

### Add an HTTP route

1. Public routes go in `main` (`run.go`); admin routes go in `registerAdmin` (`admin.go`), wrapped in `requireAdmin`.
2. If the route should accept only one method, check `r.Method` as `/api/order` does.
3. Read or change engine state as described under **Engine state** in [Conventions](#conventions).
4. Run `/update-docs` to add it to the [route index](http-api.md#route-index).

### Handle a new FIX message type

1. Add any new tag constants to the `const` block in `fixacceptor.go`.
2. Add a `case` to the `switch` in `(*FixAcceptor).FromApp`.
3. Write the handler. Do engine work through the `Engine` methods, never by touching books directly.
4. Reply with `fa.send` (ExecutionReport) or build a message with `quickfix.NewMessage` and send it with `quickfix.SendToTarget`, as `sendCxlReject` does.
5. With data-dictionary validation on (the default for FIXT.1.1), inbound messages must be valid against `spec/FIX50SP2.xml`.
6. Add a test next to `TestFixValidateNewReasons` for any new validation.

### Add an order type or time-in-force

1. Add the constant in `engine.go`.
2. Map the FIX value in `handleNewOrder` (OrdType) or `mapTIF` (TimeInForce).
3. Handle it in `match` and, if it rests, in `TIF.resting`.
4. Update `book.go` sweeps if it needs expiry-style removal.

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
