package me

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// instrMeta carries per-symbol board metadata for the engine's board-aware
// pre-trade rules (tick size, price band). Seeded at boot by loadUniverse from
// the exchange master and updated whenever a new master is imported or a symbol
// is auto-listed, so it is guarded — several goroutines (ITCH gateway, live
// bench, matching engine) read it concurrently.
type instrMetaT struct {
	Board  string
	ISIN   string
	Tick   float64
	Sector string
	// AssetClass is the DSE asset class code: EQ equity, MF mutual fund,
	// CB corporate bond, DB debenture (indices, IN, are computed, not traded).
	// Empty means "derive from the board" (see assetClassOf).
	AssetClass string
	Lot        int // market lot / minimum quantity step; <=1 means any whole quantity
	// Status flags (at most one set; none = active). Halted/Pending/Delisted all
	// make the engine reject new orders; they persist as the master file's status.
	Halted   bool // trading halt
	Pending  bool // listed but awaiting approval — not yet tradable
	Delisted bool // removed from the directory; the book stays until restart
}

// assetClasses are the tradable DSE asset classes, in display order.
var assetClasses = []struct{ Code, Label, Color string }{
	{"EQ", "Equity", "#4dd0b1"},
	{"MF", "Mutual Fund", "#c084fc"},
	{"CB", "Corporate Bond", "#eab04d"},
	{"DB", "Debenture", "#7aa2f7"},
}

// normAssetClass returns the canonical code for a tradable asset class, or "".
func normAssetClass(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	for _, a := range assetClasses {
		if s == a.Code {
			return s
		}
	}
	return ""
}

// assetClassOf is the scrip's asset class: the stored one, else derived from the
// board label for masters that predate the field.
func assetClassOf(m instrMetaT) string {
	if m.AssetClass != "" {
		return m.AssetClass
	}
	b := strings.ToLower(m.Board)
	switch {
	case strings.Contains(b, "bond"):
		return "CB"
	case strings.Contains(b, "mutual"):
		return "MF"
	}
	return "EQ"
}

// statusOf names the metadata's trading status: active | halted | pending | delisted.
func (m instrMetaT) statusOf() string {
	switch {
	case m.Delisted:
		return "delisted"
	case m.Pending:
		return "pending"
	case m.Halted:
		return "halted"
	}
	return "active"
}

// tradable reports whether the scrip accepts new orders.
func (m instrMetaT) tradable() bool { return !m.Halted && !m.Pending && !m.Delisted }

var (
	instrMetaMu sync.RWMutex
	instrMeta   = map[string]instrMetaT{}
)

func metaFor(sym string) instrMetaT {
	instrMetaMu.RLock()
	defer instrMetaMu.RUnlock()
	return instrMeta[sym]
}

// setHalted flips the trading-halt flag on a symbol, leaving the rest of its
// metadata untouched.
func setHalted(sym string, halted bool) {
	st := "active"
	if halted {
		st = "halted"
	}
	setStatus(sym, st)
}

// setStatus moves a symbol to active | halted | pending | delisted, leaving the
// rest of its metadata untouched.
func setStatus(sym, status string) {
	instrMetaMu.Lock()
	m := instrMeta[sym]
	m.Halted, m.Pending, m.Delisted = status == "halted", status == "pending", status == "delisted"
	instrMeta[sym] = m
	instrMetaMu.Unlock()
}

func setMeta(sym string, m instrMetaT) {
	instrMetaMu.Lock()
	instrMeta[sym] = m
	instrMetaMu.Unlock()
}

// universeSrc records where the live instrument universe came from, so the
// admin console can show whether the ME is on the real exchange master or on
// the synthetic built-in fallback (the cause of "unknown symbol" rejects).
var universeSrc atomic.Value // string

func universeSource() string {
	if s, ok := universeSrc.Load().(string); ok {
		return s
	}
	return "unknown"
}

