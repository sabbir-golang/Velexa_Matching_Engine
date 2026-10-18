# Testing

This page is the test plan for the matching engine. It lists every test suite, how to run it, the test cases it covers with their expected results, the checks that still need a person and the real OMS, and the results of the last full run.

## Quick reference

| Command | What it runs | Time |
|---|---|---|
| `make test` | All Go tests: unit, integration, the FIX conformance suite (PostgreSQL tests are skipped) | ~40 s |
| `make test-db` | The same, plus the PostgreSQL tests, against a throwaway `postgres:16-alpine` container | ~45 s |
| `make race` | The same tests under the race detector | ~45 s |
| `make e2e` | System test: builds the engine, starts it as a fresh install on spare ports, then drives the console, two FIX brokers and the ITCH feed, runs the browser UI test, and restarts the engine to check persistence | ~25 s |
| `make check` | `vet` + `race` + `e2e`: run this before every commit | ~75 s |
| `E2E_DB=1 make e2e` | The system test with accounts and audit in a throwaway PostgreSQL | ~30 s |
| `make cover` | Statement coverage across all packages | ~45 s |
| `make bench` | Matching throughput benchmark | ~1 s |
| `make docker-test` | Builds the image and runs the system test against engine + PostgreSQL containers on spare ports, then restart, clean-stop, backup → wipe → restore and container-hardening checks | ~2 min |
| `make conformance` / `make report` | Only the DSE FIX conformance suite / its PDF report | ~35 s |

`make e2e` never touches your `data/` folder, `me.env` or the ports of a running engine. It runs in a temporary folder on ports 18910 (HTTP), 15901 (FIX) and 15951–15953 (ITCH); the variables `E2E_HTTP_PORT`, `E2E_FIX_PORT` and `E2E_ITCH_PORT` move them. The browser part needs Node 22+ and Google Chrome or Chromium; without them it is skipped with a note.

Never point the system or UI tests at a production engine. They create accounts and brokers, and open and close the market.

## Test levels

