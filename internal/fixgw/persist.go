package fixgw

// What the gateway keeps across a restart and while a broker is away:
//   - an order's context (session, echoed fields, OrderID) is saved with the
//     order in the engine's snapshot, so a restored order still reports to
//     its broker exactly as before;
//   - reports for a broker that is logged off wait in its outbox, in order,
//     and are sent when it logs on again — also after a restart, because the
//     outbox is saved with the market state. Reports older than outboxMaxAge
//     are dropped.

import (
	"encoding/json"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/quickfixgo/quickfix"

	"velexa-sim-me/internal/engine"
)

const ctxKind = "FIX"

// outboxMaxAge is how long a report waits for its broker.
const outboxMaxAge = 7 * 24 * time.Hour

type savedParty struct {
	ID   string `json:"id"`
	Src  string `json:"src,omitempty"`
	Role string `json:"role,omitempty"`
}

// savedCtx is an orderCtx as saved.
type savedCtx struct {
	Session      quickfix.SessionID `json:"session"`
	Account      string             `json:"account,omitempty"`
	OrdType      string             `json:"ord_type,omitempty"`
	TIF          string             `json:"tif,omitempty"`
	SubType      string             `json:"sub_type,omitempty"`
	Restrictions string             `json:"restrictions,omitempty"`
	SenderSub    string             `json:"sender_sub,omitempty"`
	Parties      []savedParty       `json:"parties,omitempty"`
	OrderID      string             `json:"order_id,omitempty"`
}

func (c *orderCtx) saved() savedCtx {
	s := savedCtx{Session: c.sid, Account: c.account, OrdType: c.ordType, TIF: c.tif, SubType: c.subType,
		Restrictions: c.restrictions, SenderSub: c.senderSub, OrderID: c.getOrderID()}
	for _, p := range c.parties {
		s.Parties = append(s.Parties, savedParty{p.id, p.src, p.role})
	}
	return s
}

func (s savedCtx) ctx() *orderCtx {
	c := &orderCtx{sid: s.Session, account: s.Account, ordType: s.OrdType, tif: s.TIF, subType: s.SubType,
		restrictions: s.Restrictions, senderSub: s.SenderSub, orderID: s.OrderID}
	for _, p := range s.Parties {
		c.parties = append(c.parties, party{p.ID, p.Src, p.Role})
	}
	return c
}

// CtxKind and SaveCtx let the engine save the context with its order.
func (c *orderCtx) CtxKind() string { return ctxKind }

func (c *orderCtx) SaveCtx() (json.RawMessage, error) { return json.Marshal(c.saved()) }

func init() {
	engine.RegisterCtxDecoder(ctxKind, func(raw json.RawMessage) (any, error) {
		var s savedCtx
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		return s.ctx(), nil
	})
}

// AttachRestored makes the orders restored from the saved market report to
// their brokers through this gateway again. Call it once the engine runs.
func (fa *Acceptor) AttachRestored() int { return fa.eng.AttachNotify("FIX:", fa.deliver) }

// ── outbox ──────────────────────────────────────────────────────────────────

// savedRep is an engine report as saved (with the fields its JSON form leaves out).
type savedRep struct {
	engine.ExecReport
	ReqID     string  `json:"req_id,omitempty"`
	CumValue  float64 `json:"cum_value"`
	Aggressor bool    `json:"aggressor,omitempty"`
}

// held is one report waiting for its broker.
type held struct {
	rep  *engine.ExecReport
	ctx  *orderCtx
	at   time.Time
	save json.RawMessage // heldJSON, made when it was queued
}

type heldJSON struct {
	Broker string    `json:"broker"`
	At     time.Time `json:"at"`
	Rep    savedRep  `json:"rep"`
	Ctx    savedCtx  `json:"ctx"`
}

type outbox struct {
	mu      sync.Mutex
	q       map[string][]held // broker → reports, oldest first
	changed bool
}

func (fa *Acceptor) box() *outbox {
	fa.obOnce.Do(func() { fa.ob = &outbox{q: map[string][]held{}} })
	return fa.ob
}

func (fa *Acceptor) loggedOn(broker string) bool {
	_, ok := fa.sessionID(broker)
	return ok
}

// holdFor reports whether a report for broker must wait: it is logged off,
// or older reports for it are still waiting.
func (fa *Acceptor) holdFor(broker string) bool {
	if !fa.loggedOn(broker) {
		return true
	}
	ob := fa.box()
	ob.mu.Lock()
	defer ob.mu.Unlock()
	return len(ob.q[broker]) > 0
}

