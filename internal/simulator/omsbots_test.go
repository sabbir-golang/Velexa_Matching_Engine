package simulator

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"velexa-sim-me/internal/engine"
)

// fakeOMS plays the OMS endpoints the OMS bots use: sign-in, the dealer's
// client list, the order channel (it answers like the OMS passing on the
// exchange's reports) and the position stream. C001 holds 500 GP.
type fakeOMS struct {
	srv *httptest.Server
	mfa bool

	mu        sync.Mutex
	frames    []map[string]any
	sizes     []int
	tenant    string
	wsQuery   url.Values
	origin    string
	loggedOut bool
}

const fakeBO1, fakeBO2 = "1201950000000001", "1201950000000002"

func newFakeOMS(t *testing.T) *fakeOMS {
	t.Helper()
	f := &fakeOMS{}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/users/login/", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		f.tenant = r.Header.Get("X-Tenant-Subdomain")
		mfa := f.mfa
		f.mu.Unlock()
		switch {
		case in["password"] != "pw":
			reply(w, 400, map[string]any{"success": false, "message": "Invalid credentials"})
		case mfa:
			reply(w, 200, map[string]any{"success": true, "data": map[string]any{"mfa_required": true, "mfa_token": "m"}})
		default:
			reply(w, 200, map[string]any{"success": true, "message": "Successfully Login", "data": map[string]any{
				"id": 42, "username": in["username"], "users_status": "", "token": map[string]any{"access": "ACC", "refresh": "REF"},
				"user_detail": map[string]any{"trader_id": 7, "trader_code": "TWS0000001", "branch_id": 3, "trader_name": "Test Dealer"}}})
		}
	})
	mux.HandleFunc("/api/oms/get_investor_list_by_trader/7/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ACC" {
			reply(w, 401, map[string]any{"detail": "not signed in"})
			return
		}
		reply(w, 200, map[string]any{"success": true, "data": []any{
			map[string]any{"investor_code": "C001", "bo_id": fakeBO1, "investor_name": "First Client"},
			map[string]any{"investor_code": "C002", "bo_id": fakeBO2, "investor_name": "Second Client"}}})
	})
	mux.HandleFunc("/api/users/logout/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.loggedOut = true
		f.mu.Unlock()
		reply(w, 200, map[string]any{"success": true})
	})
	mux.HandleFunc("/position/", func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		rows := []any{}
		if strings.Contains(r.URL.Path, fakeBO1) {
			rows = append(rows, map[string]any{"client_code": "C001", "security_code": "GP", "quantity": 500})
		}
		ws.WriteJSON(map[string]any{"message_type": "connection", "data": rows})
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	})
	mux.HandleFunc("/order/TWS0000001/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.wsQuery, f.origin = r.URL.Query(), r.Header.Get("Origin")
		f.mu.Unlock()
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		ws.WriteJSON(map[string]any{"message_type": "connection", "data": []any{}})
		// someone else's order on the same dealer: the bots must leave it alone
		ws.WriteJSON(map[string]any{"message_type": "order_report", "data": map[string]any{"clientOrderId": "HUMAN1", "orderStatus": "FILLED", "lastQty": 5}})
		for {
			_, b, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]any
			json.Unmarshal(b, &m)
			f.mu.Lock()
			f.frames, f.sizes = append(f.frames, m), append(f.sizes, len(b))
			f.mu.Unlock()
			cl, orig := str(m["client_order_id"]), str(m["orig_client_order_id"])
			report := func(d map[string]any) {
				ws.WriteJSON(map[string]any{"message_type": "order_report", "message": "", "data": d})
			}
			switch m["action"] {
			case "NEW_ORDER_SINGLE_REQUEST":
				if m["side"] == "SELL" && num(m["qty"]) > 500 {
					ws.WriteJSON(map[string]any{"message_type": "order_error", "message": "Insufficient shares of GP to sell.", "data": m})
					continue
				}
				ws.WriteJSON(map[string]any{"message_type": "order_submitted", "message": "Successfully order BUY submitted.", "data": m})
				if m["timeInForce"] == "3" {
					report(map[string]any{"clientOrderId": cl, "orderId": "ORD" + cl, "orderStatus": "FILLED", "leavesQty": 0,
						"lastQty": m["qty"], "lastPx": m["price"], "execID": "E" + cl})
				} else {
					report(map[string]any{"clientOrderId": cl, "orderId": "ORD" + cl, "orderStatus": "NEW", "leavesQty": m["qty"]})
				}
			case "ORDER_CANCEL_REQUEST":
				report(map[string]any{"clientOrderId": cl, "origClOrdID": orig, "orderId": m["order_id"], "orderStatus": "CANCELED", "leavesQty": 0})
			case "ORDER_CANCEL_REPLACE_REQUEST":
				// like the live OMS: the order's state (39) with execType REPLACED (150=5)
				report(map[string]any{"clientOrderId": cl, "origClOrdID": orig, "orderId": m["order_id"], "orderStatus": "NEW", "execType": "REPLACED", "leavesQty": m["qty"]})
			}
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOMS) sent() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.frames...)
}

