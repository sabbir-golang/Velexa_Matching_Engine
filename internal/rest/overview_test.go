package rest

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/marketstate"
	"velexa-sim-me/internal/database/orderlog"
	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
)

// The dashboard's data: the whole exchange for an admin, own brokers for a
// user; trades from the trade log with both sides.
func TestOverview(t *testing.T) {
	middleware.ResetLoginThrottle()
	audit.Path = filepath.Join(t.TempDir(), "audit.jsonl")
	rec, err := orderlog.NewRecorder(orderlog.NewFileStore(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var sv *marketstate.Saver
	srv := testServerWith(t, func(eng *engine.Engine) {
		sv = marketstate.NewSaver(eng, marketstate.NewFileStore(dir, dir))
		eng.OnOrderEvent, eng.OnTrade = rec.Observe, sv.OnTrade
	})
	srv.Orders, srv.State = rec, sv
	if err := srv.Users.Add("alice", "Rabbit-Hole9", users.RoleUser); err != nil {
		t.Fatal(err)
	}
	e := &roleEnv{t: t, srv: srv, h: srv.Routes()}
	adm, alice := e.login(adminName, adminPW), e.login("alice", "Rabbit-Hole9")
	e.do(http.MethodPost, "/admin/api/broker/add", alice, map[string]string{"Sender": "ALICEBRK", "Company": "Alice Securities Ltd"})
	defer e.do(http.MethodPost, "/admin/api/broker/del", adm, map[string]string{"Sender": "ALICEBRK"})

	eng := srv.Eng
	eng.SetPhase(engine.PhaseOpen)
	place := func(actor, cl string, side engine.Side, px float64, qty int64) {
		o := eng.NewOrder("GP", side, engine.Limit, px, qty, actor)
		o.ClOrdID = cl
		eng.SubmitAndWait(o)
	}
	place("FIX:ALICEBRK", "A1", engine.Buy, 331.2, 100) // rests, then 40 fill: working
	place("FIX:OTHERBRK", "O1", engine.Sell, 331.2, 40) // filled
	place("FIX:OTHERBRK", "O2", engine.Sell, 999, 10)   // rejected (band)
	place("FIX:OTHERBRK", "O3", engine.Sell, 340, 10)   // rests, then cancelled
	eng.CancelAs("O3", "O3c", "FIX:OTHERBRK")

	get := func(c *http.Cookie) map[string]any {
		var out map[string]any
		if err := json.Unmarshal(e.do(http.MethodGet, "/admin/api/overview", c, nil).Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out["ok"] != true {
			t.Fatalf("overview: %v", out)
		}
		return out
	}
	num := func(m any, k string) float64 { return m.(map[string]any)[k].(float64) }

	all := get(adm)
	if all["scope"] != "exchange" || all["today"] != true || all["phase"] != engine.PhaseOpen {
		t.Fatalf("admin overview: %v", all)
	}
	tot := all["totals"]
	if num(tot, "trades") != 1 || num(tot, "volume") != 40 || num(tot, "turnover") != 13248 || num(tot, "buy_orders") != 1 || num(tot, "sell_orders") != 3 {
		t.Fatalf("totals %v", tot)
	}
	oc := all["outcomes"]
	if num(oc, "filled") != 1 || num(oc, "working") != 1 || num(oc, "rejected") != 1 || num(oc, "cancelled") != 1 {
		t.Fatalf("outcomes %v", oc)
	}
	var bucketTrades float64
	for _, b := range all["buckets"].([]any) {
		bucketTrades += num(b, "trades")
	}
	if bucketTrades != 1 {
		t.Fatalf("the trade is in %v buckets", bucketTrades)
	}
	gp := all["instruments"].([]any)[0].(map[string]any)
	if gp["symbol"] != "GP" || gp["turnover"].(float64) != 13248 || gp["trades"].(float64) != 1 || gp["last"].(float64) != 331.2 ||
		gp["ycp"].(float64) != 331.2 || gp["change"].(float64) != 0 || gp["high"].(float64) != 331.2 || gp["low"].(float64) != 331.2 {
		t.Fatalf("instrument row %v", gp)
	}

	// A trade away from the previous close: CHANGE = LTP - YCP, as on the DSE.
	place("FIX:OTHERBRK", "O4", engine.Sell, 340, 5)
	place("FIX:ALICEBRK", "A2", engine.Buy, 340, 5)
	gp = get(adm)["instruments"].([]any)[0].(map[string]any)
	if gp["last"].(float64) != 340 || gp["ycp"].(float64) != 331.2 || gp["change"].(float64) != 8.8 || gp["change_pct"].(float64) != 2.66 ||
		gp["high"].(float64) != 340 || gp["low"].(float64) != 331.2 {
		t.Fatalf("instrument row after a trade at 340: %v", gp)
	}
	all = get(adm)
	brs := all["brokers"].([]any)
	if len(brs) != 2 || num(brs[0], "turnover") != 13248+1700 {
		t.Fatalf("brokers %v", brs)
	}
	var trade map[string]any
	for _, f := range all["feed"].([]any) {
		if f.(map[string]any)["kind"] == "trade" {
			trade = f.(map[string]any)
		}
	}
	if trade == nil || trade["text"] != "GP 40 @ 331.20" || trade["broker"] != "Alice Securities Ltd ← OTHERBRK" {
		t.Fatalf("trade in the feed: %v", trade)
	}

	mine := get(alice)
	if mine["scope"] != "own" || num(mine["totals"], "buy_orders") != 2 || num(mine["totals"], "sell_orders") != 0 {
		t.Fatalf("alice's overview: %v %v", mine["scope"], mine["totals"])
	}
	if brs := mine["brokers"].([]any); len(brs) != 1 || brs[0].(map[string]any)["name"] != "Alice Securities Ltd" {
		t.Fatalf("alice sees other brokers: %v", brs)
	}
	for _, f := range mine["feed"].([]any) {
		if f.(map[string]any)["kind"] == "order" && f.(map[string]any)["broker"] != "Alice Securities Ltd" {
			t.Fatalf("alice sees another broker's order: %v", f)
		}
	}
	if oc := mine["outcomes"]; num(oc, "working") != 1 || num(oc, "rejected") != 0 {
		t.Fatalf("alice's outcomes %v", oc)
	}

	var bad map[string]any
	json.Unmarshal(e.do(http.MethodGet, "/admin/api/overview?date=2999-01-01", adm, nil).Body.Bytes(), &bad)
	if bad["ok"] != false {
		t.Fatalf("a future date: %v", bad)
	}
}
