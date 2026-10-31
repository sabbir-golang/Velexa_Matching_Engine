// Package simulator sends test traffic to the engine: market bots that keep
// orders and trades flowing through a broker's OMS (omsbots.go), so they show
// in its blotter, and a benchmark that measures the FIX gateway's throughput
// and latency.
//
// The benchmark logs on to the engine's own FIX gateway as SIMBENCH<n>, with
// logons that exist only in memory; SIMBOT<n> was the old engine-only market
// bots', whose leftover orders Clean Data still cancels. Their orders are
// ordinary FIX orders, so they trade, move prices and reach the ITCH feed like
// any other; the SenderCompIDs mark them as test traffic, and Clean takes them
// out of the book and the order report again. Matching is not changed in any
// way.
package simulator

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/database/orderlog"
	"velexa-sim-me/internal/engine"
)

// Sim runs the market bots and the benchmark. One per engine.
type Sim struct {
	eng    *engine.Engine
	addr   string // the FIX gateway, 127.0.0.1:<FIX_PORT>
	target string // the engine's CompID (FIX_SENDER)
	begin  string // BeginString (FIXT.1.1)
	orders *orderlog.Recorder
	run    string // short id of this engine run, part of every ClOrdID
	seq    atomic.Uint64

	op      sync.Mutex // one start, stop or clean at a time
	mu      sync.Mutex
	oms     *omsRun   // the market bots; nil when signed out
	bench   *benchRun // the running or last benchmark
	tape    []TapeRow // newest last
	rejects map[string]int
}

// New prepares the simulator; nothing runs until Start or StartBench.
func New(eng *engine.Engine, fixAddr, target, begin string, orders *orderlog.Recorder) *Sim {
	return &Sim{eng: eng, addr: fixAddr, target: target, begin: begin, orders: orders,
		run: strconv.FormatInt(time.Now().Unix()%46656, 36), rejects: map[string]int{}}
}

func (s *Sim) clOrdID(kind string) string {
	return kind + s.run + "-" + strconv.FormatUint(s.seq.Add(1), 10)
}

// ── one bot session ─────────────────────────────────────────────────────────

type working struct {
	sym, side string
	px        float64
	leaves    int64
	at        time.Time
}

// counters are a run's totals; reports update them on the reader goroutines.
type counters struct {
	Sent, Accepted, Rejected, Trades, Cancelled atomic.Int64
}

// bot is one logged-on FIX session.
type bot struct {
	comp string
	c    *client
	mu   sync.Mutex
	work map[string]*working // ClOrdID → working order
}

func (b *bot) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.work)
}

// hooks are what a run wants to hear about each report of its bots.
type hooks struct {
	stats  *counters
	trades *tradeIDs
	tapeOf func() bool // sample this report for the tape?
	onAck  func(cl string, rejected bool)
	onFill func(cl string)
}

// tradeIDs counts each trade once although both sides get a report.
type tradeIDs struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func (t *tradeIDs) first(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.seen == nil || len(t.seen) > 200000 {
		t.seen = map[string]struct{}{}
	}
	if _, ok := t.seen[id]; ok {
		return false
	}
	t.seen[id] = struct{}{}
	return true
}

func randPass() string {
	b := make([]byte, 12)
	rand.Read(b)
	return "SIM-" + hex.EncodeToString(b)
}

// connect logs comp on with a logon that exists only for this session.
func (s *Sim) connect(comp string, h hooks) (*bot, error) {
	pass := randPass()
	brokers.Fix.AddTemp(comp, pass)
	b := &bot{comp: comp, work: map[string]*working{}}
	c, err := dialFIX(s.addr, s.begin, s.target, comp, pass, func(m fixMsg) { s.report(b, m, h) })
	if err != nil {
		brokers.Fix.RemoveTemp(comp)
		return nil, err
	}
	b.c = c
	return b, nil
}

// connectAll logs the sessions on in parallel (each logon takes the gateway
// about a second). On a failure the ones that made it are logged out again.
func (s *Sim) connectAll(comps []string, h hooks) ([]*bot, error) {
	bots := make([]*bot, len(comps))
	errs := make([]error, len(comps))
	var wg sync.WaitGroup
	for i, comp := range comps {
		wg.Add(1)
		go func(i int, comp string) {
			defer wg.Done()
			bots[i], errs[i] = s.connect(comp, h)
		}(i, comp)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			logoutAll(bots)
			return nil, err
		}
	}
	return bots, nil
}