func TestOMSBotsTradeThroughTheOMS(t *testing.T) {
	s, eng, rec := newSim(t)
	eng.SetPhase(engine.PhaseOpen)
	// Someone to trade with: a bid and an ask on each share, so the bots also
	// take. The engine and order log are shared by the package's tests, so the
	// orders and their order-log rows go again at the end.
	const seeder = "OMSTEST"
	seeds := []struct {
		sym  string
		side engine.Side
		px   float64
	}{{"GP", engine.Buy, 329}, {"GP", engine.Sell, 333}, {"BATBC", engine.Buy, 508}, {"BATBC", engine.Sell, 516}}
	for i, q := range seeds {
		o := eng.NewOrder(q.sym, q.side, engine.Limit, q.px, 100000, seeder)
		o.ClOrdID = "SEED" + string(rune('A'+i))
		eng.SubmitAndWait(o)
	}
	t.Cleanup(func() {
		for i := range seeds {
			cl := "SEED" + string(rune('A'+i))
			eng.CancelAs(cl, cl+"-X", seeder)
		}
		rec.Forget(seeder)
	})
	f := newFakeOMS(t)
	err := s.StartOMS(OMSConfig{URL: "https://ibl.example.com", API: f.srv.URL, User: "dealer1", Password: "pw",
		Rate: 600, Symbols: []string{"GP", "BATBC"}, MaxValue: 50000})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StartOMS(OMSConfig{URL: "https://ibl.example.com", API: f.srv.URL, User: "dealer1", Password: "pw"}); err != errOMSRunning {
		t.Fatalf("second start: %v, want errOMSRunning", err)
	}
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end) && len(f.sent()) < 40; {
		time.Sleep(50 * time.Millisecond)
	}
	st := s.Status()["bots"].(map[string]any)
	if st["state"] != "running" || st["dealer"] != "TWS0000001" || st["tenant"] != "ibl" || st["connected"] != true {
		t.Fatalf("status while running: %v", st)
	}
	if strings.Contains(mustJSON(t, st), `"pw"`) {
		t.Fatal("the status shows the password")
	}
	if stats := st["stats"].(map[string]int64); stats["accepted"] == 0 || stats["trades"] == 0 {
		t.Fatalf("stats %v: want orders accepted by the OMS and fills", stats)
	}
	filled := false
	for _, row := range s.Status()["tape"].([]TapeRow) {
		filled = filled || (strings.HasPrefix(row.Bot, "OMS C00") && row.Status == "Filled")
	}
	if !filled {
		t.Fatal("no OMS fill on the tape")
	}
	s.StopOMS()
	if st := s.Status()["bots"].(map[string]any); st["state"] != "stopped" || st["dealer"] != "TWS0000001" {
		t.Fatalf("after Stop the bots stay signed in: %v", st)
	}
	if _, err := s.CleanOMS(); err != nil {
		t.Fatalf("Clean Data: %v", err)
	}

	f.mu.Lock()
	tenant, q, origin, out, sizes := f.tenant, f.wsQuery, f.origin, f.loggedOut, append([]int(nil), f.sizes...)
	f.mu.Unlock()
	if tenant != "ibl" || q.Get("tenant_subdomain") != "ibl" || q.Get("tenant_env") != "prod" || q.Get("token") != "ACC" {
		t.Fatalf("tenant header %q, order socket query %v", tenant, q)
	}
	if origin != "https://ibl.example.com" || !out {
		t.Fatalf("origin %q, signed out %v", origin, out)
	}
	for _, n := range sizes {
		if n > 4096 {
			t.Fatalf("a request of %d bytes: the OMS closes the socket above 4096", n)
		}
	}

	bo := map[string]string{"C001": fakeBO1, "C002": fakeBO2}
	prev := map[string]float64{"GP": 331.2, "BATBC": 512}
	newIDs := map[string]bool{}
	var news, cancels, sells int
	for _, m := range f.sent() {
		if m["trader_id"] != "TWS0000001" || bo[str(m["client_code"])] != m["bo_id"] {
			t.Fatalf("dealer or client wrong: %v", m)
		}
		switch m["action"] {
		case "NEW_ORDER_SINGLE_REQUEST", "ORDER_CANCEL_REPLACE_REQUEST":
			sym, px, qty := str(m["symbol"]), num(m["price"]), num(m["qty"])
			if num(m["trader_pk"]) != 7 || num(m["branch_id"]) != 3 || num(m["user_id"]) != 42 || m["order_type"] != "LIMIT" ||
				m["board"] != "MAIN" || m["group"] != "PUBLIC" || m["session"] != "continuous" {
				t.Fatalf("order fields: %v", m)
			}
			if p, ok := prev[sym]; !ok || int(num(m["order_book_id"])) != eng.OrderBookID(sym) || px < p*0.96 || px > p*1.04 ||
				math.Abs(px*10-math.Round(px*10)) > 1e-6 || qty <= 0 {
				t.Fatalf("instrument, price or quantity: %v", m)
			}
			if tif := m["timeInForce"]; tif != "0" && tif != "3" {
				t.Fatalf("timeInForce %v", tif)
			}
			if cl := str(m["client_order_id"]); cl == "" || len(cl) > 20 {
				t.Fatalf("ClOrdID %q: DSE takes at most 20 characters", cl)
			}
			if m["action"] == "NEW_ORDER_SINGLE_REQUEST" {
				news++
				newIDs[str(m["client_order_id"])] = true
				if m["side"] == "SELL" {
					sells++
					if m["client_code"] != "C001" || sym != "GP" || qty > 500 {
						t.Fatalf("a sell without holdings: %v", m)
					}
				} else if px*qty > 50000+1e-6 {
					t.Fatalf("a buy of Tk %.2f is above the largest-buy value: %v", px*qty, m)
				}
			}
		case "ORDER_CANCEL_REQUEST":
			cancels++
			if !strings.HasPrefix(str(m["order_id"]), "ORD") || str(m["orig_client_order_id"]) == "" {
				t.Fatalf("cancel without the order's ids: %v", m)
			}
		default:
			t.Fatalf("unexpected request %v", m)
		}
	}
	if news < 20 || cancels == 0 {
		t.Fatalf("%d new orders, %d cancels: want trading and cancels", news, cancels)
	}
	if st := s.Status()["bots"].(map[string]any); st["state"] != "stopped" || st["dealer"] != nil {
		t.Fatalf("after Clean Data: %v", st)
	}
	t.Logf("%d new orders (%d sells), %d cancels", news, sells, cancels)
}