// instrumentFile is the operator-supplied security master on the ME box. It is
// the highest-priority source: dropping a file here pins the universe without
// needing the exchange endpoint to be reachable. Remove it to fall back to the
// exchange master again.
func instrumentFile() string {
	if v := os.Getenv("ME_INSTRUMENT_FILE"); v != "" {
		return v
	}
	// Production keeps its state under /opt/velexa-sim-me. On a dev box without
	// that directory, keep the master next to the binary instead of failing every
	// admin save with "permission denied".
	if st, err := os.Stat("/opt/velexa-sim-me"); err == nil && st.IsDir() {
		return "/opt/velexa-sim-me/instruments.json"
	}
	return "instruments.json"
}

func instrumentURL() string {
	return envOr("EXCHANGE_INSTRUMENT_URL", "http://10.1.1.45:8088/api/instrument-master")
}

// masterInstr is one row of the exchange security master — the shape the
// exchange publishes over HTTP and that ME_INSTRUMENT_FILE mirrors.
type masterInstr struct {
	Symbol     string  `json:"symbol"`
	ISIN       string  `json:"isin"`
	Name       string  `json:"name"`
	Board      string  `json:"board"`
	PrevClose  float64 `json:"prev_close"`
	Tick       float64 `json:"tick"`
	Sector     string  `json:"sector,omitempty"`
	AssetClass string  `json:"asset_class,omitempty"`
	Lot        int     `json:"lot,omitempty"`
	IsIndex    bool    `json:"is_index"`
	Status     string  `json:"status"`
}

// universeSet is a parsed security master ready to hand to the engine: the
// tradeable instrument list plus the per-symbol board metadata that drives the
// tick-size and price-band checks.
type universeSet struct {
	Instr []Instrument
	Meta  map[string]instrMetaT
}

func (u universeSet) apply() {
	instrMetaMu.Lock()
	for s, m := range u.Meta {
		instrMeta[s] = m
	}
	instrMetaMu.Unlock()
}

// loadUniverse resolves the instrument universe from, in order:
//
//  1. ME_INSTRUMENT_FILE  — an operator-supplied master on the ME box (JSON or CSV/TSV)
//  2. EXCHANGE_INSTRUMENT_URL — the exchange's published master (Model 1 source of truth)
//  3. the built-in universe — synthetic fallback so the ME always boots
//
// The file wins because it is an explicit operator action: the exchange endpoint
// is not always routable from the ME's network segment, and running on the
// synthetic fallback is what makes real DSE symbols reject as unknown.
func loadUniverse() []Instrument {
	if path := strings.TrimSpace(instrumentFile()); path != "" {
		if u, err := loadUniverseFile(path); err != nil {
			if !os.IsNotExist(err) {
				log.Printf("[universe] instrument file %s unusable: %v — trying exchange master", path, err)
			}
		} else {
			u.apply()
			universeSrc.Store(fmt.Sprintf("file:%s (%d instruments)", path, len(u.Instr)))
			log.Printf("[universe] loaded %d instruments from %s", len(u.Instr), path)
			return u.Instr
		}
	}
	if u, err := fetchUniverseURL(instrumentURL()); err != nil {
		log.Printf("[universe] %v — using built-in universe (%d)", err, len(universe))
	} else {
		u.apply()
		universeSrc.Store(fmt.Sprintf("exchange:%s (%d instruments)", instrumentURL(), len(u.Instr)))
		log.Printf("[universe] loaded %d instruments from exchange master via %s", len(u.Instr), instrumentURL())
		return u.Instr
	}
	bi := universeSet{Instr: universe, Meta: map[string]instrMetaT{}}
	for _, in := range universe {
		bi.Meta[in.Symbol] = instrMetaT{Board: "MAIN", Tick: defaultTick(in.Prev), Lot: 1}
	}
	bi.enrich()
	for sym, m := range bi.Meta {
		if m.AssetClass == "CB" || m.AssetClass == "DB" {
			m.Tick = 0.01 // bonds and debentures quote to a paisa
			bi.Meta[sym] = m
		}
	}
	bi.apply()
	universeSrc.Store(fmt.Sprintf("built-in DSE reference list (%d instruments, DSE end-of-day file 14 Sep 2023) — symbols outside this list will not trade", len(universe)))
	log.Printf("[universe] WARNING: running on the built-in DSE reference list. Orders for symbols outside it are rejected as unknown unless ME_AUTO_LIST is on. Supply the real master at %s or set EXCHANGE_INSTRUMENT_URL.", instrumentFile())
	return universe
}

