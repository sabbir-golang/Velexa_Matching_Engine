package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"velexa-sim-me/internal/refdata"
)

func TestParseMasterJSONPayload(t *testing.T) {
	rows, err := ParseMaster([]byte(`{"count":2,"instruments":[
		{"symbol":"UNITEDINS","isin":"BD0123456789","name":"United Insurance","board":"MAIN","prev_close":48.7,"tick":0.2},
		{"symbol":"DSEX","is_index":true}]}`))
	if err != nil {
		t.Fatalf("parseMaster: %v", err)
	}
	u := BuildUniverse(rows)
	if len(u.Instr) != 1 || u.Instr[0].Symbol != "UNITEDINS" || u.Instr[0].Prev != 48.7 {
		t.Fatalf("want the one tradeable scrip, got %+v", u.Instr)
	}
	if m := u.Meta["UNITEDINS"]; !strings.HasPrefix(m.Board, "Main (") || m.ISIN != "BD0123456789" || m.Tick != 0.2 {
		t.Fatalf("meta not carried through: %+v", m)
	}
}

func TestParseMasterBareArrayAndCSV(t *testing.T) {
	rows, err := ParseMaster([]byte(`[{"symbol":"VAMLBDMF1","prev_close":9.5}]`))
	if err != nil || len(rows) != 1 || rows[0].Symbol != "VAMLBDMF1" {
		t.Fatalf("bare array: rows=%+v err=%v", rows, err)
	}

	// A CSV export from the OMS security master, with header names that do not
	// match the JSON field names exactly.
	rows, err = ParseMaster([]byte("Trading Code,Company Name,ISIN,Board,Prev Close\nWMSHIPYARD,Western Marine Shipyard,BD0987654321,MAIN,73.30\n"))
	if err != nil {
		t.Fatalf("csv: %v", err)
	}
	u := BuildUniverse(rows)
	if len(u.Instr) != 1 || u.Instr[0].Symbol != "WMSHIPYARD" || u.Instr[0].Prev != 73.30 {
		t.Fatalf("csv: got %+v", u.Instr)
	}
	if u.Instr[0].Name != "Western Marine Shipyard" {
		t.Fatalf("csv name: %q", u.Instr[0].Name)
	}

	// Tab-separated, same data.
	rows, err = ParseMaster([]byte("symbol\tprev_close\nGP\t331.20\n"))
	if err != nil || len(rows) != 1 || rows[0].PrevClose != 331.20 {
		t.Fatalf("tsv: rows=%+v err=%v", rows, err)
	}
}

func TestParseMasterRejectsJunk(t *testing.T) {
	if _, err := ParseMaster([]byte("")); err == nil {
		t.Fatal("empty master should error")
	}
	if _, err := ParseMaster([]byte("name,board\nfoo,MAIN\n")); err == nil {
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
	got := LoadUniverse()
	if len(got) != 2 {
		t.Fatalf("want the 2 instruments from the file, got %d (source %s)", len(got), UniverseSource())
	}
	if !strings.HasPrefix(MetaFor("UNITEDINS").Board, "Main (") {
		t.Fatalf("board default not applied: %+v", MetaFor("UNITEDINS"))
	}
}

// newTestEngine boots an engine on a tiny universe with its matching goroutine
// running, so control ops (listing) and order submission both work.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e := NewEngine([]Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 331.20}}, NewBus())
	go e.Run()
	return e
}

func TestListInstrumentsAddsAndRefreshes(t *testing.T) {
	e := newTestEngine(t)

	added, updated := e.ListInstruments(UniverseSet{
		Instr: []Instrument{{Symbol: "UNITEDINS", Name: "United Insurance", Prev: 48.7}},
		Meta:  map[string]InstrMeta{"UNITEDINS": {Board: "MAIN", Tick: 0.2}},
	})
	if added != 1 || updated != 0 {
		t.Fatalf("want 1 added, got added=%d updated=%d", added, updated)
	}
	if !e.Listed("UNITEDINS") || e.Book("UNITEDINS") == nil {
		t.Fatal("UNITEDINS has no book after listing")
	}
	if len(e.Instr()) != 2 {
		t.Fatalf("instrument list not grown: %d", len(e.Instr()))
	}

	// Re-listing the same symbol with a corrected previous close refreshes it
	// in place rather than duplicating the book.
	added, updated = e.ListInstruments(UniverseSet{
		Instr: []Instrument{{Symbol: "UNITEDINS", Name: "United Insurance Ltd.", Prev: 50.0}},
	})
	if added != 0 || updated != 1 {
		t.Fatalf("want 1 updated, got added=%d updated=%d", added, updated)
	}
	if len(e.Instr()) != 2 {
		t.Fatalf("re-listing duplicated the instrument: %d", len(e.Instr()))
	}
	md := e.MdOf("UNITEDINS")
	if md.Prev != 50.0 || md.Last != 50.0 {
		t.Fatalf("untraded instrument not re-seeded to the new close: prev=%v last=%v", md.Prev, md.Last)
	}
}

// A symbol listed after boot must still get a real ITCH orderbook id: the feeds
// key every market-data message on it, and 0 would publish an unroutable book.
func TestListedInstrumentGetsOrderBookID(t *testing.T) {
	e := newTestEngine(t)
	if got := e.OrderBookID("GP"); got != obIDBase {
		t.Fatalf("boot instrument id = %d, want %d", got, obIDBase)
	}
	e.ListInstruments(UniverseSet{Instr: []Instrument{{Symbol: "UNITEDINS", Prev: 48.7}}})
	if got := e.OrderBookID("UNITEDINS"); got != obIDBase+1 {
		t.Fatalf("instrument listed at runtime got id %d, want %d", got, obIDBase+1)
	}
	if got := e.OrderBookID("NEVERLISTED"); got != 0 {
		t.Fatalf("unlisted symbol got id %d, want 0", got)
	}
}

