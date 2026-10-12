# Velexa Simulator Matching Engine

A Go simulator of the Dhaka Stock Exchange (DSE) matching engine, built so an order-management system (OMS) can be developed and load-tested end to end without the real exchange. Brokers' FIX initiators send orders to it; it matches them on in-memory limit order books and publishes market data over DSE-ITCH (SoupBinTCP), WebSockets and Server-Sent Events. Simulated market makers and noise traders keep every instrument quoted and trading.

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
  - a SoupBinTCP DSE-ITCH feed for the exchange's C++ `itch` service
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
- **Load testing**: a synthetic pipeline benchmark (generation → RMS → matching → Postgres) and a live test that sends real orders through the full OMS pipeline, both with latency percentiles and CSV export.

## Quick start

You need Go 1.25 or newer.

```bash
go build                          # produces ./velexa-sim-me
ME_FIX_SPEC_DIR=$PWD/spec ./velexa-sim-me
```

Then:

- open <http://localhost:8010/> for the trade terminal, <http://localhost:8010/bench> for benchmarks and <http://localhost:8010/admin> for the admin console
- check it's up with `curl -s localhost:8010/api/status`
- send an order:

  ```bash
  curl -s -X POST localhost:8010/api/order -H 'Content-Type: application/json' \
    -d '{"symbol":"GP","side":"BUY","type":"LIMIT","price":331.2,"qty":100}'
  ```

Without other settings, the engine:
- runs on the 430 built-in DSE instruments, after trying the default exchange URL, which can take up to 12 s
- has FIX and ITCH authentication turned off
- looks for its data files under `/opt/velexa-sim-me/`

For a real deployment, see [configuration](docs/configuration.md) and [operations](docs/operations.md). Before exposing the engine to anyone, read the [security notes](docs/configuration.md#security-notes).

## Ports

| Port | Env var | Service |
|---|---|---|
| 8010 | `PORT` | HTTP: web pages, JSON API, WebSockets, SSE, admin console |
| 5001 | `FIX_PORT` | FIX acceptor |
| 5501 | `ITCH_PORT` | SoupBinTCP ITCH market-data feed |
| 5443 | n/a (nginx) | External FIX gateway, IP-allowlisted, forwards to 5001 |
| 80 / 443 | n/a (nginx) | Reverse proxy to 8010 |

## Documentation

| Doc | Covers |
|---|---|
| [Architecture](docs/architecture.md) | Components, order lifecycle, matching rules, sessions, pre-trade checks, bots, persistence, instrument universe |
| [Configuration](docs/configuration.md) | Every environment variable and data file, directory layout, security notes |
| [HTTP API](docs/http-api.md) | Every route, request and response, WebSocket and SSE streams, admin API |
| [Protocols](docs/protocols.md) | FIX acceptor (sessions, tags, execution reports) and ITCH feed (framing, message layouts) |
| [Operations](docs/operations.md) | Running, nginx front ends, broker onboarding, load tests, logs, health checks, troubleshooting |
| [Development](docs/development.md) | Source layout, build and test, conventions, how to make common changes |

These docs are kept in sync with the code by the `/update-docs` Claude Code skill in `.claude/skills/update-docs/`.
# Velexa_Matching_Engine
