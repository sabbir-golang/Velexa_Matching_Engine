package engine

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Saving and restoring the live market, so a restart (an update, a crash) loses
// no trading data: every resting order in its queue position, parked stops, each
// instrument's prices and day figures, the trade tape, the order and trade
// counters, and the session phase. The server saves a snapshot every second and
// at shutdown (internal/database/marketstate) and restores it before the
// matching loop starts. Bid and ask follow from the restored book.

// SnapshotVersion is bumped when the snapshot layout changes incompatibly.
const SnapshotVersion = 1

// SnapOrder is a resting order or a parked stop as saved.
type SnapOrder struct {
	ID       uint64          `json:"id"`
	Symbol   string          `json:"symbol"`
	Side     Side            `json:"side"`
	Type     OrdType         `json:"type"`
	Price    float64         `json:"price"`
	Qty      int64           `json:"qty"`    // visible quantity on the book
	Hidden   int64           `json:"hidden"` // iceberg reserve
	Display  int64           `json:"display"`
	OrigQty  int64           `json:"orig_qty"`
	Actor    string          `json:"actor"`
	ClOrdID  string          `json:"cl_ord_id"`
	TIF      TIF             `json:"tif"`
	Expire   time.Time       `json:"expire,omitzero"`
	StopPx   float64         `json:"stop_px,omitempty"`
	Time     time.Time       `json:"time"`
	CumQty   int64           `json:"cum_qty"`
	CumValue float64         `json:"cum_value"`
	Info     OrderInfo       `json:"info"`
	CtxKind  string          `json:"ctx_kind,omitempty"` // which gateway's context (e.g. "FIX")
	Ctx      json.RawMessage `json:"ctx,omitempty"`
}

