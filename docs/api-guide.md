# API guide

A simple guide to talking to the matching engine. Read it top to bottom once and you will know how to see prices, send orders, and receive the live market feed.

For every last detail, see the technical references: [HTTP API](http-api.md) and [FIX and ITCH protocols](protocols.md).

## 1. What this engine is

The engine is a stock exchange simulator modelled on the Dhaka Stock Exchange (DSE):

- **Brokers send orders** to buy and sell shares.
- **The engine matches them.** A buy meets a sell at an agreed price and a trade happens.
- **It publishes the market:** prices, the order book and every trade, as they happen.

You can talk to it in three ways:

| Way in | Port | Who uses it | What for |
|---|---|---|---|
| **HTTP** (web / JSON) | `8010` | Anyone, scripts, dashboards | See prices and order books, place simple orders, follow live updates |
| **FIX** | `5001` | A broker's order system (OMS) | Send, cancel and amend orders; receive fills |
| **ITCH** | `5501` market, `5502` index, `5503` news | Market-data systems | Receive the live order book, trades, index values and news |

In the examples below, the server is `localhost`. If the engine runs on another machine, use its address instead, for example `http://192.168.1.52:8010`.

## 2. Quick start (1 minute)

Is the engine running?

```bash
curl http://localhost:8010/healthz
```

```text
ok
```

What is the price of GP (Grameenphone)?

```bash
curl "http://localhost:8010/api/book?symbol=GP"
```

```json
{"symbol":"GP",
 "md":{"symbol":"GP","name":"Grameenphone Ltd.","last":287,"open":287,"high":287,"low":287,"prev":286.6,
       "change":0.4,"change_pct":0.14,"bid":286,"bid_qty":200,"ask":0,"ask_qty":0,"volume":40,"trades":1},
 "bids":[{"price":286,"qty":200,"orders":1}],
 "asks":[],
 "trades":[{"id":1,"symbol":"GP","price":287,"qty":40,"aggressor":"BUY","time":"2026-10-26T14:21:16.59+06:00"}]}
```

That's it: GP last traded at 287.00 Tk. Someone wants to buy 200 shares at 286.00, and nobody is selling right now.

## 3. Words you'll meet

| Word | Meaning |
|---|---|
| **Symbol** | The short name of a share, e.g. `GP`, `BATBC`, `SQURPHARMA`. |
| **Order book** | The list of waiting buy orders (**bids**) and sell orders (**asks**) for one symbol. |
| **Bid / Ask** | The best (highest) price someone will buy at / the best (lowest) price someone will sell at. |
| **LTP / last** | Last traded price. |
| **YCP / prev** | Yesterday's closing price. Today's change is measured against it. |
| **Lot** | The smallest quantity step. Order quantities must be whole lots. |
| **Tick** | The smallest price step, e.g. 0.10 Tk. A price of 286.53 is refused when the tick is 0.10. |
| **Price band (circuit limit)** | How far today's price may move from yesterday's close, e.g. ±8.75 %. Prices outside it are refused. |
| **Phase** | Which part of the trading day it is (see section 4). |
| **ClOrdID** | *Your* name for an order. You choose it, and you use it later to cancel or amend. |
| **OrderID** | The *engine's* number for an order, given when it is accepted. |
| **Fill** | A trade on your order. One order can have several fills. |
| **Time in force (TIF)** | How long an order stays: `DAY` (until the end of the day), `GTC` (until cancelled), `GTD` (until a date), `IOC` (trade what you can now, cancel the rest), `FOK` (trade everything now, or nothing). |

All times are **Dhaka time (UTC+6)**, except FIX timestamps, which are UTC as FIX requires.

## 4. The trading day

The market trades **Sunday to Thursday**. With the standard schedule a day looks like this:

| Time (Dhaka) | Phase | What you can do |
|---|---|---|
| before 09:30 | **Closed** | Nothing is accepted. |
| 09:30 – 10:00 | **Pre-Open** | Send **limit** orders (with a price). They are collected but don't trade yet. Market orders are refused. |
| 10:00 | *Opening auction* | The collected orders are matched at one opening price. |
| 10:00 – 14:30 | **Continuous Trading** | Normal trading: orders trade as soon as a matching order exists. |
| 14:30 – 14:40 | **Close** | Nothing is accepted. Closing prices are published. |
| 14:40 – 15:00 | **Post-Close** | Orders are accepted **only at the closing price**. |
| after 15:00 | **Closed** | DAY orders that are still waiting expire. |