// Stop keeps the bots' orders in the book and the bots signed in; Clean Data
// cancels them through the OMS and signs out; Start replaces a stopped run's
// orders rather than trading on top of them.
func TestOMSBotsStopKeepsOrdersCleanDataCancelsThem(t *testing.T) {
	s, eng, _ := newSim(t)
	eng.SetPhase(engine.PhaseOpen)
	f := newFakeOMS(t)
	cfg := OMSConfig{URL: "https://ibl.example.com", API: f.srv.URL, User: "dealer1", Password: "pw",
		Rate: 600, Symbols: []string{"GP"}, Clients: []string{"C002"}}
	if err := s.StartOMS(cfg); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := s.CleanOMS(); err == nil {
		t.Fatal("Clean Data must wait until the bots are stopped")
	}
	s.mu.Lock()
	r := s.oms
	s.mu.Unlock()
	s.StopOMS()
	kept := r.working()
	if st := s.Status()["bots"].(map[string]any); kept == 0 || st["state"] != "stopped" || st["working"] != kept {
		t.Fatalf("Stop must leave the orders in the book: %d kept, status %v", kept, st)
	}
	if err := s.PauseOMS(true); err == nil {
		t.Fatal("stopped bots cannot be paused")
	}
	f.mu.Lock()
	out := f.loggedOut
	f.mu.Unlock()
	if out {
		t.Fatal("Stop must keep the bots signed in, so Clean Data can cancel through the OMS")
	}

	if err := s.StartOMS(cfg); err != nil {
		t.Fatalf("start after stop: %v", err)
	}
	if left := r.working(); left != 0 {
		t.Fatalf("%d orders of the stopped run still working after Start", left)
	}
	time.Sleep(700 * time.Millisecond)
	s.StopOMS()
	f.mu.Lock()
	f.loggedOut = false
	f.mu.Unlock()
	n, err := s.CleanOMS()
	s.mu.Lock()
	gone := s.oms == nil
	s.mu.Unlock()
	f.mu.Lock()
	out = f.loggedOut
	f.mu.Unlock()
	if err != nil || n == 0 || !gone || !out {
		t.Fatalf("Clean Data: %d cancelled, %v; run cleared %v, signed out %v", n, err, gone, out)
	}
	for _, m := range f.sent() {
		if m["client_code"] != "C002" || m["side"] == "SELL" {
			t.Fatalf("C002 holds nothing and was the only client: %v", m)
		}
	}
}

