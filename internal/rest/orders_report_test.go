package rest

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/orderlog"
	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
)

func TestOrderReport(t *testing.T) {
	middleware.ResetLoginThrottle()
	audit.Path = filepath.Join(t.TempDir(), "audit.jsonl")
	rec, err := orderlog.NewRecorder(orderlog.NewFileStore(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	srv := testServerWith(t, func(eng *engine.Engine) { eng.OnOrderEvent = rec.Observe })
	srv.Orders = rec
	if err := srv.Users.Add("alice", "Rabbit-Hole9", users.RoleUser); err != nil {
		t.Fatal(err)
	}
	e := &roleEnv{t: t, srv: srv, h: srv.Routes()}
	adm, alice := e.login(adminName, adminPW), e.login("alice", "Rabbit-Hole9")
	e.do(http.MethodPost, "/admin/api/broker/add", alice, map[string]string{"Sender": "ALICEBRK", "Company": "Alice Securities Ltd"})
	defer e.do(http.MethodPost, "/admin/api/broker/del", adm, map[string]string{"Sender": "ALICEBRK"})

	eng := srv.Eng
	eng.SetPhase(engine.PhaseOpen)
	place := func(actor, cl string, side engine.Side, px float64, qty int64, client string) {
		o := eng.NewOrder("GP", side, engine.Limit, px, qty, actor)
		o.ClOrdID, o.Info = cl, engine.OrderInfo{Client: client, Account: "1206500076409418", Board: "PUBLIC", Trader: "TRDR1"}
		eng.SubmitAndWait(o)
	}
	place("FIX:ALICEBRK", "26101914123258700003", engine.Buy, 331.2, 100, "00018") // partly filled
	place("FIX:OTHERBRK", "X1", engine.Sell, 331.2, 40, "02331")                   // filled
	place("FIX:OTHERBRK", "X2", engine.Sell, 999, 10, "02331")                     // rejected (band)

	get := func(c *http.Cookie, q string) map[string]any {
		var out map[string]any
		json.Unmarshal(e.do(http.MethodGet, "/admin/api/report/orders"+q, c, nil).Body.Bytes(), &out)
		return out
	}
	all := get(adm, "")
	if all["total"].(float64) != 3 || all["scope"] != "exchange" {
		t.Fatalf("admin: %v", all)
	}
	row := all["rows"].([]any)[2].(map[string]any) // oldest last
	if row["broker"] != "Alice Securities Ltd" || row["client"] != "00018" || row["status"] != "Partial" ||
		row["filled"].(float64) != 40 || row["board"] != "Public" || row["type"] != "LIMIT" || row["side"] != "Buy" ||
		!strings.HasPrefix(row["order_id"].(string), "20") {
		t.Fatalf("alice's order row: %v", row)
	}
	if mine := get(alice, ""); mine["total"].(float64) != 1 || mine["scope"] != "own" {
		t.Fatalf("alice must see only her broker's order: %v", mine)
	}
	for q, want := range map[string]float64{"?status=rejected": 1, "?side=sell": 2, "?broker=ALICEBRK": 1,
		"?q=02331": 2, "?symbol=gp": 3, "?symbol=NOPE": 0} {
		if got := get(adm, q)["total"].(float64); got != want {
			t.Errorf("filter %s: %v, want %v", q, got, want)
		}
	}

	// CSV: one header line plus a line per order.
	csvBody := e.do(http.MethodGet, "/admin/api/report/orders?format=csv", adm, nil).Body.String()
	if lines := strings.Split(strings.TrimSpace(csvBody), "\n"); len(lines) != 4 ||
		!strings.HasPrefix(lines[0], "Time,Broker,OrderID,Client,Symbol,Side,Type,Qty,Price,Filled,Status,Board") {
		t.Fatalf("csv: %q", csvBody)
	}
	// Excel: a valid zip whose sheet keeps IDs as text (no 2.61E+019, leading zeros kept).
	x := e.do(http.MethodGet, "/admin/api/report/orders?format=xlsx", adm, nil)
	if !strings.Contains(x.Header().Get("Content-Disposition"), ".xlsx") {
		t.Fatalf("xlsx headers: %v", x.Header())
	}
	zr, err := zip.NewReader(bytes.NewReader(x.Body.Bytes()), int64(x.Body.Len()))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	var sheet string
	for _, f := range zr.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			sheet = string(b)
		}
	}
	for _, want := range []string{`t="inlineStr"><is><t xml:space="preserve">00018</t>`,
		`<t xml:space="preserve">26101914123258700003</t>`, `<t xml:space="preserve">Alice Securities Ltd</t>`} {
		if !strings.Contains(sheet, want) {
			t.Fatalf("sheet lacks %s", want)
		}
	}
}
