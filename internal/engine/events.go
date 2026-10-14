package engine

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// The market-event journal is the exchange's public record of what happened:
// orders added to and removed from the book, executions, trading-state changes
// and session events. The ITCH feed turns it, one for one, into A / E / C / D / H
// / S messages, so the feed shows the same order numbers the book really holds
// (instead of re-publishing the depth as made-up orders).
//
// Events are appended by the matching goroutine while it holds the engine lock,
// so the journal order is the book order. The journal is a fixed-size ring: a
// reader that falls further behind than its capacity loses its place and must
// start again from a snapshot of the book (SnapshotOrders).

// EvKind is the kind of market event.
type EvKind uint8

const (
	EvAdd         EvKind = iota + 1 // order added to the book (visible quantity)
	EvExec                          // execution against a resting order (continuous trading)
	EvExecAuction                   // execution of a resting order in a call auction (both sides)
	EvDelete                        // order removed from the book
	EvState                         // trading state of an order book: Code 'T' tradable, 'V' suspended
	EvSession                       // market-wide session event: Code is the ITCH system event code
	EvClose                         // closing price of an order book
	EvList                          // a new order book was listed: the feed sends its directory
	EvRef                           // new reference (previous close) price, at the start of a trading day
)

// Event is one market event. Prices are raw ITCH prices (price x 100).
type Event struct {
	Seq      uint64
	Kind     EvKind
	Side     byte // 'B' or 'S' (EvAdd)
	Code     byte // EvState / EvSession
	OrderID  uint64
	Match    uint64
	Qty      int64
	OB       int32
	PriceRaw uint32
	At       int64 // unix nanoseconds
}

// Journal is a bounded, sequence-numbered ring of events.
type Journal struct {
	mu   sync.RWMutex
	buf  []Event
	next uint64 // sequence of the next event; the first event is 1
	wake chan struct{}
}

// NewJournal returns a journal that keeps the most recent capacity events.
func NewJournal(capacity int) *Journal {
	if capacity < 1024 {
		capacity = 1024
	}
	return &Journal{buf: make([]Event, capacity), next: 1, wake: make(chan struct{})}
}

// journalCapacity is ME_JOURNAL_CAP (events kept in memory), default 400000.
func journalCapacity() int {
	if n, err := strconv.Atoi(os.Getenv("ME_JOURNAL_CAP")); err == nil && n > 0 {
		return n
	}
	return 400000
}

// Append stamps ev with the next sequence number and wakes any waiting reader.
func (j *Journal) Append(ev Event) uint64 {
	j.mu.Lock()
	ev.Seq = j.next
	if ev.At == 0 {
		ev.At = time.Now().UnixNano()
	}
	j.buf[(ev.Seq-1)%uint64(len(j.buf))] = ev
	j.next++
	wake := j.wake
	j.wake = make(chan struct{})
	j.mu.Unlock()
	close(wake)
	return ev.Seq
}

// Next is the sequence the next appended event will get.
func (j *Journal) Next() uint64 {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.next
}

// Oldest is the oldest sequence the ring still holds.
func (j *Journal) Oldest() uint64 {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if c := uint64(len(j.buf)); j.next > c {
		return j.next - c
	}
	return 1
}

// Wait returns a channel that is closed when an event is appended after the call.
func (j *Journal) Wait() <-chan struct{} {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.wake
}

// Read returns up to max events starting at sequence from, and the sequence to
// ask for next. ok is false when from is older than the ring still holds.
func (j *Journal) Read(from uint64, max int) (evs []Event, next uint64, ok bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	capacity := uint64(len(j.buf))
	if j.next > capacity && from < j.next-capacity {
		return nil, from, false
	}
	if from < 1 {
		from = 1
	}
	end := j.next
	if int(end-from) > max {
		end = from + uint64(max)
	}
	for s := from; s < end; s++ {
		evs = append(evs, j.buf[(s-1)%capacity])
	}
	return evs, end, true
}

// Journal is the engine's market-event journal.
func (e *Engine) Journal() *Journal { return e.journal }

func sideByte(s Side) byte {
	if s == Sell {
		return 'S'
	}
	return 'B'
}

func (e *Engine) emitAdd(o *Order) {
	e.journal.Append(Event{Kind: EvAdd, Side: sideByte(o.Side), OrderID: o.ID, Qty: o.Qty,
		OB: int32(e.OrderBookID(o.Symbol)), PriceRaw: uint32(px2k(o.Price))})
}

func (e *Engine) emitDelete(o *Order) {
	e.journal.Append(Event{Kind: EvDelete, OrderID: o.ID, OB: int32(e.OrderBookID(o.Symbol))})
}

func (e *Engine) emitExec(kind EvKind, o *Order, qty int64, match uint64) {
	e.journal.Append(Event{Kind: kind, OrderID: o.ID, Qty: qty, Match: match,
		OB: int32(e.OrderBookID(o.Symbol)), PriceRaw: uint32(px2k(o.Price))})
}

// PublishTradingState records the current trading state of a symbol (tradable or
// suspended) so the feed can send an ITCH trading-action message. Call it after
// changing a symbol's status.
func (e *Engine) PublishTradingState(sym string) {
	code := byte('T')
	if !MetaFor(sym).Tradable() {
		code = 'V'
	}
	if ob := e.OrderBookID(sym); ob != 0 {
		e.journal.Append(Event{Kind: EvState, Code: code, OB: int32(ob)})
	}
}

// BookOrder is a resting order in a snapshot of the book.
type BookOrder struct {
	ID       uint64
	Side     byte
	Qty      int64 // visible quantity
	OB       int32
	PriceRaw uint32
}

// SnapshotOrders returns every resting order (visible quantity only) together with
// the session phase and the journal sequence the snapshot is consistent with: replay the journal from
// that sequence to stay in step. It takes the engine read lock.
func (e *Engine) SnapshotOrders() (orders []BookOrder, next uint64, phase string) {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	for _, in := range e.Instr() {
		b := e.Book(in.Symbol)
		if b == nil {
			continue
		}
		ob := int32(e.OrderBookID(in.Symbol))
		add := func(side byte, pxs []int64, m map[int64]*PriceLevel) {
			for _, k := range pxs {
				for _, o := range m[k].Orders {
					orders = append(orders, BookOrder{ID: o.ID, Side: side, Qty: o.Qty, OB: ob, PriceRaw: uint32(k)})
				}
			}
		}
		add('B', b.bidPx, b.bids)
		add('S', b.askPx, b.asks)
	}
	return orders, e.journal.Next(), e.Phase
}

// SessionCode maps a session phase to the ITCH system-event code the feed sends
// when the market enters it (0 = no event).
func SessionCode(phase string) byte {
	switch phase {
	case "Pre-Open":
		return 'S'
	case "Continuous Trading":
		return 'Q'
	case "Close":
		return 'M'
	case "Post-Close":
		return 'V'
	case "Closed":
		return 'E'
	}
	return 0
}

// Reject texts the DSE uses; the FIX gateway prefixes them with the DSE error
// number, e.g. "(293): Orders are not being accepted at this time".
const (
	ReasonNotAccepting    = "Orders are not being accepted at this time"
	ReasonNotClosePrice   = "Order price is not equal to close price"
	ReasonPriceNotAllowed = "Price is not allowed for market order"
)

// CurrentPhase is the session phase.
func (e *Engine) CurrentPhase() string {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	return e.Phase
}

// LastPrice is a symbol's last traded price (its reference price before any trade).
func (e *Engine) LastPrice(sym string) (float64, bool) {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	md := e.MdOf(sym)
	if md == nil {
		return 0, false
	}
	return md.Last, true
}