// With Remember, a sign-in the OMS accepted is saved on the engine (mode 0600)
// and a blank password then uses it, for that login only; the page never sees
// the password; Forget deletes it.
func TestOMSLoginRemembered(t *testing.T) {
	s, eng, _ := newSim(t)
	eng.SetPhase(engine.PhaseOpen)
	f := newFakeOMS(t)
	t.Cleanup(func() { s.ForgetOMSLogin() })
	run := func(cfg OMSConfig) error {
		if err := s.StartOMS(cfg); err != nil {
			return err
		}
		s.StopOMS()
		_, err := s.CleanOMS()
		return err
	}
	cfg := OMSConfig{URL: "https://ibl.example.com/", API: f.srv.URL, User: "dealer1", Password: "pw", Rate: 6,
		Symbols: []string{"GP"}, Clients: []string{"C002"}}
	if err := run(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(loginFile()); !os.IsNotExist(err) {
		t.Fatal("a login must not be saved without Remember")
	}
	cfg.Remember = true
	if err := run(cfg); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(loginFile()); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("saved login file: %v %v, want mode 0600", st, err)
	}
	all := mustJSON(t, s.Status())
	if sl, _ := s.Status()["bots"].(map[string]any)["saved_login"].(map[string]any); sl == nil || sl["user"] != "dealer1" ||
		sl["url"] != "https://ibl.example.com" || strings.Contains(all, `"pw"`) {
		t.Fatalf("saved login in the status: %v (the password must never show)", sl)
	}

	cfg.Password, cfg.Remember = "", false
	other := cfg
	other.User = "dealer2"
	if err := s.StartOMS(other); err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("another login must not use the saved password: %v", err)
	}
	if err := run(cfg); err != nil {
		t.Fatalf("start with the password left blank: %v", err)
	}
	if err := s.ForgetOMSLogin(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(loginFile()); !os.IsNotExist(err) {
		t.Fatal("Forget must delete the saved login")
	}
	if err := s.StartOMS(cfg); err == nil {
		t.Fatal("no password typed and none saved: Start must refuse")
	}
}

// The OMS reports a completed amend as the order's state (orderStatus NEW)
// with execType REPLACED; the bots must take the new ClOrdID, or the order
// stays "amend pending" and is never cancelled again.
func TestOMSAmendReportedAsNewReplaced(t *testing.T) {
	s, _, _ := newSim(t)
	a := &omsAcct{code: "C001", bo: fakeBO1, hold: map[string]int64{}}
	r := &omsRun{s: s, accts: []*omsAcct{a}, ids: map[string]*omsOrder{}}
	o := &omsOrder{id: "SONx1", acct: a, sym: "GP", side: "BUY", px: 330, qty: 10, leaves: 10, orderID: "ORD1",
		live: true, pend: "SOAx2", newPx: 330.1, at: time.Now()}
	r.ids["SONx1"], r.ids["SOAx2"] = o, o
	r.onReport(map[string]any{"clientOrderId": "SOAx2", "origClOrdID": "SONx1", "orderId": "ORD1",
		"orderStatus": "NEW", "execType": "REPLACED", "leavesQty": 10.0}, "")
	if o.id != "SOAx2" || o.pend != "" || o.px != 330.1 || len(r.resting(a)) != 1 || r.ids["SONx1"] != nil {
		t.Fatalf("after the amend report: id %s, pending %q, price %v, resting %d", o.id, o.pend, o.px, len(r.resting(a)))
	}
}