The exchange operator can change the times, or move the phases by hand. Check the current phase at any time:

```bash
curl http://localhost:8010/api/session
```

```json
{"auto_clock":true,"phase":"Continuous Trading"}
```

`auto_clock: true` means the clock above drives the phases. `false` means the operator moves them by hand.

## 5. HTTP API

Plain web requests with JSON answers. Reading market data needs no account. Placing orders needs a signed-in account.

### 5.1 Market data (no sign-in)

| Request | What you get |
|---|---|
| `GET /api/market` | Every instrument's prices in one list |
| `GET /api/book?symbol=GP` | One symbol: prices, the best 10 bids and asks, the last 30 trades |
| `GET /api/status` | Engine summary: phase, number of instruments, orders and trades |
| `GET /api/session` | Current phase |
| `GET /health` | Is the engine up, and the phase |
| `GET /healthz` | Just `ok` when the engine is up (for monitoring) |

**`/api/market`** returns `{"index", "phase", "rows"}`, with one row per instrument:

```json
{"symbol":"GP","name":"Grameenphone Ltd.","last":286.6,"open":286.6,"high":286.6,"low":286.6,"prev":286.6,
 "change":0,"change_pct":0,"bid":0,"bid_qty":0,"ask":0,"ask_qty":0,"volume":0,"trades":0,
 "board":"Main (A)","asset_class":"EQ","sector":"Telecom","category":"A","status":"active"}
```

| Field | Meaning |
|---|---|
| `last`, `open`, `high`, `low` | Today's last, first, highest and lowest trade price (Tk) |
| `prev` | Yesterday's close (YCP) |
| `change`, `change_pct` | `last − prev` in Tk and in % |
| `bid`, `bid_qty` / `ask`, `ask_qty` | Best buy / sell price and the quantity there (`0` = none) |
| `volume`, `trades` | Shares traded today, and how many trades |
| `board`, `asset_class`, `sector`, `category` | Where it is listed. `asset_class`: `EQ` share, `MF` mutual fund, `CB` corporate bond, `DB` debenture |
| `status` | `active`, `halted` (trading stopped) or `pending` |

**`/api/book?symbol=GP`** returns:
- `md`: the same price row
- `bids` / `asks`: up to 10 price levels, best first, each `{price, qty, orders}`
- `trades`: the latest trades, newest first, each `{id, price, qty, aggressor, time}`; `aggressor` is the side that caused the trade

An unknown symbol gives `{"error":"unknown symbol"}`.

**`/api/status`**:

```json
{"phase":"Continuous Trading","running":true,"instruments":430,"orders":7498,"trades":420,"index":6000.3,
 "universe":{"count":430,"source":"built-in DSE reference list (430 instruments, …)"}}
```

`orders` and `trades` are the engine's running counts.

### 5.2 Signing in

Order entry needs an account. Create one on the welcome page (`http://localhost:8010/`, **Create account**), or ask the exchange admin.

The engine uses a browser-style session, so a script has to do three steps.

1. Get the safety cookie (`me_csrf`):
   ```bash
   curl -s -c jar -b jar http://localhost:8010/ -o /dev/null
   T=$(awk '$6=="me_csrf"{print $7}' jar)
   ```
2. Sign in with your username and password:
   ```bash
   curl -s -c jar -b jar -d user=alice -d 'pass=YOUR-PASSWORD' -d "csrf=$T" \
        http://localhost:8010/admin/login -o /dev/null
   ```
3. On every `POST`, send the cookies (`-b jar`) and the token in the `X-CSRF-Token` header (`-H "X-CSRF-Token: $T"`).

A session ends after 30 minutes without requests, or after 8 hours at most. Then sign in again.

### 5.3 Place an order: `POST /api/order`

```bash
curl -s -b jar -H "X-CSRF-Token: $T" -H 'Content-Type: application/json' \
     -X POST http://localhost:8010/api/order \
     -d '{"symbol":"GP","side":"BUY","type":"LIMIT","price":287,"qty":40,"cl_ord_id":"B-1"}'
```

| Field | Required | Meaning |
|---|---|---|
| `symbol` | yes | The share, e.g. `GP` |
| `side` | yes | `BUY` or `SELL` |
| `type` | no | `LIMIT` (default) or `MARKET`. A limit order trades at your price or better; a market order takes whatever price is available. |
| `price` | for LIMIT | Your price in Tk. `0` or no price makes it a market order. |
| `qty` | yes | Number of shares, more than 0 |
| `tif` | no | `DAY` (default), `IOC`, `FOK` or `GTC` (see section 3) |
| `cl_ord_id` | recommended | Your own ID for the order. You need it to cancel. |