// SnapMarket is one instrument's market data as saved.
type SnapMarket struct {
	Symbol string  `json:"symbol"`
	Last   float64 `json:"last"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Prev   float64 `json:"prev"`
	Ref    float64 `json:"ref"`
	Volume int64   `json:"volume"`
	Trades int64   `json:"trades"`
}

// Snapshot is the whole live market at one moment.
type Snapshot struct {
	Version   int          `json:"version"`
	Taken     time.Time    `json:"taken"`
	Phase     string       `json:"phase"`
	AutoClock bool         `json:"auto_clock"`
	Seq       uint64       `json:"seq"`
	TradeSeq  uint64       `json:"trade_seq"`
	Orders    []SnapOrder  `json:"orders"` // per book: bids best→worst, then asks; each level oldest first
	Stops     []SnapOrder  `json:"stops"`
	Markets   []SnapMarket `json:"markets"`
	Trades    []Trade      `json:"trades"` // the tape (newest last)
}

// CtxSaver is implemented by an order context that can be saved with the
// order (the FIX gateway's), so a restored order still reports to its broker.
type CtxSaver interface {
	CtxKind() string
	SaveCtx() (json.RawMessage, error)
}

var (
	ctxMu       sync.RWMutex
	ctxDecoders = map[string]func(json.RawMessage) (any, error){}
)

// RegisterCtxDecoder lets a gateway turn its saved order contexts back into
// live ones when the market is restored.
func RegisterCtxDecoder(kind string, decode func(json.RawMessage) (any, error)) {
	ctxMu.Lock()
	ctxDecoders[kind] = decode
	ctxMu.Unlock()
}

func snapOrder(o *Order) SnapOrder {
	s := SnapOrder{ID: o.ID, Symbol: o.Symbol, Side: o.Side, Type: o.Type, Price: o.Price, Qty: o.Qty,
		Hidden: o.Hidden, Display: o.Display, OrigQty: o.OrigQty, Actor: o.Actor, ClOrdID: o.ClOrdID,
		TIF: o.TIF, Expire: o.Expire, StopPx: o.StopPx, Time: o.Time, CumQty: o.CumQty, CumValue: o.CumValue,
		Info: o.Info}
	if cs, ok := o.Ctx.(CtxSaver); ok {
		if raw, err := cs.SaveCtx(); err == nil {
			s.CtxKind, s.Ctx = cs.CtxKind(), raw
		}
	}
	return s
}

func (s SnapOrder) order() *Order {
	o := &Order{ID: s.ID, Symbol: s.Symbol, Side: s.Side, Type: s.Type, Price: s.Price, Qty: s.Qty,
		Hidden: s.Hidden, Display: s.Display, OrigQty: s.OrigQty, Actor: s.Actor, ClOrdID: s.ClOrdID,
		TIF: s.TIF, Expire: s.Expire, StopPx: s.StopPx, Time: s.Time, CumQty: s.CumQty, CumValue: s.CumValue,
		Info: s.Info}
	if s.CtxKind != "" {
		ctxMu.RLock()
		decode := ctxDecoders[s.CtxKind]
		ctxMu.RUnlock()
		if decode != nil {
			if c, err := decode(s.Ctx); err == nil {
				o.Ctx = c
			} else {
				log.Printf("[state] order %d: its %s context could not be restored: %v", s.ID, s.CtxKind, err)
			}
		}
	}
	return o
}

// MarkDirty records that the market changed since the last snapshot.
func (e *Engine) MarkDirty() { e.dirty.Store(true) }

// TakeSnapshot copies the live market when it changed since the last call
// (force: always). It holds the engine's read lock only while copying, so it
// never waits for, or holds up, more than one matching step.
func (e *Engine) TakeSnapshot(force bool) *Snapshot {
	if !e.dirty.Swap(false) && !force {
		return nil
	}
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	s := &Snapshot{Version: SnapshotVersion, Taken: time.Now(), Phase: e.Phase, AutoClock: e.AutoClock(),
		Seq: e.Seq, TradeSeq: e.TradeSeq, Orders: []SnapOrder{}, Stops: []SnapOrder{}}
	for _, in := range e.Instr() {
		if b := e.Book(in.Symbol); b != nil {
			for _, side := range []struct {
				pxs []int64
				m   map[int64]*PriceLevel
			}{{b.bidPx, b.bids}, {b.askPx, b.asks}} {
				for _, k := range side.pxs {
					for _, o := range side.m[k].Orders {
						s.Orders = append(s.Orders, snapOrder(o))
					}
				}
			}
		}
		for _, st := range e.stops[in.Symbol] {
			s.Stops = append(s.Stops, snapOrder(st))
		}
		if md := e.MdOf(in.Symbol); md != nil {
			s.Markets = append(s.Markets, SnapMarket{Symbol: md.Symbol, Last: md.Last, Open: md.Open, High: md.High,
				Low: md.Low, Prev: md.Prev, Ref: md.Ref, Volume: md.Volume, Trades: md.Trades})
		}
	}
	s.Trades = append([]Trade(nil), e.Trades...)
	return s
}

// RestoreResult says what Restore put back.
type RestoreResult struct {
	Orders, Stops, Markets, Trades int
	Dropped                        int      // orders of instruments that are no longer listed
	Restored                       []*Order // the restored working orders (for the order log)
	Stale                          bool     // the snapshot is from an earlier trading day
}

// Restore puts a saved market back. Call it before Run. Bid/ask, change and the
// index follow from the restored book and prices.
func (e *Engine) Restore(s *Snapshot) (RestoreResult, error) {
	var r RestoreResult
	if s == nil {
		return r, nil
	}
	if s.Version != SnapshotVersion {
		return r, fmt.Errorf("snapshot version %d, this engine reads %d", s.Version, SnapshotVersion)
	}
	e.Mu.Lock()
	defer e.Mu.Unlock()
	for _, m := range s.Markets {
		md := e.MdOf(m.Symbol)
		if md == nil {
			continue
		}
		md.Last, md.Open, md.High, md.Low, md.Prev, md.Ref = m.Last, m.Open, m.High, m.Low, m.Prev, m.Ref
		md.Volume, md.Trades = m.Volume, m.Trades
		r.Markets++
	}
	for _, so := range s.Orders {
		b := e.Book(so.Symbol)
		if b == nil {
			r.Dropped++
			continue
		}
		o := so.order()
		b.Rest(o) // in saved order: the queue position (time priority) is kept
		if o.ClOrdID != "" {
			e.Open[openKey(o.Actor, o.ClOrdID)] = o
		}
		r.Restored = append(r.Restored, o)
		r.Orders++
	}
	for _, so := range s.Stops {
		if e.Book(so.Symbol) == nil {
			r.Dropped++
			continue
		}
		o := so.order()
		e.stops[o.Symbol] = append(e.stops[o.Symbol], o)
		r.Restored = append(r.Restored, o)
		r.Stops++
	}
	e.Trades = append(e.Trades[:0], s.Trades...)
	r.Trades = len(s.Trades)
	if s.Seq > e.Seq {
		e.Seq = s.Seq
	}
	if s.TradeSeq > e.TradeSeq {
		e.TradeSeq = s.TradeSeq
	}
	if s.Phase != "" {
		e.Phase = s.Phase
	}
	auto := s.AutoClock
	e.restoredAuto = &auto
	for _, in := range e.Instr() {
		e.refreshTop(in.Symbol)
	}
	e.recomputeIndex()
	r.Stale = ReportDay(s.Taken) != ReportDay(time.Now())
	e.staleDay = r.Stale
	return r, nil
}

// FinishStaleDay ends the trading day of a snapshot taken on an earlier day
// (the engine was down over the close): it runs the end-of-day steps — closing
// prices, expiry of day orders — so the clock starts the new day cleanly. Call
// it after Run has started and before StartClock.
func (e *Engine) FinishStaleDay() bool {
	if !e.staleDay {
		return false
	}
	e.staleDay = false
	if e.CurrentPhase() != PhaseClosed {
		log.Printf("[state] the saved market is from an earlier day — closing that day (%s -> %s)", e.CurrentPhase(), PhaseClosed)
		e.AdvanceTo(PhaseClosed)
	}
	return true
}

// AttachNotify gives the restored working orders of actors starting with
// prefix (e.g. "FIX:") a report callback again, once their gateway runs.
func (e *Engine) AttachNotify(prefix string, notify func(*ExecReport)) int {
	res := make(chan int, 1)
	e.ctrl <- func() {
		n := 0
		attach := func(o *Order) {
			if o.Notify == nil && strings.HasPrefix(o.Actor, prefix) {
				o.Notify = notify
				n++
			}
		}
		for _, in := range e.Instr() {
			if b := e.Book(in.Symbol); b != nil {
				for _, lv := range b.bids {
					for _, o := range lv.Orders {
						attach(o)
					}
				}
				for _, lv := range b.asks {
					for _, o := range lv.Orders {
						attach(o)
					}
				}
			}
			for _, s := range e.stops[in.Symbol] {
				attach(s)
			}
		}
		res <- n
	}
	return <-res
}

// TradeSide is one side of an execution.
type TradeSide struct {
	Actor   string `json:"actor"`
	OrderID uint64 `json:"order_id"`
	ClOrdID string `json:"cl_ord_id"`
}

// TradeRecord is an execution with both sides, for the permanent trade log.
type TradeRecord struct {
	Trade
	Buy  TradeSide `json:"buy"`
	Sell TradeSide `json:"sell"`
}

// tradeOut hands an execution to the trade log (OnTrade), if one is attached.
func (e *Engine) tradeOut(tr Trade, buy, sell *Order) {
	if e.OnTrade == nil {
		return
	}
	side := func(o *Order) TradeSide { return TradeSide{Actor: o.Actor, OrderID: o.ID, ClOrdID: o.ClOrdID} }
	e.OnTrade(TradeRecord{Trade: tr, Buy: side(buy), Sell: side(sell)})
}

// SetMinTradeSeq makes the next trade IDs larger than n (call before Run).
func (e *Engine) SetMinTradeSeq(n uint64) {
	e.Mu.Lock()
	if e.TradeSeq < n {
		e.TradeSeq = n
	}
	e.Mu.Unlock()
}
