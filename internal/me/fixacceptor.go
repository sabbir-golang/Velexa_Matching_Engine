package me

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quickfixgo/quickfix"
)

// FIX tags (numeric — avoids tag-package name drift across versions).
const (
	tMsgType      quickfix.Tag = 35
	tClOrdID      quickfix.Tag = 11
	tSymbol       quickfix.Tag = 55
	tSide         quickfix.Tag = 54
	tOrderQty     quickfix.Tag = 38
	tPrice        quickfix.Tag = 44
	tOrdType      quickfix.Tag = 40
	tTimeInForce  quickfix.Tag = 59
	tExpireTime   quickfix.Tag = 126
	tExpireDate   quickfix.Tag = 432
	tOrigClOrdID  quickfix.Tag = 41
	tStopPx       quickfix.Tag = 99
	tMaxFloor     quickfix.Tag = 111 // iceberg display / disclosed qty
	tCxlRejRespTo quickfix.Tag = 434
	tCxlRejReason quickfix.Tag = 102
	tOrderID      quickfix.Tag = 37
	tExecID       quickfix.Tag = 17
	tExecType     quickfix.Tag = 150
	tOrdStatus    quickfix.Tag = 39
	tLeavesQty    quickfix.Tag = 151
	tCumQty       quickfix.Tag = 14
	tAvgPx        quickfix.Tag = 6
	tLastPx       quickfix.Tag = 31
	tLastQty      quickfix.Tag = 32
	tText         quickfix.Tag = 58
	tTransactTime quickfix.Tag = 60
	tUsername     quickfix.Tag = 553
	tPassword     quickfix.Tag = 554
	tMsgSeqNum    quickfix.Tag = 34
	tSecurityID   quickfix.Tag = 48
	tSecIDSource  quickfix.Tag = 22
	tSecExchange  quickfix.Tag = 207
	tMarketSeg    quickfix.Tag = 1300
)

// ── FIX Logon authentication ───────────────────────────────────────────────
// External brokers authenticate at logon with their assigned SenderCompID plus
// a password (FIX tag 554; tag 553 Username, if sent, must equal the CompID).
// The allowlist maps broker CompID → password, loaded from ME_FIX_AUTH_FILE
// (JSON: {"ALPHAEQUITIE":"secret", ...}). Enforcement is gated by ME_FIX_AUTH=on
// so the internal single-box setup keeps working unauthenticated by default.
type fixAuth struct {
	mu      sync.RWMutex
	enabled bool
	path    string
	creds   map[string]string // CompID (upper) → password
}

// meFixAuth is the process-wide allowlist, set by StartFixAcceptor and also read
// by the Live self-heal logon and the admin panel (add/remove brokers live).
var meFixAuth = &fixAuth{creds: map[string]string{}}

// password returns the registered password for a CompID ("" if none).
func (a *fixAuth) password(comp string) string {
	if a == nil {
		return ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.creds[strings.ToUpper(strings.TrimSpace(comp))]
}

func loadFixAuth() *fixAuth {
	a := &fixAuth{
		enabled: strings.EqualFold(envOr("ME_FIX_AUTH", "off"), "on"),
		path:    envOr("ME_FIX_AUTH_FILE", stateFile("fix_brokers.json")),
		creds:   map[string]string{},
	}
	if b, err := os.ReadFile(a.path); err == nil {
		var m map[string]string
		if json.Unmarshal(b, &m) == nil {
			for k, v := range m {
				a.creds[strings.ToUpper(strings.TrimSpace(k))] = v
			}
		}
	}
	log.Printf("[FIX] logon auth enabled=%v registered_brokers=%d", a.enabled, len(a.creds))
	return a
}

// authorize validates a broker logon. broker = the counterparty CompID (the
// acceptor session's TargetCompID); user/pass come from the Logon (553/554).
// Returns "" when allowed, otherwise the rejection reason.
func (a *fixAuth) authorize(broker, user, pass string) string {
	if !a.enabled {
		return ""
	}
	a.mu.RLock()
	want, ok := a.creds[strings.ToUpper(strings.TrimSpace(broker))]
	a.mu.RUnlock()
	if !ok {
		return "broker not registered: " + broker
	}
	if pass != want {
		return "invalid credentials for " + broker
	}
	if user != "" && !strings.EqualFold(strings.TrimSpace(user), broker) {
		return "username does not match SenderCompID"
	}
	return ""
}

// list returns the registered CompIDs (sorted). Passwords are not returned.
func (a *fixAuth) list() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]string, 0, len(a.creds))
	for k := range a.creds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (a *fixAuth) has(comp string) bool { return a.password(comp) != "" }

