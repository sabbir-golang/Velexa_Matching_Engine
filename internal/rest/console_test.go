package rest

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/marketstate"
	"velexa-sim-me/internal/database/orderlog"
	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
)

// The console's Trade Terminal, Order Report detail, Market Session and
// Diagnostics pages: their server side.
func TestConsolePages(t *testing.T) {
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
	srv.Eng.SetPhase(engine.PhaseOpen)
	decode := func(t *testing.T, body string) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatalf("not JSON: %s", body)
		}
		return m
	}
	order := func(c *http.Cookie, o map[string]any) map[string]any {
		return decode(t, e.do(http.MethodPost, "/api/order", c, o).Body.String())
	}

	// Trade Terminal: validity (TIF) and cancelling one's own order.
	if r := order(alice, map[string]any{"symbol": "GP", "side": "SELL", "type": "LIMIT", "price": 331.2, "qty": 10, "tif": "IOC", "cl_ord_id": "A-IOC"}); r["status"] != "CANCELED" {
		t.Fatalf("an IOC order with nothing to trade against must be cancelled, got %v", r)
	}
	if r := order(alice, map[string]any{"symbol": "GP", "side": "SELL", "type": "LIMIT", "price": 331.2, "qty": 100, "cl_ord_id": "A-1"}); r["status"] != "NEW" {
		t.Fatalf("resting order: %v", r)
	}
	cancel := func(c *http.Cookie, cl string) map[string]any {
		return decode(t, e.do(http.MethodPost, "/api/order/cancel", c, map[string]string{"cl_ord_id": cl}).Body.String())
	}
	if r := cancel(adm, "A-1"); r["ok"] != false {
		t.Fatalf("the admin cancelled alice's order through her Trade Terminal route: %v", r)
	}
	if r := order(adm, map[string]any{"symbol": "GP", "side": "BUY", "type": "LIMIT", "price": 331.2, "qty": 40, "cl_ord_id": "B-1"}); r["status"] != "FILLED" {
		t.Fatalf("crossing order: %v", r)
	}
	if r := cancel(alice, "A-1"); r["ok"] != true {
		t.Fatalf("alice cancels her own order: %v", r)
	}
	if r := cancel(alice, "A-1"); r["ok"] != false {
		t.Fatalf("a second cancel must fail: %v", r)
	}

	// Order Report detail: the fills of one order, for its owner only.
	rows := decode(t, e.do(http.MethodGet, "/admin/api/report/orders?broker=USER:alice", adm, nil).Body.String())["rows"].([]any)
	var a1 map[string]any
	for _, r := range rows {
		if r.(map[string]any)["cl_ord_id"] == "A-1" {
			a1 = r.(map[string]any)
		}
	}
	if a1 == nil || a1["status"] != "Cancelled" || a1["updated"] == "" {
		t.Fatalf("A-1 in the order report: %v", a1)
	}
	q := "/admin/api/report/order-history?order_id=" + a1["order_id"].(string) + "&broker_id=USER:alice"
	h := decode(t, e.do(http.MethodGet, q, alice, nil).Body.String())
	fills, _ := h["fills"].([]any)
	if h["ok"] != true || len(fills) != 1 {
		t.Fatalf("A-1 history: %v", h)
	}
	if f := fills[0].(map[string]any); f["qty"].(float64) != 40 || f["price"].(float64) != 331.2 || f["side"] != "Sell" || f["against"] != "Console (admin)" {
		t.Fatalf("A-1 fill: %v", f)
	}
	if h := decode(t, e.do(http.MethodGet, strings.Replace(q, "USER:alice", "USER:admin", 1), alice, nil).Body.String()); h["ok"] != false {
		t.Fatalf("alice read the admin's order history: %v", h)
	}

	// Market Session: a phase change by hand is audited with its reason.
	if r := decode(t, e.do(http.MethodPost, "/admin/api/trading-session?action=close&note=drill", adm, nil).Body.String()); r["phase"] != engine.PhaseClose {
		t.Fatalf("close: %v", r)
	}
	found := false
	for _, a := range audit.Recent(20) {
		if a.Action == "SESSION CLOSE" && strings.Contains(a.Target, "drill") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the phase change is not in the audit trail: %v", audit.Recent(5))
	}

	// Diagnostics: admins only; the self-test checks the live engine.
	if c := e.do(http.MethodGet, "/admin/api/diagnostics", alice, nil).Code; c != http.StatusForbidden {
		t.Fatalf("a user reached diagnostics: %d", c)
	}
	d := decode(t, e.do(http.MethodGet, "/admin/api/diagnostics?test=1", adm, nil).Body.String())
	checks, _ := d["checks"].([]any)
	if d["ok"] != true || len(checks) < 3 || d["config"] == nil || d["runtime"] == nil {
		t.Fatalf("diagnostics: %v", d)
	}
	for _, c := range checks {
		c := c.(map[string]any)
		if strings.HasPrefix(c["name"].(string), "Order books") && c["ok"] != true {
			t.Fatalf("book check: %v", c)
		}
	}
}