| Level | Where | What it proves |
|---|---|---|
| Unit | `internal/*/…_test.go` | Each package on its own: matching rules, the user store, the session cookie, configuration loading, ITCH encoding. |
| Integration | `internal/rest/*_test.go`, `internal/itchgw/market_test.go`, `internal/fixgw/*_test.go` | HTTP routes through the real middleware chain; the ITCH feed over a real TCP socket; the FIX acceptor over a real QuickFIX session. |
| Conformance | `internal/fixgw/conformance_test.go` → `docs/conformance_results.json`, `docs/DSE_Conformance_Report.pdf` | 184 cases comparing the engine's FIX behaviour with the DSE's. A `FAIL` fails the build; a `GAP` is a known missing feature (set `CONF_STRICT=1` to fail on those too). |
| System | `tools/e2e.sh` + `tools/e2e_test.py` | The built binary as a fresh install, used the way the OMS and a console user use it, including a restart. |
| UI | `tools/ui_test.mjs` | The console and Trade Terminal in headless Chrome: forms, CSRF, XSS, sign-out. |
| Concurrency | `make race`, `TestConcurrentTradingKeepsTheBookConsistent` | No data races; 8 brokers trading at once leave a consistent book. |
| Performance | `make bench`, case `PERF-01` | Matching cost per order and end-to-end FIX throughput. |
| Manual with the OMS | [the checklist below](#manual-checks-with-the-oms) | The real OMS against the engine. |

## Test cases

Status is the result of the run on 17 Oct 2026 (see [Last run](#last-run)). "Automated by" names the test that checks the case.

### Matching core (`internal/engine`)

| ID | Case | Expected | Automated by | Status |
|---|---|---|---|---|
| ENG-01 | Two asks at the same price, then a crossing buy | Better price first, then the older order at the same price (price-time priority); the buyer gets price improvement | `TestPriceTimePriority` | PASS |
| ENG-02 | Buy 100 against 40 offered | 40 filled, 60 rest on the bid | `TestPartialFillRestsRemainder` | PASS |
| ENG-03 | IOC partly fillable; FOK not fully fillable; FOK fully fillable | IOC fills what it can, rest cancelled; FOK fills nothing and leaves the book alone; FOK fills completely | `TestIOCAndFOK` | PASS |
| ENG-04 | Market order on an empty book; market order sweeping two levels | Rejected (no liquidity); fills both levels, the remainder never rests | `TestMarketOrders` | PASS |
| ENG-05 | Iceberg 100 showing 30 | 30 visible; after the slice fills, the next slice shows at the back of the queue | `TestIcebergShowsOnlyItsPeak` | PASS |
| ENG-06 | Cancel and amend (qty, price, marketable price, no qty, another broker's order, cancel twice) | Amends re-rest or trade; a missing quantity keeps the quantity; another broker is refused; a second cancel is refused | `TestCancelAndAmend` | PASS |
| ENG-07 | Two brokers use the same ClOrdID | Each cancels and amends only its own order | `TestSameClOrdIDOnTwoBrokers` | PASS |
| ENG-08 | Same broker reuses the ClOrdID of a working order (new order or amend) | Rejected `Duplicate ClOrdID`; allowed again once that order is gone | `TestDuplicateClOrdIDIsRejected` | PASS |
| ENG-09 | Amend to an off-tick price, outside the band, or after the close | Refused with the reason; order untouched | `TestAmendIsCheckedLikeANewOrder` | PASS |
| ENG-10 | Pre-Open: order outside the band, market order, crossing limits, then open | Band reject; market rejected; orders rest unmatched; the open uncrosses at one price | `TestPreOpenChecksPricesAndUncrosses` | PASS |
| ENG-11 | Stop buy at 10.30, a trade prints at 10.30 | The stop triggers and buys | `TestStopTriggersOnLastPrice` | PASS |
| ENG-12 | 8 brokers × 400 random orders and cancels at once | Book never crossed; bought = sold; index holds exactly the resting orders; no data race | `TestConcurrentTradingKeepsTheBookConsistent` | PASS |
| ENG-13 | A trading day: close, post-close, end of day, next pre-open | Closing prices; post-close only at the close price; nothing accepted when closed; Day orders expire (150=C), GTC stays; close becomes the next reference | `TestTradingDayHousekeeping` | PASS |
| ENG-14 | Dhaka session clock | Phases at the right times, weekends closed, bad schedules refused | `TestScheduleFollowsTheDhakaClock` | PASS |
| ENG-15 | Market-event journal | The book can be rebuilt from it; the ring keeps the newest events | `TestJournalRebuildsTheBook`, `TestJournalRingKeepsTheNewestEvents` | PASS |
| ENG-16 | Instrument universe: file, CSV/JSON, auto-list, listing, order-book IDs across restarts | As documented in [architecture](architecture.md#instrument-universe) | `instruments_test.go`, `obids_test.go` | PASS |

### FIX protocol (`internal/fixgw`)

The conformance suite holds 184 cases in 13 groups. The full list, with what was sent and received, is in `docs/conformance_results.json` and the PDF report.

| Group | Cases | PASS | GAP / PARTIAL | INFO |
|---|---|---|---|---|
| Order type (40) | 10 | 7 | – | 3 |
| Validity (59) | 14 | 11 | 3 | – |
| Side & quantity | 13 | 13 | – | – |
| Board group (762) | 20 | 4 | 16 | – |
| ExecInst (18) | 6 | – | 6 | – |
| Quantity options | 4 | 1 | 2 | 1 |
| Tags & reports | 14 | 13 | – | 1 |
| Price band / tick | 70 | 69 | – | 1 |
| Cancel / modify / status | 13 | 12 | – | 1 |
| Phases | 5 | 5 | – | – |
| Matching | 3 | 3 | – | – |
| Rules not modelled | 6 | 2 | 2 | 2 |
| Session messages | 6 | 6 | – | – |
| **Total** | **184** | **146** | **29** | **9** |

Other FIX tests: logon passwords never reach the message log (`TestLogonPasswordsAreNeverStored`); malformed CompIDs are refused (`TestLogonCompIDMustBePlain`); DSE report and reject formats (`TestReportsLikeTheDSE`, `TestRejectsUseTheDSEForm`); logon authentication modes `on`/`off`/auto (`TestFixAuthModes`).

### ITCH market data (`internal/itchgw`)

| ID | Case | Expected | Automated by | Status |
|---|---|---|---|---|
| ITCH-U1 | Login from sequence 1 | Start-of-day order: time, system event, tick tables, directories, then the book as adds | `TestStartOfDayOrder` | PASS |
| ITCH-U2 | Live order, trade and delete | Real order numbers; executions and deletes follow | `TestLiveEventsUseRealOrderNumbers` | PASS |
| ITCH-U3 | Reconnect with the next sequence | The stream continues with no gap or repeat | `TestResumeContinuesTheStream` | PASS |
| ITCH-U4 | Halt an instrument | Trading-state `H` message | `TestHaltSendsTradingState` | PASS |
| ITCH-U5 | Index and news feeds | `Z` values; `N` session and halt announcements | `TestIndexFeedSendsValues`, `TestNewsFeedAnnouncesSessionsAndHalts` | PASS |

### Console security, accounts and roles (`internal/rest`, `internal/database/users`)

| ID | Case | Expected | Automated by | Status |
|---|---|---|---|---|
| SEC-U1 | Password rules, hashing, upgrade to 600 000 iterations | Weak passwords refused; salted PBKDF2; old hashes upgraded at sign-in | `TestPasswordPolicy`, `TestPasswordsAreHashedAndSalted`, `TestDefaultWorkFactorAndUpgrade` | PASS |
| SEC-U2 | Sign-out, password change, reset, disable, delete | Every session of the account ends at once | `TestSessionTokenChangesOnSecurityEvents`, `TestLogoutEndsTheSession`, `TestDeletedAccountsSessionEnds` | PASS |
| SEC-U3 | Forged, tampered, expired (8 h), idle (30 min), old-format cookies | All refused (401) | `TestSessionsCannotBeForgedOrReused` | PASS |
| SEC-U4 | POST without CSRF token, wrong token, cross-site, foreign Origin | 403, or back to the welcome page for forms | `TestCSRFIsEnforced` | PASS |
| SEC-U5 | GET on a state-changing route | 405 | `TestStateChangesNeedPost` | PASS |
| SEC-U6 | Spoofed X-Forwarded-For; per-username lockout (`ME_RATE_LIMIT=on`); off by default | Lockout holds; nothing locked when off | `TestSpoofedForwardedForDoesNotBypassTheLockout`, `TestUsernameLockoutAcrossAddresses`, `TestRateLimitIsOffByDefault` | PASS |
| SEC-U7 | Unknown vs known username, wrong password | Same answer | `TestFailedLoginLooksTheSameForUnknownUsers` | PASS |
| SEC-U8 | Security headers, HTTPS mode | CSP, frame, nosniff, referrer headers; `__Host-` cookie and HSTS over HTTPS | `TestSecurityHeaders` | PASS |
| SEC-U9 | Order entry and phase control | `/api/order` needs sign-in and records `USER:<name>`; pre-open/open need an admin | `TestOrderEntryAndPhaseControlNeedSignIn` | PASS |
| ACC-U1 | User role vs admin role on every console route | Users reach only their routes; admins reach all; role changes apply at once | `TestUserRoleIsReadOnly`, `TestAdminRoleReachesEverything`, `TestDisablingOrDemotingTakesEffectImmediately` | PASS |
| ACC-U2 | Self-registration | Profile validated; taken names/emails and reserved names get one answer; registration can be closed | `TestSelfRegistrationMakesReadOnlyUsers`, `TestRegisterRefusesReservedNames` | PASS |
| ACC-U3 | Per-user isolation | A user sees and manages only the brokers they registered; a re-used username inherits nothing | `TestUsersOnlySeeTheirOwnBrokers` | PASS |
| ACC-U4 | First admin, default password, break-glass admin | Created from the env or a random password; the old default is replaced; env admin only while no file admin exists | `TestBootstrapCreatesTheEnvAdmin`, `TestDefaultPasswordIsReplaced`, `TestBreakGlassOnlyWithoutAFileAdmin` | PASS |

### Database (`internal/database/db`, PostgreSQL tests — run with `make test-db`)

| ID | Case | Expected | Automated by | Status |
|---|---|---|---|---|
| DB-U1 | Start against an empty server | The database is created, migrations applied once and recorded; a second run changes nothing | `TestMigrationsApplyOnceAndInOrder` | PASS |
| DB-U2 | Accounts across a restart | Profile, role, disabled flag, password hash, session token and last login all come back; deletes stay deleted | `TestPGAccountsSurviveARestart` | PASS |
| DB-U3 | Same email twice | Refused | `TestPGEmailIsUniqueInTheDatabase` | PASS |
| DB-U4 | Database goes away during add, role change, delete | Error "could not be saved"; memory unchanged, so it never disagrees with the database | `TestPGFailedWriteIsRolledBack` | PASS |
| DB-U5 | First start with an existing `users.json` | Imported with hashes and session tokens (nobody signed out), file renamed, never imported twice | `TestPGImportsUsersJSONOnce` | PASS |
| DB-U6 | Audit trail in the database | Newest first; per-user prefix lookup (with `_` literal); old jsonl imported; entries fall back to the file when the database is down | `TestDatabaseTrail`, `TestFileTrail` | PASS |
| DB-01 | System test with `E2E_DB=1` | Accounts and audit rows in PostgreSQL, no `users.json`, everything survives a restart | `tools/e2e.sh` | PASS |

### Configuration and portability (`internal/config`, `internal/rest/handlers/broker`)

| ID | Case | Expected | Automated by | Status |
|---|---|---|---|---|
| CFG-U1 | First run without `me.env` | `me.env` written (0600) with a valid random admin password and secret; never overwritten | `TestFirstRunEnvFile` | PASS |
| CFG-U2 | `me.env.example` and the built-in template | Identical | `TestTemplateMatchesExampleFile` | PASS |
| CFG-U3 | `ME_STATE_DIR`; env file parsing; environment wins over the file | As documented | `TestStateDirSetting`, `TestReadEnvFile`, `TestEnvFileDoesNotOverrideTheEnvironment` | PASS |
| CFG-U4 | Broker sheet addresses | This PC's LAN IP (also as Host/SNI) and the real ports; env values win; TLS text only with `ME_FIXS_PORT` | `TestEndpointsFollowThisMachine` | PASS |

### System test (`make e2e`)

| ID | Case | Expected | Status |
|---|---|---|---|
| SYS-01 | Fresh install starts | `/healthz`, `/health`, `/api/status` answer; `me.env` created | PASS |
| SEC-01 | Admin sign-in | Generated password works; a wrong one is refused | PASS |
| SEC-02 | Console POST without CSRF token | 403 | PASS |
| SEC-03 | Anonymous order entry and phase control | 401 | PASS |
| SES-01 | Admin opens the market | Continuous Trading | PASS |
| BRK-01 | Issue two brokers | FIX and ITCH credentials returned; the sheet shows this PC and the real FIX port | PASS |
| BRK-02 | Issue a duplicate SenderCompID | Refused | PASS |
| MD-01 | Market data | Book and market watch for the test symbol | PASS |
| FIX-01 | Logon with the issued credentials | 35=A for both brokers | PASS |
| FIX-02 | Logon with a wrong password; unknown CompID | 35=5 with the reason | PASS |
| FIX-03 | Resting order | 150=0 with an OrderID | PASS |
| FIX-04 | Crossing order | Both brokers get 150=F, 39=2; market data shows the trade price | PASS |
| FIX-05 | Same ClOrdID on two brokers, each cancels | Each cancels only its own | PASS |
| FIX-06 | Duplicate ClOrdID of a working order | 150=8 `Duplicate ClOrdID` | PASS |
| FIX-07 | Amend outside the band | 35=9 with the band reason and the real OrderID | PASS |
| FIX-08 | Amend within the band, then cancel | 150=5, then 150=4 | PASS |
| FIX-09 | Off-tick and out-of-band new orders | 150=8 with DSE text | PASS |
| ITCH-01 | ITCH login, snapshot, live trade | Directory and adds in the snapshot; the live trade arrives as add + execution | PASS |
| WEB-01 | Signed-in user orders through `/api/order` | Accepted | PASS |
| PERF-01 | 2 brokers × 2000 pipelined orders that all trade | Every order acknowledged and filled | PASS |
| UI-01…13 | Browser: register, sign in, add a broker with a hostile name, trade from the terminal, sign out, admin users/brokers pages, phase control | All pass, no script ran from data, no CSP errors | PASS |
| PER-01 | Restart: admin sign-in | Works (accounts and secret persisted) | PASS |
| PER-02 | Restart: brokers | Still listed | PASS |
| PER-03 | Restart: FIX logon | Same credentials work | PASS |
| PER-04 | Restart: revoke a broker | Its logon is refused | PASS |
| PER-05 | Restart: order-book IDs | Unchanged | PASS |
| LOG-01 | Engine log | No panic, fatal error or data race | PASS |

### Docker deployment (`make docker-test`)

| ID | Case | Expected | Status |
|---|---|---|---|
| (system) | SYS/SEC/BRK/FIX/ITCH/WEB/PERF cases above, against the containers | 20/20 | PASS |
| DOCK-01 | `docker compose stop engine` | Clean stop on SIGTERM within seconds (log ends with `stopped`) | PASS |
| PER-01…04 | Restart the engine container | Accounts, brokers, FIX credentials and revocations survive | PASS |
| BAK-01…03 | Backup, delete the database volume and data folder, restore | Admin password, brokers (and revocations), FIX passwords and the audit trail come back | PASS |
| DOCK-02 | Engine user | Not root | PASS |
| DOCK-03 | Root filesystem | Read-only | PASS |
| DOCK-04 | Docker health check | `healthy`, version reported | PASS |
| DOCK-05 | Storage | Accounts in the compose PostgreSQL; broker files in the data folder | PASS |
| BRK-U1 | A broker credential cannot be saved (read-only folder) | Error reported, change undone; the engine refuses to start on a folder it cannot write | PASS (`TestSaveFailuresAreReported`) |
| FIX-U1 | FIX message log size | Rotated at `ME_FIX_MSGLOG_MAX_MB`, `ME_FIX_MSGLOG_KEEP` files kept, only the tail read at start-up | PASS (`TestMessageLogRotatesAndReadsOnlyItsTail`) |

## Manual checks with the OMS

Run these after `make check` passes and before a release, with the real OMS (broker terminal + `fix_initiator_go`) pointed at a test engine, never at production.

| ID | Steps | Expected |
|---|---|---|
| OMS-01 | Issue a broker in the console; copy its sheet values into the broker terminal (Itch & Fix Management); **Test Primary** | TCP reachable, logon accepted |
| OMS-02 | Place a limit buy and a crossing sell from two broker accounts | Both blotters show the fill; the order book and tape update |
| OMS-03 | Cancel and amend a working order from the OMS | 150=4 / 150=5 shown; an amend outside the band shows the reason |
| OMS-04 | Order outside the band, off tick, wrong lot | Rejected with the DSE text in the blotter |
| OMS-05 | Market watch / depth in the trader UI | Prices, depth and index update live (ITCH) |
| OMS-06 | Admin forces Pre-Open, places crossing orders, then Open | Orders rest in Pre-Open; one auction price at the open; blotters updated |
| OMS-07 | End of day | Day orders show as expired; GTC orders stay |
| OMS-08 | Restart the engine during the day | The OMS reconnects; credentials still work |
| OMS-09 | Revoke the broker in the console, force-disconnect its session | The OMS cannot log on again |

## Last run

17 Oct 2026, Go 1.26.5, Linux, on the developer PC:

- **Go tests:** all 101 test functions pass, also under `-race` (no data races).
- **Conformance:** 184 cases: 146 PASS, 26 GAP, 3 PARTIAL, 9 INFO, 0 FAIL.
- **System test:** 20/20, UI 13/13, restart 4/4 + PER-05 + LOG-01.
- **Coverage:** 71.3% of statements across all packages. Lowest: `itchgw` (41%: the legacy feed mode and the WebSocket gateway have no tests) and `Cmd` (start-up wiring, exercised only by the system test).
- **Performance:** `BenchmarkSubmitAndMatch` ≈ 2.5 µs per order (≈ 400 000 orders/s on the matching goroutine, one synchronous client, 12 allocations). End to end over FIX (`PERF-01`, two pipelined Python clients on one PC): ≈ 11 000 orders/s with every order acknowledged and filled; the Python client is the limit there.

### Defects found and fixed in this run

| # | Defect | Impact | Fix | Test |
|---|---|---|---|---|
| 1 | Working orders were indexed by ClOrdID alone, across all brokers | If two brokers used the same ClOrdID, the first broker could no longer cancel or amend its order, and a stale entry stayed behind | Index by owner + ClOrdID (`openKey`) | ENG-07, FIX-05 |
| 2 | A duplicate ClOrdID of a working order was accepted (QT-6) | The second order could never be cancelled on its own | Rejected `Duplicate ClOrdID` (new orders and amends) | ENG-08, FIX-06 |
| 3 | Amends skipped the tick, price-band and lot checks (CM-7, CM-8) | Any order could be amended to any price | Amends are checked like new orders and refused with 35=9 and the reason | ENG-09, FIX-07 |
| 4 | Pre-Open skipped the tick and band checks (PH-3) | Orders far outside the circuit limits could enter the auction | Price checks run in every session | ENG-10 |
| 5 | GTD without an expiry was accepted (TIF-6a) | The order never expired | Rejected unless 126/432 is given and in the future | TIF-6a |
| 6 | An amend without OrderQty (38) cancelled the order | An OMS sending a price-only amend lost the order | A missing quantity keeps the current quantity | ENG-06 |
| 7 | The conformance suite never failed the build on a `FAIL` verdict | Regressions passed silently | `FAIL` now fails `go test`; `CONF_STRICT=1` also fails on gaps | — |
| 8 | Stale conformance notes for ST-1/ST-2 and a broken OT-06 setup | The report understated what works | Real checks / correct setup | ST-1, ST-2, OT-06 |
| 9 | Trade Terminal showed `REJECTED · resting in book` | The reject reason was hidden | Shows the reason | UI-06 |
| 10 | `/favicon.ico` returned 404 on every page | Error noise in browser logs | Served | — |

### Known gaps (features not implemented)

These are recorded as GAP in the conformance report, and are deliberate scope decisions rather than bugs:

- **Validity:** At-the-Opening, Good-till-Crossing and At-the-Close (59=2/5/7) are treated as Day (TIF-2/5/7).
- **Board groups (762):** not validated against the instrument's board (BG-02…BG-17).
- **ExecInst (18):** Park / Unpark / Block are not interpreted, and two DSE combinations are not in the data dictionary (EI-1…EI-6).
- **MinQty (110):** ignored (QO-3, QO-4).
- **Market rules:** no automatic circuit breaker / volatility halt (ST-3), and no category- or board-specific lot, band or session rules (ST-4).
