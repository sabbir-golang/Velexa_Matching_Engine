package admin

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/fixgw"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Dashboard overview: everything the console's dashboard shows for one trading
// day, in one answer — the session clock, the day's totals against the
// previous trading day, an intraday chart, what became of the orders, the
// instruments, recent activity, the brokers and the listeners. An admin sees
// the whole exchange; a user sees their own brokers (prices and the
// instrument table are public market data and the same for everyone).

type ovBucket struct {
	Start    string  `json:"start"` // HH:MM, Dhaka
	Turnover float64 `json:"turnover"`
	Trades   int64   `json:"trades"`
	Volume   int64   `json:"volume"`
}

// ovInstrument is a row of the instrument table, in the columns of the DSE's
// "Latest Share Price" page: LTP, HIGH, LOW, YCP, CHANGE (LTP - YCP, Tk),
// TRADE, VALUE and VOLUME. Values the day does not have are null: HIGH and LOW
// before the first trade, YCP and CHANGE for a past day (not kept).
type ovInstrument struct {
	Symbol    string   `json:"symbol"`
	Last      float64  `json:"last"` // LTP: the last traded price, or YCP before the first trade
	High      *float64 `json:"high"`
	Low       *float64 `json:"low"`
	YCP       *float64 `json:"ycp"`        // yesterday's closing price
	Change    *float64 `json:"change"`     // LTP - YCP, Tk
	ChangePct *float64 `json:"change_pct"` // (LTP - YCP) / YCP, %
	Trades    int64    `json:"trades"`
	Volume    int64    `json:"volume"`
	Turnover  float64  `json:"turnover"` // VALUE, Tk
}

type ovEvent struct {
	At     time.Time `json:"-"`
	Time   string    `json:"time"` // HH:MM:SS, Dhaka
	Kind   string    `json:"kind"` // trade | order | session
	Tag    string    `json:"tag"`
	Text   string    `json:"text"`
	Broker string    `json:"broker"`
}

type ovBroker struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Turnover float64 `json:"turnover"` // value traded, its buys and sells
	Trades   int64   `json:"trades"`
	Orders   int     `json:"orders"`
	Online   bool    `json:"online"`
	IsFIX    bool    `json:"fix"`
}

// visibleActor says whether the request may see an order source's activity.
func visibleActor(r *http.Request) func(actor string) bool {
	all, own := middleware.BrokerScope(r)
	me := "USER:" + middleware.Username(r)
	return func(actor string) bool {
		if all || actor == me {
			return true
		}
		s, ok := strings.CutPrefix(actor, "FIX:")
		return ok && own[strings.ToUpper(s)]
	}
}

