package me

import (
	"sync"
	"time"
)

// RiskEngine runs DSE-style pre-trade risk checks. Each Check is a real
// sequence of factor validations; the caller times how long it takes.
type RiskEngine struct {
	mu           sync.RWMutex
	halted       map[string]bool    // per-symbol kill switch / trading halt
	refPx        map[string]float64 // reference price for the price band
	priceBandPct float64            // e.g. 0.10 = ±10%
	maxOrderQty  int64
	maxOrderVal  float64
	buyingPower  map[string]float64 // per-account cash limit
	posLimit     int64              // per-symbol position cap
}

func NewRiskEngine(refPx map[string]float64) *RiskEngine {
	r := &RiskEngine{
		halted:       map[string]bool{},
		refPx:        map[string]float64{},
		priceBandPct: 0.10,
		maxOrderQty:  1_000_000,
		maxOrderVal:  50_000_000, // 5 crore per order
		buyingPower:  map[string]float64{},
		posLimit:     5_000_000,
	}
	for s, p := range refPx {
		r.refPx[s] = p
	}
	return r
}

// Check returns (pass, rejectReason). Runs the full factor cascade every call.
func (r *RiskEngine) Check(o *Order, acct string) (bool, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1) kill switch / trading halt
	if r.halted[o.Symbol] {
		return false, "TRADING_HALTED"
	}
	// 2) max order quantity
	if o.Qty <= 0 || o.Qty > r.maxOrderQty {
		return false, "QTY_LIMIT"
	}
	// 3) price band (limit orders): |px-ref|/ref must be within band
	ref := r.refPx[o.Symbol]
	if o.Type == Limit && ref > 0 {
		dev := o.Price - ref
		if dev < 0 {
			dev = -dev
		}
		if dev/ref > r.priceBandPct {
			return false, "PRICE_BAND"
		}
	}
	// 4) max order value
	px := o.Price
	if px <= 0 {
		px = ref
	}
	val := float64(o.Qty) * px
	if val > r.maxOrderVal {
		return false, "VALUE_LIMIT"
	}
	// 5) buying power (buys)
	if o.Side == Buy {
		if bp, ok := r.buyingPower[acct]; ok && val > bp {
			return false, "BUYING_POWER"
		}
	}
	// 6) position/exposure limit
	if o.Qty > r.posLimit {
		return false, "POSITION_LIMIT"
	}
	return true, ""
}

// touch keeps the compiler honest about time import if checks evolve.
var _ = time.Now
