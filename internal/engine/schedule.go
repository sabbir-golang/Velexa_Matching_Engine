package engine

// The trading day on the DSE clock (Asia/Dhaka, Sunday to Thursday):
//
//	Closed -> Pre-Open -> Continuous Trading -> Close -> Post-Close -> Closed
//
// Each change publishes the ITCH session event, and the day's edges do the
// exchange's housekeeping: leaving Closed starts a new trading day (yesterday's
// close becomes the reference price and the day's statistics reset), entering
// Close publishes closing prices, and entering Closed expires the day orders.
// The clock can be switched off (ME_SESSION_AUTO=off, or the admin forcing a
// phase) so the phases can be driven by hand.

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// Session phases.
const (
	PhasePreOpen   = "Pre-Open"
	PhaseOpen      = "Continuous Trading"
	PhaseClose     = "Close"
	PhasePostClose = "Post-Close"
	PhaseClosed    = "Closed"
)

// phaseOrder is the order a trading day goes through.
var phaseOrder = []string{PhasePreOpen, PhaseOpen, PhaseClose, PhasePostClose, PhaseClosed}

// Dhaka is the exchange's time zone.
var Dhaka = time.FixedZone("Asia/Dhaka", 6*3600)

// Schedule gives the start of each phase as a time of day (Dhaka).
type Schedule struct {
	PreOpen, Open, Close, PostClose, End time.Duration
}

// String is the schedule in ME_SESSION_SCHEDULE form.
func (sc Schedule) String() string {
	f := func(d time.Duration) string { return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60) }
	return f(sc.PreOpen) + "," + f(sc.Open) + "," + f(sc.Close) + "," + f(sc.PostClose) + "," + f(sc.End)
}

// DefaultSchedule follows the OMS's trading hours (10:00-14:30): pre-open from
// 09:30, post-close 14:40-15:00. Change it with ME_SESSION_SCHEDULE.
const DefaultSchedule = "09:30,10:00,14:30,14:40,15:00"

// ParseSchedule reads "HH:MM,HH:MM,HH:MM,HH:MM,HH:MM": the start of pre-open,
// continuous trading, close, post-close and the end of the day, in order.
func ParseSchedule(s string) (Schedule, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 5 {
		return Schedule{}, fmt.Errorf("schedule %q: want 5 times (pre-open, open, close, post-close, end)", s)
	}
	var ts [5]time.Duration
	for i, p := range parts {
		t, err := time.Parse("15:04", strings.TrimSpace(p))
		if err != nil {
			return Schedule{}, fmt.Errorf("schedule %q: %v", s, err)
		}
		ts[i] = time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute
		if i > 0 && ts[i] <= ts[i-1] {
			return Schedule{}, fmt.Errorf("schedule %q: times must increase", s)
		}
	}
	return Schedule{ts[0], ts[1], ts[2], ts[3], ts[4]}, nil
}

// ScheduleFromEnv is ME_SESSION_SCHEDULE, or the default when unset or invalid.
func ScheduleFromEnv() Schedule {
	if v := strings.TrimSpace(os.Getenv("ME_SESSION_SCHEDULE")); v != "" {
		if sc, err := ParseSchedule(v); err == nil {
			return sc
		} else {
			log.Printf("[session] %v — using %s", err, DefaultSchedule)
		}
	}
	sc, _ := ParseSchedule(DefaultSchedule)
	return sc
}

// TradingDay reports whether the DSE trades on t's date (Sunday to Thursday).
func TradingDay(t time.Time) bool {
	wd := t.In(Dhaka).Weekday()
	return wd != time.Friday && wd != time.Saturday
}

// PhaseAt is the phase the schedule puts the market in at t.
func (sc Schedule) PhaseAt(t time.Time) string {
	t = t.In(Dhaka)
	if !TradingDay(t) {
		return PhaseClosed
	}
	since := t.Sub(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Dhaka))
	switch {
	case since < sc.PreOpen || since >= sc.End:
		return PhaseClosed
	case since < sc.Open:
		return PhasePreOpen
	case since < sc.Close:
		return PhaseOpen
	case since < sc.PostClose:
		return PhaseClose
	}
	return PhasePostClose
}

var autoClock atomic.Bool

// ScheduleText is the schedule the clock follows ("" before StartClock).
func (e *Engine) ScheduleText() string {
	if e.schedule == (Schedule{}) {
		return ""
	}
	return e.schedule.String()
}

// AutoClock reports whether the schedule drives the phases.
func (e *Engine) AutoClock() bool { return autoClock.Load() }

// SetAutoClock switches the schedule on or off.
func (e *Engine) SetAutoClock(on bool) { autoClock.Store(on) }

