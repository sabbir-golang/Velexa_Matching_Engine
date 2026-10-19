package admin

import (
	"encoding/csv"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Daily trading report: orders received (buy/sell), accepted, rejected,
// cancelled, amended, expired, trades executed, volume and turnover for one
// trading day (Dhaka date). Today's figures are live; past days come from the
// saved history. An admin sees the whole exchange; a user sees only their own
// brokers and their own Trade Terminal orders.

// reportDay reads ?date=YYYY-MM-DD (default: today); a bad or future date is an error.
func reportDay(r *http.Request) (string, error) {
	today := engine.ReportDay(time.Now())
	d := strings.TrimSpace(r.URL.Query().Get("date"))
	if d == "" {
		return today, nil
	}
	if _, err := time.Parse("2006-01-02", d); err != nil {
		return "", fmt.Errorf("date must be YYYY-MM-DD")
	}
	if d > today {
		return "", fmt.Errorf("date %s is in the future", d)
	}
	return d, nil
}

// dayFigures returns the request's view of day: the exchange totals for an
// admin, the sum of the user's own order sources otherwise.
func (h *Handler) dayFigures(r *http.Request, day string) (c engine.DayCounters, scope string, live bool, err error) {
	c, scope, live, _, err = h.dayFiguresAny(r, day)
	return
}

// dayFiguresAny is dayFigures that also reports whether anything was recorded
// for the day at all (hasData false: no trading activity, or a date before the
// history begins).
func (h *Handler) dayFiguresAny(r *http.Request, day string) (c engine.DayCounters, scope string, live, hasData bool, err error) {
	actors, live := h.Eng.DayReport(day)
	if !live {
		if h.Reports == nil {
			return c, "", false, false, fmt.Errorf("no report history is kept")
		}
		if actors, err = h.Reports.Load(day); err != nil {
			return c, "", false, false, err
		}
	}
	hasData = len(actors) > 0
	all, own := middleware.BrokerScope(r)
	if all {
		return actors[engine.ExchangeActor], "exchange", live, hasData, nil
	}
	me := "USER:" + middleware.Username(r)
	for actor, v := range actors {
		if actor == me || (strings.HasPrefix(actor, "FIX:") && own[strings.ToUpper(actor[4:])]) {
			c.Add(v)
		}
	}
	return c, "own", live, hasData, nil
}

type metric struct {
	key, label string
	value      any
}

// reportMetrics lists the report's lines in the order they are shown.
func reportMetrics(c engine.DayCounters) []metric {
	orders := c.BuyOrders + c.SellOrders
	pct := func(n int64) float64 {
		if orders == 0 {
			return 0
		}
		return engine.Round2(float64(n) * 100 / float64(orders))
	}
	avgSize, avgPx := 0.0, 0.0
	if c.Trades > 0 {
		avgSize = engine.Round2(float64(c.Volume) / float64(c.Trades))
	}
	if c.Volume > 0 {
		avgPx = engine.Round2(c.Turnover / float64(c.Volume))
	}
	return []metric{
		{"orders", "Orders received", orders},
		{"buy_orders", "Buy orders", c.BuyOrders},
		{"sell_orders", "Sell orders", c.SellOrders},
		{"accepted", "Accepted", c.Accepted},
		{"rejected", "Rejected", c.Rejected},
		{"reject_rate", "Reject rate (%)", pct(c.Rejected)},
		{"cancelled", "Cancelled", c.Cancelled},
		{"amended", "Amended", c.Amended},
		{"expired", "Expired", c.Expired},
		{"trades", "Trades executed", c.Trades},
		{"volume", "Volume (shares)", c.Volume},
		{"turnover", "Turnover (Tk)", engine.Round2(c.Turnover)},
		{"avg_trade_size", "Average trade size (shares)", avgSize},
		{"avg_price", "Average price (Tk)", avgPx},
	}
}

// DailyReport serves /admin/api/report/daily: JSON, or a CSV file with &format=csv.
func (h *Handler) DailyReport(w http.ResponseWriter, r *http.Request) {
	day, err := reportDay(r)
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	c, scope, live, hasData, err := h.dayFiguresAny(r, day)
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "the report for " + day + " could not be read"})
		return
	}
	ms := reportMetrics(c)
	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="daily-report-`+day+`.csv"`)
		cw := csv.NewWriter(w)
		cw.Write([]string{"Daily trading report", day})
		cw.Write([]string{"Scope", scopeLabel(scope, middleware.Username(r))})
		cw.Write([]string{"Generated", time.Now().In(engine.Dhaka).Format("2006-01-02 15:04:05") + " (Dhaka)"})
		cw.Write(nil)
		cw.Write([]string{"Metric", "Value"})
		for _, m := range ms {
			cw.Write([]string{m.label, fmt.Sprint(m.value)})
		}
		cw.Flush()
		return
	}
	totals := map[string]any{}
	for _, m := range ms {
		totals[m.key] = m.value
	}
	out := map[string]any{"ok": true, "date": day, "today": day == engine.ReportDay(time.Now()),
		"live": live, "has_data": hasData || live, "scope": scope, "totals": totals}
	if h.Reports != nil {
		if days, err := h.Reports.Days(90); err == nil {
			out["days"] = days
		}
	}
	utils.WriteJSON(w, out)
}

func scopeLabel(scope, user string) string {
	if scope == "exchange" {
		return "Whole exchange"
	}
	return "Brokers of " + user
}

// DailyReportPage serves /admin/report/daily: the report as a printable page
// (the browser's Print saves it as PDF).
func (h *Handler) DailyReportPage(w http.ResponseWriter, r *http.Request) {
	day, err := reportDay(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c, scope, live, err := h.dayFigures(r, day)
	if err != nil {
		http.Error(w, "the report for "+day+" could not be read", http.StatusInternalServerError)
		return
	}
	e := html.EscapeString
	var rows strings.Builder
	for _, m := range reportMetrics(c) {
		v := fmt.Sprint(m.value)
		if f, ok := m.value.(float64); ok {
			v = strconv.FormatFloat(f, 'f', 2, 64)
		}
		fmt.Fprintf(&rows, "<tr><td>%s</td><td class=n>%s</td></tr>", e(m.label), e(v))
	}
	status := "Final figures"
	if live {
		status = "Live figures — the trading day is not over"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>Daily trading report %s</title>
<style>
body{font:14px/1.45 system-ui,-apple-system,"Segoe UI",sans-serif;color:#1b2330;margin:32px auto;max-width:720px;padding:0 16px}
h1{font-size:22px;margin:0 0 4px}.sub{color:#5b6677;margin-bottom:18px}
table{border-collapse:collapse;width:100%%}td{border-bottom:1px solid #e1e5ec;padding:7px 4px}
td.n{text-align:right;font-variant-numeric:tabular-nums;font-family:ui-monospace,monospace}
tr:nth-child(1) td,tr:nth-child(10) td,tr:nth-child(12) td{font-weight:600}
.meta{margin-top:18px;color:#5b6677;font-size:12px}button{margin-top:18px;padding:7px 14px;cursor:pointer}
@media print{button{display:none}body{margin:0}}
</style></head><body>
<h1>Daily trading report — %s</h1>
<div class="sub">Velexa matching engine · %s · %s</div>
<table>%s</table>
<div class="meta">Generated %s (Dhaka). Orders received = buy + sell = accepted + rejected. Turnover = Σ trade quantity × price.</div>
<button onclick="window.print()">Print / save as PDF</button>
</body></html>`, e(day), e(day), e(scopeLabel(scope, middleware.Username(r))), e(status), rows.String(),
		e(time.Now().In(engine.Dhaka).Format("2006-01-02 15:04:05")))
}

// timeNow is the clock the reports use (a variable so tests can stop it at midnight).
var timeNow = time.Now
