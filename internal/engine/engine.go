// Package engine is the matching-engine core: orders, order books, the matching loop, market-data snapshots, the instrument universe and the market-maker bots.
package engine

import (
	"encoding/json"
	"fmt"
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
	// MarketBest is DSE market-at-best (FIX 40=Z): it executes at the best opposite
	// price only, and any remainder rests as a limit order at that price.
	MarketBest OrdType = "MARKET_AT_BEST"
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

// OrderInfo is what the broker's order says about whom it is for; the engine
// does not use it, it is carried for the order report.
type OrderInfo struct {
	Account string // BO account (FIX tag 1)
	Client  string // client code (party ID 448 with role 452=5)
	Board   string // board / order sub-type (tag 762, e.g. PUBLIC)
	Trader  string // trader (SenderSubID, tag 50)
}

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
	reply   chan *ExecReport // first synchronous report (SubmitAndWait)
	// Notify, when set, receives every report about the order, in the order the
	// engine produces them: the reply to its submission, fills while it rests,
	// cancel / replace confirmations, a triggered stop and expiry. It is called on
	// the matching goroutine, so it must not block.
	Notify func(*ExecReport)
	Ctx    any       // opaque to the engine; echoed on every report (the FIX gateway's order context)
	Info   OrderInfo // who the order is for, as the broker sent it (for the order report)

	CumQty   int64   // filled so far, over the order's life
	CumValue float64 // sum of fill qty x price, over the order's life
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

	// Event says what produced the report: "" the reply to a new order, or
	// REPLACE, TRIGGER (a stop went live), FILL (a resting order traded),
	// CANCEL, EXPIRE, STATUS.
	Event       string  `json:"event,omitempty"`
	OrigClOrdID string  `json:"orig_cl_ord_id,omitempty"`
	ReqID       string  `json:"-"` // the status request's id (FIX 790), for STATUS
	CumValue    float64 `json:"-"` // exact sum of fill qty x price (AvgPx is rounded)
	Aggressor   bool    `json:"-"` // the fills in this report took liquidity
	Ctx         any     `json:"-"`
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
	Mu sync.RWMutex
	// books/md/instr are copy-on-write snapshots: their contents are mutated in
	// place (a *Book, a *MarketData), but the containers themselves are only
	// ever replaced. Listing an instrument swaps in freshly built ones under the
	// engine lock, so the feed, gateway and bench goroutines keep reading them
	// lock-free. instrList is published last — see listInstruments.
	booksMap     atomic.Pointer[map[string]*Book]
	mdMap        atomic.Pointer[map[string]*MarketData]
	instrList    atomic.Pointer[[]Instrument]
	obIDs        atomic.Pointer[map[string]int] // symbol -> ITCH orderbook id
	actorStats   map[string]*ActorStats         // per order source ("FIX:<CompID>", "API"…); matching goroutine writes, Mu guards
	schedule     Schedule                       // the trading-day clock (set by StartClock)
	tradingDate  string                         // Dhaka date of the current trading day (set when a day starts)
	lastReplaced *Order                         // the order doReplace worked on, for ReplaceAs (matching goroutine only)
	days         dayStats                       // daily report counters (daystats.go)

	// OnOrderEvent, when set (before Run), sees every report about every order
	// — first reply, fills, cancel, amend, expiry, trigger — for the order log.
	// It runs on the matching goroutine and must not block.
	OnOrderEvent func(*Order, *ExecReport)
	journal      *Journal   // public market events, for the ITCH feed
	obStore      *obIDStore // keeps the ids permanent; nil = position based (tests)
	Trades       []Trade    // recent ring (newest last)
	Seq          uint64
	TradeSeq     uint64
	orderCount   uint64
	submit       chan *Order
	ctrl         chan func()         // serialized control ops (cancel/replace/session)
	Open         map[string]*Order   // resting orders by openKey(actor, ClOrdID), for cancel/replace
	stops        map[string][]*Order // parked stop orders by symbol
	bus          *Bus
	Phase        string
	Index        float64
	IndexPrev    float64
	Running      bool
}

func NewEngine(instr []Instrument, bus *Bus) *Engine {
	e := &Engine{
		submit:  make(chan *Order, 4096),
		ctrl:    make(chan func(), 256),
		Open:    map[string]*Order{},
		stops:   map[string][]*Order{},
		bus:     bus,
		journal: NewJournal(journalCapacity()),
		Phase:   "Continuous Trading",
		Running: true,
	}
	books := map[string]*Book{}
	md := map[string]*MarketData{}
	obs := map[string]int{}
	base := 0.0
	for i, in := range instr {
		books[in.Symbol] = NewBook(in.Symbol)
		md[in.Symbol] = newMarketData(in)
		obs[in.Symbol] = obIDBase + i
		base += in.Prev
	}
	list := append([]Instrument(nil), instr...)
	e.booksMap.Store(&books)
	e.mdMap.Store(&md)
	e.instrList.Store(&list)
	e.obIDs.Store(&obs)
	e.IndexPrev = base
	e.Index = base
	return e
}

