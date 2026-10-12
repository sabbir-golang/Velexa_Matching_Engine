package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Side string

const (
	Buy  Side = "BUY"
	Sell Side = "SELL"
)

type OrdType string

const (
	Limit   OrdType = "LIMIT"
	Market  OrdType = "MARKET"
	StopMkt OrdType = "STOP"       // FIX 40=3: parked, becomes Market when triggered
	StopLim OrdType = "STOP_LIMIT" // FIX 40=4: parked, becomes Limit(Price) when triggered
)

// TIF — Time-In-Force. Maps from FIX tag 59 (see mapTIF in fixacceptor.go).
type TIF string

const (
	Day TIF = "DAY" // 59=0 (default): rests (this continuous sim has no EOD cutoff)
	GTC TIF = "GTC" // 59=1: rests until cancelled
	IOC TIF = "IOC" // 59=3: fill what's available now, cancel the remainder
	FOK TIF = "FOK" // 59=4: fill the ENTIRE qty now or cancel the whole order
	GTD TIF = "GTD" // 59=6: rests until Expire, then the sweeper cancels it
)

// resting reports whether an unfilled remainder should rest on the book.
// IOC/FOK never rest; empty TIF is treated as Day for backward compatibility.
func (t TIF) resting() bool { return t == "" || t == Day || t == GTC || t == GTD }

type Order struct {
	ID      uint64
	Symbol  string
	Side    Side
	Type    OrdType
	Price   float64
	Qty     int64
	OrigQty int64
	Actor   string
	ClOrdID string
	TIF     TIF
	Expire  time.Time // GTD only; zero = no expiry
	StopPx  float64   // trigger price for StopMkt/StopLim
	Display int64     // iceberg peak (visible slice); 0 = not an iceberg
	Hidden  int64     // iceberg reserve not yet shown on the book
	Time    time.Time
	reply   chan *ExecReport  // first synchronous report (SubmitAndWait)
	notify  func(*ExecReport) // async follow-ups (stop triggers); nil = none
}

// ExecReport is the execution result returned for an externally-submitted order
// (the OMS/FIX contract): status + fills + cumulative qty + average price.
type ExecReport struct {
	OrderID   uint64  `json:"order_id"`
	ClOrdID   string  `json:"cl_ord_id"`
	Symbol    string  `json:"symbol"`
	Side      Side    `json:"side"`
	Status    string  `json:"status"` // NEW | PARTIALLY_FILLED | FILLED | REJECTED
	Fills     []Trade `json:"fills"`
	FilledQty int64   `json:"filled_qty"`
	LeavesQty int64   `json:"leaves_qty"`
	Price     float64 `json:"price,omitempty"`
	AvgPx     float64 `json:"avg_px"`
	LastPx    float64 `json:"last_px"`
	LastQty   int64   `json:"last_qty"`
	OrderQty  int64   `json:"order_qty,omitempty"` // qty ordered; survives a reject, where filled+leaves is 0
	Board     string  `json:"board,omitempty"`
	ISIN      string  `json:"isin,omitempty"`
	Reason    string  `json:"reason,omitempty"`
}

type Trade struct {
	ID        uint64    `json:"id"`
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Qty       int64     `json:"qty"`
	Aggressor Side      `json:"aggressor"`
	Time      time.Time `json:"time"`
}

type Level struct {
	Price  float64 `json:"price"`
	Qty    int64   `json:"qty"`
	Orders int     `json:"orders"`
}

type Instrument struct {
	Symbol string  `json:"symbol"`
	Name   string  `json:"name"`
	Prev   float64 `json:"prev"`
}

