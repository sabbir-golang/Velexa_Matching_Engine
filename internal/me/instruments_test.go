package me

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"velexa-sim-me/internal/refdata"
)

func TestParseMasterJSONPayload(t *testing.T) {
	rows, err := parseMaster([]byte(`{"count":2,"instruments":[
		{"symbol":"UNITEDINS","isin":"BD0123456789","name":"United Insurance","board":"MAIN","prev_close":48.7,"tick":0.2},
		{"symbol":"DSEX","is_index":true}]}`))
	if err != nil {
		t.Fatalf("parseMaster: %v", err)
	}
	u := buildUniverse(rows)
	if len(u.Instr) != 1 || u.Instr[0].Symbol != "UNITEDINS" || u.Instr[0].Prev != 48.7 {
		t.Fatalf("want the one tradeable scrip, got %+v", u.Instr)
	}
	if m := u.Meta["UNITEDINS"]; !strings.HasPrefix(m.Board, "Main (") || m.ISIN != "BD0123456789" || m.Tick != 0.2 {
		t.Fatalf("meta not carried through: %+v", m)
	}
}

func TestParseMasterBareArrayAndCSV(t *testing.T) {
	rows, err := parseMaster([]byte(`[{"symbol":"VAMLBDMF1","prev_close":9.5}]`))
	if err != nil || len(rows) != 1 || rows[0].Symbol != "VAMLBDMF1" {
		t.Fatalf("bare array: rows=%+v err=%v", rows, err)
	}

	// A CSV export from the OMS security master, with header names that do not
	// match the JSON field names exactly.
	rows, err = parseMaster([]byte("Trading Code,Company Name,ISIN,Board,Prev Close\nWMSHIPYARD,Western Marine Shipyard,BD0987654321,MAIN,73.30\n"))
	if err != nil {
		t.Fatalf("csv: %v", err)
	}
	u := buildUniverse(rows)
	if len(u.Instr) != 1 || u.Instr[0].Symbol != "WMSHIPYARD" || u.Instr[0].Prev != 73.30 {
		t.Fatalf("csv: got %+v", u.Instr)
	}
	if u.Instr[0].Name != "Western Marine Shipyard" {
		t.Fatalf("csv name: %q", u.Instr[0].Name)
	}

	// Tab-separated, same data.
	rows, err = parseMaster([]byte("symbol\tprev_close\nGP\t331.20\n"))
	if err != nil || len(rows) != 1 || rows[0].PrevClose != 331.20 {
		t.Fatalf("tsv: rows=%+v err=%v", rows, err)
	}
}

func TestParseMasterRejectsJunk(t *testing.T) {
	if _, err := parseMaster([]byte("")); err == nil {
		t.Fatal("empty master should error")
	}
	if _, err := parseMaster([]byte("name,board\nfoo,MAIN\n")); err == nil {
		t.Fatal("master with no symbol column should error")
	}
}