The answer is the order's report. It comes straight back, after the engine has matched what it can:

```json
{"order_id":2,"cl_ord_id":"B-1","symbol":"GP","side":"BUY","status":"FILLED",
 "fills":[{"id":1,"symbol":"GP","price":287,"qty":40,"aggressor":"BUY","time":"2026-10-26T14:21:16.59+06:00"}],
 "filled_qty":40,"leaves_qty":0,"price":287,"avg_px":287,"last_px":287,"last_qty":40,"order_qty":40,
 "board":"Main (A)","isin":"BD0001GP0004"}
```

| `status` | Meaning |
|---|---|
| `NEW` | Accepted and waiting in the book. Nothing traded yet. |
| `PARTIALLY_FILLED` | Some shares traded; the rest is waiting. |
| `FILLED` | Everything traded. |
| `CANCELED` | Cancelled. An IOC/FOK order with nothing to trade against ends like this too. |
| `REJECTED` | Refused. `reason` says why. |

`filled_qty` is the number traded so far, `leaves_qty` the number still waiting, and `avg_px` the average trade price.

### 5.4 Cancel an order: `POST /api/order/cancel`

Send the `cl_ord_id` you gave the order. You can cancel only your own orders.

```bash
curl -s -b jar -H "X-CSRF-Token: $T" -H 'Content-Type: application/json' \
     -X POST http://localhost:8010/api/order/cancel -d '{"cl_ord_id":"S-1"}'
```

```json
{"ok":true,"report":{"order_id":1,"cl_ord_id":"S-1-X","orig_cl_ord_id":"S-1","status":"CANCELED","filled_qty":40,"leaves_qty":0, "…":"…"}}
```

If the order has already finished, the answer is `{"ok":false,"error":"the order is not working (already filled, cancelled or not yours)"}`.

### 5.5 Live updates

Instead of asking again and again, you can keep a connection open and receive updates as they happen.

| Address | Type | You receive |
|---|---|---|
| `/events` | Server-Sent Events (plain HTTP) | The whole market about every 0.7 s, plus each trade |
| `ws://…/market` | WebSocket | One row per instrument every 0.5 s |
| `ws://…/marketdepth` | WebSocket | The 10-level order book per instrument every 0.5 s |
| `ws://…/index` | WebSocket | The DSEX index and up/down counts every 0.5 s |

Try the event stream (press Ctrl+C to stop):

```bash
curl -N http://localhost:8010/events
```

```text
data: {"type":"market","phase":"Continuous Trading","index":6000,"rows":[{"symbol":"GP","last":287, …}, …],"ts":1793002884651}
data: {"type":"trade","data":{"id":2,"symbol":"GP","price":287,"qty":10, …}}
```

The WebSocket streams use short field names. The common ones on `/market`:

| Key | Meaning | Key | Meaning |
|---|---|---|---|
| `sn` | symbol | `lp` / `c` | last price |
| `fn` | full name | `r` | yesterday's close |
| `o`, `h`, `l` | open, high, low | `d`, `cg` | change, change % |
| `bd`, `bq` | best bid, its quantity | `ak`, `aq` | best ask, its quantity |
| `v` | volume | `tds` | trades |
| `ob` | order book number | `s` | `u` up, `d` down, `uc` unchanged, `nt` not traded |

### 5.6 When something goes wrong

| You get | Why | What to do |
|---|---|---|
| `403` "CSRF check failed" | The `X-CSRF-Token` header or the `me_csrf` cookie is missing or wrong | Do step 1 of 5.2 again and send both |
| `401` "unauthorized: sign in first" | Not signed in, or the session expired | Sign in again (5.2) |
| `405` "POST only" | You used GET on an action | Use `POST` |
| `400` "invalid quantity …" | `qty` is 0 or less | Send a positive quantity |
| `400` "unknown symbol …" | The symbol isn't listed | Check the symbol in `/api/market` |
| `"status":"REJECTED"` with a `reason` | The engine refused the order (phase, price band, tick …) | Read `reason`; see the reject table in 6.6 |

## 6. FIX (for broker order systems)

FIX is the standard language order systems use to talk to exchanges. Your OMS connects, logs on, sends orders, and receives a report for every change to each order.

### 6.1 Connection settings