// add registers or updates a broker credential and persists the allowlist.
func (a *fixAuth) add(comp, pass string) error {
	comp = strings.ToUpper(strings.TrimSpace(comp))
	a.mu.Lock()
	a.creds[comp] = pass
	a.mu.Unlock()
	return a.save()
}

// del removes a broker credential and persists.
func (a *fixAuth) del(comp string) error {
	a.mu.Lock()
	delete(a.creds, strings.ToUpper(strings.TrimSpace(comp)))
	a.mu.Unlock()
	return a.save()
}

func (a *fixAuth) save() error {
	a.mu.RLock()
	b, _ := json.MarshalIndent(a.creds, "", " ")
	path := a.path
	a.mu.RUnlock()
	if path == "" {
		return nil
	}
	return os.WriteFile(path, b, 0o600)
}

// FixAcceptor is a quickfix acceptor that stands in for the DSE Matching Engine
// gateway: brokers' fix_initiator_go sessions connect here, send NewOrderSingle,
// and receive ExecutionReports produced by the simulator's matching engine.
type FixAcceptor struct {
	eng     *Engine
	acc     *quickfix.Acceptor
	execSeq uint64
	auth    *fixAuth

	sessMu   sync.Mutex
	sessions map[string]time.Time          // broker → logon time (currently connected)
	sids     map[string]quickfix.SessionID // broker → live SessionID (for control ops)
	seqs     map[string]*seqPair           // broker → last-seen sequence numbers
	events   []SessEvent                   // recent logon/logout/reject ring (newest last)
}

// seqPair is the last-observed FIX sequence numbers for a session (from tag 34).
type seqPair struct {
	Sender int `json:"sender"` // our outbound
	Target int `json:"target"` // their inbound
}

func (fa *FixAcceptor) trackSeq(broker string, m *quickfix.Message, outbound bool) {
	n, err := m.Header.GetInt(tMsgSeqNum)
	if err != nil || n <= 0 {
		return
	}
	fa.sessMu.Lock()
	p := fa.seqs[broker]
	if p == nil {
		p = &seqPair{}
		fa.seqs[broker] = p
	}
	if outbound {
		p.Sender = n
	} else {
		p.Target = n
	}
	fa.sessMu.Unlock()
}

// SeqNums returns the last-seen (sender, target) sequence numbers for a broker.
func (fa *FixAcceptor) SeqNums(broker string) (int, int) {
	fa.sessMu.Lock()
	defer fa.sessMu.Unlock()
	if p := fa.seqs[strings.ToUpper(strings.TrimSpace(broker))]; p != nil {
		return p.Sender, p.Target
	}
	return 0, 0
}

// SessEvent is a diagnostics record shown in the admin panel.
type SessEvent struct {
	Time   time.Time `json:"time"`
	Broker string    `json:"broker"`
	Kind   string    `json:"kind"` // LOGON | LOGOUT | REJECT
	Detail string    `json:"detail"`
}

// meFixAcceptor is the process-wide acceptor, queried by the admin panel.
var meFixAcceptor *FixAcceptor

// record appends a diagnostics event (session bookkeeping is done in the
// OnLogon/OnLogout callbacks, which also hold the live SessionID).
func (fa *FixAcceptor) record(broker, kind, detail string) {
	fa.sessMu.Lock()
	defer fa.sessMu.Unlock()
	fa.events = append(fa.events, SessEvent{Time: time.Now(), Broker: broker, Kind: kind, Detail: detail})
	if len(fa.events) > 200 {
		fa.events = fa.events[len(fa.events)-200:]
	}
}

func (fa *FixAcceptor) sessionID(broker string) (quickfix.SessionID, bool) {
	fa.sessMu.Lock()
	defer fa.sessMu.Unlock()
	s, ok := fa.sids[strings.ToUpper(strings.TrimSpace(broker))]
	return s, ok
}

// Disconnect force-terminates a broker's live FIX session: it sends a Logout
// (35=5) so the counterparty tears down, then unregisters the session.
func (fa *FixAcceptor) Disconnect(broker string) error {
	sid, ok := fa.sessionID(broker)
	if !ok {
		return fmt.Errorf("no active session for %s", broker)
	}
	lo := quickfix.NewMessage()
	lo.Header.SetString(tMsgType, "5") // Logout
	lo.Body.SetString(tText, "disconnected by administrator")
	quickfix.SendToTarget(lo, sid)
	return quickfix.UnregisterSession(sid)
}

