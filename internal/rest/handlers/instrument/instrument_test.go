package instrument

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"velexa-sim-me/internal/engine"
)

// newTestEngine boots an engine on a tiny universe with its matching goroutine
// running, so control ops (listing) and order submission both work.
func newTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	e := engine.NewEngine([]engine.Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 331.20}}, engine.NewBus())
	go e.Run()
	return e
}

// ---- admin add / update / halt -------------------------------------------

func adminPost(t *testing.T, eng *engine.Engine, h func(http.ResponseWriter, *http.Request, *engine.Engine), body string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req, eng)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response not JSON: %q", rec.Body.String())
	}
	return out
}

func addH(w http.ResponseWriter, r *http.Request, e *engine.Engine) { saveInstrument(w, r, e, true) }

func updH(w http.ResponseWriter, r *http.Request, e *engine.Engine) { saveInstrument(w, r, e, false) }

func TestAdminAddUpdateHaltAndPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instruments.json")
	t.Setenv("ME_INSTRUMENT_FILE", path)
	t.Setenv("ME_AUTO_LIST", "off")
	eng := newTestEngine(t)

	// bad inputs
	for _, c := range []struct{ body, want string }{
		{`{"Symbol":"bad sym","Prev":10}`, "invalid symbol"},
		{`{"Symbol":"NEWCO","Name":"New Co","Prev":0}`, "previous close"},
		{`{"Symbol":"NEWCO","Name":"N","Prev":10,"Board":"XYZ"}`, "board"},
		{`{"Symbol":"NEWCO","Name":"N","Prev":10,"ISIN":"123"}`, "ISIN"},
		{`{"Symbol":"GP","Prev":10}`, "already listed"},
	} {
		out := adminPost(t, eng, addH, c.body)
		if out["ok"] == true || !strings.Contains(out["error"].(string), c.want) {
			t.Fatalf("%s: want error containing %q, got %v", c.body, c.want, out)
		}
	}

	// add
	out := adminPost(t, eng, addH, `{"Symbol":"newco","Name":"New Co","Board":"SME","ISIN":"BD0001NEW007","Prev":50.5,"Tick":0.1}`)
	if out["ok"] != true {
		t.Fatalf("add failed: %v", out)
	}
	if !eng.Listed("NEWCO") || eng.MdOf("NEWCO").Prev != 50.5 {
		t.Fatal("NEWCO not listed with prev 50.5")
	}
	if m := engine.MetaFor("NEWCO"); m.Board != "SME" || m.ISIN != "BD0001NEW007" {
		t.Fatalf("meta: %+v", m)
	}
	found := false
	for _, r := range instrumentRows(eng) {
		if r["symbol"] == "NEWCO" {
			found = true
		}
	}
	if !found {
		t.Fatal("NEWCO missing from admin rows")
	}

	// update keeps unspecified fields
	out = adminPost(t, eng, updH, `{"Symbol":"NEWCO","Name":"New Company"}`)
	if out["ok"] != true || engine.MetaFor("NEWCO").Board != "SME" {
		t.Fatalf("update: %v meta=%+v", out, engine.MetaFor("NEWCO"))
	}
	if out := adminPost(t, eng, updH, `{"Symbol":"NOPE","Prev":1}`); out["ok"] == true {
		t.Fatal("update of unknown symbol should fail")
	}

	// halt rejects orders, resume accepts them
	engine.SetHalted("NEWCO", true)
	rep := eng.SubmitAndWait(&engine.Order{Symbol: "NEWCO", Side: engine.Buy, Type: engine.Limit, Price: 50.5, Qty: 10, OrigQty: 10, Actor: "FIX:T"})
	if rep.Status != "REJECTED" || !strings.Contains(rep.Reason, "halted") {
		t.Fatalf("halted order: %+v", rep)
	}
	engine.SetHalted("NEWCO", false)
	rep = eng.SubmitAndWait(&engine.Order{Symbol: "NEWCO", Side: engine.Buy, Type: engine.Limit, Price: 50.5, Qty: 10, OrigQty: 10, Actor: "FIX:T"})
	if rep.Status == "REJECTED" {
		t.Fatalf("resumed order rejected: %+v", rep)
	}

	// persistence round-trip: halted state and the whole universe survive a reload
	engine.SetHalted("NEWCO", true)
	if err := persistUniverse(eng); err != nil {
		t.Fatalf("persist: %v", err)
	}
	u, err := engine.LoadUniverseFile(path)
	if err != nil || len(u.Instr) != 2 {
		t.Fatalf("reload: %v instr=%+v", err, u.Instr)
	}
	if !u.Meta["NEWCO"].Halted || u.Meta["GP"].Halted {
		t.Fatalf("halted flag not round-tripped: %+v", u.Meta)
	}
}

