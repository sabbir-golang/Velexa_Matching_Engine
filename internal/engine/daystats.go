package engine

import (
	"sync"
	"time"
)

// Daily report counters: for every trading day (Dhaka date), per order source
// (actor: "FIX:<CompID>", "USER:<name>") and for the whole exchange (actor
// ExchangeActor). The engine counts as events happen; the server saves the
// figures (database or file) every few seconds, so past days can be reported
// and a restart continues today's counts.

// ExchangeActor keys the exchange-wide totals. A trade counts once here, but
// once for each side in the per-actor rows.
const ExchangeActor = "*"

// DayCounters are one day's figures for one actor (or the exchange).
type DayCounters struct {
	BuyOrders  int64   `json:"buy_orders"`  // new orders received, buy side
	SellOrders int64   `json:"sell_orders"` // new orders received, sell side
	Accepted   int64   `json:"accepted"`    // received and not rejected
	Rejected   int64   `json:"rejected"`    // refused on arrival (any reason)
	Cancelled  int64   `json:"cancelled"`   // cancelled by the broker
	Amended    int64   `json:"amended"`     // amended (cancel/replace) by the broker
	Expired    int64   `json:"expired"`     // ended unexecuted: Day orders at the close, GTD at expiry
	Trades     int64   `json:"trades"`      // executions
	Volume     int64   `json:"volume"`      // shares traded
	Turnover   float64 `json:"turnover"`    // value traded (Tk)
}

// Add adds o to c.
func (c *DayCounters) Add(o DayCounters) {
	c.BuyOrders += o.BuyOrders
	c.SellOrders += o.SellOrders
	c.Accepted += o.Accepted
	c.Rejected += o.Rejected
	c.Cancelled += o.Cancelled
	c.Amended += o.Amended
	c.Expired += o.Expired
	c.Trades += o.Trades
	c.Volume += o.Volume
	c.Turnover = Round2(c.Turnover + o.Turnover)
}

// DaySnapshot is one day's counters, keyed by actor.
type DaySnapshot struct {
	Day    string
	Actors map[string]DayCounters
}

type dayStats struct {
	mu       sync.Mutex
	day      string
	by       map[string]*DayCounters
	dirty    bool
	finished []DaySnapshot // past days not yet handed to the saver
	now      func() time.Time
}

// ReportDay is the Dhaka date of t ("2006-01-02"), the key of the daily report.
func ReportDay(t time.Time) string { return t.In(Dhaka).Format("2006-01-02") }

// counterLocked returns actor's counters for today, starting a new day when the
// Dhaka date has changed. Caller holds d.mu.
func (d *dayStats) counterLocked(actor string) *DayCounters {
	now := time.Now
	if d.now != nil {
		now = d.now
	}
	if day := ReportDay(now()); day != d.day {
		if d.day != "" && d.dirty {
			d.finished = append(d.finished, d.snapshotLocked())
		}
		d.day, d.by, d.dirty = day, map[string]*DayCounters{}, false
	}
	c := d.by[actor]
	if c == nil {
		c = &DayCounters{}
		d.by[actor] = c
	}
	d.dirty = true
	return c
}

func (d *dayStats) snapshotLocked() DaySnapshot {
	m := make(map[string]DayCounters, len(d.by))
	for k, v := range d.by {
		m[k] = *v
	}
	return DaySnapshot{Day: d.day, Actors: m}
}

// order applies fn to the actor's and the exchange's counters.
func (d *dayStats) order(actor string, fn func(*DayCounters)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	fn(d.counterLocked(ExchangeActor))
	if actor != "" {
		fn(d.counterLocked(actor))
	}
}

// trade adds one side of a trade to an actor's counters (not the exchange's:
// recordTrade counts each trade once there).
func (d *dayStats) trade(actor string, q int64, px float64, exchange bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	add := func(c *DayCounters) {
		c.Trades++
		c.Volume += q
		c.Turnover = Round2(c.Turnover + float64(q)*px)
	}
	if exchange {
		add(d.counterLocked(ExchangeActor))
	}
	if actor != "" {
		add(d.counterLocked(actor))
	}
}

func received(side Side) func(*DayCounters) {
	return func(c *DayCounters) {
		if side == Sell {
			c.SellOrders++
		} else {
			c.BuyOrders++
		}
	}
}

// countReply counts a new order's first reply: received (by side), then
// accepted or rejected.
func (e *Engine) countReply(o *Order, rep *ExecReport) {
	rejected := rep == nil || rep.Status == "REJECTED"
	e.days.order(o.Actor, func(c *DayCounters) {
		received(o.Side)(c)
		if rejected {
			c.Rejected++
		} else {
			c.Accepted++
		}
	})
}

// CountReject records a new order refused before it reached the engine (a FIX
// message the gateway could not accept, an /api/order request it refused).
func (e *Engine) CountReject(actor string, side Side) {
	e.days.order(actor, func(c *DayCounters) {
		received(side)(c)
		c.Rejected++
	})
}

// DayReport returns the live counters when day is the current trading day
// (ok false for any other day: those come from the saved history).
func (e *Engine) DayReport(day string) (map[string]DayCounters, bool) {
	d := &e.days
	d.mu.Lock()
	defer d.mu.Unlock()
	if day != ReportDay(time.Now()) {
		return nil, false
	}
	if d.day != day {
		return map[string]DayCounters{}, true
	}
	return d.snapshotLocked().Actors, true
}

// SeedDay restores the counters saved for today, so a restart during the day
// continues counting instead of starting again from zero.
func (e *Engine) SeedDay(day string, actors map[string]DayCounters) {
	d := &e.days
	d.mu.Lock()
	defer d.mu.Unlock()
	if day != ReportDay(time.Now()) || len(actors) == 0 {
		return
	}
	if d.day != day {
		d.day, d.by = day, map[string]*DayCounters{}
	}
	for actor, c := range actors {
		cur := d.by[actor]
		if cur == nil {
			cur = &DayCounters{}
			d.by[actor] = cur
		}
		cur.Add(c)
	}
}

// PendingDays hands the saver every day whose counters changed since the last
// call: finished days first, then today.
func (e *Engine) PendingDays() []DaySnapshot {
	d := &e.days
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.finished
	d.finished = nil
	if d.dirty && d.day != "" {
		out = append(out, d.snapshotLocked())
		d.dirty = false
	}
	return out
}