// ResetSeq resets a broker's session sequence numbers to 1.
func (fa *FixAcceptor) ResetSeq(broker string) error {
	sid, ok := fa.sessionID(broker)
	if !ok {
		return fmt.Errorf("no active session for %s", broker)
	}
	return quickfix.ResetSession(sid)
}

// ActiveSessions returns the currently-connected brokers with their logon time.
func (fa *FixAcceptor) ActiveSessions() map[string]time.Time {
	fa.sessMu.Lock()
	defer fa.sessMu.Unlock()
	out := map[string]time.Time{}
	for k, v := range fa.sessions {
		out[k] = v
	}
	return out
}

// RecentEvents returns up to n newest session events (newest first).
func (fa *FixAcceptor) RecentEvents(n int) []SessEvent {
	fa.sessMu.Lock()
	defer fa.sessMu.Unlock()
	out := []SessEvent{}
	for i := len(fa.events) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, fa.events[i])
	}
	return out
}

func StartFixAcceptor(eng *Engine, port, senderCompID string) (*FixAcceptor, error) {
	// FIX version: default FIXT.1.1 / FIX 5.0 SP2 (ApplVerID 9) to match DSE —
	// one protocol everywhere. Overridable (ME_FIX_BEGIN=FIX.4.4) for rollback.
	begin := envOr("ME_FIX_BEGIN", "FIXT.1.1")
	applLine := ""
	if begin == "FIXT.1.1" {
		applLine = "DefaultApplVerID=" + envOr("ME_FIX_APPLVER", "9") + "\n"
	}
	// Strict FIX 5.0 SP2 dictionary validation (default on for FIXT.1.1). Off →
	// no message validation (ME_FIX_DD=off). Unknown-field/order checks stay
	// relaxed so broker-specific tags (parties, etc.) don't hard-fail.
	dd := "UseDataDictionary=N\n"
	if begin == "FIXT.1.1" && !strings.EqualFold(envOr("ME_FIX_DD", "on"), "off") {
		spec := specDir()
		dd = "UseDataDictionary=Y\nTransportDataDictionary=" + spec + "/FIXT11.xml\nAppDataDictionary=" + spec + "/FIX50SP2.xml\n"
	}
	cfg := fmt.Sprintf(`[DEFAULT]
ConnectionType=acceptor
SocketAcceptPort=%s
ReconnectInterval=5
FileLogPath=/tmp
StartTime=00:00:00
EndTime=00:00:00
%s%sValidateFieldsOutOfOrder=N
ValidateUserDefinedFields=N
ValidateIncomingMessage=Y
ResetOnLogon=Y
ResetOnLogout=Y
ResetOnDisconnect=Y
DynamicSessions=Y

[SESSION]
BeginString=%s
SenderCompID=%s
TargetCompID=DSE_BROKER
`, port, dd, applLine, begin, senderCompID)
	settings, err := quickfix.ParseSettings(strings.NewReader(cfg))
	if err != nil {
		return nil, err
	}
	meFixAuth = loadFixAuth()
	initMsgLog()
	fa := &FixAcceptor{eng: eng, auth: meFixAuth,
		sessions: map[string]time.Time{}, sids: map[string]quickfix.SessionID{}, seqs: map[string]*seqPair{}}
	meFixAcceptor = fa
	acc, err := quickfix.NewAcceptor(fa, quickfix.NewMemoryStoreFactory(), settings, msgLogFactory{store: meMsgLog})
	if err != nil {
		return nil, err
	}
	fa.acc = acc
	if err := acc.Start(); err != nil {
		return nil, err
	}
	log.Printf("[FIX] acceptor listening on :%s  %s SenderCompID=%s (DynamicSessions)", port, begin, senderCompID)
	return fa, nil
}

func (fa *FixAcceptor) Stop() {
	if fa.acc != nil {
		fa.acc.Stop()
	}
}