func newMarketData(in Instrument) *MarketData {
	return &MarketData{Symbol: in.Symbol, Name: in.Name, Prev: in.Prev, Open: in.Prev,
		Last: in.Prev, High: in.Prev, Low: in.Prev, Bid: 0, Ask: 0, Ref: in.Prev}
}

// obIDBase is where DSE-ITCH orderbook ids start; a symbol's id is this plus its
// position in the instrument list, which is stable because the list only grows.
const obIDBase = 100001

// OrderBookID is the single source of the symbol -> ITCH orderbook id mapping,
// shared by the ITCH gateway, the SoupBinTCP feed and the live bench. It tracks
// instruments listed after boot; a symbol with no book has no id (0).
func (e *Engine) OrderBookID(sym string) int { return (*e.obIDs.Load())[sym] }

func (e *Engine) Book(sym string) *Book { return (*e.booksMap.Load())[sym] }

func (e *Engine) MdOf(sym string) *MarketData { return (*e.mdMap.Load())[sym] }

func (e *Engine) Instr() []Instrument { return *e.instrList.Load() }

func (e *Engine) Listed(sym string) bool { return e.MdOf(sym) != nil }

func (e *Engine) Run() {
	tick := time.NewTicker(700 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case o := <-e.submit:
			e.Mu.Lock()
			if e.Running {
				e.process(o) // process() branches on the session phase
			} else {
				e.replyTo(o, &ExecReport{ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
					Status: "REJECTED", Reason: ReasonNotAccepting})
			}
			e.Mu.Unlock()
		case fn := <-e.ctrl:
			e.Mu.Lock()
			fn()
			e.Mu.Unlock()
		case <-tick.C:
			e.Mu.Lock()
			e.sweepExpired()
			e.recomputeIndex()
			e.Mu.Unlock()
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

// priceReason applies the DSE pre-trade price checks to a limit price on an
// order from an external broker (FIX): the instrument's tick size, and the
// circuit-breaker band — a tiered +/- limit around the previous close (10% up
// to Tk200, tapering to 3.75% for high-priced scrips). It returns the reject
// reason, or "" when the price is acceptable. Console orders are not checked.
func (e *Engine) priceReason(sym, actor string, price float64) string {
	if price <= 0 || !strings.HasPrefix(actor, "FIX:") {
		return ""
	}
	m := MetaFor(sym)
	tick := m.Tick
	if tick <= 0 {
		tick = 0.10
	}
	if n := price / tick; math.Abs(n-math.Round(n)) > 1e-6 {
		return fmt.Sprintf("off-tick: %.4f is not a multiple of tick %.2f", price, tick)
	}
	anchor := 0.0
	if md := e.MdOf(sym); md != nil {
		anchor = md.Prev
		if anchor <= 0 {
			anchor = md.Last
		}
	}
	if anchor > 0 {
		pct := bandPctFor(anchor)
		up, lo := Round2(anchor*(1+pct)), Round2(anchor*(1-pct))
		if price > up+1e-9 || price < lo-1e-9 {
			return fmt.Sprintf("price band: %.2f outside +/-%.2f%% limit [%.2f-%.2f] of prev %.2f", price, pct*100, lo, up, anchor)
		}
	}
	return ""
}

// replaceReason checks an amend (35=G) the way a new order is checked: the
// market must be accepting orders, the new ClOrdID must be free, and the new
// quantity and price must pass the lot, tick and band rules. "" = acceptable.
func (e *Engine) replaceReason(o *Order, orig, newCl string, newQty int64, newPx float64) string {
	switch e.Phase {
	case PhaseClose, PhaseClosed:
		return ReasonNotAccepting
	}
	if newCl != "" && newCl != orig && e.working(o.Actor, newCl) {
		return ReasonDuplicateClOrdID + ": " + newCl
	}
	if newQty <= 0 {
		newQty = o.OrigQty
	}
	if m := MetaFor(o.Symbol); m.Lot > 1 && newQty%int64(m.Lot) != 0 && strings.HasPrefix(o.Actor, "FIX:") {
		return fmt.Sprintf("Minimum quantity step: %d", m.Lot)
	}
	if newPx > 0 && (o.Type == Limit || o.Type == StopLim) {
		return e.priceReason(o.Symbol, o.Actor, newPx)
	}
	return ""
}

func boardOr(b string) string {
	if b == "" {
		return "MAIN"
	}
	return b
}

func (e *Engine) process(o *Order) {
	if e.Book(o.Symbol) == nil {
		e.replyTo(o, &ExecReport{ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
			Status: "REJECTED", Reason: "unknown symbol " + o.Symbol + ": not in this engine's instrument universe"})
		return
	}
	if m := MetaFor(o.Symbol); !m.Tradable() {
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
	// A broker may not reuse the ClOrdID of one of its working orders: the
	// second order could never be cancelled or amended on its own.
	if e.working(o.Actor, o.ClOrdID) {
		e.replyTo(o, &ExecReport{ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
			Status: "REJECTED", Reason: ReasonDuplicateClOrdID + ": " + o.ClOrdID})
		return
	}
	// Tick size and price band apply in every session, the Pre-Open auction too.
	if o.Type == Limit || o.Type == StopLim {
		if reason := e.priceReason(o.Symbol, o.Actor, o.Price); reason != "" {
			m := MetaFor(o.Symbol)
			e.replyTo(o, &ExecReport{ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
				Status: "REJECTED", Board: boardOr(m.Board), ISIN: m.ISIN, Reason: reason})
			return
		}
	}
	if o.ID == 0 {
		e.Seq++
		o.ID = e.Seq
	}
	e.orderCount++
	e.statOf(o.Actor).Orders++

	// Session rules: nothing is accepted between the close and the next pre-open,
	// and in post-close only at the closing price.
	switch e.Phase {
	case PhaseClose, PhaseClosed:
		e.replyTo(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
			Status: "REJECTED", Reason: ReasonNotAccepting})
		return
	case PhasePostClose:
		closePx := e.MdOf(o.Symbol).Last
		switch o.Type {
		case Market, MarketBest:
			o.Type, o.Price = Limit, closePx
		case Limit:
			if px2k(o.Price) != px2k(closePx) {
				e.replyTo(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
					Status: "REJECTED", Reason: ReasonNotClosePrice})
				return
			}
		default:
			e.replyTo(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
				Status: "REJECTED", Reason: ReasonNotAccepting})
			return
		}
	}

	// Market-at-best: price it at the best opposite level, then treat it as a limit.
	if o.Type == MarketBest {
		b := e.Book(o.Symbol)
		lv := b.BestAsk()
		if o.Side == Sell {
			lv = b.BestBid()
		}
		if lv == nil {
			e.replyTo(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
				Status: "REJECTED", Reason: "no liquidity: no resting " + string(oppositeSide(o.Side)) + " orders on " + o.Symbol})
			return
		}
		o.Type, o.Price = Limit, K2Px(lv.Px)
		rep := e.match(o)
		e.replyTo(o, rep)
		e.triggerStops(o.Symbol)
		return
	}

	// Stop / Stop-Limit: park off-book until the last price crosses the trigger.
	if o.Type == StopMkt || o.Type == StopLim {
		e.stops[o.Symbol] = append(e.stops[o.Symbol], o)
		e.replyTo(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol,
			Side: o.Side, Status: "PENDING", LeavesQty: o.Qty})
		return
	}

	// Pre-Open call auction: accumulate limit orders without matching. The book
	// is uncrossed at a single equilibrium price when the market opens.
	if e.Phase == "Pre-Open" {
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

	rep := e.match(o)
	if rep != nil {
		if m := MetaFor(o.Symbol); m.Board != "" || m.ISIN != "" {
			rep.Board, rep.ISIN = m.Board, m.ISIN
		}
	}
	e.replyTo(o, rep)
	// A completed trade may have moved the last price across parked stop levels.
	e.triggerStops(o.Symbol)
}