// loadUniverseFile reads and parses an operator-supplied master file.
func loadUniverseFile(path string) (universeSet, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return universeSet{}, err
	}
	rows, err := parseMaster(b)
	if err != nil {
		return universeSet{}, err
	}
	u := buildUniverse(rows)
	if len(u.Instr) == 0 {
		return universeSet{}, fmt.Errorf("no tradeable instruments in %s (%d rows read)", path, len(rows))
	}
	return u, nil
}

// fetchUniverseURL pulls the exchange's published security master.
func fetchUniverseURL(url string) (universeSet, error) {
	if strings.TrimSpace(url) == "" {
		return universeSet{}, fmt.Errorf("no exchange instrument URL configured")
	}
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return universeSet{}, fmt.Errorf("fetch %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return universeSet{}, fmt.Errorf("fetch %s returned HTTP %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return universeSet{}, fmt.Errorf("read %s failed: %v", url, err)
	}
	rows, err := parseMaster(b)
	if err != nil {
		return universeSet{}, fmt.Errorf("decode %s failed: %v", url, err)
	}
	u := buildUniverse(rows)
	// Safety: until the exchange master is actually populated (real DSE has ~350),
	// a near-empty response means the admin has not imported the EOD file yet — keep
	// the built-in universe so the ME never boots with a crippled book set.
	if len(u.Instr) < 50 {
		return universeSet{}, fmt.Errorf("exchange master has only %d tradeable (<50) — not yet imported", len(u.Instr))
	}
	return u, nil
}

// buildUniverse turns raw master rows into the tradeable universe. Indices
// (is_index) are computed by the engine, not traded, so they are skipped, as are
// delisted scrips. Halted scrips are listed (so they keep their book and show in
// the terminal) but flagged to reject new orders.
func buildUniverse(rows []masterInstr) universeSet {
	u := universeSet{Instr: make([]Instrument, 0, len(rows)), Meta: map[string]instrMetaT{}}
	byBoard := map[string]int{}
	seen := map[string]bool{}
	for _, i := range rows {
		sym := strings.ToUpper(strings.TrimSpace(i.Symbol))
		if !validSymbol(sym) || seen[sym] {
			continue
		}
		if i.IsIndex || strings.EqualFold(i.Status, "delisted") {
			continue
		}
		seen[sym] = true
		prev := i.PrevClose
		if prev <= 0 {
			prev = 100
		}
		name := strings.TrimSpace(i.Name)
		if name == "" {
			name = sym
		}
		tick := i.Tick
		if tick <= 0 {
			tick = defaultTick(prev)
		}
		board := strings.ToUpper(strings.TrimSpace(i.Board))
		if board == "" {
			board = "MAIN"
		}
		lot := i.Lot
		if lot < 1 {
			lot = 1
		}
		status := strings.ToLower(strings.TrimSpace(i.Status))
		u.Meta[sym] = instrMetaT{Board: board, ISIN: strings.TrimSpace(i.ISIN), Tick: tick,
			Sector: strings.TrimSpace(i.Sector), Lot: lot, AssetClass: normAssetClass(i.AssetClass),
			Halted: status == "halted", Pending: status == "pending"}
		u.Instr = append(u.Instr, Instrument{Symbol: sym, Name: name, Prev: round2(prev)})
		byBoard[board]++
	}
	u.enrich()
	if len(u.Instr) > 0 {
		log.Printf("[universe] parsed %d tradeable instruments (MAIN=%d SME=%d ATB=%d)",
			len(u.Instr), byBoard["MAIN"], byBoard["SME"], byBoard["ATB"])
	}
	return u
}