// report applies one message from the engine to the bot's working orders and
// the run's counters.
func (s *Sim) report(b *bot, m fixMsg, h hooks) {
	cl := m[11]
	switch m[35] {
	case "8":
	case "9": // cancel rejected: the order is already gone
		b.mu.Lock()
		delete(b.work, m[41])
		b.mu.Unlock()
		return
	default: // session-level reject (35=3) or business reject (35=j)
		h.stats.Rejected.Add(1)
		s.noteReject(m[58])
		return
	}
	leaves, _ := strconv.ParseInt(m[151], 10, 64)
	b.mu.Lock()
	switch m[150] {
	case "0":
		px, _ := strconv.ParseFloat(m[44], 64)
		if leaves > 0 {
			b.work[cl] = &working{sym: m[55], side: m[54], px: px, leaves: leaves, at: time.Now()}
		}
	case "F":
		if w := b.work[cl]; w != nil {
			w.leaves = leaves
		}
		if m[39] == "2" || leaves == 0 {
			delete(b.work, cl)
		}
	case "4", "C":
		delete(b.work, cl)
		delete(b.work, m[41])
	case "8":
		delete(b.work, cl)
	}
	b.mu.Unlock()

	status := ""
	switch m[150] {
	case "0":
		h.stats.Accepted.Add(1)
		if h.onAck != nil {
			h.onAck(cl, false)
		}
		status = "New"
	case "F":
		if h.trades.first(m[880]) {
			h.stats.Trades.Add(1)
		}
		if h.onFill != nil {
			h.onFill(cl)
		}
		status = "Partly filled"
		if m[39] == "2" {
			status = "Filled"
		}
	case "4":
		h.stats.Cancelled.Add(1)
		status = "Cancelled"
	case "C":
		status = "Expired"
	case "8":
		h.stats.Rejected.Add(1)
		if h.onAck != nil {
			h.onAck(cl, true)
		}
		s.noteReject(m[58])
		status = "Rejected"
	default:
		return
	}
	if status == "" || (h.tapeOf != nil && !h.tapeOf() && status != "Rejected") {
		return
	}
	row := TapeRow{Time: time.Now().In(engine.Dhaka).Format("15:04:05.000"), Bot: b.comp, Symbol: m[55],
		Side: map[string]string{"1": "Buy", "2": "Sell"}[m[54]], Status: status, Note: m[58]}
	row.Qty, _ = strconv.ParseInt(m[38], 10, 64)
	row.Price, _ = strconv.ParseFloat(m[44], 64)
	if m[150] == "F" {
		row.Qty, _ = strconv.ParseInt(m[32], 10, 64)
		row.Price, _ = strconv.ParseFloat(m[31], 64)
	}
	s.mu.Lock()
	s.tape = append(s.tape, row)
	if len(s.tape) > 120 {
		s.tape = append([]TapeRow(nil), s.tape[len(s.tape)-80:]...)
	}
	s.mu.Unlock()
}

func (s *Sim) noteReject(reason string) {
	if reason == "" {
		reason = "(no reason given)"
	}
	if len(reason) > 120 {
		reason = reason[:120]
	}
	s.mu.Lock()
	if len(s.rejects) < 50 || s.rejects[reason] > 0 {
		s.rejects[reason]++
	}
	s.mu.Unlock()
}