// observe passes an order's report to the order log, if one is attached.
func (e *Engine) observe(o *Order, rep *ExecReport) {
	if e.OnOrderEvent != nil && rep != nil {
		e.OnOrderEvent(o, rep)
	}
}

// RejectUnsubmitted records a new order that was refused before it reached the
// matching engine (by the FIX gateway or the order API): it is counted in the
// daily report and appears in the order log as rejected.
func (e *Engine) RejectUnsubmitted(o *Order, reason string) {
	e.CountReject(o.Actor, o.Side)
	e.observe(o, &ExecReport{ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side, Status: "REJECTED",
		Reason: reason, OrderQty: o.OrigQty, Price: o.Price})
}

// SetMinSeq makes the next order IDs larger than n, so a restart during the
// day never hands out an OrderID that was already used (call before Run).
func (e *Engine) SetMinSeq(n uint64) {
	e.Mu.Lock()
	if e.Seq < n {
		e.Seq = n
	}
	e.Mu.Unlock()
}

func (e *Engine) replyTo(o *Order, rep *ExecReport) {
	e.countReply(o, rep) // every new order gets exactly one first reply
	if rep != nil && rep.Price == 0 {
		rep.Price = o.Price // echo limit price so the blotter "Price" column shows it
	}
	if rep != nil && rep.OrderQty == 0 {
		rep.OrderQty = o.OrigQty // echo OrderQty even on a reject, where filled+leaves is 0
	}
	if rep != nil && rep.Ctx == nil {
		rep.Ctx = o.Ctx
	}
	e.observe(o, rep)
	if rep != nil && o.Notify != nil {
		o.Notify(rep)
	}
	if o.reply != nil {
		o.reply <- rep
	}
}