// defaultTick is the tick size assumed when the master does not carry one: the
// DSE quotes every board on a flat Tk 0.10 tick. Deliberately NOT the price-
// tiered ladder the simulator's own bots quote on (tickSize) — that is a
// market-making heuristic, and using it here would reject legitimate OMS prices
// such as 73.30 as off-tick. A master that carries a real tick overrides this.
func defaultTick(float64) float64 { return 0.10 }

// validSymbol guards what may become a book: an exchange ticker, not free text.
func validSymbol(s string) bool {
	if len(s) == 0 || len(s) > 20 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// parseMaster accepts the exchange JSON payload ({"instruments":[...]}), a bare
// JSON array of the same objects, or a delimited export (CSV/TSV with a header
// row) — whichever the operator has to hand from the OMS security master.
func parseMaster(b []byte) ([]masterInstr, error) {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" {
		return nil, fmt.Errorf("empty master")
	}
	switch trimmed[0] {
	case '{':
		var payload struct {
			Count       int           `json:"count"`
			Instruments []masterInstr `json:"instruments"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return nil, err
		}
		return payload.Instruments, nil
	case '[':
		var rows []masterInstr
		if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
			return nil, err
		}
		return rows, nil
	}
	return parseDelimited(trimmed)
}

// parseDelimited reads a CSV/TSV security-master export. The header row names
// the columns; only a symbol column is required.
func parseDelimited(text string) ([]masterInstr, error) {
	head, _, _ := strings.Cut(text, "\n")
	r := csv.NewReader(strings.NewReader(text))
	if strings.Contains(head, "\t") && !strings.Contains(head, ",") {
		r.Comma = '\t'
	}
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("delimited master needs a header row plus at least one instrument")
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[normHeader(h)] = i
	}
	pick := func(rec []string, names ...string) string {
		for _, n := range names {
			if i, ok := col[n]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
		}
		return ""
	}
	if _, ok := col["symbol"]; !ok {
		if _, ok := col["ticker"]; !ok {
			if _, ok := col["tradingcode"]; !ok {
				return nil, fmt.Errorf("delimited master has no symbol/ticker/trading_code column (header: %s)", strings.Join(recs[0], ","))
			}
		}
	}
	out := make([]masterInstr, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		sym := pick(rec, "symbol", "ticker", "tradingcode", "code", "scrip")
		if sym == "" {
			continue
		}
		prev, _ := strconv.ParseFloat(strings.ReplaceAll(pick(rec, "prevclose", "prev", "close", "closingprice", "lastclose", "ltp"), ",", ""), 64)
		tick, _ := strconv.ParseFloat(pick(rec, "tick", "ticksize"), 64)
		lot, _ := strconv.Atoi(pick(rec, "lot", "marketlot", "lotsize"))
		idx := pick(rec, "isindex", "index")
		out = append(out, masterInstr{
			Symbol:     sym,
			ISIN:       pick(rec, "isin"),
			Name:       pick(rec, "name", "companyname", "company", "fullname"),
			Board:      pick(rec, "board", "market", "segment", "boardcode"),
			PrevClose:  prev,
			Tick:       tick,
			Sector:     pick(rec, "sector", "industry"),
			AssetClass: pick(rec, "assetclass", "class"),
			Lot:        lot,
			IsIndex:    idx == "1" || strings.EqualFold(idx, "true") || strings.EqualFold(idx, "y") || strings.EqualFold(idx, "yes"),
			Status:     pick(rec, "status"),
		})
	}
	return out, nil
}

// normHeader folds a header cell to a lookup key: lowercase, letters+digits only
// (so "Prev Close", "prev_close" and "PrevClose" all match).
func normHeader(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
