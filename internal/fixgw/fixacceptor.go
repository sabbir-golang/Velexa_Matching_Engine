// Package fixgw is the FIX 5.0 SP2 acceptor brokers connect to: logon, order entry, cancel/replace and execution reports, plus the searchable FIX message log.
package fixgw

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quickfixgo/quickfix"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/engine"
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
)

// Acceptor is a quickfix acceptor that stands in for the DSE Matching Engine
// gateway: brokers' fix_initiator_go sessions connect here, send NewOrderSingle,
// and receive ExecutionReports produced by the simulator's matching engine.
type Acceptor struct {
	eng     *engine.Engine
	acc     *quickfix.Acceptor
	execSeq uint64 // ExecID counter (atomic)
	auth    *brokers.FixAuth
	out     chan outItem // reports waiting to be sent, in engine order

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

func (fa *Acceptor) trackSeq(broker string, m *quickfix.Message, outbound bool) {
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
func (fa *Acceptor) SeqNums(broker string) (int, int) {
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

// Active is the process-wide acceptor, queried by the admin panel.
var Active *Acceptor

// record appends a diagnostics event (session bookkeeping is done in the
// OnLogon/OnLogout callbacks, which also hold the live SessionID).
func (fa *Acceptor) record(broker, kind, detail string) {
	fa.sessMu.Lock()
	defer fa.sessMu.Unlock()
	fa.events = append(fa.events, SessEvent{Time: time.Now(), Broker: broker, Kind: kind, Detail: detail})
	if len(fa.events) > 200 {
		fa.events = fa.events[len(fa.events)-200:]
	}
}

func (fa *Acceptor) sessionID(broker string) (quickfix.SessionID, bool) {
	fa.sessMu.Lock()
	defer fa.sessMu.Unlock()
	s, ok := fa.sids[strings.ToUpper(strings.TrimSpace(broker))]
	return s, ok
}

// Disconnect force-terminates a broker's live FIX session: it sends a Logout
// (35=5) so the counterparty tears down, then unregisters the session.
func (fa *Acceptor) Disconnect(broker string) error {
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
func (fa *Acceptor) ResetSeq(broker string) error {
	sid, ok := fa.sessionID(broker)
	if !ok {
		return fmt.Errorf("no active session for %s", broker)
	}
	return quickfix.ResetSession(sid)
}

// ActiveSessions returns the currently-connected brokers with their logon time.
func (fa *Acceptor) ActiveSessions() map[string]time.Time {
	fa.sessMu.Lock()
	defer fa.sessMu.Unlock()
	out := map[string]time.Time{}
	for k, v := range fa.sessions {
		out[k] = v
	}
	return out
}

// RecentEvents returns up to n newest session events (newest first).
func (fa *Acceptor) RecentEvents(n int) []SessEvent {
	fa.sessMu.Lock()
	defer fa.sessMu.Unlock()
	out := []SessEvent{}
	for i := len(fa.events) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, fa.events[i])
	}
	return out
}

func Start(eng *engine.Engine, port, senderCompID string) (*Acceptor, error) {
	// FIX version: default FIXT.1.1 / FIX 5.0 SP2 (ApplVerID 9) to match DSE —
	// one protocol everywhere. Overridable (ME_FIX_BEGIN=FIX.4.4) for rollback.
	begin := config.EnvOr("ME_FIX_BEGIN", "FIXT.1.1")
	applLine := ""
	if begin == "FIXT.1.1" {
		applLine = "DefaultApplVerID=" + config.EnvOr("ME_FIX_APPLVER", "9") + "\n"
	}
	// Strict FIX 5.0 SP2 dictionary validation (default on for FIXT.1.1). Off →
	// no message validation (ME_FIX_DD=off). Unknown-field/order checks stay
	// relaxed so broker-specific tags (parties, etc.) don't hard-fail.
	dd := "UseDataDictionary=N\n"
	if begin == "FIXT.1.1" && !strings.EqualFold(config.EnvOr("ME_FIX_DD", "on"), "off") {
		spec := config.SpecDir()
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
	brokers.Fix = brokers.LoadFix()
	initMsgLog()
	fa := &Acceptor{eng: eng, auth: brokers.Fix,
		sessions: map[string]time.Time{}, sids: map[string]quickfix.SessionID{}, seqs: map[string]*seqPair{}}
	Active = fa
	fa.startSender()
	acc, err := quickfix.NewAcceptor(fa, quickfix.NewMemoryStoreFactory(), settings, msgLogFactory{store: MsgLog})
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

func (fa *Acceptor) Stop() {
	if fa.acc != nil {
		fa.acc.Stop()
	}
}

// --- quickfix.Application ---
func (fa *Acceptor) OnCreate(sid quickfix.SessionID) {}

func (fa *Acceptor) OnLogon(sid quickfix.SessionID) {
	log.Printf("[FIX] LOGON broker=%s", sid.TargetCompID)
	fa.sessMu.Lock()
	fa.sessions[sid.TargetCompID] = time.Now()
	fa.sids[sid.TargetCompID] = sid
	// note: don't reset seqs here — trackSeq already captured the logon's seq
	// numbers from FromAdmin/ToAdmin; OnLogout clears them.
	fa.sessMu.Unlock()
	fa.record(sid.TargetCompID, "LOGON", "session established")
}

func (fa *Acceptor) OnLogout(sid quickfix.SessionID) {
	log.Printf("[FIX] LOGOUT broker=%s", sid.TargetCompID)
	fa.sessMu.Lock()
	delete(fa.sessions, sid.TargetCompID)
	delete(fa.sids, sid.TargetCompID)
	delete(fa.seqs, sid.TargetCompID)
	fa.sessMu.Unlock()
	fa.record(sid.TargetCompID, "LOGOUT", "session ended")
}

func (fa *Acceptor) ToAdmin(m *quickfix.Message, sid quickfix.SessionID) {
	fa.trackSeq(sid.TargetCompID, m, true)
}

// FromAdmin authenticates inbound Logons (35=A) against the broker allowlist.
// Returning a RejectLogon refuses the session; all other admin messages pass.
func (fa *Acceptor) FromAdmin(m *quickfix.Message, sid quickfix.SessionID) quickfix.MessageRejectError {
	fa.trackSeq(sid.TargetCompID, m, false)
	if mt, _ := m.Header.GetString(tMsgType); mt == "A" {
		user, _ := m.Body.GetString(tUsername)
		pass, _ := m.Body.GetString(tPassword)
		if reason := fa.auth.Authorize(sid.TargetCompID, user, pass); reason != "" {
			log.Printf("[FIX] logon REJECTED broker=%s: %s", sid.TargetCompID, reason)
			fa.record(sid.TargetCompID, "REJECT", reason)
			return quickfix.RejectLogon{Text: reason}
		}
	}
	return nil
}

func (fa *Acceptor) ToApp(m *quickfix.Message, sid quickfix.SessionID) error {
	fa.trackSeq(sid.TargetCompID, m, true)
	return nil
}

func (fa *Acceptor) FromApp(m *quickfix.Message, sid quickfix.SessionID) quickfix.MessageRejectError {
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
	case "H": // OrderStatusRequest
		fa.handleStatus(m, sid)
	}
	return nil
}

func (fa *Acceptor) getStr(m *quickfix.Message, t quickfix.Tag) string {
	v, _ := m.Body.GetString(t)
	return v
}

// validateNew screens a NewOrderSingle before it reaches the matching engine and
// returns the reject reason, or "" to accept. An unlisted symbol is auto-listed
// when ME_AUTO_LIST is on (see instruments.go) rather than rejected: the OMS's
// security master is routinely ahead of the ME's.
func (fa *Acceptor) validateNew(symbol string, qty float64, typ engine.OrdType, px, stopPx float64) string {
	switch {
	case symbol == "":
		return "missing Symbol (tag 55)"
	case !engine.ValidSymbol(symbol):
		return fmt.Sprintf("invalid symbol %q (tag 55): expected an exchange ticker", symbol)
	case qty <= 0:
		return fmt.Sprintf("invalid OrderQty (tag 38): %g — quantity must be greater than zero", qty)
	case qty != math.Trunc(qty):
		return fmt.Sprintf("invalid OrderQty (tag 38): %g — the exchange trades whole shares only", qty)
	case (typ == engine.StopMkt || typ == engine.StopLim) && stopPx <= 0:
		return "stop order with no StopPx (tag 99)"
	}
	if fa.eng.Listed(symbol) {
		return ""
	}
	seed := px
	if seed <= 0 {
		seed = stopPx
	}
	if fa.eng.AutoList(symbol, seed) {
		return ""
	}
	return fmt.Sprintf("unknown symbol %s: not in this engine's instrument universe (%s)", symbol, engine.UniverseSource())
}

func (fa *Acceptor) handleNewOrder(m *quickfix.Message, sid quickfix.SessionID) {
	clOrdID := fa.getStr(m, tClOrdID)
	symbol := fa.getStr(m, tSymbol)
	sideStr := fa.getStr(m, tSide)
	qty, _ := strconv.ParseFloat(fa.getStr(m, tOrderQty), 64)
	px, _ := strconv.ParseFloat(fa.getStr(m, tPrice), 64)
	stopPx, _ := strconv.ParseFloat(fa.getStr(m, tStopPx), 64)
	display, _ := strconv.ParseFloat(fa.getStr(m, tMaxFloor), 64)
	if display <= 0 { // the OMS sends DisplayQty (1138) rather than MaxFloor (111)
		display, _ = strconv.ParseFloat(fa.getStr(m, tDisplayQty), 64)
	}
	ordType := fa.getStr(m, tOrdType)
	ctx := newOrderCtx(m, sid)

	side := engine.Buy
	if sideStr == "2" {
		side = engine.Sell
	}
	// FIX OrdType (40): 1=Market 2=Limit 3=Stop 4=Stop-Limit Z=market-at-best (DSE).
	var typ engine.OrdType
	reason := ""
	switch ordType {
	case "1":
		typ = engine.Market
	case "2":
		typ = engine.Limit
		if px <= 0 {
			reason = "limit order without a price (tag 44)"
		}
	case "3":
		typ = engine.StopMkt
	case "4":
		typ = engine.StopLim
	case "Z": // DSE market-at-best
		typ = engine.MarketBest
	default:
		reason = fmt.Sprintf("unsupported order type (tag 40): %q", ordType)
	}
	if sideStr != "1" && sideStr != "2" {
		reason = fmt.Sprintf("unsupported side (tag 54): %q — only 1 (buy) and 2 (sell) are traded", sideStr)
	}
	// Reject with a cause the OMS can act on. A blank Text(58) here is what made
	// every one of these arrive as an unexplained "rejected by simulator ME".
	if reason == "" {
		reason = fa.validateNew(symbol, qty, typ, px, stopPx)
	}
	if reason == "" && (typ == engine.Market || typ == engine.MarketBest) {
		reason = fa.marketPriceRule(symbol, px)
	}
	if reason != "" {
		fa.deliver(&engine.ExecReport{ClOrdID: clOrdID, Symbol: symbol, Side: side,
			Status: "REJECTED", Reason: reason, OrderQty: int64(qty), Price: px, Ctx: ctx})
		return
	}
	if typ == engine.Market || typ == engine.MarketBest {
		px = 0 // a priced market order in post-close trades at the close; the engine prices it
	}
	o := fa.eng.NewOrder(symbol, side, typ, px, int64(qty), "FIX:"+sid.TargetCompID)
	o.ClOrdID = clOrdID
	o.StopPx = stopPx
	o.Display = int64(display)
	o.TIF = mapTIF(fa.getStr(m, tTimeInForce))
	if o.TIF == engine.GTD {
		o.Expire = parseExpire(fa.getStr(m, tExpireTime), fa.getStr(m, tExpireDate))
	}
	// Every report about this order — the reply, later fills while it rests,
	// cancel/replace confirmations, a triggered stop, expiry — reaches the broker
	// through Notify, in the order the engine produced them.
	o.Ctx = ctx
	o.Notify = fa.deliver
	fa.eng.SubmitAndWait(o)
}

// marketPriceRule applies the DSE rule for a price on a market order: only in
// the post-closing session, and then it must be the closing price.
func (fa *Acceptor) marketPriceRule(symbol string, px float64) string {
	if fa.eng.CurrentPhase() != "Post-Close" {
		if px > 0 {
			return engine.ReasonPriceNotAllowed
		}
		return ""
	}
	last, ok := fa.eng.LastPrice(symbol)
	if !ok || px <= 0 || engine.Round2(px) != engine.Round2(last) {
		return engine.ReasonNotClosePrice
	}
	return ""
}

// handleCancel processes an OrderCancelRequest (35=F): OrigClOrdID identifies the
// order to pull. The confirmation (150=4) reaches the broker through the order's
// Notify; an unknown, finished or another broker's order gets an OrderCancelReject.
func (fa *Acceptor) handleCancel(m *quickfix.Message, sid quickfix.SessionID) {
	orig := fa.getStr(m, tOrigClOrdID)
	cl := fa.getStr(m, tClOrdID)
	rep := fa.eng.CancelAs(orig, cl, "FIX:"+sid.TargetCompID)
	if rep.Status == "REJECT_CXL" {
		fa.queueMsg(cxlReject(cl, orig, "1"), sid)
	}
}

// handleReplace processes an OrderCancelReplaceRequest (35=G): amend qty (38)
// and/or price (44) of the order named by OrigClOrdID.
func (fa *Acceptor) handleReplace(m *quickfix.Message, sid quickfix.SessionID) {
	orig := fa.getStr(m, tOrigClOrdID)
	cl := fa.getStr(m, tClOrdID)
	newQty, _ := strconv.ParseFloat(fa.getStr(m, tOrderQty), 64)
	newPx, _ := strconv.ParseFloat(fa.getStr(m, tPrice), 64)
	rep := fa.eng.ReplaceAs(orig, cl, "FIX:"+sid.TargetCompID, int64(newQty), newPx)
	if rep.Status == "REJECT_CXL" {
		fa.queueMsg(cxlReject(cl, orig, "2"), sid)
	}
}

// handleStatus answers an OrderStatusRequest (35=H) with an execution report
// 150=I carrying the request's OrdStatusReqID (790).
func (fa *Acceptor) handleStatus(m *quickfix.Message, sid quickfix.SessionID) {
	rep := fa.eng.Status(fa.getStr(m, tClOrdID), "FIX:"+sid.TargetCompID)
	if rep.Status == "REJECT_CXL" || rep.Ctx == nil {
		fa.queueMsg(fa.statusUnknown(m), sid)
		return
	}
	rep.ReqID = fa.getStr(m, tOrdStatusReqID)
	fa.deliver(rep)
}

// mapTIF maps FIX tag 59 (TimeInForce) to the engine's TIF. Empty/unknown → Day
// (keeps orders with no tag 59 behaving exactly as before).
func mapTIF(v string) engine.TIF {
	switch v {
	case "1":
		return engine.GTC
	case "3":
		return engine.IOC
	case "4":
		return engine.FOK
	case "6":
		return engine.GTD
	default: // "0" or empty
		return engine.Day
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
