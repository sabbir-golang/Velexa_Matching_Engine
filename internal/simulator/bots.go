package simulator

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/engine"
)

// bots.go — market bots. Each step picks one instrument and either quotes it
// (a limit order a few ticks from a drifting fair price, which rests) or takes
// it (an IOC order at the best opposite price, which trades). Prices stay well
// inside the daily band and on the instrument's tick; quantities are whole
// lots. In Pre-Open the bots only quote; outside Pre-Open and Continuous
// Trading they wait.

// BotConfig is what the page sends to start the market bots.
type BotConfig struct {
	Rate    int      `json:"rate"`    // orders per second, all bots together (1–500)
	Bots    int      `json:"bots"`    // bot brokers (2–8)
	Symbols []string `json:"symbols"` // empty: Count of the best-known active shares
	Count   int      `json:"count"`   // instruments when Symbols is empty (1–100, default 20)
}

// blueChips are picked first when no symbols are given (those that are listed
// and active), then the busiest other shares.
var blueChips = []string{"GP", "BATBC", "SQURPHARMA", "BEXIMCO", "BRACBANK", "RENATA", "WALTONHIL", "ROBI",
	"LHBL", "BXPHARMA", "OLYMPIC", "ISLAMIBANK", "CITYBANK", "DUTCHBANGL", "MARICO", "UPGDCL", "SUMITPOWER",
	"BSRMLTD", "SINGERBD", "UNILEVERCL"}

// maxWorkingPerSymbol keeps the bots' resting orders in a symbol below this:
// beyond it, the oldest is cancelled instead of quoting again.
const maxWorkingPerSymbol = 8

// bandShare keeps bot prices this far inside the narrowest DSE band (±3.75%).
const bandShare = 0.03

type symState struct {
	sym  string
	tick float64
	lot  int64
	ref  float64 // drifting fair price
}

type botRun struct {
	cfg     BotConfig
	bots    []*bot
	syms    []*symState
	stats   counters
	trades  tradeIDs
	started time.Time
	paused  atomic.Bool
	note    atomic.Value // string: why the bots are idle
	stop    chan struct{}
	done    chan struct{}
	rng     *rand.Rand
}

var errRunning = errors.New("the market bots are already running")

// Start logs the bots on and starts trading.
func (s *Sim) Start(cfg BotConfig) error {
	cfg.Rate = clamp(cfg.Rate, 1, 500, 20)
	cfg.Bots = clamp(cfg.Bots, 2, 8, 4)
	cfg.Count = clamp(cfg.Count, 1, 100, 20)
	s.op.Lock()
	defer s.op.Unlock()
	s.mu.Lock()
	running := s.bots != nil
	s.mu.Unlock()
	if running {
		return errRunning
	}
	r := &botRun{cfg: cfg, started: time.Now(), stop: make(chan struct{}), done: make(chan struct{}),
		rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	r.syms = s.pickSymbols(cfg)
	if len(r.syms) == 0 {
		return errors.New("no active instrument to trade: check the symbols")
	}
	comps := make([]string, cfg.Bots)
	for i := range comps {
		comps[i] = fmt.Sprintf("%s%d", brokers.SimBotPrefix, i+1)
	}
	bots, err := s.connectAll(comps, hooks{stats: &r.stats, trades: &r.trades})
	if err != nil {
		return err
	}
	r.bots = bots
	r.note.Store("")
	s.mu.Lock()
	s.bots = r
	s.mu.Unlock()
	go r.loop(s)
	log.Printf("[sim] market bots started: %d bots, %d orders/s, %d instruments", cfg.Bots, cfg.Rate, len(r.syms))
	return nil
}

// Pause stops sending; the bots' orders stay in the book. Resume continues.
func (s *Sim) Pause(on bool) error {
	s.mu.Lock()
	r := s.bots
	s.mu.Unlock()
	if r == nil {
		return errors.New("the market bots are not running")
	}
	r.paused.Store(on)
	return nil
}

// StopBots stops the bots, cancels their orders and logs them out.
func (s *Sim) StopBots() {
	s.op.Lock()
	defer s.op.Unlock()
	s.stopBots()
}

func (s *Sim) stopBots() {
	s.mu.Lock()
	r := s.bots
	s.mu.Unlock()
	if r == nil {
		return
	}
	close(r.stop)
	<-r.done
	s.cancelAll(r.bots, 3*time.Second)
	logoutAll(r.bots)
	s.sweep(actorPrefix(brokers.SimBotPrefix)) // an order still on its way when the bots stopped
	s.mu.Lock()
	s.bots = nil
	s.mu.Unlock()
	log.Printf("[sim] market bots stopped: %d orders sent, %d trades", r.stats.Sent.Load(), r.stats.Trades.Load())
}

func clamp(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	return max(lo, min(hi, v))
}

func (s *Sim) pickSymbols(cfg BotConfig) []*symState {
	rows := s.eng.MarketRows()
	type cand struct {
		sym    string
		volume int64
	}
	ok := map[string]cand{}
	for _, r := range rows {
		if r.Status == "active" && r.Prev > 0 && r.AssetClass == "EQ" {
			ok[r.Symbol] = cand{r.Symbol, r.Volume}
		}
	}
	var pick []string
	if len(cfg.Symbols) > 0 {
		for _, sym := range cfg.Symbols {
			sym = strings.ToUpper(strings.TrimSpace(sym))
			if _, good := ok[sym]; good {
				pick = append(pick, sym)
			}
		}
	} else {
		seen := map[string]bool{}
		for _, sym := range blueChips {
			if _, good := ok[sym]; good && len(pick) < cfg.Count {
				pick, seen[sym] = append(pick, sym), true
			}
		}
		rest := make([]cand, 0, len(ok))
		for _, c := range ok {
			if !seen[c.sym] {
				rest = append(rest, c)
			}
		}
		sort.Slice(rest, func(i, j int) bool {
			if rest[i].volume != rest[j].volume {
				return rest[i].volume > rest[j].volume
			}
			return rest[i].sym < rest[j].sym
		})
		for _, c := range rest {
			if len(pick) >= cfg.Count {
				break
			}
			pick = append(pick, c.sym)
		}
	}
	out := make([]*symState, 0, len(pick))
	for _, sym := range pick {
		m := engine.MetaFor(sym)
		st := &symState{sym: sym, tick: m.Tick, lot: int64(m.Lot)}
		if st.tick <= 0 {
			st.tick = 0.10
		}
		if st.lot < 1 {
			st.lot = 1
		}
		out = append(out, st)
	}
	return out
}

func (r *botRun) loop(s *Sim) {
	defer close(r.done)
	t := time.NewTicker(time.Second / time.Duration(r.cfg.Rate))
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
		}
		if r.paused.Load() {
			r.note.Store("paused: orders stay in the book")
			continue
		}
		ph := s.eng.CurrentPhase()
		if ph != engine.PhasePreOpen && ph != engine.PhaseOpen {
			r.note.Store("waiting: the market is " + ph + " (bots trade in Pre-Open and Continuous Trading)")
			continue
		}
		alive := r.bots[:0:0]
		for _, b := range r.bots {
			if b.c.alive() {
				alive = append(alive, b)
			}
		}
		if len(alive) == 0 {
			r.note.Store("stopped: every bot session was disconnected")
			continue
		}
		r.note.Store("")
		r.step(s, ph, alive)
	}
}

