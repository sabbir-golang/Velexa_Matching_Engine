# Velexa Matching Engine

A Go simulator of the Dhaka Stock Exchange (DSE) matching engine, built so an order-management system (OMS) can be developed and load-tested end to end without the real exchange. Brokers' FIX initiators send orders to it; it matches them on in-memory limit order books and publishes market data over DSE-ITCH (SoupBinTCP), WebSockets and Server-Sent Events. There are no simulated traders: books hold only real orders, and prices move only when they trade, as on the DSE.

## Features

- **FIX 5.0 SP2 acceptor** (FIXT.1.1, on quickfixgo): NewOrderSingle, OrderCancelRequest and OrderCancelReplaceRequest. Optional per-broker logon passwords. A message log you can search.
- **Price-time matching**:
  - order types: limit, market, stop and stop-limit
  - time-in-force: DAY, GTC, IOC, FOK and GTD
  - iceberg orders
  - cancel/replace
  - a Pre-Open call auction with a single-price opening uncross
- **DSE pre-trade rules** for FIX orders: tick size from the security master, and the tiered daily price band.
- **Market data**:
  - the DSE-ITCH market, index and news feeds (SoupBinTCP) for the exchange's C++ `itch` service, streaming the real order book
  - DSE-style FIX execution reports (ack, one report per trade to both sides) and the DSE trading day on the Dhaka clock
  - WebSocket streams in the trader SPA's JSON format (`/market`, `/marketdepth`, `/index`, `/events`)
  - an SSE stream
  - a DSEX-style index
- **Instrument universe** taken, in order of preference, from an operator file, the exchange security master or 430 built-in DSE instruments. Unknown symbols are listed automatically, and new masters can be imported at runtime.
- **Admin console** for:
  - broker FIX/ITCH credentials and printable onboarding sheets
  - the source-IP allowlist for the external FIX gateway
  - FIX session control
  - the message log
  - the audit log
  - console users with roles: `admin` (full access) and `user` (read-only: dashboard, Trade Terminal, broker credentials, message log), managed on the Users page

## Quick start

You need Go 1.25 or newer.

```bash
make run                          # same as: go run .
# or build a binary: make build && ./bin/velexa-sim-me
```

Start it from the repository root: it finds the FIX dictionaries in `spec/` and keeps its data files in `data/` there (when `/opt/velexa-sim-me` doesn't exist).

Then:

- open <http://localhost:8010/> for the trade terminal, <http://localhost:8010/admin> for the admin console (non-admin accounts use <http://localhost:8010/user>)
- check it's up with `curl -s localhost:8010/api/status`
- send an order:

  ```bash
  curl -s -X POST localhost:8010/api/order -H 'Content-Type: application/json' \
    -d '{"symbol":"GP","side":"BUY","type":"LIMIT","price":331.2,"qty":100}'
  ```

Without other settings, the engine:
- runs on the 430 built-in DSE instruments, after trying the default exchange URL, which can take up to 12 s
- has FIX and ITCH authentication turned off
- looks for its data files under `/opt/velexa-sim-me/`, or in `./data/` when that directory does not exist

For a real deployment, see [configuration](docs/configuration.md) and [operations](docs/operations.md). Before exposing the engine to anyone, read the [security notes](docs/configuration.md#security-notes).

## Ports

| Port | Env var | Service |
|---|---|---|
| 8010 | `PORT` | HTTP: web pages, JSON API, WebSockets, SSE, admin console |
| 5001 | `FIX_PORT` | FIX acceptor |
| 5501 | `ITCH_PORT` | SoupBinTCP ITCH market-data feed |
| 5502 | `ME_ITCH_INDEX_PORT` | SoupBinTCP ITCH index feed |
| 5503 | `ME_ITCH_NEWS_PORT` | SoupBinTCP ITCH news feed |
| 5443 | n/a (nginx) | External FIX gateway, IP-allowlisted, forwards to 5001 |
| 80 / 443 | n/a (nginx) | Reverse proxy to 8010 |

## Documentation

| Doc | Covers |
|---|---|
| [Architecture](docs/architecture.md) | Components, order lifecycle, matching rules, sessions, pre-trade checks, persistence, instrument universe |
| [Configuration](docs/configuration.md) | Every environment variable and data file, directory layout, security notes |
| [HTTP API](docs/http-api.md) | Every route, request and response, WebSocket and SSE streams, admin API |
| [Protocols](docs/protocols.md) | FIX acceptor (sessions, tags, execution reports) and ITCH feed (framing, message layouts) |
| [Operations](docs/operations.md) | Running, nginx front ends, broker onboarding, logs, health checks, troubleshooting |
| [Development](docs/development.md) | Source layout, build and test, conventions, how to make common changes |

These docs are kept in sync with the code by the `/update-docs` Claude Code skill in `.claude/skills/update-docs/`.
# Velexa_Matching_Engine

## Project layout

```
main.go                entry point: calls cmd.Serve()
Cmd/serve.go           wiring: builds the engine, starts the gateways, serves HTTP
internal/
  rest/                HTTP layer: server.go (mux), middleware/, handlers/<feature>/ (handler.go, routes.go, one file per endpoint), deps/
  database/            file-backed stores: users/ (console accounts), brokers/ (FIX/ITCH logons, metadata), audit/
  engine/              matching-engine core: orders, books, matching, sessions, instrument universe
  fixgw/               FIX 5.0 SP2 acceptor and message log
  itchgw/              DSE-ITCH feed and WebSocket gateway
  utils/ config/ web/ refdata/   JSON helpers, settings, HTML pages, DSE reference data
spec/  docs/  tools/  deploy/      FIX dictionaries, documentation, scripts, server files
Makefile               make build | run | test | vet | report
```

See [development](docs/development.md) for the source-file map and conventions.