type MarketData struct {
	Symbol    string  `json:"symbol"`
	Name      string  `json:"name"`
	Last      float64 `json:"last"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Prev      float64 `json:"prev"`
	Change    float64 `json:"change"`
	ChangePct float64 `json:"change_pct"`
	Bid       float64 `json:"bid"`
	BidQty    int64   `json:"bid_qty"`
	Ask       float64 `json:"ask"`
	AskQty    int64   `json:"ask_qty"`
	Volume    int64   `json:"volume"`
	Trades    int64   `json:"trades"`
	Ref       float64 `json:"-"` // internal drifting fair value for bots
}

// Engine serializes all matching on a single goroutine reading from submit.
type Engine struct {
	mu       sync.RWMutex
	// books/md/instr are copy-on-write snapshots: their contents are mutated in
	// place (a *Book, a *MarketData), but the containers themselves are only
	// ever replaced. Listing an instrument swaps in freshly built ones under the
	// engine lock, so the feed, gateway and bench goroutines keep reading them
	// lock-free. instrList is published last — see listInstruments.
	booksMap  atomic.Pointer[map[string]*Book]
	mdMap     atomic.Pointer[map[string]*MarketData]
	instrList atomic.Pointer[[]Instrument]
	obIDs     atomic.Pointer[map[string]int] // symbol -> ITCH orderbook id
	trades   []Trade // recent ring (newest last)
	seq        uint64
	tradeSeq   uint64
	orderCount uint64
	submit   chan *Order
	ctrl     chan func()         // serialized control ops (cancel/replace/session)
	open     map[string]*Order   // resting orders by ClOrdID (cancel/replace)
	stops    map[string][]*Order // parked stop orders by symbol
	bus      *Bus
	phase    string
	index    float64
	indexPrev float64
	running  bool
}

func NewEngine(instr []Instrument, bus *Bus) *Engine {
	e := &Engine{
		submit:  make(chan *Order, 4096),
		ctrl:    make(chan func(), 256),
		open:    map[string]*Order{},
		stops:   map[string][]*Order{},
		bus:     bus,
		phase:   "Continuous Trading",
		running: true,
	}
	books := map[string]*Book{}
	md := map[string]*MarketData{}
	obs := map[string]int{}
	base := 0.0
	for i, in := range instr {
		books[in.Symbol] = newBook(in.Symbol)
		md[in.Symbol] = newMarketData(in)
		obs[in.Symbol] = obIDBase + i
		base += in.Prev
	}
	list := append([]Instrument(nil), instr...)
	e.booksMap.Store(&books)
	e.mdMap.Store(&md)
	e.instrList.Store(&list)
	e.obIDs.Store(&obs)
	e.indexPrev = base
	e.index = base
	return e
}

func newMarketData(in Instrument) *MarketData {
	return &MarketData{Symbol: in.Symbol, Name: in.Name, Prev: in.Prev, Open: in.Prev,
		Last: in.Prev, High: in.Prev, Low: in.Prev, Bid: 0, Ask: 0, Ref: in.Prev}
}

// obIDBase is where DSE-ITCH orderbook ids start; a symbol's id is this plus its
// position in the instrument list, which is stable because the list only grows.
const obIDBase = 100001

// orderBookID is the single source of the symbol -> ITCH orderbook id mapping,
// shared by the ITCH gateway, the SoupBinTCP feed and the live bench. It tracks
// instruments listed after boot; a symbol with no book has no id (0).
func (e *Engine) orderBookID(sym string) int { return (*e.obIDs.Load())[sym] }

func (e *Engine) book(sym string) *Book       { return (*e.booksMap.Load())[sym] }
func (e *Engine) mdOf(sym string) *MarketData { return (*e.mdMap.Load())[sym] }
func (e *Engine) instr() []Instrument         { return *e.instrList.Load() }
func (e *Engine) listed(sym string) bool      { return e.mdOf(sym) != nil }

func (e *Engine) run() {
	tick := time.NewTicker(700 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case o := <-e.submit:
			e.mu.Lock()
			if e.running {
				e.process(o) // process() branches on the session phase
			} else {
				e.replyTo(o, &ExecReport{ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
					Status: "REJECTED", Reason: "matching engine is halted"})
			}
			e.mu.Unlock()
		case fn := <-e.ctrl:
			e.mu.Lock()
			fn()
			e.mu.Unlock()
		case <-tick.C:
			e.mu.Lock()
			e.sweepExpired()
			e.recomputeIndex()
			e.mu.Unlock()
			e.broadcastSnapshot()
		}
	}
}

// bandPctFor returns the DSE daily price-fluctuation limit for a scrip, tiered
// by its reference (previous close) price — the exchange circuit breaker.
func bandPctFor(prev float64) float64 {
	switch {
	case prev <= 200:
		return 0.10
	case prev <= 500:
		return 0.0875
	case prev <= 1000:
		return 0.075
	case prev <= 2000:
		return 0.0625
	case prev <= 5000:
		return 0.05
	default:
		return 0.0375
	}
}

func (e *Engine) process(o *Order) {
	if e.book(o.Symbol) == nil {
		e.replyTo(o, &ExecReport{ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
			Status: "REJECTED", Reason: "unknown symbol " + o.Symbol + ": not in this engine's instrument universe"})
		return
	}
	if m := metaFor(o.Symbol); !m.tradable() {
		reason := "trading halted in " + o.Symbol
		switch {
		case m.Delisted:
			reason = o.Symbol + " is delisted"
		case m.Pending:
			reason = o.Symbol + " is not yet open for trading (listing pending approval)"
		}
		e.replyTo(o, &ExecReport{ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
			Status: "REJECTED", Board: m.Board, ISIN: m.ISIN, Reason: reason})
		return
	} else if m.Lot > 1 && o.Qty%int64(m.Lot) != 0 && strings.HasPrefix(o.Actor, "FIX:") {
		// DSE market lot: external orders must be a whole number of lots.
		e.replyTo(o, &ExecReport{ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
			Status: "REJECTED", Board: m.Board, ISIN: m.ISIN,
			Reason: fmt.Sprintf("Minimum quantity step: %d", m.Lot)})
		return
	}
	if o.ID == 0 {
		e.seq++
		o.ID = e.seq
	}
	e.orderCount++

	// Stop / Stop-Limit: park off-book until the last price crosses the trigger.
	if o.Type == StopMkt || o.Type == StopLim {
		e.stops[o.Symbol] = append(e.stops[o.Symbol], o)
		e.replyTo(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol,
			Side: o.Side, Status: "PENDING", LeavesQty: o.Qty})
		return
	}

	// Pre-Open call auction: accumulate limit orders without matching. The book
	// is uncrossed at a single equilibrium price when the market opens.
	if e.phase == "Pre-Open" {
		if o.Type == Limit {
			e.restTracked(o)
			e.replyTo(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol,
				Side: o.Side, Status: "NEW", LeavesQty: o.Qty + o.Hidden})
		} else {
			e.replyTo(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol,
				Side: o.Side, Status: "REJECTED", // no market orders during accumulation
				Reason: "market orders are not accepted during the Pre-Open call auction — send a limit order"})
		}
		return
	}

	// DSE-style pre-trade validation for orders arriving over FIX (external OMS
	// flow). Internal market-maker/noise bots (MM/NOISE) are exempt so they can
	// always maintain a two-sided book. Two board/instrument-aware checks:
	//   1. Tick size  — price must be a multiple of the instrument tick.
	//   2. Price band — DSE circuit-breaker: a tiered +/- limit around the prev
	//      close (10%% <= Tk200, tapering to 3.75%% for high-priced scrips).
	if o.Type == Limit && o.Price > 0 && strings.HasPrefix(o.Actor, "FIX:") {
		m := metaFor(o.Symbol)
		board := m.Board
		if board == "" {
			board = "MAIN"
		}
		tick := m.Tick
		if tick <= 0 {
			tick = 0.10
		}
		if n := o.Price / tick; math.Abs(n-math.Round(n)) > 1e-6 {
			e.replyTo(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
				Status: "REJECTED", Board: board, ISIN: m.ISIN,
				Reason: fmt.Sprintf("off-tick: %.4f is not a multiple of tick %.2f", o.Price, tick)})
			return
		}
		anchor := 0.0
		if md := e.mdOf(o.Symbol); md != nil {
			anchor = md.Prev
			if anchor <= 0 {
				anchor = md.Last
			}
		}
		if anchor > 0 {
			pct := bandPctFor(anchor)
			up, lo := round2(anchor*(1+pct)), round2(anchor*(1-pct))
			if o.Price > up+1e-9 || o.Price < lo-1e-9 {
				e.replyTo(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
					Status: "REJECTED", Board: board, ISIN: m.ISIN,
					Reason: fmt.Sprintf("price band: %.2f outside +/-%.2f%% limit [%.2f-%.2f] of prev %.2f", o.Price, pct*100, lo, up, anchor)})
				return
			}
		}
	}

	rep := e.match(o)
	if rep != nil {
		if m := metaFor(o.Symbol); m.Board != "" || m.ISIN != "" {
			rep.Board, rep.ISIN = m.Board, m.ISIN
		}
	}
	e.replyTo(o, rep)
	// A completed trade may have moved the last price across parked stop levels.
	e.triggerStops(o.Symbol)
}

func (e *Engine) replyTo(o *Order, rep *ExecReport) {
	if rep != nil && rep.Price == 0 {
		rep.Price = o.Price // echo limit price so the blotter "Price" column shows it
	}
	if rep != nil && rep.OrderQty == 0 {
		rep.OrderQty = o.OrigQty // echo OrderQty even on a reject, where filled+leaves is 0
	}
	if o.reply != nil {
		o.reply <- rep
	}
}

// match runs continuous price-time matching for an active order, rests any
// unfilled remainder per its TIF (icebergs show only their peak), and returns
// the aggressor's ExecutionReport. The engine lock must be held.
func (e *Engine) match(o *Order) *ExecReport {
	b := e.book(o.Symbol)
	md := e.mdOf(o.Symbol)

	// Fill-Or-Kill: the whole quantity must be available now, or nothing trades.
	if o.TIF == FOK && b.canFill(o.Side, o.Type, o.Price, o.Qty) < o.Qty {
		return &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol,
			Side: o.Side, Status: "CANCELED", LeavesQty: 0}
	}

	var fills []Trade
	var cumQty int64
	var notional float64
	for o.Qty > 0 {
		var lv *plevel
		if o.Side == Buy {
			lv = b.bestAsk()
			if lv == nil || (o.Type == Limit && k2px(lv.px) > o.Price) {
				break
			}
		} else {
			lv = b.bestBid()
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
		e.tradeSeq++
		tr := Trade{ID: e.tradeSeq, Symbol: o.Symbol, Price: px, Qty: q, Aggressor: o.Side, Time: time.Now()}
		e.recordTrade(tr, md)
		fills = append(fills, tr)
		cumQty += q
		notional += float64(q) * px
		o.Qty -= q
		e.consumeFront(lv, q)
		if len(lv.orders) == 0 {
			if o.Side == Buy {
				b.dropBestAsk()
			} else {
				b.dropBestBid()
			}
		}
	}

	rested := false
	remaining := o.Qty
	if o.Qty > 0 && o.Type == Limit && o.TIF.resting() {
		e.restTracked(o)
		remaining = o.Qty + o.Hidden // restTracked may split into visible+hidden
		rested = true
	}
	e.refreshTop(o.Symbol)

	var status, reason string
	var leaves int64
	if rested {
		leaves = remaining
		if cumQty > 0 {
			status = "PARTIALLY_FILLED"
		} else {
			status = "NEW"
		}
	} else { // nothing left working → any remainder is cancelled
		leaves = 0
		switch {
		case cumQty >= o.OrigQty:
			status = "FILLED"
		case cumQty > 0:
			status = "PARTIALLY_FILLED"
		case o.TIF == IOC || o.TIF == FOK:
			status = "CANCELED"
		default:
			// A market order that found an empty book: it never rests, so there
			// is nothing to report but a reject — say what was missing.
			status = "REJECTED"
			reason = "no liquidity: no resting " + string(oppositeSide(o.Side)) + " orders on " + o.Symbol
		}
	}
	avg := 0.0
	if cumQty > 0 {
		avg = round2(notional / float64(cumQty))
	}
	rep := &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
		Status: status, Fills: fills, FilledQty: cumQty, LeavesQty: leaves, AvgPx: avg, Reason: reason}
	if n := len(fills); n > 0 {
		rep.LastPx = fills[n-1].Price
		rep.LastQty = fills[n-1].Qty
	}
	return rep
}

// recordTrade appends a trade to the ring, updates market data, and publishes it.
func (e *Engine) recordTrade(tr Trade, md *MarketData) {
	e.trades = append(e.trades, tr)
	if len(e.trades) > 500 {
		e.trades = e.trades[len(e.trades)-500:]
	}
	md.Last = tr.Price
	md.Ref = tr.Price
	md.Volume += tr.Qty
	md.Trades++
	if tr.Price > md.High {
		md.High = tr.Price
	}
	if md.Low == 0 || tr.Price < md.Low {
		md.Low = tr.Price
	}
	e.emitTrade(tr)
}

// consumeFront applies a fill of q to the front order of a level, replenishing
// the next slice of an iceberg (at the back of the queue) or removing the order.
func (e *Engine) consumeFront(lv *plevel, q int64) {
	front := lv.orders[0]
	front.Qty -= q
	lv.qty -= q
	if front.Qty > 0 {
		return
	}
	if front.Display > 0 && front.Hidden > 0 { // iceberg: reveal the next slice
		slice := front.Display
		if front.Hidden < slice {
			slice = front.Hidden
		}
		front.Hidden -= slice
		front.Qty = slice
		lv.popFront()
		lv.orders = append(lv.orders, front) // back of the queue → fresh time priority
		lv.qty += slice
		return
	}
	if front.ClOrdID != "" {
		delete(e.open, front.ClOrdID)
	}
	lv.popFront()
}

// restTracked rests an order (showing only an iceberg's peak) and indexes it by
// ClOrdID so it can be cancelled or replaced later.
func (e *Engine) restTracked(o *Order) {
	if o.Display > 0 && o.Qty > o.Display {
		o.Hidden = o.Qty - o.Display
		o.Qty = o.Display
	}
	e.book(o.Symbol).rest(o)
	if o.ClOrdID != "" {
		e.open[o.ClOrdID] = o
	}
}

// triggerStops activates parked stops whose trigger the last price has crossed:
// StopMkt→Market, StopLim→Limit. Cascades, since a stop-market can move the price.
func (e *Engine) triggerStops(sym string) {
	pend := e.stops[sym]
	if len(pend) == 0 {
		return
	}
	last := e.mdOf(sym).Last
	if last <= 0 {
		return
	}
	kept := pend[:0]
	var fired []*Order
	for _, s := range pend {
		hit := (s.Side == Buy && last >= s.StopPx) || (s.Side == Sell && last <= s.StopPx)
		if hit {
			fired = append(fired, s)
		} else {
			kept = append(kept, s)
		}
	}
	e.stops[sym] = kept
	if len(fired) == 0 {
		return
	}
	for _, s := range fired {
		if s.Type == StopMkt {
			s.Type, s.Price = Market, 0
		} else {
			s.Type = Limit
		}
		rep := e.match(s)
		if s.notify != nil {
			s.notify(rep)
		}
	}
	e.triggerStops(sym) // cascade
}

// removeStop pulls a parked stop out by ClOrdID (used by cancel).
func (e *Engine) removeStop(clOrdID string) *Order {
	for sym, pend := range e.stops {
		for i, s := range pend {
			if s.ClOrdID == clOrdID {
				e.stops[sym] = append(pend[:i], pend[i+1:]...)
				return s
			}
		}
	}
	return nil
}

// Cancel removes a resting order (or parked stop) by ClOrdID. Serialized on the
// matching goroutine via ctrl.
func (e *Engine) Cancel(clOrdID string) *ExecReport {
	res := make(chan *ExecReport, 1)
	e.ctrl <- func() { res <- e.doCancel(clOrdID) }
	return <-res
}

func (e *Engine) doCancel(clOrdID string) *ExecReport {
	if o := e.open[clOrdID]; o != nil {
		e.book(o.Symbol).remove(o)
		delete(e.open, clOrdID)
		e.refreshTop(o.Symbol)
		return &ExecReport{OrderID: o.ID, ClOrdID: clOrdID, Symbol: o.Symbol, Side: o.Side,
			Status: "CANCELED", FilledQty: o.OrigQty - (o.Qty + o.Hidden), LeavesQty: 0}
	}
	if s := e.removeStop(clOrdID); s != nil {
		return &ExecReport{OrderID: s.ID, ClOrdID: clOrdID, Symbol: s.Symbol, Side: s.Side, Status: "CANCELED"}
	}
	return &ExecReport{ClOrdID: clOrdID, Status: "REJECT_CXL"}
}

// Replace amends a resting order's qty and/or price (cancel-replace semantics:
// re-rested, so time priority is reset). A newly-marketable price will match.
func (e *Engine) Replace(orig, newCl string, newQty int64, newPx float64) *ExecReport {
	res := make(chan *ExecReport, 1)
	e.ctrl <- func() { res <- e.doReplace(orig, newCl, newQty, newPx) }
	return <-res
}

func (e *Engine) doReplace(orig, newCl string, newQty int64, newPx float64) *ExecReport {
	o := e.open[orig]
	if o == nil {
		return &ExecReport{ClOrdID: newCl, Status: "REJECT_CXL"}
	}
	filled := o.OrigQty - (o.Qty + o.Hidden)
	e.book(o.Symbol).remove(o)
	delete(e.open, orig)
	remaining := newQty - filled
	if remaining <= 0 { // amended down to/below what's already filled → done
		e.refreshTop(o.Symbol)
		return &ExecReport{OrderID: o.ID, ClOrdID: newCl, Symbol: o.Symbol, Side: o.Side,
			Status: "CANCELED", FilledQty: filled, LeavesQty: 0}
	}
	o.ClOrdID = newCl
	o.OrigQty = newQty
	o.Qty = remaining
	o.Hidden = 0
	if newPx > 0 {
		o.Price = round2(newPx)
	}
	o.Time = time.Now()
	rep := e.match(o)
	if rep.FilledQty == 0 { // pure re-rest → report as Replaced (working)
		rep.Status = "REPLACED"
	}
	e.triggerStops(o.Symbol)
	return rep
}

// SetPhase transitions the session. Pre-Open→Continuous Trading runs the opening
// call auction (uncross) for every instrument, then releases any tripped stops.
func (e *Engine) SetPhase(phase string) string {
	res := make(chan string, 1)
	e.ctrl <- func() {
		prev := e.phase
		e.phase = phase
		if prev == "Pre-Open" && phase == "Continuous Trading" {
			for _, in := range e.instr() {
				e.uncross(in.Symbol)
				e.triggerStops(in.Symbol)
			}
		}
		res <- e.phase
	}
	return <-res
}

// uncross runs the opening call auction for one instrument: it finds the single
// price that maximises executable volume and trades all crossing orders there.
func (e *Engine) uncross(sym string) {
	b := e.book(sym)
	md := e.mdOf(sym)
	if len(b.bidPx) == 0 || len(b.askPx) == 0 {
		return
	}
	seen := map[int64]bool{}
	var cand []int64
	for _, k := range b.bidPx {
		if !seen[k] {
			seen[k], cand = true, append(cand, k)
		}
	}
	for _, k := range b.askPx {
		if !seen[k] {
			seen[k], cand = true, append(cand, k)
		}
	}
	ref := px2k(md.Ref)
	var bestP, bestVol int64 = 0, -1
	for _, k := range cand {
		var demand, supply int64
		for _, bk := range b.bidPx {
			if bk >= k {
				demand += b.bids[bk].qty
			}
		}
		for _, ak := range b.askPx {
			if ak <= k {
				supply += b.asks[ak].qty
			}
		}
		vol := demand
		if supply < vol {
			vol = supply
		}
		// maximise matched volume; tie-break toward the reference price
		if vol > bestVol || (vol == bestVol && absI64(k-ref) < absI64(bestP-ref)) {
			bestVol, bestP = vol, k
		}
	}
	if bestVol <= 0 {
		return
	}
	P := k2px(bestP)
	toFill := bestVol
	for toFill > 0 {
		bid := b.bestBid()
		ask := b.bestAsk()
		if bid == nil || ask == nil || k2px(bid.px) < P || k2px(ask.px) > P {
			break
		}
		q := bid.orders[0].Qty
		if af := ask.orders[0].Qty; af < q {
			q = af
		}
		if q > toFill {
			q = toFill
		}
		e.tradeSeq++
		tr := Trade{ID: e.tradeSeq, Symbol: sym, Price: P, Qty: q, Aggressor: Buy, Time: time.Now()}
		e.recordTrade(tr, md)
		toFill -= q
		e.consumeFront(bid, q)
		if len(bid.orders) == 0 {
			b.dropBestBid()
		}
		e.consumeFront(ask, q)
		if len(ask.orders) == 0 {
			b.dropBestAsk()
		}
	}
	e.refreshTop(sym)
}

func absI64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func (e *Engine) refreshTop(sym string) {
	b := e.book(sym)
	md := e.mdOf(sym)
	if lv := b.bestBid(); lv != nil {
		md.Bid, md.BidQty = k2px(lv.px), lv.qty
	} else {
		md.Bid, md.BidQty = 0, 0
	}
	if lv := b.bestAsk(); lv != nil {
		md.Ask, md.AskQty = k2px(lv.px), lv.qty
	} else {
		md.Ask, md.AskQty = 0, 0
	}
	md.Change = round2(md.Last - md.Prev)
	if md.Prev > 0 {
		md.ChangePct = round2((md.Last - md.Prev) / md.Prev * 100)
	}
}

func (e *Engine) recomputeIndex() {
	sum, prevSum := 0.0, 0.0
	for _, in := range e.instr() {
		md := e.mdOf(in.Symbol)
		sum += md.Last
		prevSum += md.Prev
	}
	if prevSum > 0 {
		e.index = round2(6000 * sum / prevSum) // DSEX-style base 6000
		e.indexPrev = 6000
	}
}

func (e *Engine) newOrder(sym string, side Side, typ OrdType, price float64, qty int64, actor string) *Order {
	return &Order{Symbol: sym, Side: side, Type: typ, Price: round2(price), Qty: qty, OrigQty: qty, Actor: actor, TIF: Day, Time: time.Now()}
}

// sweepExpired cancels resting GTD orders past their Expire time. Runs on the
// engine tick under the write lock; refreshes tops of books that changed.
func (e *Engine) sweepExpired() {
	now := time.Now()
	for _, in := range e.instr() {
		b := e.book(in.Symbol)
		if b == nil {
			continue
		}
		if removed := b.cancelExpired(now); len(removed) > 0 {
			for _, o := range removed {
				if o.ClOrdID != "" {
					delete(e.open, o.ClOrdID)
				}
			}
			e.refreshTop(in.Symbol)
		}
	}
}

// Submit is the public entry (bots + external orders).
func (e *Engine) Submit(o *Order) { e.submit <- o }

// ---- snapshots + events (JSON) ----

func (e *Engine) emitTrade(t Trade) {
	if !e.bus.hasSubs() {
		return
	}
	b, _ := json.Marshal(map[string]any{"type": "trade", "data": t})
	e.bus.publish(b)
}

func (e *Engine) broadcastSnapshot() {
	e.mu.RLock()
	rows := e.marketRowsLocked()
	idx := e.index
	phase := e.phase
	e.mu.RUnlock()
	b, _ := json.Marshal(map[string]any{"type": "market", "index": idx, "phase": phase, "rows": rows, "ts": time.Now().UnixMilli()})
	e.bus.publish(b)
}

func (e *Engine) snapshotBook(sym string) map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	b := e.book(sym)
	if b == nil {
		return map[string]any{"error": "unknown symbol"}
	}
	tr := []Trade{}
	for i := len(e.trades) - 1; i >= 0 && len(tr) < 30; i-- {
		if e.trades[i].Symbol == sym {
			tr = append(tr, e.trades[i])
		}
	}
	return map[string]any{"symbol": sym, "md": e.mdOf(sym), "bids": b.depthSide(true, 10), "asks": b.depthSide(false, 10), "trades": tr}
}

// marketRow is one Market Watch line: the live prices plus the instrument's
// reference data, so the terminal can filter by board, class, sector and status.
type marketRow struct {
	MarketData
	Board      string `json:"board"`
	AssetClass string `json:"asset_class"`
	Sector     string `json:"sector"`
	Category   string `json:"category"`
	Status     string `json:"status"`
}

// categoryOf is the DSE share category: the letter in a board label such as
// "Main (A)", S for the SME board, or "" when the board carries none.
func categoryOf(board string) string {
	if i, j := strings.IndexByte(board, '('), strings.IndexByte(board, ')'); i >= 0 && j == i+2 {
		return board[i+1 : j]
	}
	if strings.Contains(strings.ToUpper(board), "SME") {
		return "S"
	}
	return ""
}

// marketRowsLocked builds the sorted Market Watch rows; delisted scrips are left
// out. The caller holds e.mu.
func (e *Engine) marketRowsLocked() []marketRow {
	rows := make([]marketRow, 0, len(e.instr()))
	for _, in := range e.instr() {
		m := metaFor(in.Symbol)
		if m.Delisted {
			continue
		}
		board := m.Board
		if board == "" {
			board = "MAIN"
		}
		rows = append(rows, marketRow{MarketData: *e.mdOf(in.Symbol), Board: board,
			AssetClass: assetClassOf(m), Sector: m.Sector, Category: categoryOf(board), Status: m.statusOf()})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Symbol < rows[j].Symbol })
	return rows
}

func (e *Engine) marketRows() []marketRow {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.marketRowsLocked()
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// SubmitAndWait submits an external order and blocks for its ExecutionReport
// (the synchronous OMS/FIX contract). Returns nil on timeout.
func (e *Engine) SubmitAndWait(o *Order) *ExecReport {
	o.reply = make(chan *ExecReport, 1)
	e.submit <- o
	select {
	case rep := <-o.reply:
		return rep
	case <-time.After(3 * time.Second):
		return &ExecReport{ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side, Status: "REJECTED",
			Reason: "matching engine did not respond within 3s"}
	}
}

// LoadTest measures pure matching-engine throughput: it generates n orders
// (mix of resting limits + marketable orders) and times how long the single
// matching goroutine takes to process all of them. Returns orders/sec + avg
// per-order latency — the numbers RFP Annexure B §19 & §21 ask for.
func (e *Engine) LoadTest(n int, sym string) map[string]any {
	if n <= 0 {
		n = 100000
	}
	if e.mdOf(sym) == nil {
		sym = e.instr()[0].Symbol
	}
	e.mu.RLock()
	base := e.mdOf(sym).Ref
	e.mu.RUnlock()
	if base <= 0 {
		base = e.mdOf(sym).Prev
	}
	rng := rand.New(rand.NewSource(42))
	orders := make([]*Order, n)
	for i := 0; i < n; i++ {
		side := Buy
		if i%2 == 0 {
			side = Sell
		}
		typ := Limit
		px := roundTick(base + (rng.Float64()-0.5)*base*0.01)
		if rng.Float64() < 0.30 { // 30% marketable → real matches
			typ = Market
			px = 0
		}
		orders[i] = &Order{Symbol: sym, Side: side, Type: typ, Price: px, Qty: int64(100 + rng.Intn(400)), Actor: "LOADTEST", Time: time.Now()}
	}
	done := make(chan *ExecReport, 1)
	orders[n-1].reply = done
	start := time.Now()
	for _, o := range orders {
		e.submit <- o
	}
	<-done
	el := time.Since(start)
	// clear the load-test residue from the book
	e.mu.Lock()
	e.book(sym).cancelActor("LOADTEST")
	e.mu.Unlock()
	ops := int64(float64(n) / el.Seconds())
	return map[string]any{
		"orders":         n,
		"symbol":         sym,
		"elapsed_ms":     el.Milliseconds(),
		"orders_per_sec": ops,
		"ns_per_order":   el.Nanoseconds() / int64(n),
	}
}

// oppositeSide names the side an aggressor needs liquidity from, for reject text.
func oppositeSide(s Side) Side {
	if s == Buy {
		return Sell
	}
	return Buy
}