// ForcePhase moves the market to phase by hand; it switches the clock off so the
// schedule does not move it back.
func (e *Engine) ForcePhase(phase string) string {
	e.SetAutoClock(false)
	return e.AdvanceTo(phase)
}

// AdvanceTo moves the market forward through the trading day until it reaches
// phase, so every step's housekeeping runs (for example after the machine slept
// through the close). An unknown phase is set as it is.
func (e *Engine) AdvanceTo(phase string) string {
	target := indexOf(phase)
	if target < 0 {
		return e.SetPhase(phase)
	}
	cur := indexOf(e.CurrentPhase())
	if cur < 0 || cur == target {
		return e.SetPhase(phase)
	}
	for i := 0; i < len(phaseOrder) && cur != target; i++ {
		cur = (cur + 1) % len(phaseOrder)
		e.SetPhase(phaseOrder[cur])
	}
	return e.CurrentPhase()
}

func indexOf(phase string) int {
	for i, p := range phaseOrder {
		if p == phase {
			return i
		}
	}
	return -1
}

// StartClock runs the schedule: the market is moved to the scheduled phase now
// and whenever the schedule says so, while the clock is on.
func (e *Engine) StartClock(sc Schedule) {
	e.schedule = sc
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ME_SESSION_AUTO")), "off") {
		log.Printf("[session] clock off (ME_SESSION_AUTO=off): phases change only by hand")
		e.SetAutoClock(false)
	} else {
		e.SetAutoClock(true)
	}
	go func() {
		for {
			if e.AutoClock() {
				if want := sc.PhaseAt(time.Now()); want != e.CurrentPhase() {
					log.Printf("[session] %s -> %s (clock)", e.CurrentPhase(), want)
					e.AdvanceTo(want)
				}
			}
			time.Sleep(2 * time.Second)
		}
	}()
}

// onPhase does the housekeeping of entering phase from prev. It runs on the
// matching goroutine with the engine lock held.
func (e *Engine) onPhase(prev, phase string) {
	switch {
	case prev == PhaseClosed && phase != PhaseClosed:
		e.newTradingDay()
	}
	switch phase {
	case PhaseOpen:
		if prev == PhasePreOpen {
			for _, in := range e.Instr() {
				e.uncross(in.Symbol)
				e.triggerStops(in.Symbol)
			}
		}
	case PhaseClose:
		e.publishCloses()
	case PhaseClosed:
		e.expireDayOrders()
		e.journal.Append(Event{Kind: EvSession, Code: 'C'}) // end of messages
	}
}

// newTradingDay rolls the market data over: yesterday's close becomes the
// reference price and the day's figures reset. The feed sends the new
// reference prices.
func (e *Engine) newTradingDay() {
	for _, in := range e.Instr() {
		md := e.MdOf(in.Symbol)
		if md == nil {
			continue
		}
		if md.Trades > 0 && md.Last > 0 {
			md.Prev = md.Last
		}
		md.Open, md.High, md.Low, md.Last, md.Ref = md.Prev, md.Prev, md.Prev, md.Prev, md.Prev
		md.Volume, md.Trades, md.Change, md.ChangePct = 0, 0, 0, 0
		if ob := e.OrderBookID(in.Symbol); ob != 0 {
			e.journal.Append(Event{Kind: EvRef, OB: int32(ob), PriceRaw: uint32(px2k(md.Prev))})
		}
		e.refreshTop(in.Symbol)
	}
	e.tradingDate = time.Now().In(Dhaka).Format("2006-01-02")
}

// publishCloses records each instrument's closing price (its last trade).
func (e *Engine) publishCloses() {
	for _, in := range e.Instr() {
		md := e.MdOf(in.Symbol)
		if md == nil || md.Trades == 0 {
			continue
		}
		if ob := e.OrderBookID(in.Symbol); ob != 0 {
			e.journal.Append(Event{Kind: EvClose, OB: int32(ob), PriceRaw: uint32(px2k(md.Last)), Match: e.TradeSeq})
		}
	}
}

// expireDayOrders removes the day orders left at the end of the day and tells
// their owners (150=C). GTC and GTD orders stay.
func (e *Engine) expireDayOrders() {
	for _, in := range e.Instr() {
		b := e.Book(in.Symbol)
		if b == nil {
			continue
		}
		removed := b.removeWhere(func(o *Order) bool { return o.TIF == "" || o.TIF == Day })
		for _, o := range removed {
			e.forget(o)
			e.emitDelete(o)
			e.expired(o)
		}
		if len(removed) > 0 {
			e.refreshTop(in.Symbol)
		}
	}
	for sym, pend := range e.stops { // parked day stops expire too
		kept := pend[:0]
		for _, s := range pend {
			if s.TIF == "" || s.TIF == Day {
				e.expired(s)
				continue
			}
			kept = append(kept, s)
		}
		e.stops[sym] = kept
	}
}