| Setting | Value |
|---|---|
| Host / port | The engine's address, port `5001` (or the TLS gateway the exchange gives you) |
| FIX version | `FIXT.1.1` with FIX 5.0 SP2 (`1137=9` on Logon) |
| Engine's CompID (your TargetCompID, tag 56) | `DSEME` |
| Your CompID (your SenderCompID, tag 49) | Given by the exchange admin, e.g. `BRK01` |
| Username / password | Tag 553 = your CompID, tag 554 = your FIX password (from the admin) |
| Sequence numbers | Start from 1 at every logon (send `141=Y`) |
| Heartbeat | Your choice in tag 108, e.g. `30` seconds |

The exchange admin creates your broker in the console. You get a sheet with these values and your password. Once a broker exists, logons with a wrong CompID or password are refused.

A Logon looks like this (`|` stands for the invisible separator character, SOH):

```text
8=FIXT.1.1|9=…|35=A|49=BRK01|56=DSEME|34=1|52=20261026-08:21:49.000|98=0|108=30|141=Y|553=BRK01|554=YOUR-PASSWORD|1137=9|10=…
```

The engine answers with its own Logon (35=A). If the logon is refused, you get a Logout (35=5) with the reason in tag 58.

### 6.2 Send an order (35=D, NewOrderSingle)

| Tag | Name | Value |
|---|---|---|
| 11 | ClOrdID | Your unique ID for this order |
| 55 | Symbol | e.g. `GP` |
| 54 | Side | `1` buy, `2` sell |
| 38 | OrderQty | Number of shares (whole lots) |
| 40 | OrdType | `2` limit, `1` market, `3` stop, `4` stop-limit, `Z` market-at-best |
| 44 | Price | For limit orders, e.g. `286.50` |
| 59 | TimeInForce | `0` DAY (default), `1` GTC, `3` IOC, `4` FOK, `6` GTD |
| 60 | TransactTime | Now, in UTC |

