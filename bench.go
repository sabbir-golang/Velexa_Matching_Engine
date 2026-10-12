package main

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ── latency stat: atomic counters + a lock-free ring of ns samples ──
type Stat struct {
	count   int64
	sumNs   int64
	idx     int64
	samples [8192]int64
}

func (s *Stat) rec(ns int64) {
	atomic.AddInt64(&s.count, 1)
	atomic.AddInt64(&s.sumNs, ns)
	i := atomic.AddInt64(&s.idx, 1)
	s.samples[(i-1)&8191] = ns
}

// recBatch records an amortized per-row cost for a batch of n rows taking durNs.
func (s *Stat) recBatch(n, durNs int64) {
	if n <= 0 {
		return
	}
	atomic.AddInt64(&s.count, n)
	atomic.AddInt64(&s.sumNs, durNs)
	per := durNs / n
	i := atomic.AddInt64(&s.idx, 1)
	s.samples[(i-1)&8191] = per
}

func (s *Stat) reset() { atomic.StoreInt64(&s.count, 0); atomic.StoreInt64(&s.sumNs, 0); atomic.StoreInt64(&s.idx, 0) }

func (s *Stat) snap() (count, avgNs, p50, p95, p99 int64) {
	count = atomic.LoadInt64(&s.count)
	sum := atomic.LoadInt64(&s.sumNs)
	if count > 0 {
		avgNs = sum / count
	}
	n := int(atomic.LoadInt64(&s.idx))
	if n > 8192 {
		n = 8192
	}
	if n == 0 {
		return
	}
	buf := make([]int64, n)
	copy(buf, s.samples[:n])
	sort.Slice(buf, func(i, j int) bool { return buf[i] < buf[j] })
	pct := func(p int) int64 {
		k := (p * len(buf)) / 100
		if k >= len(buf) {
			k = len(buf) - 1
		}
		return buf[k]
	}
	return count, avgNs, pct(50), pct(95), pct(99)
}

type BenchConfig struct {
	Rate    int    `json:"rate"`    // target orders/sec; 0 = unlimited (max)
	Total   int    `json:"total"`   // total orders; 0 = run until Stop
	RMS     bool   `json:"rms"`     // run RMS checks
	Persist bool   `json:"persist"` // persist to Postgres
	Side    string `json:"side"`    // "BUY" | "SELL" | "BOTH"/"" (mix)
}

type Bench struct {
	pool    *pgxpool.Pool
	syms    []string
	refPx   map[string]float64
	rms     *RiskEngine
	rng     *rand.Rand
	accts   []string

	books map[string]*Book

	// metrics
	genCnt, rmsPass, rmsRej, matchCnt, tradeCnt, persistCnt int64
	rmsStat, matchStat, persistStat, e2eStat               Stat
	rejReasons                                             sync.Map // reason -> *int64

	orderSeq uint64
	execID   int64 // matcher goroutine only

	// exec tape (sampled, for the UI)
	tapeMu sync.Mutex
	tape   []map[string]any

	running int32
	startNs int64
	endNs   int64
	cfg     BenchConfig
	cancel  context.CancelFunc
}

func NewBench(pool *pgxpool.Pool, eng *Engine) *Bench {
	syms := make([]string, 0, len(eng.instr()))
	ref := map[string]float64{}
	eng.mu.RLock()
	for _, in := range eng.instr() {
		syms = append(syms, in.Symbol)
		p := eng.mdOf(in.Symbol).Last
		if p <= 0 {
			p = in.Prev
		}
		ref[in.Symbol] = p
	}
	eng.mu.RUnlock()
	accts := make([]string, 200)
	for i := range accts {
		accts[i] = fmt.Sprintf("ACC%04d", i)
	}
	return &Bench{
		pool: pool, syms: syms, refPx: ref, rms: NewRiskEngine(ref),
		rng: rand.New(rand.NewSource(7)), accts: accts,
	}
}

func (b *Bench) bumpReason(r string) {
	v, _ := b.rejReasons.LoadOrStore(r, new(int64))
	atomic.AddInt64(v.(*int64), 1)
}

