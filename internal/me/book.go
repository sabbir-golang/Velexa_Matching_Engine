package me

import (
	"sort"
	"time"
)

// High-throughput limit order book: price levels in a map keyed by integer
// ticks (price*100), plus a sorted price ladder per side (best first). Insert
// and match are O(number of price levels) at worst — not O(total orders) — so
// it sustains tens of thousands of orders/sec on a single matching goroutine.
type plevel struct {
	px     int64
	orders []*Order // FIFO (front = oldest → price-time priority)
	qty    int64
}

type Book struct {
	Symbol string
	bids   map[int64]*plevel
	asks   map[int64]*plevel
	bidPx  []int64 // sorted DESC (best bid first)
	askPx  []int64 // sorted ASC  (best ask first)
}

func newBook(sym string) *Book {
	return &Book{Symbol: sym, bids: map[int64]*plevel{}, asks: map[int64]*plevel{}}
}

func px2k(p float64) int64 { return int64(p*100 + 0.5) }
func k2px(k int64) float64 { return float64(k) / 100 }

func (b *Book) rest(o *Order) {
	k := px2k(o.Price)
	if o.Side == Buy {
		lv := b.bids[k]
		if lv == nil {
			lv = &plevel{px: k}
			b.bids[k] = lv
			i := sort.Search(len(b.bidPx), func(i int) bool { return b.bidPx[i] < k })
			b.bidPx = append(b.bidPx, 0)
			copy(b.bidPx[i+1:], b.bidPx[i:])
			b.bidPx[i] = k
		}
		lv.orders = append(lv.orders, o)
		lv.qty += o.Qty
	} else {
		lv := b.asks[k]
		if lv == nil {
			lv = &plevel{px: k}
			b.asks[k] = lv
			i := sort.Search(len(b.askPx), func(i int) bool { return b.askPx[i] > k })
			b.askPx = append(b.askPx, 0)
			copy(b.askPx[i+1:], b.askPx[i:])
			b.askPx[i] = k
		}
		lv.orders = append(lv.orders, o)
		lv.qty += o.Qty
	}
}

func (b *Book) bestBid() *plevel {
	if len(b.bidPx) == 0 {
		return nil
	}
	return b.bids[b.bidPx[0]]
}
func (b *Book) bestAsk() *plevel {
	if len(b.askPx) == 0 {
		return nil
	}
	return b.asks[b.askPx[0]]
}
func (b *Book) dropBestBid() { delete(b.bids, b.bidPx[0]); b.bidPx = b.bidPx[1:] }
func (b *Book) dropBestAsk() { delete(b.asks, b.askPx[0]); b.askPx = b.askPx[1:] }

// popFront removes the filled front order of a level once its qty is 0.
func (lv *plevel) popFront() { lv.orders = lv.orders[1:] }

func (b *Book) depthSide(buy bool, n int) []Level {
	out := []Level{}
	pxs := b.askPx
	m := b.asks
	if buy {
		pxs = b.bidPx
		m = b.bids
	}
	for i := 0; i < len(pxs) && len(out) < n; i++ {
		lv := m[pxs[i]]
		if lv.qty <= 0 {
			continue
		}
		out = append(out, Level{Price: k2px(lv.px), Qty: lv.qty, Orders: len(lv.orders)})
	}
	return out
}

// cancelActor removes all of an actor's resting orders (used by the market
// maker on re-quote). Rare relative to matching, so a full sweep is fine.
func (b *Book) cancelActor(actor string) {
	sweep := func(pxs []int64, m map[int64]*plevel) []int64 {
		kept := pxs[:0]
		for _, k := range pxs {
			lv := m[k]
			out := lv.orders[:0]
			lv.qty = 0
			for _, o := range lv.orders {
				if o.Actor != actor {
					out = append(out, o)
					lv.qty += o.Qty
				}
			}
			lv.orders = out
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
			if side == Buy && k2px(k) > price {
				break
			}
			if side == Sell && k2px(k) < price {
				break
			}
		}
		avail += m[k].qty
		if avail >= wantQty {
			return wantQty
		}
	}
	return avail
}

// cancelExpired removes resting GTD orders whose Expire has passed and returns
// them. Modeled on cancelActor (full sweep — expiry ticks are rare).
func (b *Book) cancelExpired(now time.Time) []*Order {
	var removed []*Order
	sweep := func(pxs []int64, m map[int64]*plevel) []int64 {
		kept := pxs[:0]
		for _, k := range pxs {
			lv := m[k]
			out := lv.orders[:0]
			lv.qty = 0
			for _, o := range lv.orders {
				if o.TIF == GTD && !o.Expire.IsZero() && now.After(o.Expire) {
					removed = append(removed, o)
					continue // drop the expired order
				}
				out = append(out, o)
				lv.qty += o.Qty
			}
			lv.orders = out
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

func (b *Book) removeFrom(o *Order, k int64, m map[int64]*plevel, pxs *[]int64) {
	lv := m[k]
	if lv == nil {
		return
	}
	out := lv.orders[:0]
	lv.qty = 0
	for _, x := range lv.orders {
		if x == o {
			continue
		}
		out = append(out, x)
		lv.qty += x.Qty
	}
	lv.orders = out
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