// notify sends a follow-up report to the order's owner, if it has one.
func (e *Engine) notify(o *Order, rep *ExecReport) {
	if rep == nil {
		return
	}
	if rep.Ctx == nil {
		rep.Ctx = o.Ctx
	}
	if rep.OrderQty == 0 {
		rep.OrderQty = o.OrigQty
	}
	if rep.Price == 0 {
		rep.Price = o.Price
	}
	e.observe(o, rep) // every order, also those without a Notify (console orders)
	if o.Notify != nil {
		o.Notify(rep)
	}
}

// avgOf is an order's average fill price.
func avgOf(o *Order) float64 {
	if o.CumQty <= 0 {
		return 0
	}
	return Round2(o.CumValue / float64(o.CumQty))
}

// match runs continuous price-time matching for an active order, rests any
// unfilled remainder per its TIF (icebergs show only their peak), and returns
// the aggressor's ExecutionReport. The engine lock must be held.
func (e *Engine) match(o *Order) *ExecReport {
	b := e.Book(o.Symbol)
	md := e.MdOf(o.Symbol)

	// Fill-Or-Kill: the whole quantity must be available now, or nothing trades.
	if o.TIF == FOK && b.canFill(o.Side, o.Type, o.Price, o.Qty) < o.Qty {
		return &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
			Status: "CANCELED", LeavesQty: 0, FilledQty: o.CumQty, CumValue: o.CumValue, AvgPx: avgOf(o), OrderQty: o.OrigQty}
	}

	var fills []Trade
	var cumQty int64 // filled in this call
	for o.Qty > 0 {
		var lv *PriceLevel
		if o.Side == Buy {
			lv = b.BestAsk()
			if lv == nil || (o.Type == Limit && K2Px(lv.Px) > o.Price) {
				break
			}
		} else {
			lv = b.BestBid()
			if lv == nil || (o.Type == Limit && K2Px(lv.Px) < o.Price) {
				break
			}
		}
		front := lv.Orders[0]
		q := o.Qty
		if front.Qty < q {
			q = front.Qty
		}
		px := K2Px(lv.Px)
		e.TradeSeq++
		tr := Trade{ID: e.TradeSeq, Symbol: o.Symbol, Price: px, Qty: q, Aggressor: o.Side, Time: time.Now()}
		e.recordTrade(tr, md)
		fills = append(fills, tr)
		cumQty += q
		o.CumQty += q
		o.CumValue += float64(q) * px
		o.Qty -= q
		e.statTrade(o.Actor, q, px)
		e.statTrade(lv.Orders[0].Actor, q, px)
		e.consumeFront(lv, q, EvExec, tr)
		if len(lv.Orders) == 0 {
			if o.Side == Buy {
				b.DropBestAsk()
			} else {
				b.DropBestBid()
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
		if cumQty > 0 || o.CumQty > 0 {
			status = "PARTIALLY_FILLED"
		} else {
			status = "NEW"
		}
	} else { // nothing left working → any remainder is cancelled
		leaves = 0
		switch {
		case o.CumQty >= o.OrigQty:
			status = "FILLED"
		case o.CumQty > 0:
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
	rep := &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side,
		Status: status, Fills: fills, FilledQty: o.CumQty, LeavesQty: leaves, AvgPx: avgOf(o), Reason: reason,
		CumValue: o.CumValue, Aggressor: true, OrderQty: o.OrigQty}
	if n := len(fills); n > 0 {
		rep.LastPx = fills[n-1].Price
		rep.LastQty = fills[n-1].Qty
	}
	return rep
}

// recordTrade appends a trade to the ring, updates market data, and publishes it.
func (e *Engine) recordTrade(tr Trade, md *MarketData) {
	e.Trades = append(e.Trades, tr)
	if len(e.Trades) > 500 {
		e.Trades = e.Trades[len(e.Trades)-500:]
	}
	if md.Trades == 0 { // the day's first trade opens the day's range
		md.Open, md.High, md.Low = tr.Price, tr.Price, tr.Price
	}
	md.Last = tr.Price
	md.Ref = tr.Price
	md.Volume += tr.Qty
	md.Trades++
	e.days.trade("", tr.Qty, tr.Price, true)
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
func (e *Engine) consumeFront(lv *PriceLevel, q int64, kind EvKind, tr Trade) {
	front := lv.Orders[0]
	e.emitExec(kind, front, q, tr.ID)
	front.Qty -= q
	lv.Qty -= q
	front.CumQty += q
	front.CumValue += float64(q) * tr.Price
	defer e.reportRestingFill(front, tr, q)
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
		lv.PopFront()
		lv.Orders = append(lv.Orders, front) // back of the queue → fresh time priority
		lv.Qty += slice
		e.emitAdd(front) // the exhausted slice left the feed's book; the next one is a new add
		return
	}
	e.forget(front)
	lv.PopFront()
}

// reportRestingFill tells a resting order's owner about one fill against it.
func (e *Engine) reportRestingFill(o *Order, tr Trade, q int64) {
	if o.Notify == nil && e.OnOrderEvent == nil {
		return
	}
	leaves := o.Qty + o.Hidden
	status := "PARTIALLY_FILLED"
	if leaves == 0 {
		status = "FILLED"
	}
	fill := tr
	fill.Qty = q
	e.notify(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side, Status: status,
		Fills: []Trade{fill}, FilledQty: o.CumQty, LeavesQty: leaves, AvgPx: avgOf(o), CumValue: o.CumValue,
		LastPx: tr.Price, LastQty: q, Event: "FILL"})
}

// restTracked rests an order (showing only an iceberg's peak) and indexes it by
// ClOrdID so it can be cancelled or replaced later.
func (e *Engine) restTracked(o *Order) {
	if o.Display > 0 && o.Qty > o.Display {
		o.Hidden = o.Qty - o.Display
		o.Qty = o.Display
	}
	e.Book(o.Symbol).Rest(o)
	if o.ClOrdID != "" {
		e.Open[openKey(o.Actor, o.ClOrdID)] = o
	}
	e.emitAdd(o)
}

// openKey indexes a working order by its owner and ClOrdID. A ClOrdID is only
// unique within one broker's flow, so two brokers may use the same one.
func openKey(actor, clOrdID string) string { return actor + "\x1f" + clOrdID }

// forget drops a finished order from the ClOrdID index.
func (e *Engine) forget(o *Order) {
	if o.ClOrdID != "" {
		delete(e.Open, openKey(o.Actor, o.ClOrdID))
	}
}

// FindOpen returns the resting order with this ClOrdID: the given actor's, or —
// with actor "" — anyone's (admin tools and tests). The caller holds e.Mu.
func (e *Engine) FindOpen(clOrdID, actor string) *Order {
	if actor != "" {
		return e.Open[openKey(actor, clOrdID)]
	}
	for _, o := range e.Open {
		if o.ClOrdID == clOrdID {
			return o
		}
	}
	return nil
}

// working reports whether the actor already has a live order (resting or a
// parked stop) with this ClOrdID.
func (e *Engine) working(actor, clOrdID string) bool {
	return clOrdID != "" && (e.Open[openKey(actor, clOrdID)] != nil || e.findStop(clOrdID, actor) != nil)
}

// triggerStops activates parked stops whose trigger the last price has crossed:
// StopMkt→Market, StopLim→Limit. Cascades, since a stop-market can move the price.
func (e *Engine) triggerStops(sym string) {
	pend := e.stops[sym]
	if len(pend) == 0 {
		return
	}
	last := e.MdOf(sym).Last
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
		rep.Event = "TRIGGER"
		e.notify(s, rep)
	}
	e.triggerStops(sym) // cascade
}