Optional:
- **Stop orders:** `99` (StopPx), the price that triggers a stop or stop-limit order.
- **Iceberg:** `1138` (DisplayQty), so only this much shows on the book at a time.
- **GTD expiry:** `126` (ExpireTime, UTC) or `432` (ExpireDate).
- **Echoed back unchanged:** `1` (Account, e.g. the client's BO account), `50` (trader), `453` (parties), `762` (board, e.g. `PUBLIC`).

Example, buy 50 GP at 286.50:

```text
8=FIXT.1.1|9=…|35=D|49=BRK01|56=DSEME|34=2|52=…|11=ORD-1|1=BO1001|55=GP|54=1|60=20261026-08:21:49|38=50|40=2|59=0|762=PUBLIC|44=286.50|10=…
```

### 6.3 Cancel, amend, ask for status

| You want to | Send | Key tags | You get back |
|---|---|---|---|
| **Cancel** | `35=F` | `41` = the order's ClOrdID, `11` = a new ClOrdID, `55`, `54` | A report with `150=4` (cancelled) |
| **Amend** (new price or quantity) | `35=G` | `41` = current ClOrdID, `11` = new ClOrdID, `38` = new **total** quantity, `44` = new price | A report with `150=5` (replaced) |
| **Ask for status** | `35=H` | `11` = the ClOrdID, `790` = your request ID | A report with `150=I` and the order's current state |

Notes:
- After an amend, the order uses the new ClOrdID. Use it for the next cancel or amend.
- An amended order goes to the back of the queue at its price, as on the DSE.
- If the order is unknown, already finished or not yours, you get an **OrderCancelReject** (`35=9`) with `58=(226): Invalid order number`. If the amend itself is refused (bad price, market closed …), the original order stays as it was.

### 6.4 What you get back (35=8, ExecutionReport)

Every change to an order sends you one report. Tag **150** says what happened:

| 150 | Meaning |
|---|---|
| `0` | **New**: your order is accepted |
| `F` | **Trade**: a fill. One report per trade. |
| `4` | **Cancelled** |
| `5` | **Replaced**: your amend is done |
| `8` | **Rejected**: see tag 58 for why |
| `C` | **Expired**: a DAY order at the end of the day, or a GTD order at its time |
| `A` | **Pending**: a stop order is waiting for its trigger price |
| `I` | **Status**: the answer to your 35=H |

A real example: you buy 50 GP at 286.50, then a seller sells 30 at 286.00. You receive two reports:

```text
35=8|11=ORD-1|37=202610265|150=0|39=0|55=GP|54=1|38=50|44=286.50|14=0|151=50|6=0.00|…
35=8|11=ORD-1|37=202610265|150=F|39=1|55=GP|54=1|38=50|31=286.50|32=30|14=30|151=20|6=286.50|880=2|75=20261026|64=20261028|381=8595.00|1057=N|…
```

The second report says: 30 shares traded at 286.50, 20 are still waiting.

| Tag | Meaning |
|---|---|
| 37 | OrderID: the engine's order number (trade date + number). The same on every report for the order. `NONE` on rejects. |
| 11 / 41 | ClOrdID / the original ClOrdID (on cancel, amend and expiry) |
| 39 | Order status now: `0` new, `1` partly filled, `2` filled, `4` cancelled, `8` rejected, `C` expired |
| 14 / 151 | Quantity filled so far / quantity still waiting |
| 6 | Average price of all fills so far |
| 31 / 32 | This fill's price / quantity (on `150=F`) |
| 880 | Trade number. The buyer and the seller get the same one. |
| 75 / 64 | Trade date / settlement date (T+2) |
| 381 | Trade value (quantity × price) |
| 1057 | `Y` if your order caused the trade, `N` if it was waiting in the book |
| 17 | ExecID, unique for every report |
| 58 | Text, e.g. the reject reason |

### 6.5 Good to know

- **Only your session can touch your orders.** Other brokers can't cancel or see them.
- **Missed reports aren't lost.** If one of your waiting orders trades while you are disconnected, the reports are kept and sent to you when you log on again, even across an engine restart.
- **A ClOrdID must be unique** among your waiting orders. A repeat is rejected (`Duplicate ClOrdID`).
- **Market orders aren't accepted in Pre-Open.** Send limit orders then.

### 6.6 Common reject reasons (tag 58)

| Tag 58 says | Meaning | Fix |
|---|---|---|
| `price band: 400.00 outside +/-8.75% limit [261.52-311.68] of prev 286.60` | The price is outside today's allowed range | Send a price within the range shown |
| `off-tick: 286.5300 is not a multiple of tick 0.10` | The price isn't a whole number of ticks | Round to the tick, e.g. 286.50 |
| `(792): Minimum quantity step: N` | The quantity isn't a whole number of lots | Use a multiple of N |
| `(293): Orders are not being accepted at this time` | The market is in Close or Closed | Wait for Pre-Open or Continuous Trading |
| `(1116): Order price is not equal to close price` | In Post-Close, only the closing price is allowed | Use the closing price |
| `market orders are not accepted during the Pre-Open call auction …` | A market order was sent in Pre-Open | Send a limit order |
| `no liquidity: …` | A market order found nobody on the other side | Try again later, or use a limit order |
| `unknown symbol …` / `invalid symbol …` | The symbol isn't listed | Check the symbol |
| `Duplicate ClOrdID: …` | That ClOrdID is already used by one of your waiting orders | Use a new ClOrdID |
| `(226): Invalid order number` | A cancel, amend or status for an order that is unknown, finished or not yours | Check tag 41 / 11 |

## 7. ITCH (for market-data systems)

ITCH is a fast binary feed of everything that happens in the market. It lets your system keep its own exact copy of every order book. It runs over **SoupBinTCP**, a simple framing on top of TCP.

### 7.1 The three feeds

| Feed | Port | Carries |
|---|---|---|
| Market | `5501` | The instrument list, every order added to or removed from the book, every trade, phase changes, closing prices |
| Index | `5502` | DSEX and DS30 index values, once a second |
| News | `5503` | Announcements: phase changes, trading halted or resumed, new listings |

### 7.2 Connecting

Every packet, in both directions, is:

```text
[2 bytes: length of the rest, big-endian] [1 byte: packet type] [the rest]
```

1. **Open a TCP connection and send a Login** (`L`), with fixed-width, space-padded fields: username (6), password (10), session (10, blank = start fresh), sequence number (20, right-aligned; `1` = from the start). Ask the admin for credentials if ITCH login checks are turned on; otherwise any login is accepted.
2. **The engine answers `A` (Login Accepted)** with the session name and the next sequence number, or `J` (Login Rejected).
3. **Data arrives in `S` packets** (sequenced data). Each packet holds one or more ITCH messages back to back and counts as one sequence number.
4. **The engine sends `H` (heartbeat) every 2 seconds**, so silence means the connection is broken.
5. **Reconnecting:** log in with the same session and the next sequence number you expect, and the feed continues where you left off. A blank session starts fresh.

### 7.3 What you receive

**First, a start-of-day picture**, so you can build the market from nothing:
1. Settings: `T` time, `S` start of messages, `L`/`M` tick tables
2. For each instrument: `P` company, `R` order book directory (symbol, ISIN, lot, sector …), `H` trading state
3. For each instrument, its previous close: an `A` message with order number 0 and quantity 0
4. Every order waiting in the book: `A` messages
5. `S` with the current phase

**Then, live as it happens:**

| In the market | ITCH message |
|---|---|
| An order joins the book | `A` add order |
| A waiting order trades | `E` executed (`C` in the opening auction) |
| An order is cancelled, amended or expires | `D` delete (an amended order comes back as a new `A`) |
| Trading halted or resumed | `H` trading state |
| Phase changes | `S` system event |
| Market closes | `Q` closing price for each traded instrument |
| Nothing happened for a second | `T` time |

### 7.4 Message cheat-sheet

Every message starts with its type letter. All messages except `T` then carry a 4-byte timestamp (nanoseconds within the last `T` second).

| Type | Name | Main fields after the timestamp |
|---|---|---|
| `T` | Seconds | seconds since Dhaka midnight (4) |
| `S` | System event | group (8), event code (1): `O` start, `S` pre-open, `Q` continuous, `M` close, `V` post-close, `E` end, `C` end of messages; order book (4) |
| `P` | Company | company ID (4), name (30), symbol (30) |
| `R` | Order book directory | order book ID, ISIN, symbol, lot size, board, sector, name … |
| `H` | Trading state | order book (4), state `T` tradable / `V` suspended (1), reason (1) |
| `A` | Add order | order number (8), side `B`/`S` (1), quantity (8), order book (4), price (4), yield (4) |
| `E` | Order executed | order number (8), quantity (8), match number = trade number (8) |
| `C` | Executed with price | order number (8), quantity (8), match number (8), printable (1), price (4), order book (4) |
| `D` | Order delete | order number (8) |
| `Q` | Closing price | quantity (8), order book (4), printable (1), price (4), yield (4), match number (8) |
| `Z` | Index value (index feed) | index order book (4), value × 100 (8) |
| `N` | News (news feed) | order book (4), news ID (4), firm (30), then title, reference and text, each ending in a zero byte |

### 7.5 Reading the numbers

- **Numbers are big-endian**, with the biggest byte first.
- **Prices are whole numbers equal to price × 100:** `28650` means 286.50 Tk.
- **Text fields are fixed width**, padded with spaces.
- **Order numbers in `A`, `E` and `D` are the engine's own order numbers.** Your copy of the book matches the engine's exactly.
- **Order book IDs never change for a symbol.** DSEX is `90001` and DS30 is `90002`.

A real `A` message, a buy of 50 GP at 286.50:

```text
41 | 00 12 7a b3 | 00 00 00 00 00 00 00 05 | 42 | 00 00 00 00 00 00 00 32 | 00 01 87 48 | 00 00 6f ea | 00 00 00 00
A    timestamp     order number = 5          B    quantity = 50             book 100168   price 28650   yield 0
                                                                                          = 286.50
```

## 8. Questions and problems

**I get `401` or `403` from the HTTP API.**
`403` means the CSRF token is missing: send the `me_csrf` cookie and the `X-CSRF-Token` header. `401` means you aren't signed in, or the session expired. See 5.2.

**My FIX logon is refused.**
Check tag 58 of the Logout:
- `broker not registered` means the CompID (tag 49) is wrong, or the admin hasn't created your broker.
- `invalid credentials` means the password (554) is wrong.
- `username does not match SenderCompID` means tag 553 must equal tag 49.

**My order is rejected with "price band" or "off-tick".**
The message tells you the allowed range or the tick size. See 6.6.

**My orders are refused with `(293)`.**
The market is in Close or Closed. Check `GET /api/session` and the times in section 4.

**The ITCH feed sends nothing.**
- Check the port (5501 market, 5502 index, 5503 news).
- You must send the Login packet first.
- You should get `A` back, then data and a heartbeat every 2 s.

**Which time zone?**
Dhaka time (UTC+6) everywhere, except FIX tags 52, 60 and 126, which are UTC.

**Where are the full details?**
- [HTTP API reference](http-api.md): every route, including the admin console API
- [FIX and ITCH protocols](protocols.md): every tag and message layout
- [Architecture](architecture.md): how matching, auctions and sessions work
