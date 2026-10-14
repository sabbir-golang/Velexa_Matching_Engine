# HTTP API

This page documents everything served on the HTTP port (`PORT`, default `8010`): the web pages, the JSON API, the CSV exports, the WebSocket market-data streams, the `/events` stream and the admin console API. All routes are registered on one `http.ServeMux` in `Cmd/serve.go`, `rest/` and `engine/instruments.go`.

Unless a route says otherwise:

- Responses are JSON, written by `utils.WriteJSON` in `utils/respond.go`.
- Reads use `GET`. Every route that changes state answers only `POST` (anything else gets `405`).
- Every `POST` needs the browser's CSRF token: the `me_csrf` cookie (handed out on any `GET`) sent back in the `X-CSRF-Token` header, or in a `csrf` form field for HTML forms. A request without it, or one a browser marks as coming from another site (`Sec-Fetch-Site`, `Origin`), gets `403` (a form post is sent back to `/?e=csrf`). The bundled pages do this automatically.
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
| any | `/api/status` | none | Engine counters: phase, index, instrument/order/trade counts, universe source. |
| any | `/api/market` | none | Full market watch: index, phase and one row per instrument. |
| any | `/api/book` | none | Order book (10 levels per side), market data and last 30 trades for `?symbol=`. |
| POST | `/api/order` | any role | Submits an order in the signed-in user's name and returns its execution report. |
| GET | `/api/session` | none | Current trading phase. |
| POST | `/api/session/preopen` | admin role | Switches to the Pre-Open call auction. |
| POST | `/api/session/open` | admin role | Switches to Continuous Trading, running the opening auction first. |
| POST | `/register` | none | Self-registration with profile details: creates a `user` account, then returns to the sign-in form. A GET redirects to `/`. |
| POST | `/admin/login` | none | Sign-in form submit (posted from the welcome page). A GET redirects to `/`. |
| POST | `/admin/logout` | none | Ends every session of the signed-in account and clears the cookie. A GET only redirects to `/`. |
| any | `/admin` | admin login | Admin console web page. |
| any | `/admin/api/status` | admin login | Engine state, FIX sessions, listener checks, recent events and audit log. |
| any | `/admin/api/brokers` | any role (a user: own brokers only) | Broker records and credential counts. |
| any | `/admin/api/broker/detail` | any role (a user: own brokers only) | One broker's credentials and connection details (`?sender=`). |
| POST | `/admin/api/broker/add` | any role (a user: own brokers only) | Issues FIX and ITCH credentials for a broker. |
| POST | `/admin/api/broker/del` | any role (a user: own brokers only) | Revokes a broker's FIX and ITCH credentials and metadata. |
| POST | `/admin/api/broker/sheet` | any role (a user: own brokers only) | Renders a printable broker onboarding sheet (HTML). |
| any | `/admin/api/allowlist` | admin login | Lists the external FIX gateway IP allowlist. |
| POST | `/admin/api/allowlist/add` | admin login | Adds an IP to the allowlist. |
| POST | `/admin/api/allowlist/del` | admin login | Removes an IP from the allowlist. |
| GET, POST | `/admin/api/trading-session` | admin role | Market session phase and the DSE clock; changing it (`?action=`) needs POST. |
| POST | `/admin/api/session` | any role (a user: own brokers only) | Disconnects a FIX session or resets its sequence numbers. |
| any | `/admin/api/fixlog` | any role (a user: own brokers only) | Searches the FIX message log. |
| any | `/admin/api/instruments` | admin login | Lists the instrument universe and auto-listed symbols. |
| POST | `/admin/api/instruments/reload` | admin login | Reloads the security master from the configured sources. |
| POST | `/admin/api/instruments/import` | admin role | Imports a security master from the request body. |
| GET | `/admin/api/me` | any role | Current `user`, `role`, `can_write`. |
| POST | `/admin/api/me/password` | any role | Body `{Old, New}`. |
| GET | `/admin/api/users` | admin role | Console accounts (no hashes). |
| POST | `/admin/api/user/add`, `/user/update`, `/user/password`, `/user/delete` | admin role | Manage accounts (`Username`, `Password`, `Role`, `Disabled`). |
<!-- /AUTO:routes -->