// removeStop pulls a parked stop out of its symbol's list (used by cancel).
func (e *Engine) removeStop(stop *Order) *Order {
	for sym, pend := range e.stops {
		for i, s := range pend {
			if s == stop {
				e.stops[sym] = append(pend[:i], pend[i+1:]...)
				return s
			}
		}
	}
	return nil
}

// Cancel removes a resting order (or parked stop) by ClOrdID. Serialized on the
// matching goroutine via ctrl.
func (e *Engine) Cancel(clOrdID string) *ExecReport { return e.CancelAs(clOrdID, clOrdID, "") }

// CancelAs cancels the order orig on behalf of actor ("" = anyone) with the
// cancel request's own ClOrdID cl. The confirmation also goes to the order's
// Notify. An order that is unknown, done, or owned by another actor is not
// touched and the report's Status is REJECT_CXL.
func (e *Engine) CancelAs(orig, cl, actor string) *ExecReport {
	res := make(chan *ExecReport, 1)
	e.ctrl <- func() { res <- e.doCancel(orig, cl, actor) }
	return <-res
}

func (e *Engine) doCancel(orig, cl, actor string) *ExecReport {
	if o := e.FindOpen(orig, actor); o != nil {
		e.Book(o.Symbol).remove(o)
		e.forget(o)
		e.emitDelete(o)
		e.refreshTop(o.Symbol)
		rep := &ExecReport{OrderID: o.ID, ClOrdID: cl, OrigClOrdID: orig, Symbol: o.Symbol, Side: o.Side,
			Status: "CANCELED", FilledQty: o.CumQty, CumValue: o.CumValue, AvgPx: avgOf(o), LeavesQty: 0,
			OrderQty: o.OrigQty, Price: o.Price, Event: "CANCEL", Ctx: o.Ctx}
		e.days.order(o.Actor, func(c *DayCounters) { c.Cancelled++ })
		e.notify(o, rep)
		return rep
	}
	if s := e.findStop(orig, actor); s != nil {
		e.removeStop(s)
		rep := &ExecReport{OrderID: s.ID, ClOrdID: cl, OrigClOrdID: orig, Symbol: s.Symbol, Side: s.Side,
			Status: "CANCELED", OrderQty: s.OrigQty, Price: s.Price, Event: "CANCEL", Ctx: s.Ctx}
		e.days.order(s.Actor, func(c *DayCounters) { c.Cancelled++ })
		e.notify(s, rep)
		return rep
	}
	return &ExecReport{ClOrdID: cl, OrigClOrdID: orig, Status: "REJECT_CXL"}
}