// --- quickfix.Application ---
func (fa *FixAcceptor) OnCreate(sid quickfix.SessionID) {}
func (fa *FixAcceptor) OnLogon(sid quickfix.SessionID) {
	log.Printf("[FIX] LOGON broker=%s", sid.TargetCompID)
	fa.sessMu.Lock()
	fa.sessions[sid.TargetCompID] = time.Now()
	fa.sids[sid.TargetCompID] = sid
	// note: don't reset seqs here — trackSeq already captured the logon's seq
	// numbers from FromAdmin/ToAdmin; OnLogout clears them.
	fa.sessMu.Unlock()
	fa.record(sid.TargetCompID, "LOGON", "session established")
}
func (fa *FixAcceptor) OnLogout(sid quickfix.SessionID) {
	log.Printf("[FIX] LOGOUT broker=%s", sid.TargetCompID)
	fa.sessMu.Lock()
	delete(fa.sessions, sid.TargetCompID)
	delete(fa.sids, sid.TargetCompID)
	delete(fa.seqs, sid.TargetCompID)
	fa.sessMu.Unlock()
	fa.record(sid.TargetCompID, "LOGOUT", "session ended")
}
func (fa *FixAcceptor) ToAdmin(m *quickfix.Message, sid quickfix.SessionID) {
	fa.trackSeq(sid.TargetCompID, m, true)
}

// FromAdmin authenticates inbound Logons (35=A) against the broker allowlist.
// Returning a RejectLogon refuses the session; all other admin messages pass.
func (fa *FixAcceptor) FromAdmin(m *quickfix.Message, sid quickfix.SessionID) quickfix.MessageRejectError {
	fa.trackSeq(sid.TargetCompID, m, false)
	if mt, _ := m.Header.GetString(tMsgType); mt == "A" {
		user, _ := m.Body.GetString(tUsername)
		pass, _ := m.Body.GetString(tPassword)
		if reason := fa.auth.authorize(sid.TargetCompID, user, pass); reason != "" {
			log.Printf("[FIX] logon REJECTED broker=%s: %s", sid.TargetCompID, reason)
			fa.record(sid.TargetCompID, "REJECT", reason)
			return quickfix.RejectLogon{Text: reason}
		}
	}
	return nil
}
func (fa *FixAcceptor) ToApp(m *quickfix.Message, sid quickfix.SessionID) error {
	fa.trackSeq(sid.TargetCompID, m, true)
	return nil
}

func (fa *FixAcceptor) FromApp(m *quickfix.Message, sid quickfix.SessionID) quickfix.MessageRejectError {
	fa.trackSeq(sid.TargetCompID, m, false)
	mt, err := m.Header.GetString(tMsgType)
	if err != nil {
		return nil
	}
	switch mt {
	case "D": // NewOrderSingle
		fa.handleNewOrder(m, sid)
	case "F": // OrderCancelRequest
		fa.handleCancel(m, sid)
	case "G": // OrderCancelReplaceRequest
		fa.handleReplace(m, sid)
	}
	return nil
}

func (fa *FixAcceptor) getStr(m *quickfix.Message, t quickfix.Tag) string {
	v, _ := m.Body.GetString(t)
	return v
}

// validateNew screens a NewOrderSingle before it reaches the matching engine and
// returns the reject reason, or "" to accept. An unlisted symbol is auto-listed
// when ME_AUTO_LIST is on (see instruments.go) rather than rejected: the OMS's
// security master is routinely ahead of the ME's.
func (fa *FixAcceptor) validateNew(symbol string, qty float64, typ OrdType, px, stopPx float64) string {
	switch {
	case symbol == "":
		return "missing Symbol (tag 55)"
	case !validSymbol(symbol):
		return fmt.Sprintf("invalid symbol %q (tag 55): expected an exchange ticker", symbol)
	case qty <= 0:
		return fmt.Sprintf("invalid OrderQty (tag 38): %g — quantity must be greater than zero", qty)
	case qty != math.Trunc(qty):
		return fmt.Sprintf("invalid OrderQty (tag 38): %g — the exchange trades whole shares only", qty)
	case (typ == StopMkt || typ == StopLim) && stopPx <= 0:
		return "stop order with no StopPx (tag 99)"
	}
	if fa.eng.listed(symbol) {
		return ""
	}
	seed := px
	if seed <= 0 {
		seed = stopPx
	}
	if fa.eng.autoList(symbol, seed) {
		return ""
	}
	return fmt.Sprintf("unknown symbol %s: not in this engine's instrument universe (%s)", symbol, universeSource())
}

