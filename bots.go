package main

import (
	"sync/atomic"
	"math/rand"
	"time"
)

// startBots launches, per instrument, a market-maker (posts two-sided resting
// liquidity around a drifting fair value) and a noise trader (sends crossing
// orders that print trades and move the price). Together they produce a live,
// continuously-moving simulated market — the market data the demo needs.
// botsPaused, when 1, silences the market bots (no orders) while keeping
// the goroutines alive so they resume instantly.
var botsPaused int32

func (e *Engine) startBots(seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for i, in := range e.instr() {
		sym := in.Symbol
		go e.marketMaker(sym, rand.New(rand.NewSource(seed+int64(i)+1)))
		go e.noiseTrader(sym, rand.New(rand.NewSource(seed+int64(i)+1000)))
	}
	_ = rng
}

func tickSize(p float64) float64 {
	switch {
	case p >= 200:
		return 0.50
	case p >= 50:
		return 0.20
	case p >= 10:
		return 0.10
	default:
		return 0.05
	}
}

func roundTick(p float64) float64 {
	t := tickSize(p)
	return round2(float64(int64(p/t+0.5)) * t)
}

// marketMaker re-quotes a ladder of bids/asks around the drifting fair value.
func (e *Engine) marketMaker(sym string, rng *rand.Rand) {
	actor := "MM"
	for {
		time.Sleep(time.Duration(800+rng.Intn(700)) * time.Millisecond)
		if atomic.LoadInt32(&botsPaused) == 1 {
			continue
		}
		if !metaFor(sym).tradable() {
			continue
		}
		e.mu.Lock()
		md := e.mdOf(sym)
		ref := md.Ref
		if ref <= 0 {
			ref = md.Prev
		}
		// mean-reverting drift toward prev close + small random walk
		ref += (md.Prev-ref)*0.02 + (rng.NormFloat64() * ref * 0.0009)
		if ref < md.Prev*0.85 {
			ref = md.Prev * 0.85
		}
		if ref > md.Prev*1.15 {
			ref = md.Prev * 1.15
		}
		md.Ref = round2(ref)
		e.book(sym).cancelActor(actor)
		e.mu.Unlock()
		t := tickSize(ref)
		for l := 1; l <= 5; l++ {
			bidPx := roundTick(ref - float64(l)*t)
			askPx := roundTick(ref + float64(l)*t)
			sz := int64((6-l)*100 + rng.Intn(400))
			e.Submit(e.mkOrder(sym, Buy, Limit, bidPx, sz, actor))
			e.Submit(e.mkOrder(sym, Sell, Limit, askPx, sz, actor))
		}
	}
}

// noiseTrader sends occasional marketable orders that consume liquidity → trades.
func (e *Engine) noiseTrader(sym string, rng *rand.Rand) {
	for {
		time.Sleep(time.Duration(600+rng.Intn(1800)) * time.Millisecond)
		if atomic.LoadInt32(&botsPaused) == 1 {
			continue
		}
		if !metaFor(sym).tradable() {
			continue
		}
		side := Buy
		if rng.Float64() < 0.5 {
			side = Sell
		}
		sz := int64(100 + rng.Intn(600))
		e.Submit(e.mkOrder(sym, side, Market, 0, sz, "NOISE"))
	}
}

// mkOrder is a lock-free helper for bots (sequence assigned in process()).
func (e *Engine) mkOrder(sym string, side Side, typ OrdType, price float64, qty int64, actor string) *Order {
	return &Order{Symbol: sym, Side: side, Type: typ, Price: round2(price), Qty: qty, OrigQty: qty, Actor: actor, Time: time.Now()}
}