func (b *Bench) seedBooks() {
	b.books = map[string]*Book{}
	for _, s := range b.syms {
		bk := newBook(s)
		ref := b.refPx[s]
		tick := roundTick(ref * 0.001)
		if tick <= 0 {
			tick = 0.1
		}
		// 300 levels each side around ref → deep liquidity so orders match
		for i := 1; i <= 300; i++ {
			bid := roundTick(ref - float64(i)*tick)
			ask := roundTick(ref + float64(i)*tick)
			if bid > 0 {
				bk.rest(&Order{Symbol: s, Side: Buy, Type: Limit, Price: bid, Qty: 500, Actor: "SEED"})
			}
			bk.rest(&Order{Symbol: s, Side: Sell, Type: Limit, Price: ask, Qty: 500, Actor: "SEED"})
		}
		b.books[s] = bk
	}
}

func (b *Bench) genOrder() *benchOrder {
	sym := b.syms[b.rng.Intn(len(b.syms))]
	ref := b.refPx[sym]
	side := Buy
	switch b.cfg.Side {
	case "BUY":
		side = Buy
	case "SELL":
		side = Sell
	default: // BOTH / "" → mixed
		if b.rng.Intn(2) == 0 {
			side = Sell
		}
	}
	typ := Limit
	px := 0.0
	switch r := b.rng.Float64(); {
	case r < 0.30: // 30% marketable → guaranteed match
		typ = Market
	case r < 0.34: // ~4% outside the price band → RMS PRICE_BAND reject
		px = roundTick(ref * (1 + 0.15))
	default: // resting/crossing limits within band
		px = roundTick(ref * (1 + (b.rng.Float64()-0.5)*0.04))
	}
	qty := int64(100 + b.rng.Intn(900))
	if b.rng.Float64() < 0.005 { // ~0.5% oversize → RMS QTY_LIMIT reject
		qty = 2_000_000
	}
	id := atomic.AddUint64(&b.orderSeq, 1)
	o := &Order{ID: id, Symbol: sym, Side: side, Type: typ, Price: px, Qty: qty, OrigQty: qty, Actor: "BENCH", Time: time.Now()}
	return &benchOrder{o: o, acct: b.accts[b.rng.Intn(len(b.accts))], t0: time.Now()}
}

type benchOrder struct {
	o     *Order
	acct  string
	t0    time.Time
	rmsNs int32
}

// match runs the matcher stage (single goroutine): real price-time matching
// against the seeded book, returns the exec rows produced.
func (b *Bench) match(bo *benchOrder) []simExecRow {
	o := bo.o
	bk := b.books[o.Symbol]
	if bk == nil {
		return nil
	}
	var cum int64
	var execs []simExecRow
	for o.Qty > 0 {
		var lv *plevel
		if o.Side == Buy {
			lv = bk.bestAsk()
			if lv == nil || (o.Type == Limit && k2px(lv.px) > o.Price) {
				break
			}
		} else {
			lv = bk.bestBid()
			if lv == nil || (o.Type == Limit && k2px(lv.px) < o.Price) {
				break
			}
		}
		front := lv.orders[0]
		q := o.Qty
		if front.Qty < q {
			q = front.Qty
		}
		px := k2px(lv.px)
		o.Qty -= q
		front.Qty -= q
		lv.qty -= q
		cum += q
		b.execID++
		execs = append(execs, simExecRow{execID: b.execID, orderID: int64(o.ID), symbol: o.Symbol, side: string(o.Side), lastPx: px, lastQty: q, cumQty: cum, status: "FILLED"})
		if front.Qty == 0 {
			lv.popFront()
		}
		if len(lv.orders) == 0 {
			if o.Side == Buy {
				bk.dropBestAsk()
			} else {
				bk.dropBestBid()
			}
		}
	}
	if o.Qty > 0 && o.Type == Limit {
		bk.rest(o) // unmatched remainder replenishes liquidity
	}
	return execs
}

