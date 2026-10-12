# HTTP API

This page documents everything served on the HTTP port (`PORT`, default `8010`): the web pages, the JSON API, the CSV exports, the WebSocket market-data streams, the `/events` stream and the admin console API. All routes are registered on one `http.ServeMux` in `run.go`, `admin.go` and `instruments.go`.

Unless a route says otherwise:

- Responses are JSON, written by `writeJSON` in `run.go`.
- The handler doesn't check the HTTP method, so any method works. The bundled pages use `GET` for reads and `POST` for actions; do the same.
- Errors inside a JSON action come back as HTTP 200 with `{"ok": false, "error": "…"}`.

## Route index

<!-- AUTO:routes -->
| Method | Path | Auth | Purpose |
|---|---|---|---|
| any | `/` | none | Trade terminal web page (market watch, order book, trade tape, order entry). Any other unmatched path returns 404. |
| any | `/health` | none | Gateway health: `ok`, session `phase`, `running`, and the list of feeds served. |
| any | `/healthz` | none | Liveness probe; returns the plain text `ok`. |
| GET (WebSocket) | `/market` | none | Streams one JSON row per instrument every 500 ms. |
| GET (WebSocket) | `/marketdepth` | none | Streams 10-level depth per order book every 500 ms. |
| GET (WebSocket) | `/index` | none | Streams the DSEX index and market breadth every 500 ms. |
| GET | `/events` | none | WebSocket upgrade: time-and-sales and news stream. Plain GET: Server-Sent Events market stream. |
| any | `/bench` | none | Pipeline benchmark web page. |
| any | `/api/status` | none | Engine counters: phase, index, instrument/order/trade counts, universe source. |
| any | `/api/market` | none | Full market watch: index, phase and one row per instrument. |
| any | `/api/book` | none | Order book (10 levels per side), market data and last 30 trades for `?symbol=`. |
| POST | `/api/order` | none | Submits an order and returns its execution report. |
| any | `/api/session` | none | Current trading phase. |
| any | `/api/session/preopen` | none | Switches to the Pre-Open call auction. |
| any | `/api/session/open` | none | Switches to Continuous Trading, running the opening auction first. |
| any | `/api/bots` | none | Whether the market-maker and noise-trader bots are paused. |
| any | `/api/bots/pause` | none | Pauses the bots. |
| any | `/api/bots/resume` | none | Resumes the bots. |
| any | `/api/loadtest` | none | Times raw matching throughput on one symbol. |
| any | `/api/bench/start` | none | Starts the synthetic pipeline benchmark. |
| any | `/api/bench/stop` | none | Stops the synthetic benchmark. |
| any | `/api/bench/stats` | none | Live benchmark metrics. |
| any | `/api/bench/summary.csv` | none | Downloads the benchmark report as CSV. |
| any | `/api/bench/orders.csv` | none | Downloads persisted benchmark orders as CSV. |
| any | `/api/live/start` | none | Starts the live load test through the real OMS pipeline. |
| any | `/api/live/stop` | none | Stops the live load test. |
| any | `/api/live/stats` | none | Live load-test metrics and the broker list. |
| any | `/api/live/latency` | none | Per-stage latency gathered from the OMS API, the FIX initiator and this engine. |
| any | `/api/live/cleanup` | none | **Destructive.** Truncates OMS order tables in every broker database (see [below](#live-load-test)). |
| any | `/admin/login` | none | Login form (GET) and login submit (POST). |
| any | `/admin/logout` | none | Clears the admin cookie. |
| any | `/admin` | admin login | Admin console web page. |
| any | `/admin/api/status` | admin login | Engine state, FIX sessions, listener checks, recent events and audit log. |
| any | `/admin/api/brokers` | admin login | Broker records and credential counts. |
| any | `/admin/api/broker/detail` | admin login | One broker's credentials and connection details (`?sender=`). |
| any | `/admin/api/broker/add` | admin login | Issues FIX and ITCH credentials for a broker. |
| any | `/admin/api/broker/del` | admin login | Revokes a broker's FIX and ITCH credentials and metadata. |
| any | `/admin/api/broker/sheet` | admin login | Renders a printable broker onboarding sheet (HTML). |
| any | `/admin/api/allowlist` | admin login | Lists the external FIX gateway IP allowlist. |
| any | `/admin/api/allowlist/add` | admin login | Adds an IP to the allowlist. |
| any | `/admin/api/allowlist/del` | admin login | Removes an IP from the allowlist. |
| any | `/admin/api/sim` | admin login | Bot and session controls (`?action=`). |
| any | `/admin/api/session` | admin login | Disconnects a FIX session or resets its sequence numbers. |
| any | `/admin/api/fixlog` | admin login | Searches the FIX message log. |
| any | `/admin/api/instruments` | admin login | Lists the instrument universe and auto-listed symbols. |
| POST | `/admin/api/instruments/reload` | admin login | Reloads the security master from the configured sources. |
| POST | `/admin/api/instruments/import` | admin login | Imports a security master from the request body. |
<!-- /AUTO:routes -->

None of the `/api/*` routes need authentication. See [Security notes](configuration.md#security-notes).

## Market data and status

### `GET /api/status`

```bash
curl -s localhost:8010/api/status
```

```json
{"index":6000.3,"instruments":400,"orders":7498,"phase":"Continuous Trading","running":true,"trades":420,
 "universe":{"auto_list":true,"auto_listed":0,"count":400,
             "source":"built-in DSE reference list (430 instruments, DSE end-of-day file 14 Sep 2023) — symbols outside this list will not trade"}}
```

`orders` and `trades` are the engine's running sequence numbers; the bots count too. `universe.source` says where the instrument list came from (see [Instrument universe](architecture.md#instrument-universe)).

### `GET /api/market`

Returns `{"index", "phase", "rows"}`. `rows` holds one `MarketData` object per instrument, sorted by symbol, with these fields: `symbol`, `name`, `last`, `open`, `high`, `low`, `prev`, `change`, `change_pct`, `bid`, `bid_qty`, `ask`, `ask_qty`, `volume` and `trades`.

### `GET /api/book?symbol=SYM`

```bash
curl -s 'localhost:8010/api/book?symbol=GP'
```

Returns `{"symbol", "md", "bids", "asks", "trades"}`:
- `bids` and `asks` hold up to 10 levels each, best first, as `{price, qty, orders}`.
- `md` is the `MarketData` row.
- `trades` holds up to 30 recent trades for the symbol, newest first, as `{id, symbol, price, qty, aggressor, time}`.

An unknown symbol returns `{"error":"unknown symbol"}`.

### `/health` and `/healthz`

`/healthz` returns the plain text `ok`. `/health` returns:

```json
{"feeds":{"depth":true,"index":true,"market":true,"news":true},"ok":true,"phase":"Continuous Trading","running":true}
```

The `feeds` values are constant. The only live fields are `phase` and `running`.

## Order entry

### `POST /api/order`

Submits one order and waits up to 3 seconds for its execution report.

```bash
curl -s -X POST localhost:8010/api/order -H 'Content-Type: application/json' \
  -d '{"symbol":"GP","side":"BUY","type":"LIMIT","price":331.2,"qty":100,"cl_ord_id":"DOC-1"}'
```

| Field | Type | Notes |
|---|---|---|
| `symbol` | string | Upper-cased and trimmed. Must be listed, or auto-listable when `ME_AUTO_LIST` is on. |
| `side` | string | `SELL` or `sell` means sell; anything else means buy. |
| `type` | string | `MARKET` or `market` means a market order; anything else means a limit order. A `price` of 0 or less also makes it a market order. |
| `price` | number | Limit price, rounded to 2 decimals. |
| `qty` | integer | Must be greater than 0. |
| `actor` | string | Free-text owner tag; defaults to `API`. |
| `cl_ord_id` | string | Client order ID. Needed if the order should later be cancellable (only FIX can cancel). |

Orders sent here always have time-in-force DAY. They skip the tick-size and price-band checks, which apply only to FIX orders (see [Pre-trade checks](architecture.md#pre-trade-checks-fix-orders-only)). Stop orders, icebergs, cancel and replace are available only over FIX.

The response is an `ExecReport`, returned with HTTP 200 whatever the order status:

```json
{"order_id":7559,"cl_ord_id":"DOC-1","symbol":"GP","side":"BUY","status":"FILLED",
 "fills":[{"id":421,"symbol":"GP","price":331,"qty":100,"aggressor":"BUY","time":"2026-10-11T14:59:29.375+06:00"}],
 "filled_qty":100,"leaves_qty":0,"price":331.2,"avg_px":331,"last_px":331,"last_qty":100,"order_qty":100}
```

- `status` is one of `NEW`, `PARTIALLY_FILLED`, `FILLED`, `CANCELED` or `REJECTED`. A rejection includes a `reason`.
- `board` and `isin` are added when the security master provides them.

Errors that come back as plain text instead of a report:

| Status | Body | Cause |
|---|---|---|
| 405 | `POST only` | Any method other than POST. |
| 400 | `malformed order JSON` | The body isn't valid JSON. |
| 400 | `invalid quantity …` | `qty` is 0 or less. |
| 400 | `unknown symbol …` | The symbol isn't listed and can't be auto-listed. |

## Session and bot controls

| Route | Response | Effect |
|---|---|---|
| `/api/session` | `{"phase": "…"}` | Shows the current phase. |
| `/api/session/preopen` | `{"phase": "Pre-Open"}` | Starts collecting limit orders for the call auction. Market orders are rejected. |
| `/api/session/open` | `{"phase": "Continuous Trading"}` | Runs the opening auction for each instrument, then triggers any stop orders that have been hit. |
| `/api/bots` | `{"paused": bool}` | Shows whether the bots are paused. |
| `/api/bots/pause` | `{"paused": true}` | Bots stop sending orders; their goroutines keep running. |
| `/api/bots/resume` | `{"paused": false}` | Bots resume. |

```bash
curl -s -X POST localhost:8010/api/session/preopen
curl -s -X POST localhost:8010/api/session/open
```

The phase names and the auction itself are described in [architecture](architecture.md#trading-sessions).

## Benchmarks

### `GET /api/loadtest?n=N&symbol=SYM`

Measures how fast the live engine matches. It generates `n` orders on one symbol (about 30% market, the rest limit orders within ±0.5% of the reference price), feeds them to the matching goroutine, times them, then removes the leftover orders. `n` defaults to 100000 and is capped at 2,000,000. An unknown or missing `symbol` falls back to the first instrument.

```bash
curl -s 'localhost:8010/api/loadtest?n=20000&symbol=GP'
# {"elapsed_ms":16,"ns_per_order":814,"orders":20000,"orders_per_sec":1228331,"symbol":"GP"}
```

### Synthetic pipeline benchmark: `/api/bench/*`

Runs a generate → RMS → match → Postgres pipeline on separate books seeded with 300 levels per side. It doesn't touch the live engine's books. See [operations](operations.md#synthetic-pipeline-benchmark) for how to run it.

`/api/bench/start` takes a JSON body:

| Field | Default when the body omits it | Meaning |
|---|---|---|
| `rate` | `0` | Target orders/sec; `0` means as fast as possible. |
| `total` | `0` | Orders to generate; `0` means run until stopped. |
| `rms` | `true` | Run the RMS checks in `rms.go`. |
| `persist` | `true` | Write orders and executions to Postgres. Truncates `sim_orders` and `sim_execs` at start. |
| `side` | `""` | `BUY`, `SELL`, or mixed (`BOTH` or empty). |

```bash
curl -s -X POST localhost:8010/api/bench/start -d '{"rate":8000,"total":100000,"rms":true,"persist":false}'
curl -s localhost:8010/api/bench/stats
curl -s -X POST localhost:8010/api/bench/stop
```

Calling start with no body runs until stopped, at full speed, with persistence on. A second start while one is running returns `{"ok":false,"error":"benchmark already running"}`.

`/api/bench/stats` returns:
- counters: `generated`, `rms_pass`, `rms_rej`, `matched`, `trades`, `persisted`
- rates: `orders_per_sec`, `trades_per_sec`
- `reject_reasons`, keyed by RMS code
- per-stage latency objects `stage_rms`, `stage_match`, `stage_persist` and `stage_e2e`, each `{count, avg_us, p50_us, p95_us, p99_us}`
- a sampled `tape`
- `target_tps` (8000)

`/api/bench/summary.csv` downloads `velexa_bench_summary.csv`: throughput, per-stage latency in microseconds, and RMS reject reasons.

`/api/bench/orders.csv?limit=N` downloads `velexa_bench_orders.csv`, streaming rows from `sim_orders` newest first. `limit` defaults to 100000 and can't exceed 2,000,000. It returns 500 if the Postgres pool couldn't be created.

### Live load test

Drives real orders through the external pipeline (OMS API → RMS → FIX initiator → this engine → blotter). See [operations](operations.md#live-load-test) for prerequisites.

`/api/live/start` takes a JSON body:

| Field | Meaning |
|---|---|
| `broker` | An OMS subdomain from the broker list. `ALL`, `*` or empty means every broker, with orders split evenly between them. |
| `rate` | Target orders/sec; `0` means as fast as possible. |
| `total` | Orders to send; `0` means run until stopped. |
| `side` | `BUY` (default), `SELL`, or `BOTH` (about 30% sells). Sells only use `GP`, `SQURPHARMA`, `BEXIMCO`, `ROBI` and `BATBC`. |
| `sells` | Older flag: `true` means `BOTH` when `side` is empty. |
| `workers` | Concurrent HTTP senders; defaults to 96, maximum 4096. |

```bash
curl -s -X POST localhost:8010/api/live/start -d '{"broker":"ALL","rate":500,"total":5000,"side":"BOTH"}'
curl -s localhost:8010/api/live/stats
curl -s -X POST localhost:8010/api/live/stop
```

`/api/live/stats` returns:
- counters: `sent`, `accepted`, `rejected`, `filled`
- rates: `sent_per_sec`, `accepted_per_sec`
- latency in milliseconds: `lat_avg_ms`, `lat_p50_ms`, `lat_p95_ms`, `lat_p99_ms`
- `reject_reasons`, normalised
- a sampled `tape`
- `brokers`, the subdomains available to `broker`
- `targets`

`filled` is counted only when the test targets 8 brokers or fewer.

`/api/live/latency` returns `{"stages": [...], "target_ms": 10}`. The stages are API, RMS, Kafka, FIX Initiator, Matching Engine and DB Store, each with `count`, `avg_us`, `p50_us`, `p95_us` and `p99_us`. The values are null when the source service doesn't report that stage. The Matching Engine row is measured here: the time from receiving a FIX NewOrderSingle to sending its execution report.

> **Warning: `/api/live/cleanup` deletes data and needs no login.** It stops any running live test, then:
> 1. Finds every `terminal_*` and `tenant_*` database on the OMS Postgres.
> 2. In each one, runs `TRUNCATE orders_fixorder, orders_fixorderexec, orders_fixmessage, orders_fixorderstatus RESTART IDENTITY CASCADE`, and resets intraday cash and positions in `broker_house_clientcashlimit` and `broker_house_clientposition`.
> 3. Truncates `client_order_routes` in `bdtask_trade_master`.
> 4. Truncates `sim_orders` and `sim_execs` in `velexa_sim`.
> 5. Posts to `ME_AUDIT_URL/purge`.
>
> The code comment says only simulator (`LIVE-*`) rows are touched, but the statements truncate whole tables. Point it only at OMS databases that hold nothing but test data.

## Streams

### WebSocket market streams: `/market`, `/marketdepth`, `/index`

Served by `ItchGateway` in `itchgw.go` in the JSON shape the trader SPA expects. On connect, the client gets a full snapshot of its stream, then a full refresh every 500 ms. Messages from the client are read and ignored. The server accepts any `Origin`.

- `/market` sends one frame per instrument. The keys are `ob` (order book ID), `sn` (symbol), `fn` (name), `fln`, `ic` (ISIN), `b` (board label: `Main Board`, `Small Cap Board` or `Alternative Trading Board`), `g`, `sc`, `it`, `mt`, `d` (change), `s` (status: `nt` not traded, `u` up, `d` down, `uc` unchanged), `r` (previous close), `c`/`lp` (last), `o`, `h`, `l`, `v` (volume), `bd`/`ak` (bid/ask), `bq`/`aq`, `cg` (change %), `tds` (trades) and `ltt` (Unix time).
- `/marketdepth` sends one frame per instrument: `{"ob": 100001, "100001": {"buy": [...], "sell": [...]}}`. Each level is `{o: orders, p: price, q: qty, cq: cumulative qty}`, up to 10 per side.
- `/index` sends one frame: `{"sn":"DSEX", "civ": value, "ivd": change, "ip": change %, "v", "vl", "tds", "nt", "uc", "d", "u"}`.

```bash
websocat ws://localhost:8010/index
```

### `/events`

What `/events` does depends on the request:

- **WebSocket upgrade:** the time-and-sales and news stream for the trader SPA. On connect, the client gets a fixed list of simulated news items (`{"tp":"news","d":[…]}`) and each instrument's recent trades (`{"tp":"tsm","ob":…,"d":[…]}`). After that, one `tsm` frame arrives per new trade: `{"p","v","tst","s","tsm_id"}`.
- **Plain GET:** a Server-Sent Events stream used by the trade terminal page. A `market` event arrives every 700 ms with the whole market (`{"type":"market","index","phase","rows","ts"}`), and a `trade` event for each trade (`{"type":"trade","data":{…}}`). A `: keepalive` comment is sent every 15 s. Slow clients drop messages instead of blocking the engine.

```bash
curl -N localhost:8010/events
```

## Admin console API

### Logging in

1. `POST /admin/login` with form fields `user` and `pass`, which must match `ME_ADMIN_USER` and `ME_ADMIN_PASS`.
2. On success, the server sets the `me_admin` cookie (HttpOnly, `Path=/admin`, valid for 8 hours, SameSite=Lax, and `Secure` when `X-Forwarded-Proto` is `https`) and redirects to `/admin`. The cookie value is an HMAC-SHA256 of the username, keyed with `ME_ADMIN_SECRET`. Changing the secret or the username logs everyone out.
3. After 5 failed attempts from one client IP, logins from that IP are refused for 5 minutes. The IP is taken from `CF-Connecting-IP`, then `X-Forwarded-For`, then the connection address. Those headers can be set by any client, so the throttle only holds when a proxy in front sets them.

Without a valid cookie, `/admin/api/*` returns `401 {"error":"unauthorized"}` and `/admin` redirects to `/admin/login`.

```bash
curl -s -c jar -d 'user=admin' -d "pass=$ME_ADMIN_PASS" localhost:8010/admin/login -o /dev/null
curl -s -b jar localhost:8010/admin/api/status
```

### Endpoints

| Route | Input | Output and effect |
|---|---|---|
| `/admin/api/status` | none | `phase`, `instruments`, `orders`, `trades`, `running`, `bots_paused`, `fix_auth`, `itch_auth`, `uptime`; `listeners` (TCP checks of `127.0.0.1` ports 5001, 5443, 5501 and 8010; the port numbers are fixed in code); `sessions` (connected FIX brokers with logon time and last sequence numbers); `events` (last 50 logon, logout and reject events); `audit` (last 60 entries); `universe`. |
| `/admin/api/brokers` | none | `brokers` (records from `broker_meta.json`, newest first), `fix_total`, `itch_total`. |
| `/admin/api/broker/detail` | `?sender=` | Company and contact, **FIX and ITCH passwords**, the linked ITCH user, `endpoints` (onboarding values from the `ME_PUB_*`, `ME_INT_*` and `ME_ITCH_*` variables) and active flags. |
| `/admin/api/broker/add` | JSON `{Sender, Company, Contact, FixPass, ItchUser, ItchPass}`; only `Sender` is required | Stores FIX and ITCH credentials and the metadata, and writes the audit action `ISSUE`. Missing passwords are generated (`FIX-<12 hex>` and `ITCH-<12 hex>`). A missing ITCH user defaults to the first 6 characters of the SenderCompID. Returns the issued credentials. |
| `/admin/api/broker/del` | JSON `{Sender}` | Removes the FIX credential, the linked ITCH credential and the metadata, and writes the audit action `REVOKE`. Live sessions aren't disconnected. |
| `/admin/api/broker/sheet` | Form fields `company`, `sender`, `fix_pass`, `itch_user`, `itch_pass` | HTML onboarding sheet with connection details and credentials, with copy, download and print. |
| `/admin/api/allowlist` | none | `{"list": [{ip, label}], "count"}` |
| `/admin/api/allowlist/add` | JSON `{IP, Label}` | Validates the IP, writes `fix_allow.json`, and writes the audit action `ALLOW+`. Doesn't reload nginx (see [operations](operations.md#external-fix-gateway-port-5443)). |
| `/admin/api/allowlist/del` | JSON `{IP}` | Removes the IP and writes the audit action `ALLOW-`. |
| `/admin/api/sim` | `?action=bots_pause`, `bots_resume`, `preopen` or `open` | Same effects as the matching `/api/*` controls. Returns `phase` and `bots_paused`. |
| `/admin/api/session` | `?broker=` and `?action=disconnect` or `reset` | `disconnect` sends a FIX Logout (35=5) and unregisters the session. `reset` resets the session's sequence numbers. Writes an audit entry. |
| `/admin/api/fixlog` | `?broker=` (exact, case-insensitive), `?clordid=` (substring), `?limit=` (1 to 1000, default 250) | `{"messages": [{time, session, dir, msg_type, clordid, raw}]}`, newest first, from the 4000-message in-memory log. |
| `/admin/api/instruments` | none | `count`, `source`, `file`, `exchange_url`, `auto_list`, `auto_listed` (symbols opened on the fly), `symbols`, `rows` (per instrument: `symbol`, `name`, `asset_class`, `board`, `sector`, `lot`, `isin`, `tick`, `prev`, `last`, `volume`, `status`), `summary` (`total`, `main_equity`, `bonds`, `pending`), `classes` (asset-class counts), `note`. |
| `/admin/api/instrument/add` | POST `{Symbol,Name,AssetClass,Board,Sector,Lot,Tick,Prev,ISIN,Status}` | Lists a new scrip live (book, ITCH id, bots; shows in the Trade Terminal) and saves the whole universe to `ME_INSTRUMENT_FILE`. `Board` is one of Main (A)/(B)/(N)/(Z), Bond, Mutual Fund, SME / ATB (legacy MAIN/SME/ATB still accepted). `AssetClass` is `EQ`, `MF`, `CB` or `DB` (derived from the board when omitted). `Status` is `active` (default), `halted` or `pending`. Listing a delisted symbol re-lists it. Errors: invalid symbol, already listed, bad board, bad ISIN, `Prev` <= 0. Returns `ok`, `symbol`, `count`, optional `warning`. |
| `/admin/api/instrument/update` | POST same body | Edits an existing scrip; blank/zero fields keep their current value. |
| `/admin/api/instrument/halt` | POST `{Symbol,Halted}` | `Halted:true` halts; `Halted:false` activates (also lifts a `pending` listing). New orders on a halted, pending or delisted scrip are rejected and the bots stop quoting it. |
| `/admin/api/instrument/delist` | POST `{Symbol}` | Delists: orders are rejected, it leaves the directory and the saved master (gone after a restart). |
| `/admin/api/instruments/reload` | admin login | Reloads the security master from the configured sources. |
| POST | `/admin/api/instruments/import` | admin login | Imports a security master from the request body. |
<!-- /AUTO:routes -->

None of the `/api/*` routes need authentication. See [Security notes](configuration.md#security-notes).

## Market data and status

### `GET /api/status`

```bash
curl -s localhost:8010/api/status
```

```json
{"index":6000.3,"instruments":400,"orders":7498,"phase":"Continuous Trading","running":true,"trades":420,
 "universe":{"auto_list":true,"auto_listed":0,"count":400,
             "source":"built-in DSE reference list (430 instruments, DSE end-of-day file 14 Sep 2023) — symbols outside this list will not trade"}}
```

`orders` and `trades` are the engine's running sequence numbers; the bots count too. `universe.source` says where the instrument list came from (see [Instrument universe](architecture.md#instrument-universe)).

### `GET /api/market`

Returns `{"index", "phase", "rows"}`. `rows` holds one `MarketData` object per instrument, sorted by symbol, with these fields: `symbol`, `name`, `last`, `open`, `high`, `low`, `prev`, `change`, `change_pct`, `bid`, `bid_qty`, `ask`, `ask_qty`, `volume` and `trades`.

### `GET /api/book?symbol=SYM`

```bash
curl -s 'localhost:8010/api/book?symbol=GP'
```

Returns `{"symbol", "md", "bids", "asks", "trades"}`:
- `bids` and `asks` hold up to 10 levels each, best first, as `{price, qty, orders}`.
- `md` is the `MarketData` row.
- `trades` holds up to 30 recent trades for the symbol, newest first, as `{id, symbol, price, qty, aggressor, time}`.

An unknown symbol returns `{"error":"unknown symbol"}`.

### `/health` and `/healthz`

`/healthz` returns the plain text `ok`. `/health` returns:

```json
{"feeds":{"depth":true,"index":true,"market":true,"news":true},"ok":true,"phase":"Continuous Trading","running":true}
```

The `feeds` values are constant. The only live fields are `phase` and `running`.

## Order entry

### `POST /api/order`

Submits one order and waits up to 3 seconds for its execution report.

```bash
curl -s -X POST localhost:8010/api/order -H 'Content-Type: application/json' \
  -d '{"symbol":"GP","side":"BUY","type":"LIMIT","price":331.2,"qty":100,"cl_ord_id":"DOC-1"}'
```

| Field | Type | Notes |
|---|---|---|
| `symbol` | string | Upper-cased and trimmed. Must be listed, or auto-listable when `ME_AUTO_LIST` is on. |
| `side` | string | `SELL` or `sell` means sell; anything else means buy. |
| `type` | string | `MARKET` or `market` means a market order; anything else means a limit order. A `price` of 0 or less also makes it a market order. |
| `price` | number | Limit price, rounded to 2 decimals. |
| `qty` | integer | Must be greater than 0. |
| `actor` | string | Free-text owner tag; defaults to `API`. |
| `cl_ord_id` | string | Client order ID. Needed if the order should later be cancellable (only FIX can cancel). |

Orders sent here always have time-in-force DAY. They skip the tick-size and price-band checks, which apply only to FIX orders (see [Pre-trade checks](architecture.md#pre-trade-checks-fix-orders-only)). Stop orders, icebergs, cancel and replace are available only over FIX.

The response is an `ExecReport`, returned with HTTP 200 whatever the order status:

```json
{"order_id":7559,"cl_ord_id":"DOC-1","symbol":"GP","side":"BUY","status":"FILLED",
 "fills":[{"id":421,"symbol":"GP","price":331,"qty":100,"aggressor":"BUY","time":"2026-10-11T14:59:29.375+06:00"}],
 "filled_qty":100,"leaves_qty":0,"price":331.2,"avg_px":331,"last_px":331,"last_qty":100,"order_qty":100}
```

- `status` is one of `NEW`, `PARTIALLY_FILLED`, `FILLED`, `CANCELED` or `REJECTED`. A rejection includes a `reason`.
- `board` and `isin` are added when the security master provides them.

Errors that come back as plain text instead of a report:

| Status | Body | Cause |
|---|---|---|
| 405 | `POST only` | Any method other than POST. |
| 400 | `malformed order JSON` | The body isn't valid JSON. |
| 400 | `invalid quantity …` | `qty` is 0 or less. |
| 400 | `unknown symbol …` | The symbol isn't listed and can't be auto-listed. |

## Session and bot controls

| Route | Response | Effect |
|---|---|---|
| `/api/session` | `{"phase": "…"}` | Shows the current phase. |
| `/api/session/preopen` | `{"phase": "Pre-Open"}` | Starts collecting limit orders for the call auction. Market orders are rejected. |
| `/api/session/open` | `{"phase": "Continuous Trading"}` | Runs the opening auction for each instrument, then triggers any stop orders that have been hit. |
| `/api/bots` | `{"paused": bool}` | Shows whether the bots are paused. |
| `/api/bots/pause` | `{"paused": true}` | Bots stop sending orders; their goroutines keep running. |
| `/api/bots/resume` | `{"paused": false}` | Bots resume. |

```bash
curl -s -X POST localhost:8010/api/session/preopen
curl -s -X POST localhost:8010/api/session/open
```

The phase names and the auction itself are described in [architecture](architecture.md#trading-sessions).

## Benchmarks

### `GET /api/loadtest?n=N&symbol=SYM`

Measures how fast the live engine matches. It generates `n` orders on one symbol (about 30% market, the rest limit orders within ±0.5% of the reference price), feeds them to the matching goroutine, times them, then removes the leftover orders. `n` defaults to 100000 and is capped at 2,000,000. An unknown or missing `symbol` falls back to the first instrument.

```bash
curl -s 'localhost:8010/api/loadtest?n=20000&symbol=GP'
# {"elapsed_ms":16,"ns_per_order":814,"orders":20000,"orders_per_sec":1228331,"symbol":"GP"}
```

### Synthetic pipeline benchmark: `/api/bench/*`

Runs a generate → RMS → match → Postgres pipeline on separate books seeded with 300 levels per side. It doesn't touch the live engine's books. See [operations](operations.md#synthetic-pipeline-benchmark) for how to run it.

`/api/bench/start` takes a JSON body:

| Field | Default when the body omits it | Meaning |
|---|---|---|
| `rate` | `0` | Target orders/sec; `0` means as fast as possible. |
| `total` | `0` | Orders to generate; `0` means run until stopped. |
| `rms` | `true` | Run the RMS checks in `rms.go`. |
| `persist` | `true` | Write orders and executions to Postgres. Truncates `sim_orders` and `sim_execs` at start. |
| `side` | `""` | `BUY`, `SELL`, or mixed (`BOTH` or empty). |

```bash
curl -s -X POST localhost:8010/api/bench/start -d '{"rate":8000,"total":100000,"rms":true,"persist":false}'
curl -s localhost:8010/api/bench/stats
curl -s -X POST localhost:8010/api/bench/stop
```

Calling start with no body runs until stopped, at full speed, with persistence on. A second start while one is running returns `{"ok":false,"error":"benchmark already running"}`.

`/api/bench/stats` returns:
- counters: `generated`, `rms_pass`, `rms_rej`, `matched`, `trades`, `persisted`
- rates: `orders_per_sec`, `trades_per_sec`
- `reject_reasons`, keyed by RMS code
- per-stage latency objects `stage_rms`, `stage_match`, `stage_persist` and `stage_e2e`, each `{count, avg_us, p50_us, p95_us, p99_us}`
- a sampled `tape`
- `target_tps` (8000)

`/api/bench/summary.csv` downloads `velexa_bench_summary.csv`: throughput, per-stage latency in microseconds, and RMS reject reasons.

`/api/bench/orders.csv?limit=N` downloads `velexa_bench_orders.csv`, streaming rows from `sim_orders` newest first. `limit` defaults to 100000 and can't exceed 2,000,000. It returns 500 if the Postgres pool couldn't be created.

### Live load test

Drives real orders through the external pipeline (OMS API → RMS → FIX initiator → this engine → blotter). See [operations](operations.md#live-load-test) for prerequisites.

`/api/live/start` takes a JSON body:

| Field | Meaning |
|---|---|
| `broker` | An OMS subdomain from the broker list. `ALL`, `*` or empty means every broker, with orders split evenly between them. |
| `rate` | Target orders/sec; `0` means as fast as possible. |
| `total` | Orders to send; `0` means run until stopped. |
| `side` | `BUY` (default), `SELL`, or `BOTH` (about 30% sells). Sells only use `GP`, `SQURPHARMA`, `BEXIMCO`, `ROBI` and `BATBC`. |
| `sells` | Older flag: `true` means `BOTH` when `side` is empty. |
| `workers` | Concurrent HTTP senders; defaults to 96, maximum 4096. |

```bash
curl -s -X POST localhost:8010/api/live/start -d '{"broker":"ALL","rate":500,"total":5000,"side":"BOTH"}'
curl -s localhost:8010/api/live/stats
curl -s -X POST localhost:8010/api/live/stop
```

`/api/live/stats` returns:
- counters: `sent`, `accepted`, `rejected`, `filled`
- rates: `sent_per_sec`, `accepted_per_sec`
- latency in milliseconds: `lat_avg_ms`, `lat_p50_ms`, `lat_p95_ms`, `lat_p99_ms`
- `reject_reasons`, normalised
- a sampled `tape`
- `brokers`, the subdomains available to `broker`
- `targets`

`filled` is counted only when the test targets 8 brokers or fewer.

`/api/live/latency` returns `{"stages": [...], "target_ms": 10}`. The stages are API, RMS, Kafka, FIX Initiator, Matching Engine and DB Store, each with `count`, `avg_us`, `p50_us`, `p95_us` and `p99_us`. The values are null when the source service doesn't report that stage. The Matching Engine row is measured here: the time from receiving a FIX NewOrderSingle to sending its execution report.

> **Warning: `/api/live/cleanup` deletes data and needs no login.** It stops any running live test, then:
> 1. Finds every `terminal_*` and `tenant_*` database on the OMS Postgres.
> 2. In each one, runs `TRUNCATE orders_fixorder, orders_fixorderexec, orders_fixmessage, orders_fixorderstatus RESTART IDENTITY CASCADE`, and resets intraday cash and positions in `broker_house_clientcashlimit` and `broker_house_clientposition`.
> 3. Truncates `client_order_routes` in `bdtask_trade_master`.
> 4. Truncates `sim_orders` and `sim_execs` in `velexa_sim`.
> 5. Posts to `ME_AUDIT_URL/purge`.
>
> The code comment says only simulator (`LIVE-*`) rows are touched, but the statements truncate whole tables. Point it only at OMS databases that hold nothing but test data.

## Streams

### WebSocket market streams: `/market`, `/marketdepth`, `/index`

Served by `ItchGateway` in `itchgw.go` in the JSON shape the trader SPA expects. On connect, the client gets a full snapshot of its stream, then a full refresh every 500 ms. Messages from the client are read and ignored. The server accepts any `Origin`.

- `/market` sends one frame per instrument. The keys are `ob` (order book ID), `sn` (symbol), `fn` (name), `fln`, `ic` (ISIN), `b` (board label: `Main Board`, `Small Cap Board` or `Alternative Trading Board`), `g`, `sc`, `it`, `mt`, `d` (change), `s` (status: `nt` not traded, `u` up, `d` down, `uc` unchanged), `r` (previous close), `c`/`lp` (last), `o`, `h`, `l`, `v` (volume), `bd`/`ak` (bid/ask), `bq`/`aq`, `cg` (change %), `tds` (trades) and `ltt` (Unix time).
- `/marketdepth` sends one frame per instrument: `{"ob": 100001, "100001": {"buy": [...], "sell": [...]}}`. Each level is `{o: orders, p: price, q: qty, cq: cumulative qty}`, up to 10 per side.
- `/index` sends one frame: `{"sn":"DSEX", "civ": value, "ivd": change, "ip": change %, "v", "vl", "tds", "nt", "uc", "d", "u"}`.

```bash
websocat ws://localhost:8010/index
```

### `/events`

What `/events` does depends on the request:

- **WebSocket upgrade:** the time-and-sales and news stream for the trader SPA. On connect, the client gets a fixed list of simulated news items (`{"tp":"news","d":[…]}`) and each instrument's recent trades (`{"tp":"tsm","ob":…,"d":[…]}`). After that, one `tsm` frame arrives per new trade: `{"p","v","tst","s","tsm_id"}`.
- **Plain GET:** a Server-Sent Events stream used by the trade terminal page. A `market` event arrives every 700 ms with the whole market (`{"type":"market","index","phase","rows","ts"}`), and a `trade` event for each trade (`{"type":"trade","data":{…}}`). A `: keepalive` comment is sent every 15 s. Slow clients drop messages instead of blocking the engine.

```bash
curl -N localhost:8010/events
```

## Admin console API

### Logging in

1. `POST /admin/login` with form fields `user` and `pass`, which must match `ME_ADMIN_USER` and `ME_ADMIN_PASS`.
2. On success, the server sets the `me_admin` cookie (HttpOnly, `Path=/admin`, valid for 8 hours, SameSite=Lax, and `Secure` when `X-Forwarded-Proto` is `https`) and redirects to `/admin`. The cookie value is an HMAC-SHA256 of the username, keyed with `ME_ADMIN_SECRET`. Changing the secret or the username logs everyone out.
3. After 5 failed attempts from one client IP, logins from that IP are refused for 5 minutes. The IP is taken from `CF-Connecting-IP`, then `X-Forwarded-For`, then the connection address. Those headers can be set by any client, so the throttle only holds when a proxy in front sets them.

Without a valid cookie, `/admin/api/*` returns `401 {"error":"unauthorized"}` and `/admin` redirects to `/admin/login`.

```bash
curl -s -c jar -d 'user=admin' -d "pass=$ME_ADMIN_PASS" localhost:8010/admin/login -o /dev/null
curl -s -b jar localhost:8010/admin/api/status
```

### Endpoints

| Route | Input | Output and effect |
|---|---|---|
| `/admin/api/status` | none | `phase`, `instruments`, `orders`, `trades`, `running`, `bots_paused`, `fix_auth`, `itch_auth`, `uptime`; `listeners` (TCP checks of `127.0.0.1` ports 5001, 5443, 5501 and 8010; the port numbers are fixed in code); `sessions` (connected FIX brokers with logon time and last sequence numbers); `events` (last 50 logon, logout and reject events); `audit` (last 60 entries); `universe`. |
| `/admin/api/brokers` | none | `brokers` (records from `broker_meta.json`, newest first), `fix_total`, `itch_total`. |
| `/admin/api/broker/detail` | `?sender=` | Company and contact, **FIX and ITCH passwords**, the linked ITCH user, `endpoints` (onboarding values from the `ME_PUB_*`, `ME_INT_*` and `ME_ITCH_*` variables) and active flags. |
| `/admin/api/broker/add` | JSON `{Sender, Company, Contact, FixPass, ItchUser, ItchPass}`; only `Sender` is required | Stores FIX and ITCH credentials and the metadata, and writes the audit action `ISSUE`. Missing passwords are generated (`FIX-<12 hex>` and `ITCH-<12 hex>`). A missing ITCH user defaults to the first 6 characters of the SenderCompID. Returns the issued credentials. |
| `/admin/api/broker/del` | JSON `{Sender}` | Removes the FIX credential, the linked ITCH credential and the metadata, and writes the audit action `REVOKE`. Live sessions aren't disconnected. |
| `/admin/api/broker/sheet` | Form fields `company`, `sender`, `fix_pass`, `itch_user`, `itch_pass` | HTML onboarding sheet with connection details and credentials, with copy, download and print. |
| `/admin/api/allowlist` | none | `{"list": [{ip, label}], "count"}` |
| `/admin/api/allowlist/add` | JSON `{IP, Label}` | Validates the IP, writes `fix_allow.json`, and writes the audit action `ALLOW+`. Doesn't reload nginx (see [operations](operations.md#external-fix-gateway-port-5443)). |
| `/admin/api/allowlist/del` | JSON `{IP}` | Removes the IP and writes the audit action `ALLOW-`. |
| `/admin/api/sim` | `?action=bots_pause`, `bots_resume`, `preopen` or `open` | Same effects as the matching `/api/*` controls. Returns `phase` and `bots_paused`. |
| `/admin/api/session` | `?broker=` and `?action=disconnect` or `reset` | `disconnect` sends a FIX Logout (35=5) and unregisters the session. `reset` resets the session's sequence numbers. Writes an audit entry. |
| `/admin/api/fixlog` | `?broker=` (exact, case-insensitive), `?clordid=` (substring), `?limit=` (1 to 1000, default 250) | `{"messages": [{time, session, dir, msg_type, clordid, raw}]}`, newest first, from the 4000-message in-memory log. |
| `/admin/api/instruments` | none | `count`, `source`, `file`, `exchange_url`, `auto_list`, `auto_listed` (symbols opened on the fly), `symbols`, `rows` (per instrument: `symbol`, `name`, `board`, `sector`, `lot`, `isin`, `tick`, `prev`, `last`, `volume`, `status`), `summary` (`total`, `main_equity`, `bonds`, `pending`), `classes` (asset-class counts), `note`. |
| `/admin/api/instrument/add` | POST `{Symbol,Name,Board,ISIN,Prev,Tick}` | Lists a new scrip live (book, ITCH id, bots; shows in the Trade Terminal) and saves the whole universe to `ME_INSTRUMENT_FILE`. Errors: invalid symbol, already listed, bad board (MAIN/SME/ATB), bad ISIN, `Prev` <= 0. Returns `ok`, `symbol`, `count`, optional `warning`. |
| `/admin/api/instrument/update` | POST same body | Edits an existing scrip; blank/zero fields keep their current value. |
| `/admin/api/instrument/halt` | POST `{Symbol,Halted}` | Halts or resumes trading. New orders are rejected with `trading halted in <SYM>` and the bots stop quoting. Persisted as `status: "halted"`. |
| `/admin/api/instruments/export` | GET `?format=xml\|csv\|json` and optional `board`, `class` (EQ/MF/CB/DB), `status`, `q` | Downloads the instrument directory as an attachment. `xml` is a DSE end-of-day ticker file (`<EODTickers>` with one `<Ticker SecurityCode ISIN Category AssetClass Sector CompulsorySpot TradeDate Close Open High Low Var VarPercent />` per instrument, plus the DSEX index when unfiltered), downloaded as `YYYYMMDD-HHMMSS-eodtickers-ME.xml`; `json` is in the master-file shape, so it can be re-imported. Header `X-Export-Count` carries the row count. |
| `/admin/api/instruments/reload` | POST | Reloads the configured sources (file, then exchange URL, then built-in) into the running engine. Returns `added`, `updated`, `count` and `source`. |
| `/admin/api/instruments/import` | POST body: a security master in any format `parseMaster` accepts (up to 32 MiB) | Merges it into the running engine. If it has at least as many instruments as are currently listed, it is also saved to `ME_INSTRUMENT_FILE`. A smaller import is applied in memory only, with a `warning`. Returns `instruments`, `added`, `updated`, `count`, `persisted_to` and `warning`. |

```bash
curl -s -b jar -X POST --data-binary @instruments.json localhost:8010/admin/api/instruments/import
```

## Web pages

| Path | Source | What it is |
|---|---|---|
| `/` | `web.go` (`indexHTML`) | Trade terminal: market watch from the `/events` SSE stream, order book and tape polled from `/api/book`, an order form posting to `/api/order` (actor `MANUAL`), and counters from `/api/status`. |
| `/bench` | `benchweb.go` (`benchHTML`) | Benchmark page. Switches between the synthetic benchmark (`/api/bench/*`) and the live load test (`/api/live/*`, including cleanup), with a pipeline latency view from `/api/live/latency` and a bot toggle. |
| `/admin` | `admin_html.go` (`adminDashboardHTML`) | Admin console. Views: Dashboard, Trade Terminal (embeds `/`), FIX Sessions, Simulator (embeds `/bench`), Broker Credentials, External OMS Access (IP allowlist), Message Log and Diagnostics. Refreshes status every 3 s. |