// findStop returns the actor's parked stop with this ClOrdID (anyone's with
// actor ""), or nil.
func (e *Engine) findStop(clOrdID, actor string) *Order {
	for _, pend := range e.stops {
		for _, s := range pend {
			if s.ClOrdID == clOrdID && (actor == "" || s.Actor == actor) {
				return s
			}
		}
	}
	return nil
}

// Status reports the current state of a working order (resting or a parked
// stop) for an order status request; Status REJECT_CXL when there is none.
func (e *Engine) Status(clOrdID, actor string) *ExecReport {
	res := make(chan *ExecReport, 1)
	e.ctrl <- func() {
		o := e.FindOpen(clOrdID, actor)
		if o == nil {
			o = e.findStop(clOrdID, actor)
		}
		if o == nil {
			res <- &ExecReport{ClOrdID: clOrdID, Status: "REJECT_CXL", Event: "STATUS"}
			return
		}
		status := "NEW"
		if o.CumQty > 0 {
			status = "PARTIALLY_FILLED"
		}
		res <- &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, Symbol: o.Symbol, Side: o.Side, Status: status,
			FilledQty: o.CumQty, CumValue: o.CumValue, AvgPx: avgOf(o), LeavesQty: o.Qty + o.Hidden,
			OrderQty: o.OrigQty, Price: o.Price, Event: "STATUS", Ctx: o.Ctx}
	}
	return <-res
}

// Replace amends a resting order's qty and/or price (cancel-replace semantics:
// re-rested, so time priority is reset). A newly-marketable price will match.
func (e *Engine) Replace(orig, newCl string, newQty int64, newPx float64) *ExecReport {
	return e.ReplaceAs(orig, newCl, "", newQty, newPx)
}

// ReplaceAs amends the order orig on behalf of actor ("" = anyone). The result
// also goes to the order's Notify; REJECT_CXL means the order was not touched.
func (e *Engine) ReplaceAs(orig, newCl, actor string, newQty int64, newPx float64) *ExecReport {
	res := make(chan *ExecReport, 1)
	e.ctrl <- func() {
		rep := e.doReplace(orig, newCl, actor, newQty, newPx)
		if rep.Status != "REJECT_CXL" {
			if o := e.lastReplaced; o != nil {
				e.notify(o, rep)
			}
		}
		e.lastReplaced = nil
		res <- rep
	}
	return <-res
}