func (b *Bench) pushTape(bo *benchOrder, execs []simExecRow, status string) {
	// sample ~1 in 400 to keep the UI tape light
	if b.rng.Intn(400) != 0 {
		return
	}
	last := 0.0
	var lq int64
	if len(execs) > 0 {
		last = execs[len(execs)-1].lastPx
		lq = execs[len(execs)-1].cumQty
	}
	row := map[string]any{
		"id": bo.o.ID, "sym": bo.o.Symbol, "side": string(bo.o.Side),
		"status": status, "last_px": last, "cum": lq, "t": bo.t0.Format("15:04:05.000"),
	}
	b.tapeMu.Lock()
	b.tape = append([]map[string]any{row}, b.tape...)
	if len(b.tape) > 40 {
		b.tape = b.tape[:40]
	}
	b.tapeMu.Unlock()
}

func (b *Bench) Running() bool { return atomic.LoadInt32(&b.running) == 1 }

func (b *Bench) Start(cfg BenchConfig) error {
	if !atomic.CompareAndSwapInt32(&b.running, 0, 1) {
		return fmt.Errorf("benchmark already running")
	}
	b.cfg = cfg
	// reset metrics
	atomic.StoreInt64(&b.genCnt, 0)
	atomic.StoreInt64(&b.rmsPass, 0)
	atomic.StoreInt64(&b.rmsRej, 0)
	atomic.StoreInt64(&b.matchCnt, 0)
	atomic.StoreInt64(&b.tradeCnt, 0)
	atomic.StoreInt64(&b.persistCnt, 0)
	b.rmsStat.reset()
	b.matchStat.reset()
	b.persistStat.reset()
	b.e2eStat.reset()
	b.rejReasons = sync.Map{}
	b.tape = nil
	b.execID = 0
	b.seedBooks()
	if cfg.Persist {
		b.pool.Exec(context.Background(), "TRUNCATE sim_orders, sim_execs")
	}

	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel
	b.startNs = time.Now().UnixNano()
	atomic.StoreInt64(&b.endNs, 0)

	rmsCh := make(chan *benchOrder, 8192)
	matchCh := make(chan *benchOrder, 8192)
	var persist *Persister
	if cfg.Persist {
		persist = newPersister(b.pool, &b.persistStat, &b.persistCnt)
		go persist.run()
	}

	// matcher (single goroutine)
	matchDone := make(chan struct{})
	go func() {
		defer close(matchDone)
		for bo := range matchCh {
			t := time.Now()
			execs := b.match(bo)
			mns := time.Since(t).Nanoseconds()
			b.matchStat.rec(mns)
			atomic.AddInt64(&b.matchCnt, 1)
			atomic.AddInt64(&b.tradeCnt, int64(len(execs)))
			b.e2eStat.rec(time.Since(bo.t0).Nanoseconds())
			status := "NEW"
			if len(execs) > 0 {
				status = "FILLED"
			}
			if persist != nil {
				persist.in <- persistItem{ord: simOrderRow{
					id: int64(bo.o.ID), clOrdID: fmt.Sprintf("B%d", bo.o.ID), account: bo.acct,
					symbol: bo.o.Symbol, side: string(bo.o.Side), qty: bo.o.OrigQty, price: bo.o.Price,
					status: status, rmsNs: bo.rmsNs, matchNs: int32(mns),
				}, execs: execs}
			}
			b.pushTape(bo, execs, status)
		}
	}()

	// RMS worker pool
	nw := runtime.NumCPU() / 2
	if nw < 4 {
		nw = 4
	}
	var rmsWg sync.WaitGroup
	for i := 0; i < nw; i++ {
		rmsWg.Add(1)
		go func() {
			defer rmsWg.Done()
			for bo := range rmsCh {
				if cfg.RMS {
					t := time.Now()
					pass, reason := b.rms.Check(bo.o, bo.acct)
					rns := time.Since(t).Nanoseconds()
					b.rmsStat.rec(rns)
					bo.rmsNs = int32(rns)
					if !pass {
						atomic.AddInt64(&b.rmsRej, 1)
						b.bumpReason(reason)
						if persist != nil {
							persist.in <- persistItem{ord: simOrderRow{
								id: int64(bo.o.ID), clOrdID: fmt.Sprintf("B%d", bo.o.ID), account: bo.acct,
								symbol: bo.o.Symbol, side: string(bo.o.Side), qty: bo.o.OrigQty, price: bo.o.Price,
								status: "REJECTED", reason: reason, rmsNs: bo.rmsNs,
							}}
						}
						continue
					}
					atomic.AddInt64(&b.rmsPass, 1)
				}
				matchCh <- bo
			}
		}()
	}

	// generator
	go func() {
		defer func() {
			close(rmsCh)
			rmsWg.Wait()
			close(matchCh)
			<-matchDone
			if persist != nil {
				close(persist.in)
				<-persist.done
			}
			atomic.StoreInt64(&b.endNs, time.Now().UnixNano())
			atomic.StoreInt32(&b.running, 0)
		}()
		emit := func() bool {
			bo := b.genOrder()
			select {
			case rmsCh <- bo:
				atomic.AddInt64(&b.genCnt, 1)
				return true
			case <-ctx.Done():
				return false
			}
		}
		count := 0
		done := func() bool { return cfg.Total != 0 && count >= cfg.Total }
		if cfg.Rate > 0 {
			// millisecond-burst pacing: emit rate/1000 orders every 1ms —
			// reliable at high target rates where a per-order ticker collapses.
			perTick := cfg.Rate / 1000
			if perTick < 1 {
				perTick = 1
			}
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			for !done() {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
				for k := 0; k < perTick && !done(); k++ {
					if !emit() {
						return
					}
					count++
				}
			}
		} else {
			for !done() {
				if !emit() {
					return
				}
				count++
			}
		}
	}()
	return nil
}