func TestLoadUniverseFilePreferredOverBuiltIn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instruments.json")
	if err := os.WriteFile(path, []byte(`[{"symbol":"UNITEDINS","prev_close":48.7},{"symbol":"WMSHIPYARD","prev_close":73.3}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ME_INSTRUMENT_FILE", path)
	got := loadUniverse()
	if len(got) != 2 {
		t.Fatalf("want the 2 instruments from the file, got %d (source %s)", len(got), universeSource())
	}
	if !strings.HasPrefix(metaFor("UNITEDINS").Board, "Main (") {
		t.Fatalf("board default not applied: %+v", metaFor("UNITEDINS"))
	}
}

// newTestEngine boots an engine on a tiny universe with its matching goroutine
// running, so control ops (listing) and order submission both work.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e := NewEngine([]Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 331.20}}, NewBus())
	go e.run()
	return e
}

func TestListInstrumentsAddsAndRefreshes(t *testing.T) {
	e := newTestEngine(t)

	added, updated := e.ListInstruments(universeSet{
		Instr: []Instrument{{Symbol: "UNITEDINS", Name: "United Insurance", Prev: 48.7}},
		Meta:  map[string]instrMetaT{"UNITEDINS": {Board: "MAIN", Tick: 0.2}},
	})
	if added != 1 || updated != 0 {
		t.Fatalf("want 1 added, got added=%d updated=%d", added, updated)
	}
	if !e.listed("UNITEDINS") || e.book("UNITEDINS") == nil {
		t.Fatal("UNITEDINS has no book after listing")
	}
	if len(e.instr()) != 2 {
		t.Fatalf("instrument list not grown: %d", len(e.instr()))
	}

	// Re-listing the same symbol with a corrected previous close refreshes it
	// in place rather than duplicating the book.
	added, updated = e.ListInstruments(universeSet{
		Instr: []Instrument{{Symbol: "UNITEDINS", Name: "United Insurance Ltd.", Prev: 50.0}},
	})
	if added != 0 || updated != 1 {
		t.Fatalf("want 1 updated, got added=%d updated=%d", added, updated)
	}
	if len(e.instr()) != 2 {
		t.Fatalf("re-listing duplicated the instrument: %d", len(e.instr()))
	}
	md := e.mdOf("UNITEDINS")
	if md.Prev != 50.0 || md.Last != 50.0 {
		t.Fatalf("untraded instrument not re-seeded to the new close: prev=%v last=%v", md.Prev, md.Last)
	}
}

// A symbol listed after boot must still get a real ITCH orderbook id: the feeds
// key every market-data message on it, and 0 would publish an unroutable book.
func TestListedInstrumentGetsOrderBookID(t *testing.T) {
	e := newTestEngine(t)
	if got := e.orderBookID("GP"); got != obIDBase {
		t.Fatalf("boot instrument id = %d, want %d", got, obIDBase)
	}
	e.ListInstruments(universeSet{Instr: []Instrument{{Symbol: "UNITEDINS", Prev: 48.7}}})
	if got := e.orderBookID("UNITEDINS"); got != obIDBase+1 {
		t.Fatalf("instrument listed at runtime got id %d, want %d", got, obIDBase+1)
	}
	if got := e.orderBookID("NEVERLISTED"); got != 0 {
		t.Fatalf("unlisted symbol got id %d, want 0", got)
	}
}

func TestAutoListOpensBookForUnknownSymbol(t *testing.T) {
	t.Setenv("ME_AUTO_LIST", "on")
	e := newTestEngine(t)
	if !e.autoList("VAMLBDMF1", 9.5) {
		t.Fatal("auto-list refused a well-formed symbol")
	}
	if md := e.mdOf("VAMLBDMF1"); md == nil || md.Prev != 9.5 {
		t.Fatalf("auto-listed instrument not seeded from the order price: %+v", md)
	}
	// The order that triggered the listing must now pass the price band, since
	// the band is anchored on the price it was listed at.
	o := e.newOrder("VAMLBDMF1", Buy, Limit, 9.5, 20, "FIX:BDTASK")
	rep := e.SubmitAndWait(o)
	if rep == nil || rep.Status == "REJECTED" {
		t.Fatalf("order that auto-listed the symbol was still rejected: %+v", rep)
	}
}

func TestAutoListOffRejectsUnknownSymbol(t *testing.T) {
	t.Setenv("ME_AUTO_LIST", "off")
	e := newTestEngine(t)
	if e.autoList("NOSUCHSCRIP", 10) {
		t.Fatal("auto-list ran with ME_AUTO_LIST=off")
	}
	if e.listed("NOSUCHSCRIP") {
		t.Fatal("book created despite auto-list being off")
	}
}

func TestAutoListRejectsImplausibleSymbols(t *testing.T) {
	t.Setenv("ME_AUTO_LIST", "on")
	e := newTestEngine(t)
	for _, bad := range []string{"", "lower case", "WAY-TOO-LONG-A-SYMBOL-INDEED", "SEMI;COLON"} {
		if e.autoList(bad, 10) {
			t.Fatalf("auto-listed junk symbol %q", bad)
		}
	}
}

func TestRejectsCarryAReason(t *testing.T) {
	t.Setenv("ME_AUTO_LIST", "off")
	e := newTestEngine(t)

	// Unknown symbol reaching the engine directly.
	rep := e.SubmitAndWait(e.newOrder("NOSUCHSCRIP", Buy, Limit, 10, 100, "FIX:BDTASK"))
	if rep.Status != "REJECTED" || rep.Reason == "" {
		t.Fatalf("unknown symbol reject has no reason: %+v", rep)
	}

	// Market order into an empty book.
	rep = e.SubmitAndWait(e.newOrder("GP", Buy, Market, 0, 100, "FIX:BDTASK"))
	if rep.Status != "REJECTED" || rep.Reason == "" {
		t.Fatalf("no-liquidity reject has no reason: %+v", rep)
	}

	// Off-tick and out-of-band limits (these already had reasons; guard them).
	rep = e.SubmitAndWait(e.newOrder("GP", Buy, Limit, 331.23, 100, "FIX:BDTASK"))
	if rep.Status != "REJECTED" || rep.Reason == "" {
		t.Fatalf("off-tick reject has no reason: %+v", rep)
	}
	rep = e.SubmitAndWait(e.newOrder("GP", Buy, Limit, 50, 100, "FIX:BDTASK"))
	if rep.Status != "REJECTED" || rep.Reason == "" {
		t.Fatalf("price-band reject has no reason: %+v", rep)
	}
}

func TestFixValidateNewReasons(t *testing.T) {
	t.Setenv("ME_AUTO_LIST", "off")
	fa := &FixAcceptor{eng: newTestEngine(t)}
	cases := []struct {
		name   string
		sym    string
		qty    float64
		typ    OrdType
		px     float64
		stopPx float64
	}{
		{"no symbol", "", 10, Limit, 50, 0},
		{"bad symbol", "not a ticker", 10, Limit, 50, 0},
		{"zero qty", "GP", 0, Limit, 50, 0},
		{"fractional qty", "GP", 1.5, Limit, 50, 0},
		{"stop with no trigger", "GP", 10, StopMkt, 0, 0},
		{"unlisted", "UNITEDINS", 10, Limit, 50, 0},
	}
	for _, c := range cases {
		if got := fa.validateNew(c.sym, c.qty, c.typ, c.px, c.stopPx); got == "" {
			t.Errorf("%s: accepted, want a reject reason", c.name)
		}
	}
	if got := fa.validateNew("GP", 100, Limit, 331.20, 0); got != "" {
		t.Errorf("valid order rejected: %s", got)
	}
}

// ---- admin add / update / halt -------------------------------------------

func adminPost(t *testing.T, eng *Engine, h func(http.ResponseWriter, *http.Request, *Engine), body string) map[string]any {
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

func addH(w http.ResponseWriter, r *http.Request, e *Engine) { saveInstrument(w, r, e, true) }
func updH(w http.ResponseWriter, r *http.Request, e *Engine) { saveInstrument(w, r, e, false) }

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
	if !eng.listed("NEWCO") || eng.mdOf("NEWCO").Prev != 50.5 {
		t.Fatal("NEWCO not listed with prev 50.5")
	}
	if m := metaFor("NEWCO"); m.Board != "SME" || m.ISIN != "BD0001NEW007" {
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
	if out["ok"] != true || metaFor("NEWCO").Board != "SME" {
		t.Fatalf("update: %v meta=%+v", out, metaFor("NEWCO"))
	}
	if out := adminPost(t, eng, updH, `{"Symbol":"NOPE","Prev":1}`); out["ok"] == true {
		t.Fatal("update of unknown symbol should fail")
	}

	// halt rejects orders, resume accepts them
	setHalted("NEWCO", true)
	rep := eng.SubmitAndWait(&Order{Symbol: "NEWCO", Side: Buy, Type: Limit, Price: 50.5, Qty: 10, OrigQty: 10, Actor: "FIX:T"})
	if rep.Status != "REJECTED" || !strings.Contains(rep.Reason, "halted") {
		t.Fatalf("halted order: %+v", rep)
	}
	setHalted("NEWCO", false)
	rep = eng.SubmitAndWait(&Order{Symbol: "NEWCO", Side: Buy, Type: Limit, Price: 50.5, Qty: 10, OrigQty: 10, Actor: "FIX:T"})
	if rep.Status == "REJECTED" {
		t.Fatalf("resumed order rejected: %+v", rep)
	}

	// persistence round-trip: halted state and the whole universe survive a reload
	setHalted("NEWCO", true)
	if err := persistUniverse(eng); err != nil {
		t.Fatalf("persist: %v", err)
	}
	u, err := loadUniverseFile(path)
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
	fixOrder := func(sym string, qty int64) *ExecReport {
		o := eng.newOrder(sym, Buy, Limit, eng.mdOf(sym).Prev, qty, "FIX:T")
		return eng.SubmitAndWait(o)
	}

	out := adminPost(t, eng, addH, `{"Symbol":"LOTCO","Name":"Lot Co","Board":"main (a)","Sector":"Pharma","Lot":1000,"Prev":50,"Tick":0.1}`)
	if out["ok"] != true {
		t.Fatalf("add: %v", out)
	}
	if m := metaFor("LOTCO"); m.Board != "Main (A)" || m.Sector != "Pharma" || m.Lot != 1000 {
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
	if out["ok"] != true || metaFor("PENDCO").statusOf() != "pending" {
		t.Fatalf("pending add: %v %+v", out, metaFor("PENDCO"))
	}
	if rep := fixOrder("PENDCO", 10); rep.Status != "REJECTED" || !strings.Contains(rep.Reason, "pending") {
		t.Fatalf("pending order: %+v", rep)
	}
	rows := instrumentRows(eng)
	sum, classes := instrumentSummary(rows)
	if sum["pending"] != 1 || len(classes) != 4 {
		t.Fatalf("summary: %v %v", sum, classes)
	}
	setStatus("PENDCO", "active")
	if rep := fixOrder("PENDCO", 10); rep.Status == "REJECTED" {
		t.Fatalf("activated order rejected: %+v", rep)
	}

	// persistence keeps sector/lot/status
	if err := persistUniverse(eng); err != nil {
		t.Fatal(err)
	}
	u, err := loadUniverseFile(path)
	if err != nil || u.Meta["LOTCO"].Lot != 1000 || u.Meta["LOTCO"].Sector != "Pharma" {
		t.Fatalf("round trip: %v %+v", err, u.Meta["LOTCO"])
	}

	// delist: leaves the directory and the saved master, rejects orders, can be re-listed
	setStatus("LOTCO", "delisted")
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
	if u, _ = loadUniverseFile(path); u.Meta["LOTCO"].Lot != 0 {
		t.Fatal("delisted scrip was saved to the master")
	}
	out = adminPost(t, eng, addH, `{"Symbol":"LOTCO","Name":"Lot Co","Prev":50}`)
	if out["ok"] != true || !metaFor("LOTCO").tradable() {
		t.Fatalf("relist: %v %+v", out, metaFor("LOTCO"))
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
	u, err := loadUniverseFile(path)
	if err != nil || u.Meta["FUND1"].AssetClass != "MF" || u.Meta["CBOND1"].AssetClass != "CB" {
		t.Fatalf("round trip: %v %+v", err, u.Meta["FUND1"])
	}
}

func TestEnrichMeta(t *testing.T) {
	// A real DSE ticker takes its reference values.
	m := enrichMeta("PTL", "Paramount Textile", instrMetaT{Board: "MAIN", Tick: 0.1})
	if m.AssetClass != "EQ" || m.Sector != "Textile" || m.Board != "Main (A)" || m.ISIN != "BD0458PTL002" {
		t.Fatalf("real ticker: %+v", m)
	}
	// A synthetic scrip is classified deterministically, with a name hint for its sector.
	a := enrichMeta("BANK042", "Bank 042 Ltd.", instrMetaT{Board: "MAIN"})
	b := enrichMeta("BANK042", "Bank 042 Ltd.", instrMetaT{Board: "MAIN"})
	if a != b || a.Sector != "Bank" || a.AssetClass != "EQ" || !strings.HasPrefix(a.Board, "Main (") && a.Board != "SME / ATB" {
		t.Fatalf("synthetic: %+v vs %+v", a, b)
	}
	// A real master's explicit board is never overwritten with invented data.
	if m := enrichMeta("NOTINREF", "X", instrMetaT{Board: "Main (B)"}); m.AssetClass != "" || m.Sector != "" || m.Board != "Main (B)" {
		t.Fatalf("explicit board touched: %+v", m)
	}
	// An already-classified scrip is left alone.
	if m := enrichMeta("PTL", "x", instrMetaT{Board: "Main (N)", Sector: "Jute", AssetClass: "EQ"}); m.Sector != "Jute" || m.Board != "Main (N)" {
		t.Fatalf("classified scrip changed: %+v", m)
	}
	// The built-in list is the real DSE end-of-day set: every class and board is present.
	classes, boards, sectors := map[string]int{}, map[string]int{}, map[string]int{}
	for _, in := range universe {
		e := enrichMeta(in.Symbol, in.Name, instrMetaT{Board: "MAIN"})
		classes[e.AssetClass]++
		boards[e.Board]++
		sectors[e.Sector]++
		if _, real := refdata.DSE[in.Symbol]; !real {
			t.Fatalf("built-in %s is not in the DSE reference data", in.Symbol)
		}
	}
	if len(universe) != 430 || classes["EQ"] != 374 || classes["MF"] != 36 || classes["CB"] != 12 || classes["DB"] != 8 {
		t.Fatalf("universe %d classes=%v", len(universe), classes)
	}
	for _, b := range []string{"Main (A)", "Main (B)", "Main (N)", "Main (Z)", "SME / ATB", "Mutual Fund", "Bond"} {
		if boards[b] == 0 {
			t.Fatalf("board %q missing: %v", b, boards)
		}
	}
	if len(sectors) != 21 {
		t.Fatalf("sectors: %v", sectors)
	}
}

// On a machine without /opt/velexa-sim-me (a developer laptop) the FIX acceptor
// must still start — it falls back to the dictionaries shipped in ./spec — and its
// data files must default to somewhere writable.
func TestFixAcceptorStartsWithDefaultPaths(t *testing.T) {
	if onServer() {
		t.Skip("running on a server with /opt/velexa-sim-me")
	}
	t.Chdir(repoRoot(t)) // the defaults ("spec", "data/…") are relative to where the engine is started
	t.Setenv("ME_FIX_SPEC_DIR", "")
	t.Setenv("ME_FIX_MSGLOG_FILE", filepath.Join(t.TempDir(), "m.log"))
	if specDir() != "spec" {
		t.Fatalf("specDir = %q", specDir())
	}
	if got := stateFile("fix_brokers.json"); got != "data/fix_brokers.json" {
		t.Fatalf("stateFile = %q", got)
	}
	eng := newTestEngine(t)
	fa, err := StartFixAcceptor(eng, strconv.Itoa(freePort(t)), "DSEME")
	if err != nil {
		t.Fatalf("acceptor failed to start with default paths: %v", err)
	}
	_ = fa // not stopped: quickfix's Acceptor.Stop races on a session that never logged on
}

func TestInstrumentExport(t *testing.T) {
	t.Setenv("ME_INSTRUMENT_FILE", filepath.Join(t.TempDir(), "i.json"))
	eng := newTestEngine(t)
	adminPost(t, eng, addH, `{"Symbol":"EXPA","Name":"Export & Co <Ltd>","AssetClass":"EQ","Board":"Main (B)","Sector":"Bank","Prev":50,"ISIN":"BD0001EXPA09"}`)
	adminPost(t, eng, addH, `{"Symbol":"EXPF","Name":"Export Fund","AssetClass":"MF","Board":"Mutual Fund","Sector":"MutFund","Prev":8.5,"Status":"halted"}`)

	// make EXPA trade so it carries OHLC and a variation
	eng.SubmitAndWait(eng.newOrder("EXPA", Sell, Limit, 51, 100, "SEED"))
	eng.SubmitAndWait(eng.newOrder("EXPA", Buy, Limit, 51, 100, "SEED"))

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
		// a traded equity: every attribute, Var/VarPercent against the previous close of 50
		`<Ticker SecurityCode="EXPA" ISIN="BD0001EXPA09" Category="B" AssetClass="EQ" Sector="Bank" CompulsorySpot="N" TradeDate="`,
		`Close="51" Open="50" High="51" Low="50" Var="1.00000" VarPercent="2.00000" />`,
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
	rows, err := parseMaster(body)
	if err != nil || len(rows) != 2 || rows[0].Sector != "Bank" || rows[1].AssetClass != "MF" {
		t.Fatalf("json re-import: %v %+v", err, rows)
	}
}
