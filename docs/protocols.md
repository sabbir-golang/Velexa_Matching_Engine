# FIX and ITCH protocols

This page is for anyone connecting to the engine's two binary interfaces: the FIX acceptor that brokers' order-management systems send orders to, and the SoupBinTCP ITCH feed that publishes market data. It covers session settings, authentication, the messages and tags the engine understands, what it sends back, and the message log.

## FIX acceptor

The acceptor is built on [quickfixgo](https://github.com/quickfixgo/quickfix) in `fixgw/fixacceptor.go` (`StartFixAcceptor`). It stands in for the exchange's order gateway: brokers' FIX initiators connect, send orders, and get execution reports produced by the matching engine.

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
| Message store | In memory only, the newest 20,000 sent messages per session. A ResendRequest (35=2) reaching further back gets a SequenceReset-GapFill (35=4, 123=Y) for the missing range, then the kept messages (43=Y). Nothing survives a restart | `ME_FIX_RESEND_KEEP` (`0` keeps all) |
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
| `H` | OrderStatusRequest | `handleStatus` |

Other application message types are accepted at session level and then ignored; no business reject is sent.

#### NewOrderSingle (35=D)

| Tag | Field | Handling |
|---|---|---|
| 11 | ClOrdID | Echoed back; the key for later cancel, replace and status requests. Must not repeat the ClOrdID of one of the session's working orders (rejected `Duplicate ClOrdID`). Different brokers may use the same values. |
| 55 | Symbol | Required. Must be a valid ticker: 1–20 characters from `A–Z`, `0–9`, `.`, `-` and `_`. |
| 54 | Side | `1` buy, `2` sell. Anything else is rejected. |
| 38 | OrderQty | Required. Must be greater than 0 and a whole number. |
| 40 | OrdType | `1` market, `2` limit (needs tag 44), `3` stop, `4` stop-limit, `Z` market-at-best (DSE). Anything else is rejected. |
| 44 | Price | Limit price, rounded to 2 decimals. On a market order it is allowed only in post-close and must be the close price. |
| 99 | StopPx | Trigger price. Required for OrdType 3 and 4. |
| 111 / 1138 | MaxFloor / DisplayQty | Iceberg display size: only this much shows on the book at a time. The OMS sends 1138. |
| 59 | TimeInForce | `0` or missing: DAY. `1` GTC, `3` IOC, `4` FOK, `6` GTD (needs 126 or 432 in the future, else rejected). `S` (session) and other values: DAY. |
| 126 | ExpireTime | GTD expiry, UTC, as `YYYYMMDD-HH:MM:SS` or `YYYYMMDD-HH:MM:SS.sss`. |
| 432 | ExpireDate | GTD expiry date if 126 is missing; the order expires at 23:59:59 UTC that day. |
| 1, 50, 453, 529, 762 | Account, SenderSubID, Parties, OrderRestrictions, SecuritySubType | Not used for matching; echoed on every report about the order (50 comes back as 57). |
| 18, 110 | ExecInst, MinQty | Accepted (`18=S`, `q`, `w` are in the dictionary) but not acted on. |

Orders are tagged with the actor `FIX:<CompID>`; only that session can cancel, replace or query them. The engine's behaviour for each order type and time-in-force is described in [architecture](architecture.md#matching-rules).

**Rejects.** A rejected order gets an ExecutionReport with 150=8, 39=8, OrderID `NONE`, OrdRejReason 103=99 and the reason in tag 58. Reasons the DSE also uses carry its error number in front, in the DSE form `(nnn): text`; the others are sent as plain text (no numbers are made up):

| Tag 58 | Cause |
|---|---|
| `(792): Minimum quantity step: N` | Quantity is not a whole number of market lots. |
| `(293): Orders are not being accepted at this time` | The market is closed (Close or Closed phase). |
| `(1116): Order price is not equal to close price` | Post-close order at any price other than the close. |
| `Price is not allowed for market order` | Market order with a price outside post-close. |
| `price band: P outside +/-x% limit [lo-hi] of prev N` | Limit price outside the daily band. **The OMS parses this exact wording** to learn the band, so it is kept as it is. |
| `off-tick: …` | Limit price isn't a multiple of the instrument's tick size. |
| `limit order without a price (tag 44)`, `unsupported order type (tag 40): …`, `unsupported side (tag 54): …` | Malformed order. |
| `missing Symbol (tag 55)`, `invalid symbol … (tag 55)`, `invalid OrderQty (tag 38)`, `stop order with no StopPx (tag 99)`, `unknown symbol …` | Validation. |
| `market orders are not accepted during the Pre-Open call auction …` | Market order during Pre-Open. |
| `no liquidity: …` | Market order found nothing on the other side. |

The tick-size and price-band checks are described in [architecture](architecture.md#pre-trade-checks-fix-orders-only).

#### OrderCancelRequest (35=F)

Send OrigClOrdID (41) of the order and a new ClOrdID (11).
- **Success:** an ExecutionReport 150=4, 39=4 with 11 = the cancel's ClOrdID, 41 = the original and the same OrderID (37). A parked stop order can be cancelled the same way.
- **Unknown, finished or another session's order:** an OrderCancelReject (35=9) as the DSE sends it: 37=`NONE`, 39=8, 434=1, 102=99, 58=`(226): Invalid order number`.

#### OrderCancelReplaceRequest (35=G)

Send OrigClOrdID (41), a new ClOrdID (11), the new total OrderQty (38) and optionally a new Price (44). The engine pulls the order off the book and matches it again, so it loses its place in the time queue. A new price can trade straight away.

| Situation | Result |
|---|---|
| New qty is no more than what has already filled | 150=4, 39=4 (the order is done). Leaving out tag 38 keeps the current quantity. |
| Otherwise | 150=5 Replaced (39=0, or 1 if it had fills), then one 150=F per trade if the new price trades. |
| Order unknown, finished, another session's, or a parked stop | OrderCancelReject (35=9) with 434=2, 37=`NONE`, 58=`(226): Invalid order number`. |
| New price off tick or outside the band, quantity not whole lots, market closed (Close/Closed), or the new ClOrdID already used by a working order | OrderCancelReject (35=9) with 434=2, the order's OrderID (37), 39=0 and the reason in 58 (DSE number where known). The order is untouched. |

#### OrderStatusRequest (35=H)

Send the order's ClOrdID (11) and an OrdStatusReqID (790). A working order is answered with an ExecutionReport 150=I carrying its current 39/14/151/6 and the 790. Any other order gets 150=I, 39=8, 37=`NONE`, 58=`(226): Invalid order number`.

### Outbound ExecutionReport (35=8)

Reports follow the DSE: every event is its own report, and both sides of a trade are told.

| Event | Reports |
|---|---|
| New order accepted | 150=0, 39=0, then **one 150=F per trade** (39=1 or 2), then 150=4 for any IOC/FOK/market remainder that does not rest. |
| Resting order traded by someone else | one 150=F per trade, to the resting order's owner. |
| Opening auction trade | one 150=F per trade to each side. |
| Cancel / replace | as above. |
| Stop accepted / triggered | 150=A, 39=A; when it triggers: 150=0 then the trades. |
| Expiry (GTD time, or DAY at end of day) | 150=C, 39=C, with 11 = 41 = the order's ClOrdID, 14 = filled so far. |

| Tag | Field | Value |
|---|---|---|
| 37 | OrderID | Dhaka trade date + engine order number, e.g. `2026101412345`; the same on every report about the order. `NONE` on rejects. |
| 11 / 41 | ClOrdID / OrigClOrdID | 41 on cancel, replace and expiry reports. |
| 17 | ExecID | Unique across restarts (start time + counter, hex). |
| 150 / 39 | ExecType / OrdStatus | `0`, `F`, `4`, `5`, `8`, `A`, `C`, `I` (FIX 5.0 SP2 values). |
| 55 / 54 / 38 | Symbol / Side / OrderQty | |
| 40 / 59 / 44 | OrdType / TimeInForce / Price | As sent; 44 only for priced orders. The OMS needs 40 to book market-order fills. |
| 1, 453, 529, 762, header 57 | Account, Parties, OrderRestrictions, SecuritySubType, TargetSubID | Echoed from the order (57 = the order's 50). |
| 14 / 151 / 6 | CumQty / LeavesQty / AvgPx | Over the order's life. |
| 31 / 32 | LastPx / LastQty | On 150=F only: this trade. |
| 880 | TrdMatchID | On 150=F: the trade number, the same on both sides of the trade. |
| 75 / 64 | TradeDate / SettlDate | On 150=F: Dhaka trade date and T+2 business days (Friday and Saturday skipped; holidays are not known). |
| 381 | GrossTradeAmt | On 150=F: LastQty × LastPx. |
| 1057 | AggressorIndicator | On 150=F: `Y` for the order that took liquidity, `N` for the resting one (both `N` in the auction). |
| 103 / 58 | OrdRejReason / Text | On rejects: 103=99 and the reason. |
| 60 | TransactTime | UTC, `YYYYMMDD-HH:MM:SS.sss`. |

Tags 48/22/207/1300 are no longer sent: the DSE identifies the instrument by Symbol only.

All reports are queued and sent by one goroutine in the order the engine produced them, so a resting order's fills can never overtake its own acknowledgement.

### Session control and diagnostics

From the admin console (see [http-api](http-api.md#admin-console-api)) an operator can:
- force a broker to log out (`disconnect`), which sends 35=5 with the text `disconnected by administrator`
- reset a session's sequence numbers (`reset`)
- view connected sessions with their last-seen MsgSeqNum (34) in each direction
- view the last 200 logon, logout and reject events (the console shows 50)

### Message log

`fixgw/msglog.go` plugs into quickfix as its log. Every message in and out, except heartbeats (35=0), goes to:
- an in-memory log of the newest 4000 messages, searchable through `/admin/api/fixlog`
- an append-only file, `ME_FIX_MSGLOG_FILE`

Each file line looks like:

```text
2026-01-31 09:15:02.123 IN  EXAMPLEBRKR    8=FIXT.1.1|9=…|35=D|…|11=ORD-1|…|10=…|
```

The fields are the UTC time, the direction (`IN` or `OUT`), the session (the broker's CompID) and the raw message with SOH shown as `|`. The ClOrdID used for searching is taken from tag 11, or from tag 41 if 11 is missing. At startup, the last 4000 lines of the file are loaded back into memory.

## ITCH feeds

The engine serves the three DSE-ITCH feeds the OMS itch service connects to, each a SoupBinTCP server:

| Feed | Port (env) | Default | Carries |
|---|---|---|---|
| Market | `ITCH_PORT` | `5501` | Directory, trading state, reference prices, the order book (A/E/C/D), session events, closing prices. `itchgw/market.go` |
| Index | `ME_ITCH_INDEX_PORT` | `5502` | `Z` index values once a second. `itchgw/index.go` |
| News | `ME_ITCH_NEWS_PORT` | `5503` | `N` announcements. `itchgw/news.go` |

Set a port to `off` to disable that feed. `ME_ITCH_MODE=legacy` brings back the old market feed (synthetic depth re-published every 700 ms, `itchgw/feed_legacy.go`).

To use the index and news feeds, point the itch service's `index:` and `news:` sections (its `config.docker.yml`) at the engine, the same way `market:` already is: host `172.20.0.1`, ports `5502` and `5503`.

### Framing

Every SoupBinTCP packet has this frame:

```text
[2-byte big-endian length][1-byte packet type][body (length − 1 bytes)]
```

| Direction | Type | Packet |
|---|---|---|
| server → client | `A` | LoginAccept: session (10 bytes) and next sequence number (20 bytes, right-aligned), padded with spaces. |
| server → client | `J` | LoginReject (`A` = not authorised), when ITCH authentication is on. |
| server → client | `H` | Server heartbeat, every 2 s. |
| server → client | `S` | SequencedData: one or more ITCH messages back to back, at most about 8000 bytes. Each packet is one sequence number. |
| client → server | `L` | LoginRequest: user (6), password (10), session (10), sequence (20). |
| client → server | `O` | Logout: the server closes the connection. |
| client → server | other | Read and ignored (including `R` client heartbeats). |
| client → server | (nothing) | A client that sends nothing, not even a heartbeat, for `ME_ITCH_CLIENT_TIMEOUT` seconds (default 15, SoupBinTCP's usual figure) is disconnected. This frees a connection the client has lost track of, so its own reconnect can go ahead. |

### Login, sequence numbers and replay

With `ME_ITCH_AUTH` off (the default), any login is accepted; with it on, the username and password are checked against `itch_brokers.json`.

A login with a blank session (or sequence 1) starts fresh: the start-of-day set below, then the live stream. A client that logs in again with the session and the next sequence number it last received continues where it stopped, without a second start-of-day set, as long as the engine still holds the missed events (`ME_JOURNAL_CAP`, default 400000 events). Otherwise it starts fresh.

### Market feed message flow

The feed is driven by the engine's market-event journal (`engine/events.go`): every order added to the book, every execution of a resting order, every removal, every trading-state and session change is one ITCH message, in the order the engine made them. **Order numbers are the engine's own order numbers**, so the receiver's book is the engine's book.

**Start of day**, in the order the itch decoders need:
1. `T` seconds, `S` code `O` (start of messages), `L` price tick and `M` quantity tick tables
2. per instrument: `P` company, `R` order book directory, `H` trading state (`T` tradable, `V` suspended)
3. per index (DSEX, DS30): `P`, `R` with market type `I` and group `INDEX`, `H`
4. per instrument and index: the reference price, an `A` with order number 0 and quantity 0 (previous close; the receiver derives the circuit limits from it)
5. every order resting on the book (`A`)
6. `S` for the current session (`S` pre-open, `Q` continuous, `M` close, `V` post-close, `E` end)

**Live**, as it happens:

| Engine event | ITCH |
|---|---|
| Order rests on the book (also each new iceberg slice) | `A` add order (visible quantity) |
| A resting order trades in continuous trading | `E` order executed, once per trade, on the resting order; the match number is the trade number |
| A resting order trades in the opening auction | `C` executed with price, for each side (same match number) |
| Order cancelled, replaced (re-added as a new `A`) or expired | `D` delete |
| Instrument halted / resumed / delisted | `H` trading state |
| New instrument listed | `P`, `R`, `H` and its reference price |
| Session change | `S` with the code above, and `C` (end of messages) after `E` |
| Close | `Q` closing price per traded instrument |
| New trading day | reference-price `A` per instrument (the previous close) |
| No event for a second | `T` seconds |

### Message layouts

All integers are big-endian. Text fields are fixed-width and padded with spaces. Sizes don't include the 1-byte message type. Every message except `T` starts with a 4-byte nanosecond timestamp within the second given by the last `T` (seconds since Dhaka midnight).

| Type | Size | Fields (bytes) |
|---|---|---|
| `T` seconds | 4 | seconds since Dhaka midnight (4) |
| `S` system event | 17 | timestamp (4), group (8), event code (1), order book (4) |
| `L` price tick | 16 | timestamp, tick table ID `1`, tick size `1` (= 0.01), price start `0` |
| `M` quantity tick | 24 | timestamp (4), tick table ID `1` (4), tick size `1` (8), quantity start `0` (8) |
| `P` company directory | 68 | timestamp (4), company = order book ID (4), name (30), symbol (30) |
| `R` order book directory | 170 | timestamp, order book, price type `P`, ISIN (12), security code (12), currency `BDT` (3), group (8: `PUBLIC`, `SPUBLIC`, `ATBPUB`, `DEBT` or `INDEX`), min qty = market lot (8), qty and price tick tables `1`, price decimals `2`, delisting date and time `0`, market type (`P` or `I`), company, listing type (the category letter), sector (12), instrument type (12: `EQ`, `MF`, `CB`, `DB`, `IN`), security name (60), maturity date, yield decimals |
| `H` trading state | 10 | timestamp (4), order book (4), state `T`/`V` (1), reason (1) |
| `A` add order | 33 | timestamp (4), order number (8), side `B`/`S` (1), quantity (8), order book (4), price (4), yield `0` (4) |
| `E` order executed | 28 | timestamp (4), order number (8), executed quantity (8), match number (8) |
| `C` executed with price | 37 | timestamp (4), order number (8), quantity (8), match number (8), printable `Y` (1), price (4), order book (4) |
| `D` order delete | 12 | timestamp (4), order number (8) |
| `Q` closing price | 33 | timestamp (4), quantity (8), order book (4), printable (1), price (4), yield (4), match number (8) |
| `Z` index value (index feed) | 16 | timestamp (4), index order book (4), value × 100 (8) |
| `N` news (news feed) | variable | timestamp (4), order book (4), news id (4), firm (30), then title, reference and text, each ending in a NUL byte |

- **Prices** are unsigned 32-bit integers equal to price × 100 (2 decimals).
- **Order book IDs** are permanent per symbol (`data/orderbook_ids.json`, see `engine/obids.go`). Index order books are `90001` (DSEX) and `90002` (DS30).

### Index feed

`Z` messages carry the current value of each index once a second. DSEX covers every tradable instrument; DS30 the 30 highest-priced equities. Both are price-sum indices scaled to a base value (6000 and 2200), because the simulator has no market capitalisation to weight by. The previous value comes from the market feed's reference price for the index order book; the up / down / unchanged / trades / volume / value figures shown with an index are counted by the receiver from the market feed's executions.

### News feed

`N` announcements are generated from the journal: session changes (pre-open, continuous trading, market closed, post-close, end of day), trading suspended / resumed for an instrument (with its order book) and new listings. A new connection first receives the day's announcements still held in the journal, one per packet.