func hhmm(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// sinceMidnight is t's time of day in Dhaka.
func sinceMidnight(t time.Time) time.Duration {
	t = t.In(engine.Dhaka)
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second
}

// dayTrades are a day's executions: from the trade log, or — when the market
// is not saved — the engine's recent tape.
func (h *Handler) dayTrades(day string) []engine.TradeRecord {
	if h.State != nil {
		if t, err := h.State.Trades(day); err == nil {
			return t
		}
	}
	h.Eng.Mu.RLock()
	defer h.Eng.Mu.RUnlock()
	var out []engine.TradeRecord
	for _, t := range h.Eng.Trades {
		if engine.ReportDay(t.Time) == day {
			out = append(out, engine.TradeRecord{Trade: t})
		}
	}
	return out
}

// Overview serves /admin/api/overview?date=YYYY-MM-DD (default today).
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	day, err := reportDay(r)
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	now := time.Now()
	today := day == engine.ReportDay(now)
	visible := visibleActor(r)
	all, own := middleware.BrokerScope(r)

	out := map[string]any{"ok": true, "date": day, "today": today, "scope": "exchange",
		"now": now.In(engine.Dhaka).Format("15:04:05")}
	if !all {
		out["scope"], out["my_brokers"] = "own", len(own)
	}
	weekday, _ := time.ParseInLocation("2006-01-02", day, engine.Dhaka)
	out["weekday"] = weekday.Format("Monday, 2 Jan 2006")

	// The session clock.
	sc := h.Eng.SessionSchedule()
	out["phase"], out["auto_clock"] = h.Eng.CurrentPhase(), h.Eng.AutoClock()
	out["schedule"] = map[string]string{"pre_open": hhmm(sc.PreOpen), "open": hhmm(sc.Open), "close": hhmm(sc.Close),
		"post_close": hhmm(sc.PostClose), "end": hhmm(sc.End)}
	out["trading_day"] = engine.TradingDay(weekday)

	// The day's totals and the previous trading day's.
	c, _, _, hasData, err := h.dayFiguresAny(r, day)
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "the figures for " + day + " could not be read"})
		return
	}
	out["totals"] = c
	out["has_data"] = hasData || today
	if h.Reports != nil {
		if days, err := h.Reports.Days(30); err == nil {
			sort.Sort(sort.Reverse(sort.StringSlice(days)))
			for _, d := range days {
				if d >= day {
					continue
				}
				if pc, _, _, ok, err := h.dayFiguresAny(r, d); err == nil && ok && pc.BuyOrders+pc.SellOrders+pc.Trades > 0 {
					out["prev"] = map[string]any{"date": d, "totals": pc}
					break
				}
			}
		}
	}

	trades := h.dayTrades(day)
	var feed []ovEvent
	brokers := map[string]*ovBroker{}
	broker := func(actor string) *ovBroker {
		b := brokers[actor]
		if b == nil {
			id, label := brokerLabel(actor)
			b = &ovBroker{ID: id, Name: label, IsFIX: strings.HasPrefix(actor, "FIX:")}
			brokers[actor] = b
		}
		return b
	}

	// Intraday buckets: the trading hours, widened to any trade outside them
	// (a market opened by hand), 10 minutes each — wider if the day is long.
	from, to := sc.Open, sc.Close
	var mine []engine.TradeRecord
	for _, t := range trades {
		if all || visible(t.Buy.Actor) || visible(t.Sell.Actor) {
			mine = append(mine, t)
		}
	}
	var first, last time.Time
	for _, t := range mine {
		if first.IsZero() || t.Time.Before(first) {
			first = t.Time
		}
		if t.Time.After(last) {
			last = t.Time
		}
	}
	if !first.IsZero() { // the span trading actually happened in, for the trade rate
		out["first_trade"], out["last_trade"] = first.In(engine.Dhaka).Format("15:04:05"), last.In(engine.Dhaka).Format("15:04:05")
	}
	for _, t := range mine {
		if m := sinceMidnight(t.Time); m < from {
			from = m
		} else if m >= to {
			to = m + time.Minute
		}
	}
	step := 10 * time.Minute
	for (to-from)/step > 48 {
		step += 10 * time.Minute
	}
	from = from.Truncate(step)
	n := int((to - from + step - 1) / step)
	if n < 1 {
		n = 1
	}
	buckets := make([]ovBucket, n)
	for i := range buckets {
		buckets[i].Start = hhmm(from + time.Duration(i)*step)
	}
	for _, t := range mine {
		i := int((sinceMidnight(t.Time) - from) / step)
		if i < 0 || i >= n {
			continue
		}
		v := float64(t.Qty) * t.Price
		buckets[i].Trades++
		buckets[i].Volume += t.Qty
		buckets[i].Turnover += v
	}
	out["buckets"], out["bucket_minutes"], out["buckets_end"] = buckets, int(step.Minutes()), hhmm(from+time.Duration(n)*step)
	out["now_bucket"] = -1
	if today {
		out["now_bucket"] = int((sinceMidnight(now) - from) / step)
	}

	// Instruments: today's prices and day figures from the market, the value
	// traded from the trade log (every trade: the tape is public).
	type agg struct {
		trades, vol      int64
		to, last, hi, lo float64
		at               time.Time
	}
	bySym := map[string]*agg{}
	for _, t := range trades {
		a := bySym[t.Symbol]
		if a == nil {
			a = &agg{}
			bySym[t.Symbol] = a
		}
		a.trades++
		a.vol += t.Qty
		a.to += float64(t.Qty) * t.Price
		if a.trades == 1 || t.Price > a.hi {
			a.hi = t.Price
		}
		if a.trades == 1 || t.Price < a.lo {
			a.lo = t.Price
		}
		if !t.Time.Before(a.at) { // the latest trade by time (backfilled trades have later numbers)
			a.last, a.at = t.Price, t.Time
		}
	}
	instr := []ovInstrument{}
	if today {
		for _, m := range h.Eng.MarketRows() {
			row := ovInstrument{Symbol: m.Symbol, Last: m.Last, Trades: m.Trades, Volume: m.Volume}
			ycp, chg, pct := m.Prev, m.Change, m.ChangePct
			row.YCP, row.Change, row.ChangePct = &ycp, &chg, &pct
			if m.Trades > 0 {
				hi, lo := m.High, m.Low
				row.High, row.Low = &hi, &lo
			}
			if h.State != nil { // the day's figures from the trade log, like the turnover
				row.Trades, row.Volume, row.High, row.Low = 0, 0, nil, nil
			}
			if a := bySym[m.Symbol]; a != nil {
				row.Turnover = engine.Round2(a.to)
				if h.State != nil {
					hi, lo := a.hi, a.lo
					row.Trades, row.Volume, row.High, row.Low = a.trades, a.vol, &hi, &lo
				}
			}
			instr = append(instr, row)
		}
	} else {
		for sym, a := range bySym {
			hi, lo := a.hi, a.lo
			instr = append(instr, ovInstrument{Symbol: sym, Last: a.last, High: &hi, Low: &lo, Trades: a.trades, Volume: a.vol,
				Turnover: engine.Round2(a.to)})
		}
	}
	sort.Slice(instr, func(i, j int) bool {
		if instr[i].Turnover != instr[j].Turnover {
			return instr[i].Turnover > instr[j].Turnover
		}
		return instr[i].Symbol < instr[j].Symbol
	})
	out["instruments"] = instr

	// Trades: the activity feed and each broker's value traded.
	for _, t := range mine {
		v := float64(t.Qty) * t.Price
		for _, a := range []string{t.Buy.Actor, t.Sell.Actor} {
			if a != "" && visible(a) {
				b := broker(a)
				b.Turnover += v
				b.Trades++
			}
		}
		side := func(actor string) string {
			if actor == "" { // not recorded (a trade from before the trade log kept both sides)
				return "not recorded"
			}
			_, label := brokerLabel(actor)
			return label
		}
		who := side(t.Buy.Actor) + " ← " + side(t.Sell.Actor)
		feed = append(feed, ovEvent{At: t.Time, Kind: "trade", Tag: "TRADE", Broker: who,
			Text: t.Symbol + " " + strconv.FormatInt(t.Qty, 10) + " @ " + strconv.FormatFloat(t.Price, 'f', 2, 64)})
	}

	// Orders: what became of them, and the feed.
	outcomes := map[string]int{"filled": 0, "working": 0, "cancelled": 0, "rejected": 0, "expired": 0}
	if h.Orders != nil {
		if recs, err := h.Orders.List(day); err == nil {
			for _, rec := range recs {
				if !visible(rec.Actor) {
					continue
				}
				broker(rec.Actor).Orders++
				tag := ""
				switch rec.Status {
				case "Filled":
					outcomes["filled"]++
					tag = "FILLED"
				case "New", "Partial", "Pending":
					outcomes["working"]++
					tag = map[string]string{"New": "NEW", "Partial": "PARTIAL", "Pending": "STOP"}[rec.Status]
				case "Cancelled":
					outcomes["cancelled"]++
					tag = "CANCEL"
				case "Rejected":
					outcomes["rejected"]++
					tag = "REJECT"
				case "Expired":
					outcomes["expired"]++
					tag = "EXPIRED"
				}
				text := strings.ToUpper(rec.Side) + " " + rec.Symbol + " " + strconv.FormatInt(rec.Qty, 10)
				if rec.Price > 0 {
					text += " @ " + strconv.FormatFloat(rec.Price, 'f', 2, 64)
				}
				if rec.Status == "Rejected" && rec.Reason != "" {
					text += " — " + rec.Reason
				}
				at := rec.Updated
				if at.IsZero() {
					at = rec.Entered
				}
				_, label := brokerLabel(rec.Actor)
				feed = append(feed, ovEvent{At: at, Kind: "order", Tag: tag, Text: text, Broker: label})
			}
		}
	}
	out["outcomes"] = outcomes

	// FIX sessions: logons and logouts of the day, and who is connected.
	sessions := 0
	if fixgw.Active != nil {
		for b := range fixgw.Active.ActiveSessions() {
			if visible("FIX:" + strings.ToUpper(b)) {
				sessions++
				if today {
					broker("FIX:" + b).Online = true
				}
			}
		}
		for _, ev := range fixgw.Active.RecentEvents(200) {
			if engine.ReportDay(ev.Time) != day || !visible("FIX:"+strings.ToUpper(ev.Broker)) {
				continue
			}
			_, label := brokerLabel("FIX:" + ev.Broker)
			feed = append(feed, ovEvent{At: ev.Time, Kind: "session", Tag: ev.Kind, Text: "FIX " + strings.ToLower(ev.Detail), Broker: label})
		}
	}
	sort.SliceStable(feed, func(i, j int) bool { return feed[i].At.After(feed[j].At) })
	if len(feed) > 60 {
		feed = feed[:60]
	}
	for i := range feed {
		feed[i].Time = feed[i].At.In(engine.Dhaka).Format("15:04:05")
	}
	if feed == nil {
		feed = []ovEvent{}
	}
	out["feed"] = feed

	bl := make([]*ovBroker, 0, len(brokers))
	for _, b := range brokers {
		bl = append(bl, b)
	}
	sort.Slice(bl, func(i, j int) bool {
		if bl[i].Turnover != bl[j].Turnover {
			return bl[i].Turnover > bl[j].Turnover
		}
		if bl[i].Orders != bl[j].Orders {
			return bl[i].Orders > bl[j].Orders
		}
		return bl[i].Name < bl[j].Name
	})
	for _, b := range bl {
		b.Turnover = engine.Round2(b.Turnover)
	}
	out["brokers"] = bl

	// Connectivity.
	desc := map[string]string{"FIX": "Broker order entry", "FIXS": "TLS FIX gateway", "Gateway": "Console & REST API",
		"ITCH": "Market data feed"}
	type listener struct {
		Name string `json:"name"`
		Desc string `json:"desc"`
		Up   bool   `json:"up"`
	}
	ls := []listener{}
	for name, up := range listeners() {
		kind, _, _ := strings.Cut(name, " ")
		ls = append(ls, listener{Name: name, Desc: desc[kind], Up: up})
	}
	order := map[string]int{"FIX": 0, "FIXS": 1, "Gateway": 2, "ITCH": 3}
	sort.Slice(ls, func(i, j int) bool {
		a, _, _ := strings.Cut(ls[i].Name, " ")
		b, _, _ := strings.Cut(ls[j].Name, " ")
		return order[a] < order[b]
	})
	out["listeners"], out["sessions"] = ls, sessions
	out["instruments_listed"] = len(h.Eng.Instr())
	out["uptime"] = time.Since(meStart).Round(time.Second).String()
	out["engine"] = "velexa-sim-me " + config.Version
	utils.WriteJSON(w, out)
}
