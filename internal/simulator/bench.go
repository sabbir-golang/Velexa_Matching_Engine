package simulator

import (
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"velexa-sim-me/internal/database/brokers"
)

// bench.go — the benchmark. Two sessions, SIMBENCH1 and SIMBENCH2, send orders
// at a set rate (or as fast as the gateway answers, with at most benchWindow
// orders waiting for their acknowledgement) for a set time. In "trade" mode
// SIMBENCH1 sells, and as soon as the sell is accepted SIMBENCH2 buys the same
// quantity at the same price with an IOC order, so every pair prints a trade
// (the two sessions are separate connections: sent together, the buy could
// arrive first and find nothing to trade); in "orders" mode SIMBENCH2 sends
// IOC buys at the bottom of the band, which are accepted and cancelled without
// trading. Latency is measured from sending an order to its acknowledgement
// (150=0 or 8) and, for the IOC buys, to their first fill.

// BenchConfig is what the page sends to start a benchmark.
type BenchConfig struct {
	Mode    string `json:"mode"`    // "trade" (default) or "orders"
	Rate    int    `json:"rate"`    // orders per second; 0 = as fast as possible
	Seconds int    `json:"seconds"` // 1–300, default 10
	Symbol  string `json:"symbol"`  // default GP
}

const (
	benchWindow  = 1000   // orders sent but not yet acknowledged, at most
	benchSamples = 200000 // latency samples kept per kind
)

type pair struct {
	px  float64
	qty int64
}

type benchRun struct {
	sim    *Sim
	cfg    BenchConfig
	sym    *symState
	bots   []*bot
	stats  counters
	trades tradeIDs
	window chan struct{}
	stop   chan struct{}
	done   chan struct{}
	tapeN  atomic.Int64

	mu      sync.Mutex
	state   string // running, done, stopped
	started time.Time
	ended   time.Time
	sentAt  map[string]time.Time
	takers  map[string]time.Time
	pairs   map[string]pair // sells waiting to be accepted, then bought
	ack     []float64       // milliseconds
	fill    []float64
	series  []int64 // acknowledgements per second
	last    int64
}

// StartBench logs the benchmark sessions on and starts sending.
func (s *Sim) StartBench(cfg BenchConfig) error {
	cfg.Mode = strings.ToLower(strings.TrimSpace(cfg.Mode))
	if cfg.Mode != "orders" {
		cfg.Mode = "trade"
	}
	if cfg.Rate < 0 {
		cfg.Rate = 0
	}
	cfg.Rate = min(cfg.Rate, 100000)
	cfg.Seconds = clamp(cfg.Seconds, 1, 300, 10)
	cfg.Symbol = strings.ToUpper(strings.TrimSpace(cfg.Symbol))
	if cfg.Symbol == "" {
		cfg.Symbol = "GP"
	}
	s.op.Lock()
	defer s.op.Unlock()
	s.mu.Lock()
	running := s.bench != nil && s.bench.running()
	s.mu.Unlock()
	if running {
		return errors.New("a benchmark is already running")
	}
	syms := s.pickSymbols(BotConfig{Symbols: []string{cfg.Symbol}, Count: 1})
	if len(syms) == 0 {
		return fmt.Errorf("%s is not an active listed share", cfg.Symbol)
	}
	r := &benchRun{sim: s, cfg: cfg, sym: syms[0], window: make(chan struct{}, benchWindow), stop: make(chan struct{}),
		done: make(chan struct{}), state: "running", sentAt: map[string]time.Time{}, takers: map[string]time.Time{},
		pairs: map[string]pair{}}
	h := hooks{stats: &r.stats, trades: &r.trades, onAck: r.acked, onFill: r.filled,
		tapeOf: func() bool { return r.tapeN.Add(1)%200 == 1 }}
	bots, err := s.connectAll([]string{brokers.SimBenchPrefix + "1", brokers.SimBenchPrefix + "2"}, h)
	if err != nil {
		return err
	}
	r.bots = bots
	r.started = time.Now()
	s.mu.Lock()
	s.bench = r
	s.mu.Unlock()
	go r.loop(s)
	rate := "as fast as possible"
	if cfg.Rate > 0 {
		rate = fmt.Sprintf("%d orders/s", cfg.Rate)
	}
	log.Printf("[sim] benchmark started: %s mode, %s, %d s, %s", cfg.Mode, rate, cfg.Seconds, cfg.Symbol)
	return nil
}