// cancelAll cancels the bots' working orders over FIX, also those whose
// acknowledgement arrives meanwhile, and waits up to wait for the engine to
// confirm.
func (s *Sim) cancelAll(bots []*bot, wait time.Duration) {
	asked := map[string]bool{}
	end := time.Now().Add(wait)
	for {
		left := 0
		for _, b := range bots {
			if b == nil || !b.c.alive() {
				continue
			}
			b.mu.Lock()
			type one struct {
				cl string
				w  working
			}
			var list []one
			for cl, w := range b.work {
				list = append(list, one{cl, *w})
			}
			b.mu.Unlock()
			left += len(list)
			for _, o := range list {
				if !asked[o.cl] {
					asked[o.cl] = true
					b.c.cancel(s.clOrdID("X"), o.cl, o.w.sym, o.w.side, o.w.leaves)
				}
			}
		}
		if left == 0 || time.Now().After(end) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// actorPrefix matches the engine actors of one kind of simulator session.
func actorPrefix(prefix string) func(string) bool {
	return func(actor string) bool { return strings.HasPrefix(actor, "FIX:"+prefix) }
}

func logoutAll(bots []*bot) {
	var wg sync.WaitGroup
	for _, b := range bots {
		if b == nil {
			continue
		}
		wg.Add(1)
		go func(b *bot) {
			defer wg.Done()
			b.c.logout()
			brokers.Fix.RemoveTemp(b.comp)
		}(b)
	}
	wg.Wait()
}

// ── clean-up ─────────────────────────────────────────────────────────────────

// simActor reports whether an engine actor is one of the simulator's sessions.
func simActor(actor string) bool {
	return strings.HasPrefix(actor, "FIX:") && brokers.IsSimulator(strings.TrimPrefix(actor, "FIX:"))
}

// Sweep cancels every simulator order still in the book and returns how many
// it cancelled.
func (s *Sim) Sweep() int { return s.sweep(simActor) }

// SweepBench cancels the benchmark orders still in the book (left from before
// a restart). The market bots' orders stay: Stop keeps them on purpose.
func (s *Sim) SweepBench() int { return s.sweep(actorPrefix(brokers.SimBenchPrefix)) }

// sweep cancels, in the engine, the resting orders whose actor match accepts.
func (s *Sim) sweep(match func(actor string) bool) int {
	snap := s.eng.TakeSnapshot(true)
	s.eng.MarkDirty() // the copy above must not keep the market saver from saving
	n := 0
	for _, o := range append(snap.Orders, snap.Stops...) {
		if !match(o.Actor) || o.ClOrdID == "" {
			continue
		}
		if rep := s.eng.CancelAs(o.ClOrdID, o.ClOrdID+"-CLN", o.Actor); rep != nil && rep.Status == "CANCELED" {
			n++
		}
	}
	return n
}

// CleanResult says what Clean removed.
type CleanResult struct {
	Cancelled  int `json:"cancelled"`   // simulator orders taken out of the book
	ReportRows int `json:"report_rows"` // today's rows removed from the order report
}

// Clean stops the market bots and the benchmark, takes every simulator order
// out of the book and removes the simulator's rows from the order report (all
// days). Trades stay: they happened, and the prices, volumes and trade log
// follow from them. The market bots' orders are cancelled through the OMS and
// the bots signed out; being the broker's own orders, they stay in the report.
func (s *Sim) Clean() (CleanResult, error) {
	s.op.Lock()
	defer s.op.Unlock()
	s.mu.Lock()
	r := s.oms
	s.mu.Unlock()
	if r != nil {
		s.releaseOMS(r, true)
	}
	s.stopBench()
	res := CleanResult{Cancelled: s.Sweep()}
	if s.orders != nil {
		n, err := s.orders.Forget("FIX:"+brokers.SimBotPrefix, "FIX:"+brokers.SimBenchPrefix)
		res.ReportRows = n
		if err != nil {
			return res, fmt.Errorf("the order report could not be cleaned: %w", err)
		}
	}
	s.mu.Lock()
	s.tape, s.rejects = nil, map[string]int{}
	s.mu.Unlock()
	log.Printf("[sim] test data cleaned: %d orders cancelled, %d order-report rows removed", res.Cancelled, res.ReportRows)
	return res, nil
}

// ── status ───────────────────────────────────────────────────────────────────

// TapeRow is one line of the live order tape.
type TapeRow struct {
	Time   string  `json:"time"`
	Bot    string  `json:"bot"`
	Symbol string  `json:"symbol"`
	Side   string  `json:"side"`
	Qty    int64   `json:"qty"`
	Price  float64 `json:"price"`
	Status string  `json:"status"`
	Note   string  `json:"note,omitempty"`
}

// Status is everything the Simulator page shows.
func (s *Sim) Status() map[string]any {
	s.mu.Lock()
	tape := make([]TapeRow, 0, 40)
	for i := len(s.tape) - 1; i >= 0 && len(tape) < 40; i-- {
		tape = append(tape, s.tape[i])
	}
	type rc struct {
		Reason string `json:"reason"`
		Count  int    `json:"count"`
	}
	var rej []rc
	for k, v := range s.rejects {
		rej = append(rej, rc{k, v})
	}
	oms, bench := s.oms, s.bench
	s.mu.Unlock()
	for i := 1; i < len(rej); i++ {
		for j := i; j > 0 && rej[j].Count > rej[j-1].Count; j-- {
			rej[j], rej[j-1] = rej[j-1], rej[j]
		}
	}
	if len(rej) > 6 {
		rej = rej[:6]
	}
	bots := oms.status()
	bots["leftover"] = s.botOrders() // the old engine-only bots' orders, for Clean Data
	bots["orphans"] = s.omsOrphans() // bot orders no run follows, for Clean Data
	bots["saved_login"] = savedLoginStatus()
	return map[string]any{"ok": true, "phase": s.eng.CurrentPhase(), "bots": bots, "bench": bench.status(),
		"tape": tape, "rejects": rej}
}

func statsMap(c *counters) map[string]int64 {
	return map[string]int64{"sent": c.Sent.Load(), "accepted": c.Accepted.Load(), "rejected": c.Rejected.Load(),
		"trades": c.Trades.Load(), "cancelled": c.Cancelled.Load()}
}