func TestInstrumentSectorLotStatusDelist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instruments.json")
	t.Setenv("ME_INSTRUMENT_FILE", path)
	t.Setenv("ME_AUTO_LIST", "off")
	eng := newTestEngine(t)
	fixOrder := func(sym string, qty int64) *engine.ExecReport {
		o := eng.NewOrder(sym, engine.Buy, engine.Limit, eng.MdOf(sym).Prev, qty, "FIX:T")
		return eng.SubmitAndWait(o)
	}

	out := adminPost(t, eng, addH, `{"Symbol":"LOTCO","Name":"Lot Co","Board":"main (a)","Sector":"Pharma","Lot":1000,"Prev":50,"Tick":0.1}`)
	if out["ok"] != true {
		t.Fatalf("add: %v", out)
	}
	if m := engine.MetaFor("LOTCO"); m.Board != "Main (A)" || m.Sector != "Pharma" || m.Lot != 1000 {
		t.Fatalf("meta not stored/canonicalised: %+v", m)
	}
	if rep := fixOrder("LOTCO", 1500); rep.Status != "REJECTED" || !strings.Contains(rep.Reason, "Minimum quantity step: 1000") {
		t.Fatalf("lot rule: %+v", rep)
	}
	if rep := fixOrder("LOTCO", 2000); rep.Status == "REJECTED" {
		t.Fatalf("whole lots rejected: %+v", rep)
	}

	// pending listing: listed but not tradable until activated
	out = adminPost(t, eng, addH, `{"Symbol":"PENDCO","Name":"Pending Co","Board":"Main (N)","Prev":20,"Status":"pending"}`)
	if out["ok"] != true || engine.MetaFor("PENDCO").StatusOf() != "pending" {
		t.Fatalf("pending add: %v %+v", out, engine.MetaFor("PENDCO"))
	}
	if rep := fixOrder("PENDCO", 10); rep.Status != "REJECTED" || !strings.Contains(rep.Reason, "pending") {
		t.Fatalf("pending order: %+v", rep)
	}
	rows := instrumentRows(eng)
	sum, classes := instrumentSummary(rows)
	if sum["pending"] != 1 || len(classes) != 4 {
		t.Fatalf("summary: %v %v", sum, classes)
	}
	engine.SetStatus("PENDCO", "active")
	if rep := fixOrder("PENDCO", 10); rep.Status == "REJECTED" {
		t.Fatalf("activated order rejected: %+v", rep)
	}

	// persistence keeps sector/lot/status
	if err := persistUniverse(eng); err != nil {
		t.Fatal(err)
	}
	u, err := engine.LoadUniverseFile(path)
	if err != nil || u.Meta["LOTCO"].Lot != 1000 || u.Meta["LOTCO"].Sector != "Pharma" {
		t.Fatalf("round trip: %v %+v", err, u.Meta["LOTCO"])
	}

	// delist: leaves the directory and the saved master, rejects orders, can be re-listed
	engine.SetStatus("LOTCO", "delisted")
	for _, r := range instrumentRows(eng) {
		if r["symbol"] == "LOTCO" {
			t.Fatal("delisted scrip still in directory rows")
		}
	}
	if rep := fixOrder("LOTCO", 1000); rep.Status != "REJECTED" || !strings.Contains(rep.Reason, "delisted") {
		t.Fatalf("delisted order: %+v", rep)
	}
	if err := persistUniverse(eng); err != nil {
		t.Fatal(err)
	}
	if u, _ = engine.LoadUniverseFile(path); u.Meta["LOTCO"].Lot != 0 {
		t.Fatal("delisted scrip was saved to the master")
	}
	out = adminPost(t, eng, addH, `{"Symbol":"LOTCO","Name":"Lot Co","Prev":50}`)
	if out["ok"] != true || !engine.MetaFor("LOTCO").Tradable() {
		t.Fatalf("relist: %v %+v", out, engine.MetaFor("LOTCO"))
	}
}