func (fa *FixAcceptor) handleNewOrder(m *quickfix.Message, sid quickfix.SessionID) {
	t0 := time.Now()
	defer func() { meFixLatency.rec(time.Since(t0).Nanoseconds()) }()
	clOrdID := fa.getStr(m, tClOrdID)
	symbol := fa.getStr(m, tSymbol)
	sideStr := fa.getStr(m, tSide)
	qty, _ := strconv.ParseFloat(fa.getStr(m, tOrderQty), 64)
	px, _ := strconv.ParseFloat(fa.getStr(m, tPrice), 64)
	stopPx, _ := strconv.ParseFloat(fa.getStr(m, tStopPx), 64)
	display, _ := strconv.ParseFloat(fa.getStr(m, tMaxFloor), 64)
	ordType := fa.getStr(m, tOrdType)

	side := Buy
	if sideStr == "2" {
		side = Sell
	}
	// FIX OrdType (40): 1=Market 2=Limit 3=Stop 4=Stop-Limit.
	var typ OrdType
	switch ordType {
	case "1":
		typ = Market
	case "3":
		typ = StopMkt
	case "4":
		typ = StopLim
	default:
		typ = Limit
		if px <= 0 {
			typ = Market
		}
	}
	// Reject with a cause the OMS can act on. A blank Text(58) here is what made
	// every one of these arrive as an unexplained "rejected by simulator ME".
	if reason := fa.validateNew(symbol, qty, typ, px, stopPx); reason != "" {
		fa.send(sid, &ExecReport{ClOrdID: clOrdID, Symbol: symbol, Side: side,
			Status: "REJECTED", Reason: reason, OrderQty: int64(qty), Price: px})
		return
	}
	o := fa.eng.newOrder(symbol, side, typ, px, int64(qty), "FIX:"+sid.TargetCompID)
	o.ClOrdID = clOrdID
	o.StopPx = stopPx
	o.Display = int64(display)
	o.TIF = mapTIF(fa.getStr(m, tTimeInForce))
	if o.TIF == GTD {
		o.Expire = parseExpire(fa.getStr(m, tExpireTime), fa.getStr(m, tExpireDate))
	}
	// Async follow-ups (e.g. a stop triggering later) reach the broker here.
	o.notify = func(rep *ExecReport) { fa.send(sid, rep) }
	fa.send(sid, fa.eng.SubmitAndWait(o))
}

// handleCancel processes an OrderCancelRequest (35=F): OrigClOrdID identifies the
// resting order to pull; on success an ExecutionReport(Canceled), else a reject.
func (fa *FixAcceptor) handleCancel(m *quickfix.Message, sid quickfix.SessionID) {
	orig := fa.getStr(m, tOrigClOrdID)
	cl := fa.getStr(m, tClOrdID)
	rep := fa.eng.Cancel(orig)
	rep.ClOrdID = cl
	if rep.Status == "REJECT_CXL" {
		fa.sendCxlReject(sid, cl, orig, "1")
		return
	}
	fa.send(sid, rep)
}

// handleReplace processes an OrderCancelReplaceRequest (35=G): amend qty (38)
// and/or price (44) of the order named by OrigClOrdID.
func (fa *FixAcceptor) handleReplace(m *quickfix.Message, sid quickfix.SessionID) {
	orig := fa.getStr(m, tOrigClOrdID)
	cl := fa.getStr(m, tClOrdID)
	newQty, _ := strconv.ParseFloat(fa.getStr(m, tOrderQty), 64)
	newPx, _ := strconv.ParseFloat(fa.getStr(m, tPrice), 64)
	rep := fa.eng.Replace(orig, cl, int64(newQty), newPx)
	if rep.Status == "REJECT_CXL" {
		fa.sendCxlReject(sid, cl, orig, "2")
		return
	}
	fa.send(sid, rep)
}

// sendCxlReject emits an OrderCancelReject (35=9) for an unknown/undone order.
// respTo: 1=cancel request, 2=cancel/replace request.
func (fa *FixAcceptor) sendCxlReject(sid quickfix.SessionID, cl, orig, respTo string) {
	m := quickfix.NewMessage()
	m.Header.SetString(tMsgType, "9")
	m.Body.SetString(tClOrdID, cl)
	m.Body.SetString(tOrigClOrdID, orig)
	m.Body.SetString(tOrdStatus, "8") // Rejected
	m.Body.SetString(tCxlRejRespTo, respTo)
	m.Body.SetString(tCxlRejReason, "1") // Unknown order
	m.Body.SetString(tText, "unknown or already-done order")
	if err := quickfix.SendToTarget(m, sid); err != nil {
		log.Printf("[FIX] send cxl-reject failed: %v", err)
	}
}