Market data and status under `/api/*` need no sign-in; order entry needs a signed-in console user and the session controls need an admin. See [Security notes](configuration.md#security-notes).

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

`orders` and `trades` are the engine's running sequence numbers. `universe.source` says where the instrument list came from (see [Instrument universe](architecture.md#instrument-universe)).

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

## Session controls

| Route | Response | Effect |
|---|---|---|
| `/api/session` | `{"phase": "…", "auto_clock": bool}` | Shows the current phase and whether the DSE clock drives it. |
| `POST /api/session/preopen` (admin) | `{"phase": "Pre-Open"}` | Starts collecting limit orders for the call auction. Market orders are rejected. Switches the clock to manual. |
| `POST /api/session/open` (admin) | `{"phase": "Continuous Trading"}` | Runs the opening auction for each instrument, then triggers any stop orders that have been hit. Switches the clock to manual. |

```bash
T=$(awk '$6=="me_csrf"{print $7}' jar)   # cookie jar of a signed-in admin, see Logging in
curl -s -b jar -H "X-CSRF-Token: $T" -X POST localhost:8010/api/session/preopen
curl -s -b jar -H "X-CSRF-Token: $T" -X POST localhost:8010/api/session/open
```

The phase names and the auction itself are described in [architecture](architecture.md#trading-sessions).

## Streams

### WebSocket market streams: `/market`, `/marketdepth`, `/index`

Served by `ItchGateway` in `itchgw/gateway.go` in the JSON shape the trader SPA expects. On connect, the client gets a full snapshot of its stream, then a full refresh every 500 ms. Messages from the client are read and ignored. The server accepts any `Origin`.

- `/market` sends one frame per instrument. The keys are `ob` (order book ID), `sn` (symbol), `fn` (name), `fln`, `ic` (ISIN), `b` (board label: `Main Board`, `Small Cap Board` or `Alternative Trading Board`), `g`, `sc`, `it`, `mt`, `d` (change), `s` (status: `nt` not traded, `u` up, `d` down, `uc` unchanged), `r` (previous close), `c`/`lp` (last), `o`, `h`, `l`, `v` (volume), `bd`/`ak` (bid/ask), `bq`/`aq`, `cg` (change %), `tds` (trades) and `ltt` (Unix time).
- `/marketdepth` sends one frame per instrument: `{"ob": 100001, "100001": {"buy": [...], "sell": [...]}}`. Each level is `{o: orders, p: price, q: qty, cq: cumulative qty}`, up to 10 per side.
- `/index` sends one frame: `{"sn":"DSEX", "civ": value, "ivd": change, "ip": change %, "v", "vl", "tds", "nt", "uc", "d", "u"}`.

```bash
websocat ws://localhost:8010/index
```

### `/events`

What `/events` does depends on the request:

- **WebSocket upgrade:** the time-and-sales and news stream for the trader SPA. On connect, the client gets the day's exchange announcements (session changes, suspensions, listings — the same as the ITCH news feed) (`{"tp":"news","d":[…]}`) and each instrument's recent trades (`{"tp":"tsm","ob":…,"d":[…]}`). After that, one `tsm` frame arrives per new trade: `{"p","v","tst","s","tsm_id"}`.
- **Plain GET:** a Server-Sent Events stream used by the trade terminal page. A `market` event arrives every 700 ms with the whole market (`{"type":"market","index","phase","rows","ts"}`), and a `trade` event for each trade (`{"type":"trade","data":{…}}`). A `: keepalive` comment is sent every 15 s. Slow clients drop messages instead of blocking the engine.

```bash
curl -N localhost:8010/events
```

## Admin console API

### Logging in

1. `GET /` once to receive the `me_csrf` cookie, then `POST /admin/login` with form fields `user`, `pass` and `csrf` (the cookie's value). They must match an enabled console account. With `ME_RATE_LIMIT=on`, 5 wrong passwords lock that client IP, and separately that username, for 15 minutes; the client IP comes from a forwarding header only when the connection is from `ME_TRUSTED_PROXIES`.
2. On success, the server sets the session cookie and redirects to `/admin` (admins) or `/user` (other roles). The cookie is `me_admin` on plain HTTP and `__Host-me_admin` over HTTPS (`Secure`); it is `HttpOnly`, `SameSite=Strict`, `Path=/`. Its value is signed (HMAC-SHA256 with `session.key` or `ME_ADMIN_SECRET`) and carries the username, the account's session token, the sign-in time and the last activity. A session lasts at most 8 hours and ends after 30 minutes without a request; an active console gets a refreshed cookie about once a minute. The role is looked up on every request.
3. Signing out (`POST /admin/logout`), a password change or reset, disabling or deleting the account changes its session token, which ends all of its sessions at once.

Without a valid cookie, `/admin/api/*` and `/api/order` return `401 {"ok":false,"error":"unauthorized: sign in first"}` and `/admin` redirects to `/`, the welcome page.

```bash
curl -s -c jar -b jar localhost:8010/ -o /dev/null                       # CSRF cookie
T=$(awk '$6=="me_csrf"{print $7}' jar)
curl -s -c jar -b jar -d 'user=admin' -d "pass=$ME_ADMIN_PASS" -d "csrf=$T" localhost:8010/admin/login -o /dev/null
curl -s -b jar localhost:8010/admin/api/status
```

### Endpoints

| Route | Input | Output and effect |
|---|---|---|
| `/admin/api/status` | none | `phase`, `instruments`, `orders`, `trades`, `running`, `auto_clock`, `schedule`, `fix_auth`, `itch_auth`, `uptime`; `listeners` (TCP checks of `127.0.0.1` ports 5001, 5443, 5501 and 8010; the port numbers are fixed in code); `sessions` (connected FIX brokers with logon time and last sequence numbers); `events` (last 50 logon, logout and reject events); `audit` (last 60 entries); `universe`. |
| `/admin/api/brokers` | none | `brokers` (records from `broker_meta.json`, newest first, each with its `owner`), `fix_total`, `itch_total`, `scope` (`all` for an admin, `own` for a user, who gets only their brokers). |
| `/admin/api/broker/detail` | `?sender=` | Company and contact, **FIX and ITCH passwords**, the linked ITCH user, `endpoints` (onboarding values from the `ME_PUB_*`, `ME_INT_*` and `ME_ITCH_*` variables) and active flags. |
| `/admin/api/broker/add` | JSON `{Sender, Company, Contact, FixPass, ItchUser, ItchPass}`; only `Sender` is required | Refuses a SenderCompID or ITCH username that is already registered. The broker belongs to the user who adds it (an admin's belong to the exchange). Stores FIX and ITCH credentials and the metadata, and writes the audit action `ISSUE`. Missing passwords are generated (`FIX-<12 hex>` and `ITCH-<12 hex>`). A missing ITCH user defaults to the first 6 characters of the SenderCompID. Returns the issued credentials. |
| `/admin/api/broker/del` | JSON `{Sender}` | Removes the FIX credential, the linked ITCH credential and the metadata, and writes the audit action `REVOKE`. Live sessions aren't disconnected. |
| `/admin/api/broker/sheet` | Form fields `company`, `sender`, `fix_pass`, `itch_user`, `itch_pass` | HTML onboarding sheet with connection details and credentials, with copy, download and print. |
| `/admin/api/allowlist` | none | `{"list": [{ip, label}], "count"}` |
| `/admin/api/allowlist/add` | JSON `{IP, Label}` | Validates the IP, writes `fix_allow.json`, and writes the audit action `ALLOW+`. Doesn't reload nginx (see [operations](operations.md#external-fix-gateway-port-5443)). |
| `/admin/api/allowlist/del` | JSON `{IP}` | Removes the IP and writes the audit action `ALLOW-`. |
| `/admin/api/trading-session` | POST `?action=preopen`, `open`, `close`, `postclose`, `endofday` (force a phase; switches the clock to manual), `auto_on` or `auto_off` | Returns `phase`, `auto_clock` and `schedule`. |
| `/admin/api/session` | POST `?broker=` and `?action=disconnect` or `reset` | `disconnect` sends a FIX Logout (35=5) and unregisters the session. `reset` resets the session's sequence numbers. Writes an audit entry. |
| `/admin/api/fixlog` | `?broker=` (exact, case-insensitive), `?clordid=` (substring), `?limit=` (1 to 1000, default 250) | `{"messages": [{time, session, dir, msg_type, clordid, raw}]}`, newest first, from the 4000-message in-memory log. |
| `/admin/api/instruments` | none | `count`, `source`, `file`, `exchange_url`, `auto_list`, `auto_listed` (symbols opened on the fly), `symbols`, `rows` (per instrument: `symbol`, `name`, `asset_class`, `board`, `sector`, `lot`, `isin`, `tick`, `prev`, `last`, `volume`, `status`), `summary` (`total`, `main_equity`, `bonds`, `pending`), `classes` (asset-class counts), `note`. |
| `/admin/api/instrument/add` | POST `{Symbol,Name,AssetClass,Board,Sector,Lot,Tick,Prev,ISIN,Status}` | Lists a new scrip live (book, ITCH id, market-feed directory; shows in the Trade Terminal) and saves the whole universe to `ME_INSTRUMENT_FILE`. `Board` is one of Main (A)/(B)/(N)/(Z), Bond, Mutual Fund, SME / ATB (legacy MAIN/SME/ATB still accepted). `AssetClass` is `EQ`, `MF`, `CB` or `DB` (derived from the board when omitted). `Status` is `active` (default), `halted` or `pending`. Listing a delisted symbol re-lists it. Errors: invalid symbol, already listed, bad board, bad ISIN, `Prev` <= 0. Returns `ok`, `symbol`, `count`, optional `warning`. |
| `/admin/api/instrument/update` | POST same body | Edits an existing scrip; blank/zero fields keep their current value. |
| `/admin/api/instrument/halt` | POST `{Symbol,Halted}` | `Halted:true` halts; `Halted:false` activates (also lifts a `pending` listing). New orders on a halted, pending or delisted scrip are rejected, and the market feed sends its trading state (`H`). |
| `/admin/api/instrument/delist` | POST `{Symbol}` | Delists: orders are rejected, it leaves the directory and the saved master (gone after a restart). |
| `/admin/api/me/password` | POST `{Old, New}` | Changes your own password. Throttled like sign-in. Ends the account's other sessions; this browser gets a fresh cookie. |
| `/admin/api/instruments/export` | GET `?format=xml\|csv\|json` and optional `board`, `class` (EQ/MF/CB/DB), `status`, `q` | Downloads the instrument directory as an attachment. `xml` is a DSE end-of-day ticker file (`<EODTickers>` with one `<Ticker SecurityCode ISIN Category AssetClass Sector CompulsorySpot TradeDate Close Open High Low Var VarPercent />` per instrument, plus the DSEX index when unfiltered), downloaded as `YYYYMMDD-HHMMSS-eodtickers-ME.xml`; `json` is in the master-file shape, so it can be re-imported. Header `X-Export-Count` carries the row count. |
| `/admin/api/instruments/reload` | POST | Reloads the configured sources (file, then exchange URL, then built-in) into the running engine. Returns `added`, `updated`, `count` and `source`. |
| `/admin/api/instruments/import` | POST body: a security master in any format `parseMaster` accepts (up to 32 MiB) | Merges it into the running engine. If it has at least as many instruments as are currently listed, it is also saved to `ME_INSTRUMENT_FILE`. A smaller import is applied in memory only, with a `warning`. Returns `instruments`, `added`, `updated`, `count`, `persisted_to` and `warning`. |

```bash
curl -s -b jar -H "X-CSRF-Token: $T" -X POST --data-binary @instruments.json localhost:8010/admin/api/instruments/import
```

## Web pages

| Path | Source | What it is |
|---|---|---|
| `/terminal` | `web/terminal.go` (`IndexHTML`) | Trade terminal: market watch from the `/events` SSE stream, order book and tape polled from `/api/book`, an order form posting to `/api/order` (signed-in users only; the order's actor is `USER:<username>`), and counters from `/api/status`. |

## Console roles

Console routes need a signed-in account.

- **admin** sees and manages everything: every broker (with its owner), all FIX sessions and messages, the audit log, instruments, the market session, the IP allowlist and the users.
- **user** has a private workspace. A broker a user registers belongs to them (`owner` in `broker_meta.json`); they see and manage **only their own brokers**: the broker list, credentials and onboarding sheet (`broker/detail`, `broker/sheet`), revoking (`broker/del`), FIX session control (`/admin/api/session`), the FIX message log (`fixlog`, filtered to their sessions) and the dashboard (`status`: their sessions, logon events, order/trade counts and their own audit entries, with `"scope":"own"`). Asking for another user's or the exchange's broker returns `403 {"ok":false,"error":"not found or not yours"}`. Admin-only routes return `403 … admin role required`.
- Brokers registered by an admin, and brokers of a deleted account, belong to the exchange (no owner): only admins see them. A SenderCompID or ITCH username that is already registered is refused, so one user can never take over another's broker.
- Market data (Trade Terminal, `/api/market`, ITCH) is shared, as on a real exchange.

Audit entries record `username (ip)`. Sign-ins are audited too: `login.ok`, `login.fail` (with the name tried, quoted), `login.locked` and `logout`.

`GET /` is the welcome page with **User login** and **Admin login** (both post to `/admin/login` with an `as` field; each option only accepts accounts of its own role). The Trade Terminal is at `/terminal` and, embedded, `/?embed=1`.

`POST /register` (form `full_name`, `user`, `email`, `phone`, `gender` = male|female|other, `country`, `pass`, `pass2`; all required; email must be unique) creates a read-only `user` account and redirects to the sign-in form (no automatic sign-in); it never creates admins. At most 5 sign-ups per client IP per hour; `ME_REGISTRATION=off` closes it.

Console address by role: admins use `/admin`, other accounts use `/user` (same console, fewer pages). Signing in redirects to the right one; opening the other redirects across; not signed in redirects to `/`. The API stays under `/admin/api/*` and the `me_admin` session cookie is now `Path=/`.