// StopBench ends a running benchmark early.
func (s *Sim) StopBench() {
	s.op.Lock()
	defer s.op.Unlock()
	s.stopBench()
}

func (s *Sim) stopBench() {
	s.mu.Lock()
	r := s.bench
	s.mu.Unlock()
	if r == nil || !r.running() {
		return
	}
	r.mu.Lock()
	r.state = "stopped"
	r.mu.Unlock()
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	<-r.done
}

func (r *benchRun) running() bool {
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}

func (r *benchRun) acked(cl string, rejected bool) {
	now := time.Now()
	r.mu.Lock()
	t, ok := r.sentAt[cl]
	if ok {
		delete(r.sentAt, cl)
		r.ack = addSample(r.ack, float64(now.Sub(t).Microseconds())/1000)
		r.release()
	}
	p, isSell := r.pairs[cl]
	delete(r.pairs, cl)
	r.mu.Unlock()
	if !isSell {
		return
	}
	if rejected || !r.place(r.bots[1], "1", p.px, p.qty, "3") {
		r.release() // the buy's place in the window
	}
}

// release frees one place in the window.
func (r *benchRun) release() {
	select {
	case <-r.window:
	default:
	}
}

// place sends one order; its place in the window is already taken.
func (r *benchRun) place(b *bot, side string, px float64, qty int64, tif string) bool {
	cl := r.sim.clOrdID("Q")
	t := time.Now()
	r.mu.Lock()
	r.sentAt[cl] = t
	if tif == "3" && r.cfg.Mode == "trade" {
		r.takers[cl] = t
	}
	if side == "2" {
		r.pairs[cl] = pair{px, qty}
	}
	r.mu.Unlock()
	if err := b.c.order(cl, r.sym.sym, side, qty, px, tif); err != nil {
		r.mu.Lock()
		delete(r.sentAt, cl)
		delete(r.takers, cl)
		delete(r.pairs, cl)
		r.mu.Unlock()
		return false
	}
	r.stats.Sent.Add(1)
	return true
}

func (r *benchRun) filled(cl string) {
	now := time.Now()
	r.mu.Lock()
	if t, ok := r.takers[cl]; ok {
		delete(r.takers, cl)
		r.fill = addSample(r.fill, float64(now.Sub(t).Microseconds())/1000)
	}
	r.mu.Unlock()
}

func addSample(s []float64, v float64) []float64 {
	if len(s) < benchSamples {
		return append(s, v)
	}
	s[int(time.Now().UnixNano()%benchSamples)] = v // keep a sample of the whole run
	return s
}

// send starts the next order (in trade mode, the next pair: two orders) when
// the window has room; false when it is full.
func (r *benchRun) send(px, lo float64, qty int64) bool {
	slots := 1
	if r.cfg.Mode == "trade" {
		slots = 2
	}
	for i := 0; i < slots; i++ {
		select {
		case r.window <- struct{}{}:
		default:
			for ; i > 0; i-- {
				r.release()
			}
			return false
		}
	}
	ok := false
	if r.cfg.Mode == "orders" {
		ok = r.place(r.bots[1], "1", lo, qty, "3")
	} else {
		ok = r.place(r.bots[0], "2", px, qty, "0")
	}
	if !ok {
		for i := 0; i < slots; i++ {
			r.release()
		}
	}
	return ok
}

func (r *benchRun) sample() {
	cur := r.stats.Accepted.Load() + r.stats.Rejected.Load()
	r.mu.Lock()
	r.series = append(r.series, cur-r.last)
	if len(r.series) > 300 {
		r.series = r.series[len(r.series)-300:]
	}
	r.last = cur
	r.mu.Unlock()
}