func (b *Bench) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
}

func (b *Bench) Stats() map[string]any {
	end := atomic.LoadInt64(&b.endNs)
	if end == 0 {
		end = time.Now().UnixNano()
	}
	elapsed := float64(end-b.startNs) / 1e9
	if elapsed <= 0 {
		elapsed = 1e-6
	}
	rc, ra, r50, r95, r99 := b.rmsStat.snap()
	mc, ma, m50, m95, m99 := b.matchStat.snap()
	pc, pa, p50, p95, p99 := b.persistStat.snap()
	ec, ea, e50, e95, e99 := b.e2eStat.snap()
	trades := atomic.LoadInt64(&b.tradeCnt)
	matched := atomic.LoadInt64(&b.matchCnt)
	reasons := map[string]int64{}
	b.rejReasons.Range(func(k, v any) bool { reasons[k.(string)] = atomic.LoadInt64(v.(*int64)); return true })
	b.tapeMu.Lock()
	tape := make([]map[string]any, len(b.tape))
	copy(tape, b.tape)
	b.tapeMu.Unlock()
	us := func(ns int64) float64 { return float64(ns) / 1000.0 }
	stage := func(c, a, x50, x95, x99 int64) map[string]any {
		return map[string]any{"count": c, "avg_us": us(a), "p50_us": us(x50), "p95_us": us(x95), "p99_us": us(x99)}
	}
	return map[string]any{
		"running":         b.Running(),
		"elapsed_s":       elapsed,
		"cfg":             b.cfg,
		"generated":       atomic.LoadInt64(&b.genCnt),
		"rms_pass":        atomic.LoadInt64(&b.rmsPass),
		"rms_rej":         atomic.LoadInt64(&b.rmsRej),
		"matched":         matched,
		"trades":          trades,
		"persisted":       atomic.LoadInt64(&b.persistCnt),
		"orders_per_sec":  int64(float64(matched) / elapsed),
		"trades_per_sec":  int64(float64(trades) / elapsed),
		"reject_reasons":  reasons,
		"stage_rms":       stage(rc, ra, r50, r95, r99),
		"stage_match":     stage(mc, ma, m50, m95, m99),
		"stage_persist":   stage(pc, pa, p50, p95, p99),
		"stage_e2e":       stage(ec, ea, e50, e95, e99),
		"tape":            tape,
		"target_tps":      8000,
	}
}

func mustPool() (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(meDSN("velexa_sim"))
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 8
	return pgxpool.NewWithConfig(context.Background(), cfg)
}
