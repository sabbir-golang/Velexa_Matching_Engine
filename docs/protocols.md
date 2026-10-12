# FIX and ITCH protocols

This page is for anyone connecting to the engine's two binary interfaces: the FIX acceptor that brokers' order-management systems send orders to, and the SoupBinTCP ITCH feed that publishes market data. It covers session settings, authentication, the messages and tags the engine understands, what it sends back, and the message log.

## FIX acceptor

The acceptor is built on [quickfixgo](https://github.com/quickfixgo/quickfix) in `fixacceptor.go` (`StartFixAcceptor`). It stands in for the exchange's order gateway: brokers' FIX initiators connect, send orders, and get execution reports produced by the matching engine.

### Session settings

| Setting | Value | Source |
|---|---|---|
| Port | `5001` | `FIX_PORT` |
| BeginString | `FIXT.1.1` | `ME_FIX_BEGIN` |
| DefaultApplVerID | `9` (FIX 5.0 SP2); set only when the BeginString is `FIXT.1.1` | `ME_FIX_APPLVER` |
| SenderCompID (engine side) | `DSEME` | `FIX_SENDER` |
| TargetCompID (broker side) | Any. `DynamicSessions=Y` accepts any CompID; the `DSE_BROKER` in the template is only a placeholder | fixed |
| Session schedule | Always open (`StartTime` = `EndTime` = `00:00:00`) | fixed |
| Sequence numbers | Reset to 1 on every Logon, Logout and disconnect (`ResetOnLogon`, `ResetOnLogout`, `ResetOnDisconnect`) | fixed |
| Message store | In memory only. Nothing survives a restart, so earlier messages can't be resent | fixed |
| Validation | Inbound messages are checked against `FIXT11.xml` and `FIX50SP2.xml` from `ME_FIX_SPEC_DIR`. Field order and user-defined fields aren't checked, so broker-specific tags pass | `ME_FIX_DD`, `ME_FIX_SPEC_DIR` |

With `ME_FIX_DD=off`, or a BeginString other than `FIXT.1.1`, the data dictionary isn't used.

The acceptor runs inside the engine process. If it fails to start (for example, because the dictionaries are missing), the engine logs `[FIX] acceptor start failed` and keeps serving HTTP and ITCH.

### Logon authentication

Authentication is off unless `ME_FIX_AUTH=on`. When it's on, `FromAdmin` checks every inbound Logon (35=A):

1. The broker's CompID (the session's TargetCompID) must exist in `fix_brokers.json`.
2. Password (554) must match the stored password exactly.
3. Username (553), if sent, must equal the CompID; case is ignored.

A failed check rejects the logon with the reason in Text (58): `broker not registered: …`, `invalid credentials for …` or `username does not match SenderCompID`. It's also recorded as a `REJECT` event on the admin console. Brokers and their passwords are managed from the admin console (see [operations](operations.md#onboarding-a-broker)).