func (r *benchRun) loop(s *Sim) {
	defer close(r.done)
	prev, _, _ := s.market(r.sym.sym)
	st := r.sym
	// Every pair uses the same price and quantity: with many pairs in flight, a
	// buy may take an earlier pair's sell (time priority), and each buy still
	// finds exactly one sell of its size.
	base := onTick(prev, st.tick)
	lo := onTick(prev*(1-bandShare), st.tick) + st.tick
	qty := st.lot
	if st.lot == 1 {
		qty = 100
	}
	end := r.started.Add(time.Duration(r.cfg.Seconds) * time.Second)
	sec := time.NewTicker(time.Second)
	defer sec.Stop()
	var n int64 // orders started
loop:
	for time.Now().Before(end) {
		select {
		case <-r.stop:
			break loop
		case <-sec.C:
			r.sample()
		default:
		}
		target := int64(math.MaxInt64)
		if r.cfg.Rate > 0 {
			target = int64(time.Since(r.started).Seconds()*float64(r.cfg.Rate)) + 1
		}
		for burst := 0; n < target && burst < 2000; burst++ {
			if !r.send(base, lo, qty) {
				break
			}
			if r.cfg.Mode == "trade" {
				n += 2
			} else {
				n++
			}
		}
		time.Sleep(200 * time.Microsecond)
	}
	// Let the last answers arrive, then cancel what rested and log out.
	wait := time.Now().Add(3 * time.Second)
	for len(r.window) > 0 && time.Now().Before(wait) {
		time.Sleep(20 * time.Millisecond)
	}
	r.sample()
	r.mu.Lock()
	r.ended = time.Now()
	r.mu.Unlock()
	s.cancelAll(r.bots, 3*time.Second)
	logoutAll(r.bots)
	s.sweep(actorPrefix(brokers.SimBenchPrefix))
	r.mu.Lock()
	if r.state == "running" {
		r.state = "done"
	}
	r.mu.Unlock()
	log.Printf("[sim] benchmark finished: %d orders sent, %d acknowledged, %d trades", r.stats.Sent.Load(),
		r.stats.Accepted.Load()+r.stats.Rejected.Load(), r.stats.Trades.Load())
}

func pct(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	return math.Round(sorted[max(0, min(i, len(sorted)-1))]*1000) / 1000
}

func latency(s []float64) map[string]any {
	c := append([]float64(nil), s...)
	sort.Float64s(c)
	sum := 0.0
	for _, v := range c {
		sum += v
	}
	avg := 0.0
	if len(c) > 0 {
		avg = math.Round(sum/float64(len(c))*1000) / 1000
	}
	return map[string]any{"n": len(c), "avg": avg, "p50": pct(c, .5), "p95": pct(c, .95), "p99": pct(c, .99), "max": pct(c, 1)}
}

func (r *benchRun) status() map[string]any {
	if r == nil {
		return map[string]any{"state": "idle"}
	}
	r.mu.Lock()
	ack := latency(r.ack)
	fill := latency(r.fill)
	series := append([]int64(nil), r.series...)
	state, started, ended := r.state, r.started, r.ended
	r.mu.Unlock()
	if ended.IsZero() {
		ended = time.Now()
	}
	el := ended.Sub(started).Seconds()
	acks := r.stats.Accepted.Load() + r.stats.Rejected.Load()
	now, peak := int64(0), int64(0)
	if len(series) > 0 {
		now = series[len(series)-1]
	}
	for _, v := range series {
		peak = max(peak, v)
	}
	if state != "running" {
		now = 0
	}
	return map[string]any{"state": state, "config": r.cfg, "stats": statsMap(&r.stats), "elapsed": math.Round(el*10) / 10,
		"throughput_now": now, "throughput_peak": peak, "throughput_avg": math.Round(float64(acks) / math.Max(el, 0.001)),
		"series": series, "latency": map[string]any{"ack": ack, "fill": fill}}
}