func TestInstrumentAssetClass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instruments.json")
	t.Setenv("ME_INSTRUMENT_FILE", path)
	eng := newTestEngine(t)

	if out := adminPost(t, eng, addH, `{"Symbol":"BADCLS","Name":"x","Prev":10,"AssetClass":"XX"}`); out["ok"] == true {
		t.Fatal("unknown asset class accepted")
	}
	adminPost(t, eng, addH, `{"Symbol":"FUND1","Name":"Fund One","AssetClass":"mf","Board":"Mutual Fund","Sector":"MutFund","Prev":8.5}`)
	adminPost(t, eng, addH, `{"Symbol":"CBOND1","Name":"Corp Bond","AssetClass":"CB","Board":"Bond","Prev":1000}`)
	adminPost(t, eng, addH, `{"Symbol":"OLDBOND","Name":"Legacy","Board":"Bond","Prev":100}`) // no class → derived from board
	adminPost(t, eng, addH, `{"Symbol":"EQ1","Name":"Equity One","Prev":50}`)

	class := map[string]string{}
	for _, r := range instrumentRows(eng) {
		class[r["symbol"].(string)] = r["asset_class"].(string)
	}
	want := map[string]string{"FUND1": "MF", "CBOND1": "CB", "OLDBOND": "CB", "EQ1": "EQ", "GP": "EQ"}
	for sym, w := range want {
		if class[sym] != w {
			t.Fatalf("%s asset class = %q, want %q (all: %v)", sym, class[sym], w, class)
		}
	}
	sum, classes := instrumentSummary(instrumentRows(eng))
	if sum["bonds"] != 2 || len(classes) != 4 || classes[1]["count"] != 1 {
		t.Fatalf("summary %v classes %v", sum, classes)
	}

	if err := persistUniverse(eng); err != nil {
		t.Fatal(err)
	}
	u, err := engine.LoadUniverseFile(path)
	if err != nil || u.Meta["FUND1"].AssetClass != "MF" || u.Meta["CBOND1"].AssetClass != "CB" {
		t.Fatalf("round trip: %v %+v", err, u.Meta["FUND1"])
	}
}

func TestInstrumentExport(t *testing.T) {
	t.Setenv("ME_INSTRUMENT_FILE", filepath.Join(t.TempDir(), "i.json"))
	eng := newTestEngine(t)
	adminPost(t, eng, addH, `{"Symbol":"EXPA","Name":"Export & Co <Ltd>","AssetClass":"EQ","Board":"Main (B)","Sector":"Bank","Prev":50,"ISIN":"BD0001EXPA09"}`)
	adminPost(t, eng, addH, `{"Symbol":"EXPF","Name":"Export Fund","AssetClass":"MF","Board":"Mutual Fund","Sector":"MutFund","Prev":8.5,"Status":"halted"}`)

	// make EXPA trade so it carries OHLC and a variation
	eng.SubmitAndWait(eng.NewOrder("EXPA", engine.Sell, engine.Limit, 51, 100, "SEED"))
	eng.SubmitAndWait(eng.NewOrder("EXPA", engine.Buy, engine.Limit, 51, 100, "SEED"))

	all := instrumentRows(eng)
	b, err := instrumentsXML(filterInstrumentRows(all, "", "", "", "EXP"), 6000.5)
	if err != nil {
		t.Fatal(err)
	}
	x := string(b)
	for _, want := range []string{
		`<?xml version="1.0" encoding="utf-8"?>`, "<EODTickers>", "</EODTickers>",
		// the index comes first, as in the DSE file
		`  <Ticker SecurityCode="DSEX" ISIN="DSEX" AssetClass="IN" CompulsorySpot="N" TradeDate="`,
		// a traded equity: every attribute, Var/VarPercent against the previous close of 50;
		// the day's open/high/low come from its trades (one, at 51)
		`<Ticker SecurityCode="EXPA" ISIN="BD0001EXPA09" Category="B" AssetClass="EQ" Sector="Bank" CompulsorySpot="N" TradeDate="`,
		`Close="51" Open="51" High="51" Low="51" Var="1.00000" VarPercent="2.00000" />`,
		// an untraded fund: only its previous close, category defaults to A
		`<Ticker SecurityCode="EXPF" ISIN="" Category="A" AssetClass="MF" Sector="MutFund" CompulsorySpot="N" TradeDate="`,
		`Close="8.5" />`,
	} {
		if !strings.Contains(x, want) {
			t.Fatalf("XML missing %q in:\n%s", want, x)
		}
	}
	if strings.Contains(x, "></Ticker>") || strings.Contains(x, "<Instrument") {
		t.Fatalf("not in the EODTickers layout:\n%s", x)
	}
	var doc eodTickers
	if err := xml.Unmarshal(b, &doc); err != nil || len(doc.Tickers) != 3 {
		t.Fatalf("export is not well-formed EODTickers XML: %v (%d tickers)", err, len(doc.Tickers))
	}
	// filters
	if n := len(filterInstrumentRows(all, "", "MF", "", "")); n != 1 {
		t.Fatalf("class filter: %d", n)
	}
	if n := len(filterInstrumentRows(all, "Main (B)", "", "", "")); n != 1 {
		t.Fatalf("board filter: %d", n)
	}
	// CSV and JSON round trip through the importer
	if c, _ := instrumentsCSV(filterInstrumentRows(all, "", "", "halted", "")); !strings.Contains(string(c), "EXPF") || strings.Contains(string(c), "EXPA") {
		t.Fatalf("csv: %s", c)
	}
	body, _ := json.Marshal(map[string]any{"instruments": masterRows(filterInstrumentRows(all, "", "", "", "EXP"))})
	rows, err := engine.ParseMaster(body)
	if err != nil || len(rows) != 2 || rows[0].Sector != "Bank" || rows[1].AssetClass != "MF" {
		t.Fatalf("json re-import: %v %+v", err, rows)
	}
}
