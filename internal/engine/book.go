package engine

import (
	"sort"
	"time"
)

// High-throughput limit order book: price levels in a map keyed by integer
// ticks (price*100), plus a sorted price ladder per side (best first). Insert
// and match are O(number of price levels) at worst — not O(total orders) — so
// it sustains tens of thousands of orders/sec on a single matching goroutine.
type PriceLevel struct {
	Px     int64
	Orders []*Order // FIFO (front = oldest → price-time priority)
	Qty    int64
}

type Book struct {
	Symbol string
	bids   map[int64]*PriceLevel
	asks   map[int64]*PriceLevel
	bidPx  []int64 // sorted DESC (best bid first)
	askPx  []int64 // sorted ASC  (best ask first)
}

func NewBook(sym string) *Book {
	return &Book{Symbol: sym, bids: map[int64]*PriceLevel{}, asks: map[int64]*PriceLevel{}}
}

func px2k(p float64) int64 { return int64(p*100 + 0.5) }

func K2Px(k int64) float64 { return float64(k) / 100 }

func (b *Book) Rest(o *Order) {
	k := px2k(o.Price)
	if o.Side == Buy {
		lv := b.bids[k]
		if lv == nil {
			lv = &PriceLevel{Px: k}
			b.bids[k] = lv
			i := sort.Search(len(b.bidPx), func(i int) bool { return b.bidPx[i] < k })
			b.bidPx = append(b.bidPx, 0)
			copy(b.bidPx[i+1:], b.bidPx[i:])
			b.bidPx[i] = k
		}
		lv.Orders = append(lv.Orders, o)
		lv.Qty += o.Qty
	} else {
		lv := b.asks[k]
		if lv == nil {
			lv = &PriceLevel{Px: k}
			b.asks[k] = lv
			i := sort.Search(len(b.askPx), func(i int) bool { return b.askPx[i] > k })
			b.askPx = append(b.askPx, 0)
			copy(b.askPx[i+1:], b.askPx[i:])
			b.askPx[i] = k
		}
		lv.Orders = append(lv.Orders, o)
		lv.Qty += o.Qty
	}
}

func (b *Book) BestBid() *PriceLevel {
	if len(b.bidPx) == 0 {
		return nil
	}
	return b.bids[b.bidPx[0]]
}

func (b *Book) BestAsk() *PriceLevel {
	if len(b.askPx) == 0 {
		return nil
	}
	return b.asks[b.askPx[0]]
}

func (b *Book) DropBestBid() { delete(b.bids, b.bidPx[0]); b.bidPx = b.bidPx[1:] }

func (b *Book) DropBestAsk() { delete(b.asks, b.askPx[0]); b.askPx = b.askPx[1:] }

// PopFront removes the filled front order of a level once its qty is 0.
func (lv *PriceLevel) PopFront() { lv.Orders = lv.Orders[1:] }

func (b *Book) DepthSide(buy bool, n int) []Level {
	out := []Level{}
	pxs := b.askPx
	m := b.asks
	if buy {
		pxs = b.bidPx
		m = b.bids
	}
	for i := 0; i < len(pxs) && len(out) < n; i++ {
		lv := m[pxs[i]]
		if lv.Qty <= 0 {
			continue
		}
		out = append(out, Level{Price: K2Px(lv.Px), Qty: lv.Qty, Orders: len(lv.Orders)})
	}
	return out
}

// cancelActor removes all of an actor's resting orders (used by the market
// maker on re-quote). Rare relative to matching, so a full sweep is fine.
func (b *Book) cancelActor(actor string) []*Order {
	var removed []*Order
	sweep := func(pxs []int64, m map[int64]*PriceLevel) []int64 {
		kept := pxs[:0]
		for _, k := range pxs {
			lv := m[k]
			out := lv.Orders[:0]
			lv.Qty = 0
			for _, o := range lv.Orders {
				if o.Actor != actor {
					out = append(out, o)
					lv.Qty += o.Qty
				} else {
					removed = append(removed, o)
				}
			}
			lv.Orders = out
			if len(out) == 0 {
				delete(m, k)
			} else {
				kept = append(kept, k)
			}
		}
		return kept
	}
	b.bidPx = sweep(b.bidPx, b.bids)
	b.askPx = sweep(b.askPx, b.asks)
	return removed
}