func (e *Engine) doReplace(orig, newCl, actor string, newQty int64, newPx float64) *ExecReport {
	o := e.FindOpen(orig, actor)
	if o == nil {
		return &ExecReport{ClOrdID: newCl, OrigClOrdID: orig, Status: "REJECT_CXL"}
	}
	if newQty <= 0 { // no OrderQty (38) on the amend: keep the order's quantity
		newQty = o.OrigQty
	}
	if reason := e.replaceReason(o, orig, newCl, newQty, newPx); reason != "" {
		return &ExecReport{OrderID: o.ID, ClOrdID: newCl, OrigClOrdID: orig, Symbol: o.Symbol, Side: o.Side,
			Status: "REJECT_CXL", Reason: reason, Ctx: o.Ctx}
	}
	e.lastReplaced = o
	e.days.order(o.Actor, func(c *DayCounters) { c.Amended++ })
	filled := o.CumQty
	e.Book(o.Symbol).remove(o)
	e.forget(o)
	e.emitDelete(o) // the re-rest below publishes the order again as a new add
	remaining := newQty - filled
	if remaining <= 0 { // amended down to/below what's already filled → done
		e.refreshTop(o.Symbol)
		return &ExecReport{OrderID: o.ID, ClOrdID: newCl, OrigClOrdID: orig, Symbol: o.Symbol, Side: o.Side,
			Status: "CANCELED", FilledQty: filled, CumValue: o.CumValue, AvgPx: avgOf(o), LeavesQty: 0,
			OrderQty: newQty, Price: o.Price, Event: "REPLACE", Ctx: o.Ctx}
	}
	o.ClOrdID = newCl
	o.OrigQty = newQty
	o.Qty = remaining
	o.Hidden = 0
	if newPx > 0 {
		o.Price = Round2(newPx)
	}
	o.Time = time.Now()
	rep := e.match(o)
	rep.Event, rep.OrigClOrdID, rep.Ctx = "REPLACE", orig, o.Ctx
	if len(rep.Fills) == 0 && rep.LeavesQty > 0 { // pure re-rest → report as Replaced (working)
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
		prev := e.Phase
		e.Phase = phase
		if phase != prev {
			if code := SessionCode(phase); code != 0 {
				e.journal.Append(Event{Kind: EvSession, Code: code})
			}
			e.onPhase(prev, phase)
		}
		res <- e.Phase
	}
	return <-res
}