// market reads one instrument's previous close and best prices.
func (s *Sim) market(sym string) (prev, bid, ask float64) {
	s.eng.Mu.RLock()
	defer s.eng.Mu.RUnlock()
	if md := s.eng.MdOf(sym); md != nil {
		prev, bid, ask = md.Prev, md.Bid, md.Ask
		if prev <= 0 {
			prev = md.Last
		}
	}
	return
}

func onTick(px, tick float64) float64 { return engine.Round2(math.Round(px/tick) * tick) }

func (r *botRun) step(s *Sim, phase string, bots []*bot) {
	rng := r.rng
	st := r.syms[rng.Intn(len(r.syms))]
	prev, bid, ask := s.market(st.sym)
	if prev <= 0 {
		return
	}
	lo := onTick(prev*(1-bandShare), st.tick) + st.tick
	hi := onTick(prev*(1+bandShare), st.tick) - st.tick
	if st.ref <= 0 {
		st.ref = prev
		if bid > 0 && ask > 0 {
			st.ref = (bid + ask) / 2
		}
	}
	// The fair price wanders and is pulled back toward the previous close.
	st.ref += (prev-st.ref)*0.01 + rng.NormFloat64()*st.ref*0.0015
	st.ref = math.Max(lo, math.Min(hi, st.ref))

	// Too many bot orders resting here: cancel the oldest instead.
	var oldest *working
	var oldCl string
	var owner *bot
	n := 0
	for _, b := range bots {
		b.mu.Lock()
		for cl, w := range b.work {
			if w.sym == st.sym {
				n++
				if oldest == nil || w.at.Before(oldest.at) {
					cp := *w
					oldest, oldCl, owner = &cp, cl, b
				}
			}
		}
		b.mu.Unlock()
	}
	if n >= maxWorkingPerSymbol && owner != nil {
		owner.c.cancel(s.clOrdID("X"), oldCl, oldest.sym, oldest.side, oldest.leaves)
		r.stats.Sent.Add(1)
		return
	}

	qty := st.lot * int64(1+rng.Intn(10))
	if st.lot == 1 {
		qty = int64(10 * (1 + rng.Intn(20)))
	}
	side := "1"
	if rng.Intn(2) == 0 {
		side = "2"
	}
	b := bots[rng.Intn(len(bots))]
	take := phase == engine.PhaseOpen && rng.Float64() < 0.45
	var px float64
	tif := "0"
	switch {
	case take && side == "1" && ask > 0:
		px, tif = ask, "3"
	case take && side == "2" && bid > 0:
		px, tif = bid, "3"
	case side == "1":
		px = st.ref - float64(1+rng.Intn(4))*st.tick
	default:
		px = st.ref + float64(1+rng.Intn(4))*st.tick
	}
	px = math.Max(lo, math.Min(hi, onTick(px, st.tick)))
	if err := b.c.order(s.clOrdID("B"), st.sym, side, qty, px, tif); err == nil {
		r.stats.Sent.Add(1)
	}
}

func (r *botRun) status() map[string]any {
	if r == nil {
		return map[string]any{"state": "stopped"}
	}
	state := "running"
	if r.paused.Load() {
		state = "paused"
	}
	select {
	case <-r.stop:
		state = "stopping"
	default:
	}
	var sess []map[string]any
	work := 0
	for _, b := range r.bots {
		n := b.count()
		work += n
		sess = append(sess, map[string]any{"comp": b.comp, "up": b.c.alive(), "working": n})
	}
	syms := make([]string, 0, len(r.syms))
	for _, st := range r.syms {
		syms = append(syms, st.sym)
	}
	note, _ := r.note.Load().(string)
	el := time.Since(r.started).Seconds()
	return map[string]any{"state": state, "note": note, "config": r.cfg, "symbols": syms, "sessions": sess,
		"working": work, "stats": statsMap(&r.stats), "running_for": math.Round(el),
		"orders_per_sec": math.Round(float64(r.stats.Sent.Load())/math.Max(el, 1)*10) / 10}
}