func TestAutoListOpensBookForUnknownSymbol(t *testing.T) {
	t.Setenv("ME_AUTO_LIST", "on")
	e := newTestEngine(t)
	if !e.AutoList("VAMLBDMF1", 9.5) {
		t.Fatal("auto-list refused a well-formed symbol")
	}
	if md := e.MdOf("VAMLBDMF1"); md == nil || md.Prev != 9.5 {
		t.Fatalf("auto-listed instrument not seeded from the order price: %+v", md)
	}
	// The order that triggered the listing must now pass the price band, since
	// the band is anchored on the price it was listed at.
	o := e.NewOrder("VAMLBDMF1", Buy, Limit, 9.5, 20, "FIX:BDTASK")
	rep := e.SubmitAndWait(o)
	if rep == nil || rep.Status == "REJECTED" {
		t.Fatalf("order that auto-listed the symbol was still rejected: %+v", rep)
	}
}

func TestAutoListOffRejectsUnknownSymbol(t *testing.T) {
	t.Setenv("ME_AUTO_LIST", "off")
	e := newTestEngine(t)
	if e.AutoList("NOSUCHSCRIP", 10) {
		t.Fatal("auto-list ran with ME_AUTO_LIST=off")
	}
	if e.Listed("NOSUCHSCRIP") {
		t.Fatal("book created despite auto-list being off")
	}
}

func TestAutoListRejectsImplausibleSymbols(t *testing.T) {
	t.Setenv("ME_AUTO_LIST", "on")
	e := newTestEngine(t)
	for _, bad := range []string{"", "lower case", "WAY-TOO-LONG-A-SYMBOL-INDEED", "SEMI;COLON"} {
		if e.AutoList(bad, 10) {
			t.Fatalf("auto-listed junk symbol %q", bad)
		}
	}
}

func TestRejectsCarryAReason(t *testing.T) {
	t.Setenv("ME_AUTO_LIST", "off")
	e := newTestEngine(t)

	// Unknown symbol reaching the engine directly.
	rep := e.SubmitAndWait(e.NewOrder("NOSUCHSCRIP", Buy, Limit, 10, 100, "FIX:BDTASK"))
	if rep.Status != "REJECTED" || rep.Reason == "" {
		t.Fatalf("unknown symbol reject has no reason: %+v", rep)
	}

	// Market order into an empty book.
	rep = e.SubmitAndWait(e.NewOrder("GP", Buy, Market, 0, 100, "FIX:BDTASK"))
	if rep.Status != "REJECTED" || rep.Reason == "" {
		t.Fatalf("no-liquidity reject has no reason: %+v", rep)
	}

	// Off-tick and out-of-band limits (these already had reasons; guard them).
	rep = e.SubmitAndWait(e.NewOrder("GP", Buy, Limit, 331.23, 100, "FIX:BDTASK"))
	if rep.Status != "REJECTED" || rep.Reason == "" {
		t.Fatalf("off-tick reject has no reason: %+v", rep)
	}
	rep = e.SubmitAndWait(e.NewOrder("GP", Buy, Limit, 50, 100, "FIX:BDTASK"))
	if rep.Status != "REJECTED" || rep.Reason == "" {
		t.Fatalf("price-band reject has no reason: %+v", rep)
	}
}

func TestEnrichMeta(t *testing.T) {
	// A real DSE ticker takes its reference values.
	m := enrichMeta("PTL", "Paramount Textile", InstrMeta{Board: "MAIN", Tick: 0.1})
	if m.AssetClass != "EQ" || m.Sector != "Textile" || m.Board != "Main (A)" || m.ISIN != "BD0458PTL002" {
		t.Fatalf("real ticker: %+v", m)
	}
	// A synthetic scrip is classified deterministically, with a name hint for its sector.
	a := enrichMeta("BANK042", "Bank 042 Ltd.", InstrMeta{Board: "MAIN"})
	b := enrichMeta("BANK042", "Bank 042 Ltd.", InstrMeta{Board: "MAIN"})
	if a != b || a.Sector != "Bank" || a.AssetClass != "EQ" || !strings.HasPrefix(a.Board, "Main (") && a.Board != "SME / ATB" {
		t.Fatalf("synthetic: %+v vs %+v", a, b)
	}
	// A real master's explicit board is never overwritten with invented data.
	if m := enrichMeta("NOTINREF", "X", InstrMeta{Board: "Main (B)"}); m.AssetClass != "" || m.Sector != "" || m.Board != "Main (B)" {
		t.Fatalf("explicit board touched: %+v", m)
	}
	// An already-classified scrip is left alone.
	if m := enrichMeta("PTL", "x", InstrMeta{Board: "Main (N)", Sector: "Jute", AssetClass: "EQ"}); m.Sector != "Jute" || m.Board != "Main (N)" {
		t.Fatalf("classified scrip changed: %+v", m)
	}
	// The built-in list is the real DSE end-of-day set: every class and board is present.
	classes, boards, sectors := map[string]int{}, map[string]int{}, map[string]int{}
	for _, in := range universe {
		e := enrichMeta(in.Symbol, in.Name, InstrMeta{Board: "MAIN"})
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