// uncross runs the opening call auction for one instrument: it finds the single
// price that maximises executable volume and trades all crossing orders there.
func (e *Engine) uncross(sym string) {
	b := e.Book(sym)
	md := e.MdOf(sym)
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
				demand += b.bids[bk].Qty
			}
		}
		for _, ak := range b.askPx {
			if ak <= k {
				supply += b.asks[ak].Qty
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
	P := K2Px(bestP)
	toFill := bestVol
	for toFill > 0 {
		bid := b.BestBid()
		ask := b.BestAsk()
		if bid == nil || ask == nil || K2Px(bid.Px) < P || K2Px(ask.Px) > P {
			break
		}
		q := bid.Orders[0].Qty
		if af := ask.Orders[0].Qty; af < q {
			q = af
		}
		if q > toFill {
			q = toFill
		}
		e.TradeSeq++
		tr := Trade{ID: e.TradeSeq, Symbol: sym, Price: P, Qty: q, Aggressor: Buy, Time: time.Now()}
		e.recordTrade(tr, md)
		toFill -= q
		e.statTrade(bid.Orders[0].Actor, q, P)
		e.statTrade(ask.Orders[0].Actor, q, P)
		e.consumeFront(bid, q, EvExecAuction, tr)
		if len(bid.Orders) == 0 {
			b.DropBestBid()
		}
		e.consumeFront(ask, q, EvExecAuction, tr)
		if len(ask.Orders) == 0 {
			b.DropBestAsk()
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
	b := e.Book(sym)
	md := e.MdOf(sym)
	if lv := b.BestBid(); lv != nil {
		md.Bid, md.BidQty = K2Px(lv.Px), lv.Qty
	} else {
		md.Bid, md.BidQty = 0, 0
	}
	if lv := b.BestAsk(); lv != nil {
		md.Ask, md.AskQty = K2Px(lv.Px), lv.Qty
	} else {
		md.Ask, md.AskQty = 0, 0
	}
	md.Change = Round2(md.Last - md.Prev)
	if md.Prev > 0 {
		md.ChangePct = Round2((md.Last - md.Prev) / md.Prev * 100)
	}
}

func (e *Engine) recomputeIndex() {
	sum, prevSum := 0.0, 0.0
	for _, in := range e.Instr() {
		md := e.MdOf(in.Symbol)
		sum += md.Last
		prevSum += md.Prev
	}
	if prevSum > 0 {
		e.Index = Round2(6000 * sum / prevSum) // DSEX-style base 6000
		e.IndexPrev = 6000
	}
}

func (e *Engine) NewOrder(sym string, side Side, typ OrdType, price float64, qty int64, actor string) *Order {
	return &Order{Symbol: sym, Side: side, Type: typ, Price: Round2(price), Qty: qty, OrigQty: qty, Actor: actor, TIF: Day, Time: time.Now()}
}

// sweepExpired cancels resting GTD orders past their Expire time. Runs on the
// engine tick under the write lock; refreshes tops of books that changed.
func (e *Engine) sweepExpired() {
	now := time.Now()
	for _, in := range e.Instr() {
		b := e.Book(in.Symbol)
		if b == nil {
			continue
		}
		if removed := b.cancelExpired(now); len(removed) > 0 {
			for _, o := range removed {
				e.forget(o)
				e.emitDelete(o)
				e.expired(o)
			}
			e.refreshTop(in.Symbol)
		}
	}
}

// expired tells an order's owner that it expired unexecuted (in full or in part).
func (e *Engine) expired(o *Order) {
	e.days.order(o.Actor, func(c *DayCounters) { c.Expired++ })
	e.notify(o, &ExecReport{OrderID: o.ID, ClOrdID: o.ClOrdID, OrigClOrdID: o.ClOrdID, Symbol: o.Symbol,
		Side: o.Side, Status: "EXPIRED", FilledQty: o.CumQty, CumValue: o.CumValue, AvgPx: avgOf(o),
		LeavesQty: 0, OrderQty: o.OrigQty, Price: o.Price, Event: "EXPIRE"})
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
	e.Mu.RLock()
	rows := e.marketRowsLocked()
	idx := e.Index
	phase := e.Phase
	e.Mu.RUnlock()
	b, _ := json.Marshal(map[string]any{"type": "market", "index": idx, "phase": phase, "rows": rows, "ts": time.Now().UnixMilli()})
	e.bus.publish(b)
}

func (e *Engine) SnapshotBook(sym string) map[string]any {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	b := e.Book(sym)
	if b == nil {
		return map[string]any{"error": "unknown symbol"}
	}
	tr := []Trade{}
	for i := len(e.Trades) - 1; i >= 0 && len(tr) < 30; i-- {
		if e.Trades[i].Symbol == sym {
			tr = append(tr, e.Trades[i])
		}
	}
	return map[string]any{"symbol": sym, "md": e.MdOf(sym), "bids": b.DepthSide(true, 10), "asks": b.DepthSide(false, 10), "trades": tr}
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

// CategoryOf is the DSE share category: the letter in a board label such as
// "Main (A)", S for the SME board, or "" when the board carries none.
func CategoryOf(board string) string {
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
	rows := make([]marketRow, 0, len(e.Instr()))
	for _, in := range e.Instr() {
		m := MetaFor(in.Symbol)
		if m.Delisted {
			continue
		}
		board := m.Board
		if board == "" {
			board = "MAIN"
		}
		rows = append(rows, marketRow{MarketData: *e.MdOf(in.Symbol), Board: board,
			AssetClass: AssetClassOf(m), Sector: m.Sector, Category: CategoryOf(board), Status: m.StatusOf()})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Symbol < rows[j].Symbol })
	return rows
}

func (e *Engine) MarketRows() []marketRow {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	return e.marketRowsLocked()
}

func Round2(f float64) float64 { return math.Round(f*100) / 100 }

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

// oppositeSide names the side an aggressor needs liquidity from, for reject text.
func oppositeSide(s Side) Side {
	if s == Buy {
		return Sell
	}
	return Buy
}

// ActorStats counts one order source's activity since the engine started.
type ActorStats struct {
	Orders int64   `json:"orders"`
	Trades int64   `json:"trades"` // fills on its orders
	Volume int64   `json:"volume"`
	Value  float64 `json:"value"`
}

// statOf returns the counters for actor. The engine lock is held.
func (e *Engine) statOf(actor string) *ActorStats {
	if e.actorStats == nil {
		e.actorStats = map[string]*ActorStats{}
	}
	st := e.actorStats[actor]
	if st == nil {
		st = &ActorStats{}
		e.actorStats[actor] = st
	}
	return st
}

func (e *Engine) statTrade(actor string, q int64, px float64) {
	e.days.trade(actor, q, px, false)
	st := e.statOf(actor)
	st.Trades++
	st.Volume += q
	st.Value += float64(q) * px
}

// StatsFor sums the counters of the given order sources.
func (e *Engine) StatsFor(actors []string) ActorStats {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	var sum ActorStats
	for _, a := range actors {
		if st := e.actorStats[a]; st != nil {
			sum.Orders += st.Orders
			sum.Trades += st.Trades
			sum.Volume += st.Volume
			sum.Value += st.Value
		}
	}
	return sum
}