// canFill reports how much of wantQty an incoming order could fill immediately
// against the resting opposite side — WITHOUT mutating the book. Used for the
// Fill-Or-Kill pre-check. For a Limit order, price caps which levels count;
// levels are already best-first, so we can stop at the first non-crossing one.
func (b *Book) canFill(side Side, typ OrdType, price float64, wantQty int64) int64 {
	pxs, m := b.askPx, b.asks // a Buy lifts asks
	if side == Sell {
		pxs, m = b.bidPx, b.bids // a Sell hits bids
	}
	var avail int64
	for _, k := range pxs {
		if typ == Limit {
			if side == Buy && K2Px(k) > price {
				break
			}
			if side == Sell && K2Px(k) < price {
				break
			}
		}
		avail += m[k].Qty
		if avail >= wantQty {
			return wantQty
		}
	}
	return avail
}

// removeWhere removes every resting order for which drop is true and returns them.
func (b *Book) removeWhere(drop func(*Order) bool) []*Order {
	var removed []*Order
	sweep := func(pxs []int64, m map[int64]*PriceLevel) []int64 {
		kept := pxs[:0]
		for _, k := range pxs {
			lv := m[k]
			out := lv.Orders[:0]
			lv.Qty = 0
			for _, o := range lv.Orders {
				if drop(o) {
					removed = append(removed, o)
					continue
				}
				out = append(out, o)
				lv.Qty += o.Qty
			}
			lv.Orders = out
			if len(out) == 0 {
				delete(m, k)
			} else {
				kept = append(kept, k)
			}
		}
		return kept
	}
	b.bidPx = sweep(b.bidPx, b.bids)
	b.askPx = sweep(b.askPx, b.asks)
	return removed
}

// cancelExpired removes resting GTD orders whose Expire has passed and returns
// them. Modeled on cancelActor (full sweep — expiry ticks are rare).
func (b *Book) cancelExpired(now time.Time) []*Order {
	var removed []*Order
	sweep := func(pxs []int64, m map[int64]*PriceLevel) []int64 {
		kept := pxs[:0]
		for _, k := range pxs {
			lv := m[k]
			out := lv.Orders[:0]
			lv.Qty = 0
			for _, o := range lv.Orders {
				if o.TIF == GTD && !o.Expire.IsZero() && now.After(o.Expire) {
					removed = append(removed, o)
					continue // drop the expired order
				}
				out = append(out, o)
				lv.Qty += o.Qty
			}
			lv.Orders = out
			if len(out) == 0 {
				delete(m, k)
			} else {
				kept = append(kept, k)
			}
		}
		return kept
	}
	b.bidPx = sweep(b.bidPx, b.bids)
	b.askPx = sweep(b.askPx, b.asks)
	return removed
}

// remove deletes a specific resting order from its price level (cancel/replace).
func (b *Book) remove(o *Order) {
	k := px2k(o.Price)
	if o.Side == Buy {
		b.removeFrom(o, k, b.bids, &b.bidPx)
	} else {
		b.removeFrom(o, k, b.asks, &b.askPx)
	}
}

func (b *Book) removeFrom(o *Order, k int64, m map[int64]*PriceLevel, pxs *[]int64) {
	lv := m[k]
	if lv == nil {
		return
	}
	out := lv.Orders[:0]
	lv.Qty = 0
	for _, x := range lv.Orders {
		if x == o {
			continue
		}
		out = append(out, x)
		lv.Qty += x.Qty
	}
	lv.Orders = out
	if len(out) == 0 {
		delete(m, k)
		s := *pxs
		for i, kk := range s {
			if kk == k {
				*pxs = append(s[:i], s[i+1:]...)
				break
			}
		}
	}
}
