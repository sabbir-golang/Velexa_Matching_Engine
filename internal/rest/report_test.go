package rest

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"velexa-sim-me/internal/database/reports"
	"velexa-sim-me/internal/engine"
)

func TestDailyReport(t *testing.T) {
	e := newRoleEnv(t)
	e.srv.Reports = reports.NewFileStore(t.TempDir() + "/daily.json")
	e.srv.Eng.SetPhase(engine.PhaseOpen)
	adm, alice := e.login(adminName, adminPW), e.login("alice", "Rabbit-Hole9")
	// alice registers a broker; activity on it is hers, the rest is the exchange's.
	e.do(http.MethodPost, "/admin/api/broker/add", alice, map[string]string{"Sender": "ALICEBRK"})
	defer e.do(http.MethodPost, "/admin/api/broker/del", adm, map[string]string{"Sender": "ALICEBRK"})
	eng := e.srv.Eng
	place := func(actor, cl string, side engine.Side, px float64, qty int64) {
		o := eng.NewOrder("GP", side, engine.Limit, px, qty, actor)
		o.ClOrdID = cl
		eng.SubmitAndWait(o)
	}
	place("FIX:ALICEBRK", "A1", engine.Buy, 331.2, 100) // accepted, rests
	place("FIX:OTHER", "O1", engine.Sell, 331.2, 40)    // trades 40 with alice
	place("FIX:OTHER", "O2", engine.Sell, 900, 10)      // rejected: price band
	eng.CountReject("FIX:ALICEBRK", engine.Sell)        // refused by the gateway

	get := func(c *http.Cookie, q string) map[string]any {
		var out map[string]any
		rec := e.do(http.MethodGet, "/admin/api/report/daily"+q, c, nil)
		json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	tot := func(m map[string]any, k string) float64 { return m["totals"].(map[string]any)[k].(float64) }

	ex := get(adm, "")
	if ex["scope"] != "exchange" || ex["live"] != true || tot(ex, "orders") != 4 || tot(ex, "buy_orders") != 1 ||
		tot(ex, "sell_orders") != 3 || tot(ex, "rejected") != 2 || tot(ex, "trades") != 1 || tot(ex, "volume") != 40 ||
		tot(ex, "turnover") != 13248 || tot(ex, "reject_rate") != 50 {
		t.Fatalf("exchange report: %v", ex)
	}
	own := get(alice, "")
	if own["scope"] != "own" || tot(own, "orders") != 2 || tot(own, "rejected") != 1 || tot(own, "trades") != 1 {
		t.Fatalf("alice sees: %v", own)
	}

	// CSV and printable page.
	rec := e.do(http.MethodGet, "/admin/api/report/daily?format=csv", adm, nil)
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "daily-report-") ||
		!strings.Contains(rec.Body.String(), "Trades executed,1") || !strings.Contains(rec.Body.String(), "Turnover (Tk),13248") {
		t.Fatalf("csv: %q", rec.Body.String())
	}
	page := e.do(http.MethodGet, "/admin/report/daily", alice, nil).Body.String()
	if !strings.Contains(page, "Brokers of alice") || !strings.Contains(page, "Live figures") {
		t.Fatal("printable page")
	}

	// A past day comes from the saved history.
	for _, d := range eng.PendingDays() {
		e.srv.Reports.Save(d.Day, d.Actors)
	}
	e.srv.Reports.Save("2026-01-05", map[string]engine.DayCounters{engine.ExchangeActor: {Trades: 7, BuyOrders: 9}})
	past := get(adm, "?date=2026-01-05")
	if past["live"] != false || tot(past, "trades") != 7 || tot(past, "orders") != 9 {
		t.Fatalf("past day: %v", past)
	}
	if none := get(adm, "?date=2025-06-01"); none["has_data"] != false || none["ok"] != true {
		t.Fatalf("a day without figures: %v", none)
	}
	if days, _ := past["days"].([]any); len(days) != 2 {
		t.Fatalf("days with figures: %v", past["days"])
	}
	for _, bad := range []string{"?date=2026-13-40", "?date=" + time.Now().AddDate(0, 0, 2).Format("2006-01-02")} {
		if r := get(adm, bad); r["ok"] != false {
			t.Errorf("%s accepted: %v", bad, r)
		}
	}
}