The source-IP allowlist for external connections is enforced by nginx on port 5443, not by the acceptor (see [operations](operations.md#external-fix-gateway-port-5443)).

### Inbound application messages

| MsgType | Message | Handler |
|---|---|---|
| `D` | NewOrderSingle | `handleNewOrder` |
| `F` | OrderCancelRequest | `handleCancel` |
| `G` | OrderCancelReplaceRequest | `handleReplace` |

Other application message types are accepted at session level and then ignored; no business reject is sent.

#### NewOrderSingle (35=D)

| Tag | Field | Handling |
|---|---|---|
| 11 | ClOrdID | Echoed back; the key for later cancel and replace requests. |
| 55 | Symbol | Required. Must be a valid ticker: 1–20 characters from `A–Z`, `0–9`, `.`, `-` and `_`. |
| 54 | Side | `2` = sell; any other value = buy. |
| 38 | OrderQty | Required. Must be greater than 0 and a whole number. |
| 40 | OrdType | `1` market, `2` limit, `3` stop, `4` stop-limit. Missing or any other value is treated as limit, or as market if the price is 0 or less. |
| 44 | Price | Limit price, rounded to 2 decimals. |
| 99 | StopPx | Trigger price. Required for OrdType 3 and 4. |
| 111 | MaxFloor | Iceberg display size: only this much shows on the book at a time. |
| 59 | TimeInForce | `0` or missing: DAY. `1` GTC, `3` IOC, `4` FOK, `6` GTD. Any other value: DAY. |
| 126 | ExpireTime | GTD expiry, UTC, as `YYYYMMDD-HH:MM:SS` or `YYYYMMDD-HH:MM:SS.sss`. |
| 432 | ExpireDate | GTD expiry date if 126 is missing; the order expires at 23:59:59 UTC that day. |

Orders are tagged with the actor `FIX:<CompID>`. The engine's behaviour for each order type and time-in-force is described in [architecture](architecture.md#matching-rules).

**Rejects.** A rejected order gets an ExecutionReport with 150=8, 39=8 and the reason in tag 58:

| Reason text starts with | Cause |
|---|---|
| `missing Symbol (tag 55)` | No symbol. |
| `invalid symbol … (tag 55)` | Not a valid ticker. |
| `invalid OrderQty (tag 38)` | Quantity is 0 or less, or not a whole number. |
| `stop order with no StopPx (tag 99)` | Stop or stop-limit order without a trigger price. |
| `unknown symbol …` | Not listed, and auto-listing is off (`ME_AUTO_LIST`). |
| `off-tick: …` | Limit price isn't a multiple of the instrument's tick size. |
| `price band: …` | Limit price is outside the daily price band. |
| `market orders are not accepted during the Pre-Open call auction …` | Market order sent during Pre-Open. |
| `no liquidity: …` | Market order found nothing on the other side. |
| `matching engine did not respond within 3s` | The engine took longer than 3 s to answer. |

The tick-size and price-band checks are described in [architecture](architecture.md#pre-trade-checks-fix-orders-only).

#### OrderCancelRequest (35=F)

Send OrigClOrdID (41) of the resting order and a new ClOrdID (11).
- **Success:** an ExecutionReport with 150=4, 39=4, carrying the new ClOrdID. A parked stop order can be cancelled the same way.
- **Unknown or already-finished order:** an OrderCancelReject (35=9) with 39=8, 434=1, 102=1 and the text `unknown or already-done order`.

#### OrderCancelReplaceRequest (35=G)

Send OrigClOrdID (41), a new ClOrdID (11), the new total OrderQty (38) and optionally a new Price (44). The engine pulls the order off the book and matches it again, so it loses its place in the time queue. A new price can trade straight away.

| Situation | Result |
|---|---|
| New qty is no more than what has already filled. **This includes leaving out tag 38.** | ExecutionReport Canceled (150=4, 39=4). |
| Nothing fills on resubmission | ExecutionReport Replaced (150=5, 39=0). |
| It fills on resubmission | ExecutionReport for the fill (see below). |
| Order unknown, already finished, or a parked stop order | OrderCancelReject (35=9) with 434=2. |

A price of 0 or a missing tag 44 keeps the current price. An iceberg keeps its display size: the new remaining quantity is split into a visible slice and a hidden reserve again.

### Outbound ExecutionReport (35=8)

Each order gets one report when it's accepted. If it trades on arrival, that report covers all its fills: tag 14 is the total filled, and tags 31/32 are the last fill.

| Tag | Field | Value |
|---|---|---|
| 37 | OrderID | Engine order sequence number (`0` on validation rejects). |
| 11 | ClOrdID | From the request. |
| 17 | ExecID | Counter for this process; restarts from 1 on every restart. |
| 150 | ExecType | See the status table below. |
| 39 | OrdStatus | See the status table below. |
| 55 / 54 | Symbol / Side | `1` buy, `2` sell. |
| 38 | OrderQty | The quantity ordered (filled plus leaves if unknown). |
| 151 / 14 | LeavesQty / CumQty | Quantity still working on the book and total quantity filled. |
| 6 | AvgPx | Average fill price, 2 decimals. |
| 44 | Price | Limit price, when greater than 0. |
| 31 / 32 | LastPx / LastQty | Last fill; only sent when there was a fill. |
| 58 | Text | Reject reason. Only on rejects. |
| 207 | SecurityExchange | Always `XDHA`. |
| 48 / 22 | SecurityID / SecurityIDSource | ISIN with source `4`, when the security master has one. |
| 1300 | MarketSegmentID | Board (`MAIN`, `SME`, `ATB`…), when known. |
| 60 | TransactTime | UTC, `YYYYMMDD-HH:MM:SS.sss`. |

| Engine status | 150 ExecType | 39 OrdStatus | When |
|---|---|---|---|
| `NEW` | `0` | `0` | Resting on the book with no fills. |
| `PARTIALLY_FILLED` | `TRADE` | `1` | Partly filled. |
| `FILLED` | `TRADE` | `2` | Completely filled. |
| `PENDING` | `A` | `A` | Stop order accepted and waiting for its trigger. |
| `REPLACED` | `5` | `0` | Cancel/replace succeeded with no fill. |
| `CANCELED` | `4` | `4` | Cancelled. Also: an IOC order that filled nothing, a FOK order that couldn't fill completely, or a replace down to the filled quantity. |
| `REJECTED` | `8` | `8` | Rejected; reason in tag 58. |

Tag 150 carries the literal string `TRADE` for fills, not the FIX enum value `F`. The engine doesn't validate its own outbound messages, so receivers see exactly that string.

**Later reports.** When a parked stop order triggers, the result of matching it goes to the broker's session as a separate ExecutionReport. No other later events are reported:
- When another order fills a resting order, the resting order's owner gets no ExecutionReport.
- When a GTD order expires, its owner gets no ExecutionReport.

### Session control and diagnostics

From the admin console (see [http-api](http-api.md#admin-console-api)) an operator can:
- force a broker to log out (`disconnect`), which sends 35=5 with the text `disconnected by administrator`
- reset a session's sequence numbers (`reset`)
- view connected sessions with their last-seen MsgSeqNum (34) in each direction
- view the last 200 logon, logout and reject events (the console shows 50)

### Message log

`fixmsglog.go` plugs into quickfix as its log. Every message in and out, except heartbeats (35=0), goes to:
- an in-memory log of the newest 4000 messages, searchable through `/admin/api/fixlog`
- an append-only file, `ME_FIX_MSGLOG_FILE`

Each file line looks like:

```text
2026-01-31 09:15:02.123 IN  EXAMPLEBRKR    8=FIXT.1.1|9=…|35=D|…|11=ORD-1|…|10=…|
```

The fields are the UTC time, the direction (`IN` or `OUT`), the session (the broker's CompID) and the raw message with SOH shown as `|`. The ClOrdID used for searching is taken from tag 11, or from tag 41 if 11 is missing. At startup, the last 4000 lines of the file are loaded back into memory.

## ITCH market-data feed

`itchfeed.go` implements a SoupBinTCP server on `ITCH_PORT` (default `5501`) that streams DSE-ITCH messages. It's designed for the exchange's C++ `itch` service. The byte layouts match what that service's decoders expect, per the file header comment.

### Framing

Every SoupBinTCP packet has this frame:

```text
[2-byte big-endian length][1-byte packet type][body (length − 1 bytes)]
```

| Direction | Type | Packet |
|---|---|---|
| server → client | `A` | LoginAccept: session `SIM0000001` (10 bytes) and sequence `1` (20 bytes, right-aligned), padded with spaces. |
| server → client | `H` | Server heartbeat, every 5 s. |
| server → client | `S` | SequencedData: one or more ITCH messages back to back, about 8000 bytes or less per packet. |
| client → server | `L` | LoginRequest. The first packet of any type is treated as the login; non-`L` types are logged and accepted. |
| client → server | `O` | Logout: the server closes the connection. |
| client → server | other | Read and ignored (including `R` client heartbeats). |

### Login and authentication

With `ME_ITCH_AUTH` off (the default), any login is accepted. With `ME_ITCH_AUTH=on`, the username (body bytes 0–6) and password (bytes 6–16), with spaces trimmed, are checked against `itch_brokers.json`. A failed check closes the connection without a LoginReject. A login body shorter than 16 bytes isn't checked (see [Security notes](configuration.md#security-notes)).

The session and sequence number the client asks for in its LoginRequest are ignored. Every connection starts fresh at sequence 1 and receives the full start-of-day snapshot. Missed data can't be replayed.

### Message flow

**On connect**, in SequencedData packets:
1. `S` system event with code `O` (start of messages)
2. `T` seconds
3. `L` price tick table
4. for each instrument: `P` company directory, `R` order book directory, and an `A` add-order carrying the reference price (previous close) with order number 0 and quantity 0

**Every 700 ms:**
1. `T` seconds
2. for each new trade since the last cycle: an `A` add-order on the aggressor's side, immediately followed by an `E` full execution with an increasing match number. This sets last price, volume, trade count and high/low without leaving the order on the book.
3. for each order book: a `D` delete for every synthetic order sent last cycle, then an `A` for each current depth level, up to 10 bid and 10 ask levels. This keeps the client's top of book and depth in line with the engine.

### Message layouts

All integers are big-endian. Text fields are fixed-width and padded with spaces. Sizes don't include the 1-byte message type.

| Type | Size | Fields (bytes) |
|---|---|---|
| `S` system event | 17 | timestamp (4), group (8), event code (1), order book (4) |
| `T` seconds | 4 | seconds since local midnight (4) |
| `L` price tick | 16 | timestamp (4), tick table ID `1` (4), tick size `1` (4), price start `0` (4) |
| `P` company directory | 68 | timestamp (4), company = order book ID (4), name (30), symbol (30) |
| `R` order book directory | 170 | timestamp (4), order book (4), price type `P` (1), ISIN (12: the symbol, as a placeholder), security code (12: the symbol), currency `BDT` (3), group `PUBLIC` (8), min qty `1` (8), qty tick table `1` (4), price tick table `1` (4), price decimals `2` (4), delisting date (4), delisting time (4), market type `P` (1), company (4), listing type `N` (1), `General` (12), `EQ` (12), security name (60), maturity date (4), yield decimals (4) |
| `A` add order | 33 | timestamp (4), order number (8), side `B`/`S` (1), quantity (8), order book (4), price (4), yield `0` (4) |
| `E` order executed | 28 | timestamp (4), order number (8), executed quantity (8), match number (8) |
| `D` order delete | 12 | timestamp (4), order number (8) |

- **Timestamps** are the nanosecond part of the current second; the `T` message gives the second.
- **Prices** are unsigned 32-bit integers equal to price × 100 (2 decimals).
- **Order book IDs** are `100001` plus the instrument's position in the instrument list. They stay stable while the process runs, because the list only grows. `orderBookID` in `engine.go` is the one source of these IDs for the ITCH feed, the WebSocket gateway and the live load test.
- **Order numbers** and **match numbers** are counted separately for each connection.