// mapTIF maps FIX tag 59 (TimeInForce) to the engine's TIF. Empty/unknown → Day
// (keeps orders with no tag 59 behaving exactly as before).
func mapTIF(v string) TIF {
	switch v {
	case "1":
		return GTC
	case "3":
		return IOC
	case "4":
		return FOK
	case "6":
		return GTD
	default: // "0" or empty
		return Day
	}
}

// parseExpire reads GTD expiry from ExpireTime (tag 126, a UTC timestamp) or,
// failing that, ExpireDate (tag 432, a date → end of that day). Zero = none.
func parseExpire(t, d string) time.Time {
	if t != "" {
		for _, layout := range []string{"20060102-15:04:05.000", "20060102-15:04:05"} {
			if ts, err := time.Parse(layout, t); err == nil {
				return ts.UTC()
			}
		}
	}
	if d != "" {
		if ts, err := time.Parse("20060102", d); err == nil {
			return ts.Add(24*time.Hour - time.Second).UTC()
		}
	}
	return time.Time{}
}

func (fa *FixAcceptor) send(sid quickfix.SessionID, rep *ExecReport) {
	fa.execSeq++
	m := quickfix.NewMessage()
	m.Header.SetString(tMsgType, "8") // ExecutionReport
	m.Body.SetString(tOrderID, strconv.FormatUint(rep.OrderID, 10))
	m.Body.SetString(tClOrdID, rep.ClOrdID)
	m.Body.SetString(tExecID, strconv.FormatUint(fa.execSeq, 10))
	execType, ordStatus := "0", "0"
	switch rep.Status {
	case "FILLED":
		execType, ordStatus = "TRADE", "2"
	case "PARTIALLY_FILLED":
		execType, ordStatus = "TRADE", "1"
	case "NEW":
		execType, ordStatus = "0", "0"
	case "PENDING":
		execType, ordStatus = "A", "A" // PendingNew — a stop accepted but parked
	case "REPLACED":
		execType, ordStatus = "5", "0" // Replaced, still working
	case "CANCELED":
		execType, ordStatus = "4", "4" // Canceled (IOC/FOK remainder, cancel/replace)
	case "REJECTED":
		execType, ordStatus = "8", "8"
	}
	m.Body.SetString(tExecType, execType)
	m.Body.SetString(tOrdStatus, ordStatus)
	m.Body.SetString(tSymbol, rep.Symbol)
	sv := "1"
	if rep.Side == Sell {
		sv = "2"
	}
	m.Body.SetString(tSide, sv)
	m.Body.SetString(tLeavesQty, strconv.FormatInt(rep.LeavesQty, 10))
	m.Body.SetString(tCumQty, strconv.FormatInt(rep.FilledQty, 10))
	ordQty := rep.OrderQty // tag 38 OrderQty — what the OMS asked for
	if ordQty <= 0 {
		ordQty = rep.FilledQty + rep.LeavesQty
	}
	m.Body.SetString(tOrderQty, strconv.FormatInt(ordQty, 10))
	m.Body.SetString(tAvgPx, strconv.FormatFloat(rep.AvgPx, 'f', 2, 64))
	if rep.Price > 0 {
		m.Body.SetString(tPrice, strconv.FormatFloat(rep.Price, 'f', 2, 64)) // tag 44 limit price -> blotter
	}
	if rep.LastQty > 0 {
		m.Body.SetString(tLastPx, strconv.FormatFloat(rep.LastPx, 'f', 2, 64))
		m.Body.SetString(tLastQty, strconv.FormatInt(rep.LastQty, 10))
	}
	if rep.Status == "REJECTED" {
		txt := rep.Reason
		if txt == "" {
			txt = "rejected by simulator ME"
		}
		m.Body.SetString(tText, txt)
	}
	// FIX enrichment — echo the DSE market identity so the OMS side can key on
	// ISIN / exchange / board without a separate security master.
	m.Body.SetString(tSecExchange, "XDHA")
	if rep.ISIN != "" {
		m.Body.SetString(tSecurityID, rep.ISIN)
		m.Body.SetString(tSecIDSource, "4") // 4 = ISIN
	}
	if rep.Board != "" {
		m.Body.SetString(tMarketSeg, rep.Board)
	}
	m.Body.SetString(tTransactTime, time.Now().UTC().Format("20060102-15:04:05.000"))
	if err := quickfix.SendToTarget(m, sid); err != nil {
		log.Printf("[FIX] send exec report failed: %v", err)
	}
}