// Clean Data cancels, in the engine, OMS-bot orders that no run follows any
// more (left by an earlier run), under their own ClOrdID, and nothing else.
func TestOMSCleanDataCancelsOrphans(t *testing.T) {
	s, eng, rec := newSim(t)
	var reports []string
	for _, cl := range []string{"SOAorph1", "SONorph2", "HUMANKEEP"} {
		o := eng.NewOrder("RENATA", engine.Buy, engine.Limit, 690, 10, "FIX:REALBRK")
		o.ClOrdID = cl
		o.Notify = func(r *engine.ExecReport) {
			if r.Status == "CANCELED" {
				reports = append(reports, r.ClOrdID+"/"+r.OrigClOrdID)
			}
		}
		eng.SubmitAndWait(o)
	}
	t.Cleanup(func() {
		eng.CancelAs("HUMANKEEP", "HUMANKEEP-X", "FIX:REALBRK")
		rec.Forget("FIX:REALBRK")
	})
	if n := s.Status()["bots"].(map[string]any)["orphans"]; n != 2 {
		t.Fatalf("orphans counted: %v, want 2", n)
	}
	n, err := s.CleanOMS()
	if err != nil || n != 2 || len(reports) != 2 || reports[0] != "SOAorph1/SOAorph1" && reports[0] != "SONorph2/SONorph2" {
		t.Fatalf("Clean Data: %d cancelled, %v; reports %v", n, err, reports)
	}
	eng.Mu.RLock()
	kept := eng.FindOpen("HUMANKEEP", "FIX:REALBRK")
	eng.Mu.RUnlock()
	if kept == nil || s.Status()["bots"].(map[string]any)["orphans"] != 0 {
		t.Fatal("only the bots' orphans may go")
	}
}

func TestOMSBotsStartErrors(t *testing.T) {
	s, _, _ := newSim(t)
	f := newFakeOMS(t)
	cfg := OMSConfig{URL: "https://ibl.example.com", API: f.srv.URL, User: "dealer1", Password: "nope"}
	if err := s.StartOMS(cfg); err == nil || !strings.Contains(err.Error(), "Invalid credentials") {
		t.Fatalf("wrong password: %v", err)
	}
	cfg.Password = "pw"
	cfg.Clients = []string{"X9"}
	if err := s.StartOMS(cfg); err == nil || !strings.Contains(err.Error(), "X9/<BO ID>") {
		t.Fatalf("unknown client: %v", err)
	}
	f.mu.Lock()
	f.mfa = true
	f.mu.Unlock()
	cfg.Clients = nil
	if err := s.StartOMS(cfg); err == nil || !strings.Contains(err.Error(), "one-time code") {
		t.Fatalf("MFA login: %v", err)
	}
}

func TestParseOMSSite(t *testing.T) {
	site, err := parseOMSSite("ibl-uat.bdtaskoms.com", "")
	if err != nil || site.Tenant != "ibl" || site.Env != "uat" || site.API != "https://api.bdtaskoms.com" ||
		site.WS != "wss://api.bdtaskoms.com" || site.Origin != "https://ibl-uat.bdtaskoms.com" {
		t.Fatalf("%+v %v", site, err)
	}
	if site, err := parseOMSSite("https://ibl.bdtaskoms.com/", ""); err != nil || site.Tenant != "ibl" || site.Env != "prod" {
		t.Fatalf("%+v %v", site, err)
	}
	for _, bad := range []string{"", "https://103.169.105.170", "https://api.bdtaskoms.com", "bdtaskoms.com", "ftp://ibl.bdtaskoms.com"} {
		if _, err := parseOMSSite(bad, ""); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