// hold queues a report for its broker. It runs on the sender goroutine.
func (fa *Acceptor) hold(rep *engine.ExecReport, ctx *orderCtx, at time.Time) {
	if rep.OrderID != 0 {
		ctx.fixOrderID(rep.OrderID, at) // the OrderID the broker would have seen now
	}
	broker := ctx.sid.TargetCompID
	b, err := json.Marshal(heldJSON{Broker: broker, At: at, Ctx: ctx.saved(),
		Rep: savedRep{ExecReport: *rep, ReqID: rep.ReqID, CumValue: rep.CumValue, Aggressor: rep.Aggressor}})
	if err != nil {
		log.Printf("[FIX] report for %s could not be queued: %v", broker, err)
		return
	}
	ob := fa.box()
	ob.mu.Lock()
	first := len(ob.q[broker]) == 0
	ob.q[broker] = append(ob.q[broker], held{rep: rep, ctx: ctx, at: at, save: b})
	ob.changed = true
	ob.mu.Unlock()
	if first {
		log.Printf("[FIX] %s is logged off: its reports are kept and sent when it logs on", broker)
	}
	// It may have logged on between the check and now: send the queue then.
	if fa.loggedOn(broker) {
		fa.drainOutbox(broker)
	}
}

// drainOutbox sends a logged-on broker's waiting reports, oldest first. It
// runs on the sender goroutine (or before it starts).
func (fa *Acceptor) drainOutbox(broker string) {
	ob := fa.box()
	sent, dropped := 0, 0
	for fa.loggedOn(broker) {
		ob.mu.Lock()
		q := ob.q[broker]
		if len(q) == 0 {
			ob.mu.Unlock()
			break
		}
		h := q[0]
		ob.mu.Unlock()
		if time.Since(h.at) > outboxMaxAge {
			dropped++
		} else if !fa.sendReport(h.rep, h.ctx, h.at) {
			break // logged off again: the rest waits for the next logon
		} else {
			sent++
		}
		ob.mu.Lock()
		if q := ob.q[broker]; len(q) > 0 {
			ob.q[broker] = q[1:]
			if len(ob.q[broker]) == 0 {
				delete(ob.q, broker)
			}
		}
		ob.changed = true
		ob.mu.Unlock()
	}
	if sent > 0 {
		log.Printf("[FIX] sent %s the %d report(s) kept while it was logged off", broker, sent)
	}
	if dropped > 0 {
		log.Printf("[FIX] dropped %d report(s) for %s older than %s", dropped, broker, outboxMaxAge)
	}
}

// Queued is the number of reports waiting, per broker.
func (fa *Acceptor) Queued() map[string]int {
	ob := fa.box()
	ob.mu.Lock()
	defer ob.mu.Unlock()
	out := map[string]int{}
	for b, q := range ob.q {
		out[b] = len(q)
	}
	return out
}

// OutboxState returns the outbox to save and whether it changed since the
// last call (the state saver's hook).
func (fa *Acceptor) OutboxState() (json.RawMessage, bool) {
	ob := fa.box()
	ob.mu.Lock()
	defer ob.mu.Unlock()
	if !ob.changed {
		return nil, false
	}
	ob.changed = false
	brokers := make([]string, 0, len(ob.q))
	for b := range ob.q {
		brokers = append(brokers, b)
	}
	sort.Strings(brokers)
	list := []json.RawMessage{}
	for _, b := range brokers {
		for _, h := range ob.q[b] {
			list = append(list, h.save)
		}
	}
	raw, _ := json.Marshal(list)
	return raw, true
}

// RestoreOutbox puts back the reports saved before a restart. Brokers already
// logged on get theirs at once.
func (fa *Acceptor) RestoreOutbox(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return 0, err
	}
	ob := fa.box()
	n, old := 0, 0
	ob.mu.Lock()
	for _, item := range list {
		var h heldJSON
		if err := json.Unmarshal(item, &h); err != nil {
			continue
		}
		if time.Since(h.At) > outboxMaxAge {
			old++
			continue
		}
		rep := h.Rep.ExecReport
		rep.ReqID, rep.CumValue, rep.Aggressor = h.Rep.ReqID, h.Rep.CumValue, h.Rep.Aggressor
		ctx := h.Ctx.ctx()
		rep.Ctx = ctx
		ob.q[h.Broker] = append(ob.q[h.Broker], held{rep: &rep, ctx: ctx, at: h.At, save: item})
		n++
	}
	ob.changed = old > 0
	brokers := make([]string, 0, len(ob.q))
	for b := range ob.q {
		brokers = append(brokers, b)
	}
	ob.mu.Unlock()
	if old > 0 {
		log.Printf("[FIX] dropped %d saved report(s) older than %s", old, outboxMaxAge)
	}
	for _, b := range brokers {
		fa.queueDrain(b)
	}
	return n, nil
}

// queueDrain asks the sender to send what waits for broker, after whatever
// it is sending now.
func (fa *Acceptor) queueDrain(broker string) {
	if fa.out == nil {
		fa.drainOutbox(broker)
		return
	}
	fa.out <- outItem{drain: broker}
}
