package main

// instruments.go — keeping the engine's tradeable universe in step with the
// exchange/OMS security master at runtime.
//
// The ME can only match symbols it has a book for. When its universe drifts
// behind the OMS's (the exchange master endpoint unreachable, an EOD import not
// yet run), every order for a symbol the OMS knows and the ME does not comes
// back rejected. This file provides the two ways out:
//
//   - listInstruments — apply a security master to a RUNNING engine, adding
//     books for new scrips and refreshing the previous close (the price-band
//     anchor) of existing ones, with no restart.
//   - autoList — open a book on demand for a well-formed symbol arriving over
//     FIX, so a stale master degrades the simulation rather than halting it.

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---- auto-listing ---------------------------------------------------------

// autoListEnabled reports whether an order for an unlisted symbol should open a
// book instead of being rejected. On by default: this is a simulator standing in
// for the exchange, and an OMS whose security master is ahead of the ME's should
// still be able to trade. Set ME_AUTO_LIST=off to enforce the master strictly.
func autoListEnabled() bool {
	switch strings.ToLower(envOr("ME_AUTO_LIST", "on")) {
	case "off", "0", "false", "no":
		return false
	}
	return true
}

var (
	autoListMu   sync.Mutex
	autoListedAt = map[string]time.Time{}
)

func noteAutoListed(sym string) {
	autoListMu.Lock()
	autoListedAt[sym] = time.Now()
	autoListMu.Unlock()
}

// autoListedSymbols returns the symbols this process opened books for on the
// fly. A non-empty list means the loaded security master is behind the OMS's —
// the admin console surfaces it as exactly that.
func autoListedSymbols() []string {
	autoListMu.Lock()
	defer autoListMu.Unlock()
	out := make([]string, 0, len(autoListedAt))
	for s := range autoListedAt {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// autoList opens a book for a symbol that is not in the loaded master, seeding
// its reference price from the order that asked for it (so the price band is
// anchored somewhere sane rather than rejecting the very order that listed it).
// Returns false if auto-listing is off or the symbol is not a plausible ticker.
func (e *Engine) autoList(sym string, seedPx float64) bool {
	if !autoListEnabled() || !validSymbol(sym) {
		return false
	}
	if seedPx <= 0 {
		seedPx = 100
	}
	seedPx = round2(seedPx)
	in := Instrument{Symbol: sym, Name: sym, Prev: seedPx}
	added, _ := e.ListInstruments(universeSet{
		Instr: []Instrument{in},
		Meta:  map[string]instrMetaT{sym: {Board: "MAIN", Tick: defaultTick(seedPx)}},
	})
	if added == 0 {
		return e.listed(sym) // lost a race with a concurrent listing — still fine
	}
	noteAutoListed(sym)
	log.Printf("[universe] AUTO-LISTED %s at %.2f — it is not in the loaded security master (%s). Import the real master to get its true previous close and tick.",
		sym, seedPx, universeSource())
	return true
}

// ---- applying a master to a running engine --------------------------------

// ListInstruments applies a security master to the running engine and returns
// how many instruments it added and refreshed. Safe to call at any time.
func (e *Engine) ListInstruments(u universeSet) (added, updated int) {
	u.apply() // metadata first: the pre-trade checks read it the moment a book exists
	type result struct {
		added, updated int
		fresh          []string
	}
	done := make(chan result, 1)
	e.ctrl <- func() {
		a, up, fresh := e.listInstruments(u.Instr)
		done <- result{a, up, fresh}
	}
	r := <-done
	e.startBotsFor(r.fresh)
	return r.added, r.updated
}

// listInstruments rebuilds the books/market-data/instrument snapshots with the
// given instruments folded in. Runs on the engine's control goroutine with the
// engine lock held; the containers are swapped copy-on-write so the lock-free
// readers (ITCH feed, gateway, bench) never observe a half-built map.
func (e *Engine) listInstruments(ins []Instrument) (added, updated int, fresh []string) {
	books := *e.booksMap.Load()
	md := *e.mdMap.Load()
	list := e.instr()

	nb := make(map[string]*Book, len(books)+len(ins))
	for k, v := range books {
		nb[k] = v
	}
	nm := make(map[string]*MarketData, len(md)+len(ins))
	for k, v := range md {
		nm[k] = v
	}
	nl := append(make([]Instrument, 0, len(list)+len(ins)), list...)
	pos := make(map[string]int, len(nl))
	for i, in := range nl {
		pos[in.Symbol] = i
	}
	obs := make(map[string]int, len(nl)+len(ins))
	for k, v := range *e.obIDs.Load() {
		obs[k] = v
	}

	for _, in := range ins {
		in.Symbol = strings.ToUpper(strings.TrimSpace(in.Symbol))
		if !validSymbol(in.Symbol) {
			continue
		}
		if in.Prev <= 0 {
			in.Prev = 100
		}
		if in.Name == "" {
			in.Name = in.Symbol
		}
		cur := nm[in.Symbol]
		if cur == nil { // newly listed scrip
			nb[in.Symbol] = newBook(in.Symbol)
			nm[in.Symbol] = newMarketData(in)
			obs[in.Symbol] = obIDBase + len(nl)
			pos[in.Symbol] = len(nl)
			nl = append(nl, in)
			fresh = append(fresh, in.Symbol)
			added++
			continue
		}
		// Already listed: refresh the reference data in place. An instrument
		// that has not traded yet is re-seeded to the new previous close; one
		// that has is left on its traded prices, but still re-anchors its band.
		cur.Name = in.Name
		if cur.Prev != in.Prev {
			cur.Prev = in.Prev
			if cur.Trades == 0 {
				cur.Open, cur.Last, cur.High, cur.Low, cur.Ref = in.Prev, in.Prev, in.Prev, in.Prev, in.Prev
			}
		}
		nl[pos[in.Symbol]] = in
		updated++
	}
	if added == 0 && updated == 0 {
		return 0, 0, nil
	}
	// Order matters: instrList is published last, so a lock-free reader that
	// iterates the instrument list and then looks a symbol up always finds the
	// books and market data for it (both maps only ever grow).
	e.booksMap.Store(&nb)
	e.mdMap.Store(&nm)
	e.obIDs.Store(&obs)
	e.instrList.Store(&nl)
	return added, updated, fresh
}

// startBotsFor gives newly listed instruments the same market-maker + noise
// trader every boot-time instrument gets, so a new book is quotable immediately
// instead of sitting empty (which would reject every market order against it).
func (e *Engine) startBotsFor(syms []string) {
	for i, sym := range syms {
		seed := time.Now().UnixNano() + int64(i)
		go e.marketMaker(sym, rand.New(rand.NewSource(seed)))
		go e.noiseTrader(sym, rand.New(rand.NewSource(seed+1000)))
	}
}

// ---- admin API ------------------------------------------------------------

// registerInstrumentAdmin wires the security-master endpoints into the admin
// console: inspect what the engine is trading, re-read the configured sources,
// and import a master directly (which is also persisted, so it survives a
// restart).
func registerInstrumentAdmin(mux *http.ServeMux, eng *Engine) {
	mux.HandleFunc("/admin/api/instruments", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		instr := eng.instr()
		rows := instrumentRows(eng)
		summary, classes := instrumentSummary(rows)
		syms := make([]string, 0, len(instr))
		for _, in := range instr {
			syms = append(syms, in.Symbol)
		}
		sort.Strings(syms)
		auto := autoListedSymbols()
		note := "the loaded security master covers every symbol traded so far"
		if len(auto) > 0 {
			note = fmt.Sprintf("%d symbol(s) had to be auto-listed because the loaded master does not carry them — import the real master to give them their true previous close and tick", len(auto))
		}
		writeJSON(w, map[string]any{
			"count":        len(instr),
			"source":       universeSource(),
			"file":         instrumentFile(),
			"exchange_url": instrumentURL(),
			"auto_list":    autoListEnabled(),
			"auto_listed":  auto,
			"symbols":      syms,
			"rows":         rows,
			"summary":      summary,
			"classes":      classes,
			"note":         note,
		})
	}))

	mux.HandleFunc("/admin/api/instrument/add", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		saveInstrument(w, r, eng, true)
	}))
	mux.HandleFunc("/admin/api/instrument/update", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		saveInstrument(w, r, eng, false)
	}))
	mux.HandleFunc("/admin/api/instrument/halt", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Symbol string
			Halted bool
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad request"})
			return
		}
		sym := strings.ToUpper(strings.TrimSpace(req.Symbol))
		if !eng.listed(sym) {
			writeJSON(w, map[string]any{"ok": false, "error": "unknown symbol " + sym})
			return
		}
		if metaFor(sym).Delisted {
			writeJSON(w, map[string]any{"ok": false, "error": sym + " is delisted — list it again to trade"})
			return
		}
		// Activating (Halted=false) also lifts a Pending listing.
		if req.Halted {
			setStatus(sym, "halted")
		} else {
			setStatus(sym, "active")
		}
		action := "instrument.resume"
		if req.Halted {
			action = "instrument.halt"
		}
		recordAudit(clientIP(r), action, sym)
		out := map[string]any{"ok": true, "symbol": sym, "halted": req.Halted}
		if err := persistUniverse(eng); err != nil {
			out["warning"] = fmt.Sprintf("applied, but could not be persisted to %s: %v", instrumentFile(), err)
		}
		writeJSON(w, out)
	}))

	mux.HandleFunc("/admin/api/instrument/delist", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct{ Symbol string }
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad request"})
			return
		}
		sym := strings.ToUpper(strings.TrimSpace(req.Symbol))
		if !eng.listed(sym) || metaFor(sym).Delisted {
			writeJSON(w, map[string]any{"ok": false, "error": "unknown symbol " + sym})
			return
		}
		// The book and ITCH id stay (the engine's maps only grow); the scrip
		// rejects orders, drops out of the directory and is not saved to the master.
		setStatus(sym, "delisted")
		recordAudit(clientIP(r), "instrument.delist", sym)
		log.Printf("[universe] admin delisted %s", sym)
		out := map[string]any{"ok": true, "symbol": sym, "count": len(instrumentRows(eng))}
		if err := persistUniverse(eng); err != nil {
			out["warning"] = fmt.Sprintf("applied, but could not be persisted to %s: %v", instrumentFile(), err)
		}
		writeJSON(w, out)
	}))

	mux.HandleFunc("/admin/api/instruments/export", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		format := strings.ToLower(strings.TrimSpace(q.Get("format")))
		if format == "" {
			format = "xml"
		}
		rows := filterInstrumentRows(instrumentRows(eng), q.Get("board"), q.Get("class"), q.Get("status"), q.Get("q"))
		stamp := time.Now().Format("20060102-150405")
		var body []byte
		var ctype string
		var err error
		switch format {
		case "xml":
			// An unfiltered export is the whole market, so it carries the index too.
			idx := 0.0
			if q.Get("board") == "" && q.Get("class") == "" && q.Get("status") == "" && q.Get("q") == "" {
				eng.mu.RLock()
				idx = eng.index
				eng.mu.RUnlock()
			}
			body, err = instrumentsXML(rows, idx)
			ctype = "application/xml; charset=utf-8"
		case "csv":
			body, err = instrumentsCSV(rows)
			ctype = "text/csv; charset=utf-8"
		case "json":
			body, err = json.MarshalIndent(map[string]any{"count": len(rows), "instruments": masterRows(rows)}, "", " ")
			ctype = "application/json; charset=utf-8"
		default:
			http.Error(w, `{"error":"format must be xml, csv or json"}`, http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"export failed"}`, http.StatusInternalServerError)
			return
		}
		recordAudit(clientIP(r), "instruments.export", fmt.Sprintf("format=%s rows=%d", format, len(rows)))
		w.Header().Set("Content-Type", ctype)
		fname := fmt.Sprintf("instruments_dse_%s.%s", stamp, format)
		if format == "xml" {
			fname = stamp + "-eodtickers-ME.xml" // same naming as the DSE end-of-day file
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
		w.Header().Set("X-Export-Count", strconv.Itoa(len(rows)))
		_, _ = w.Write(body)
	}))

	mux.HandleFunc("/admin/api/instruments/reload", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
			return
		}
		added, updated := eng.ListInstruments(universeSet{Instr: loadUniverse()})
		recordAudit("admin", "instruments.reload", fmt.Sprintf("added=%d updated=%d", added, updated))
		log.Printf("[universe] reload: +%d new, %d refreshed (source: %s)", added, updated, universeSource())
		writeJSON(w, map[string]any{"ok": true, "added": added, "updated": updated,
			"count": len(eng.instr()), "source": universeSource()})
	}))

	mux.HandleFunc("/admin/api/instruments/import", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			http.Error(w, `{"error":"read failed"}`, http.StatusBadRequest)
			return
		}
		rows, err := parseMaster(body)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		u := buildUniverse(rows)
		if len(u.Instr) == 0 {
			writeJSON(w, map[string]any{"ok": false,
				"error": fmt.Sprintf("no tradeable instruments in the %d rows submitted", len(rows))})
			return
		}
		// An import MERGES onto the running universe, but the persisted file is
		// the SOLE source at next boot. So a partial master applied now would
		// silently shrink the universe on restart — apply it, but don't persist
		// it, and say so plainly.
		before := len(eng.instr())
		partial := len(u.Instr) < before
		saved, warning := "", ""
		if partial {
			warning = fmt.Sprintf("applied to the running engine only: %d instruments is fewer than the %d currently listed, "+
				"so persisting it would shrink the universe on restart. Re-import the FULL security master to make it permanent.",
				len(u.Instr), before)
			log.Printf("[universe] import: %s", warning)
		} else if err := saveInstrumentFile(body); err != nil {
			warning = fmt.Sprintf("applied, but could not be persisted to %s: %v", instrumentFile(), err)
			log.Printf("[universe] import: could not persist to %s: %v (applied in memory only)", instrumentFile(), err)
		} else {
			saved = instrumentFile()
		}
		added, updated := eng.ListInstruments(u)
		recordAudit("admin", "instruments.import", fmt.Sprintf("rows=%d added=%d updated=%d", len(u.Instr), added, updated))
		if !partial {
			universeSrc.Store(fmt.Sprintf("admin import (%d instruments)", len(u.Instr)))
		}
		log.Printf("[universe] admin import: %d instruments (+%d new, %d refreshed), persisted=%q", len(u.Instr), added, updated, saved)
		writeJSON(w, map[string]any{"ok": true, "instruments": len(u.Instr), "added": added,
			"updated": updated, "count": len(eng.instr()), "persisted_to": saved, "warning": warning})
	}))
}

// instrumentBoards are the board labels the Instruments page offers (the DSE
// board/category names the OMS uses). MAIN/SME/ATB are the older labels already
// in existing master files.
var instrumentBoards = []string{"Main (A)", "Main (B)", "Main (N)", "Main (Z)", "Bond", "Mutual Fund", "SME / ATB", "MAIN", "SME", "ATB"}

func canonBoard(b string) (string, bool) {
	b = strings.TrimSpace(b)
	for _, c := range instrumentBoards {
		if strings.EqualFold(b, c) {
			return c, true
		}
	}
	return b, false
}

// boardClass groups a board label into an asset class.
func boardClass(board string) string {
	b := strings.ToLower(board)
	switch {
	case strings.Contains(b, "bond"):
		return "bond"
	case strings.Contains(b, "mutual"):
		return "mf"
	case strings.Contains(b, "sme"), strings.Contains(b, "atb"):
		return "sme"
	}
	return "main"
}

// instrumentRows is the per-instrument table behind the admin Instruments page:
// reference data from the master plus live price/volume from the engine. Delisted
// scrips are left out.
func instrumentRows(eng *Engine) []map[string]any {
	instr := eng.instr()
	rows := make([]map[string]any, 0, len(instr))
	for _, in := range instr {
		m := metaFor(in.Symbol)
		if m.Delisted {
			continue
		}
		board := m.Board
		if board == "" {
			board = "MAIN"
		}
		tick := m.Tick
		if tick <= 0 {
			tick = defaultTick(in.Prev) // what the pre-trade check enforces when no tick is on file
		}
		lot := m.Lot
		if lot < 1 {
			lot = 1
		}
		row := map[string]any{"symbol": in.Symbol, "name": in.Name, "board": board, "sector": m.Sector,
			"isin": m.ISIN, "tick": tick, "lot": lot, "prev": in.Prev, "status": m.statusOf(),
			"asset_class": assetClassOf(m),
			"halted":      m.Halted}
		if md := eng.mdOf(in.Symbol); md != nil {
			row["prev"], row["last"], row["volume"] = md.Prev, md.Last, md.Volume
			row["open"], row["high"], row["low"], row["trades"] = md.Open, md.High, md.Low, md.Trades
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["symbol"].(string) < rows[j]["symbol"].(string) })
	return rows
}

// instrumentSummary builds the KPI cards and the Asset Classes panel.
func instrumentSummary(rows []map[string]any) (map[string]any, []map[string]any) {
	cls := map[string]int{}
	var mainTradable, pending int
	for _, r := range rows {
		ac := r["asset_class"].(string)
		cls[ac]++
		st := r["status"].(string)
		if st == "pending" {
			pending++
		}
		if ac == "EQ" && boardClass(r["board"].(string)) == "main" && st == "active" {
			mainTradable++
		}
	}
	summary := map[string]any{"total": len(rows), "main_equity": mainTradable, "bonds": cls["CB"] + cls["DB"], "pending": pending}
	classes := make([]map[string]any, 0, len(assetClasses))
	for _, a := range assetClasses {
		classes = append(classes, map[string]any{"code": a.Code, "label": a.Label, "count": cls[a.Code], "color": a.Color})
	}
	return summary, classes
}

// saveInstrument handles list (create=true) and edit (create=false). Both go
// through Engine.ListInstruments, which creates the book/market data/ITCH id for
// a new scrip (so it shows in the Trade Terminal on the next push) or refreshes
// an existing one in place. Listing a delisted symbol again re-lists it.
func saveInstrument(w http.ResponseWriter, r *http.Request, eng *Engine, create bool) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Symbol, Name, ISIN, Board, Sector, Status, AssetClass string
		Prev, Tick                                            float64
		Lot                                                   int
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "bad request"})
		return
	}
	fail := func(msg string) { writeJSON(w, map[string]any{"ok": false, "error": msg}) }

	sym := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if !validSymbol(sym) {
		fail("invalid symbol: use up to 20 characters from A-Z, 0-9, . - _")
		return
	}
	cur := metaFor(sym)
	exists := eng.listed(sym) && !cur.Delisted
	if create && exists {
		fail(sym + " is already listed — use edit instead")
		return
	}
	if !create && !exists {
		fail("unknown symbol " + sym)
		return
	}
	var curPrev float64
	var curName string
	if md := eng.mdOf(sym); md != nil {
		curPrev = md.Prev
	}
	for _, in := range eng.instr() {
		if in.Symbol == sym {
			curName = in.Name
		}
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = curName
	}
	if name == "" {
		fail("name is required")
		return
	}
	board := strings.TrimSpace(req.Board)
	if board == "" {
		board = cur.Board
	}
	if board == "" {
		board = "Main (A)"
	}
	if c, ok := canonBoard(board); ok {
		board = c
	} else if !strings.EqualFold(board, cur.Board) {
		fail("board must be one of: " + strings.Join(instrumentBoards[:7], ", "))
		return
	}
	isin := strings.ToUpper(strings.TrimSpace(req.ISIN))
	if isin == "" {
		isin = cur.ISIN
	}
	if isin != "" && !validISIN(isin) {
		fail("ISIN must be 12 characters: 2 letters then 10 letters/digits")
		return
	}
	sector := strings.TrimSpace(req.Sector)
	if sector == "" {
		sector = cur.Sector
	}
	assetClass := strings.TrimSpace(req.AssetClass)
	if assetClass == "" {
		assetClass = cur.AssetClass
	}
	if assetClass != "" {
		if assetClass = normAssetClass(assetClass); assetClass == "" {
			fail("asset class must be EQ, MF, CB or DB")
			return
		}
	}
	prev := req.Prev
	if prev <= 0 {
		prev = curPrev
	}
	if prev <= 0 {
		fail("previous close must be greater than zero")
		return
	}
	tick := req.Tick
	if tick <= 0 {
		tick = cur.Tick
	}
	if tick <= 0 {
		tick = defaultTick(prev)
	}
	lot := req.Lot
	if lot <= 0 {
		lot = cur.Lot
	}
	if lot <= 0 {
		lot = 1
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	switch status {
	case "active", "halted", "pending":
	case "":
		status = "active"
		if !create {
			status = cur.statusOf()
		}
	default:
		fail("status must be active, halted or pending")
		return
	}

	meta := instrMetaT{Board: board, ISIN: isin, Tick: tick, Sector: sector, Lot: lot, AssetClass: assetClass,
		Halted: status == "halted", Pending: status == "pending"}
	added, updated := eng.ListInstruments(universeSet{
		Instr: []Instrument{{Symbol: sym, Name: name, Prev: round2(prev)}},
		Meta:  map[string]instrMetaT{sym: meta},
	})
	action := "instrument.update"
	if create {
		action = "instrument.add"
	}
	recordAudit(clientIP(r), action, fmt.Sprintf("%s board=%s lot=%d prev=%.2f tick=%.2f status=%s class=%s", sym, board, lot, prev, tick, status, assetClass))
	log.Printf("[universe] admin %s %s (+%d new, %d refreshed)", action, sym, added, updated)
	out := map[string]any{"ok": true, "symbol": sym, "added": added, "updated": updated, "count": len(instrumentRows(eng))}
	if err := persistUniverse(eng); err != nil {
		out["warning"] = fmt.Sprintf("applied, but could not be persisted to %s: %v", instrumentFile(), err)
	}
	writeJSON(w, out)
}

// validISIN checks the ISO 6166 shape: 2 letters, 9 alphanumerics, 1 check digit.
func validISIN(s string) bool {
	if len(s) != 12 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i >= 2:
		default:
			return false
		}
	}
	return true
}

var persistMu sync.Mutex

// filterInstrumentRows narrows the directory rows by board, asset class, status
// and a case-insensitive symbol/name search; blank filters match everything.
func filterInstrumentRows(rows []map[string]any, board, class, status, q string) []map[string]any {
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if board != "" && r["board"].(string) != board {
			continue
		}
		if class != "" && r["asset_class"].(string) != class {
			continue
		}
		if status != "" && r["status"].(string) != status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r["symbol"].(string)+" "+r["name"].(string)), q) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// masterRows converts directory rows to the master-file shape (which the import
// accepts back, so an exported JSON can be re-imported).
func masterRows(rows []map[string]any) []masterInstr {
	out := make([]masterInstr, 0, len(rows))
	for _, in := range rows {
		out = append(out, masterInstr{
			Symbol: in["symbol"].(string), Name: in["name"].(string), ISIN: in["isin"].(string),
			Board: in["board"].(string), PrevClose: in["prev"].(float64), Tick: in["tick"].(float64),
			Sector: in["sector"].(string), AssetClass: in["asset_class"].(string), Lot: in["lot"].(int),
			Status: in["status"].(string),
		})
	}
	return out
}

// eodTicker is one <Ticker .../> of the DSE end-of-day ticker file
// (YYYYMMDD-HHMMSS-eodtickers-DSE.xml). Every field is an attribute; Category,
// Sector and the trading figures are omitted when the instrument has none, exactly
// as the DSE file does for a scrip that did not trade that day.
type eodTicker struct {
	XMLName        xml.Name `xml:"Ticker"`
	SecurityCode   string   `xml:"SecurityCode,attr"`
	ISIN           string   `xml:"ISIN,attr"`
	Category       string   `xml:"Category,attr,omitempty"`
	AssetClass     string   `xml:"AssetClass,attr"`
	Sector         string   `xml:"Sector,attr,omitempty"`
	CompulsorySpot string   `xml:"CompulsorySpot,attr"`
	TradeDate      string   `xml:"TradeDate,attr"`
	Close          string   `xml:"Close,attr"`
	Open           string   `xml:"Open,attr,omitempty"`
	High           string   `xml:"High,attr,omitempty"`
	Low            string   `xml:"Low,attr,omitempty"`
	Var            string   `xml:"Var,attr,omitempty"`
	VarPercent     string   `xml:"VarPercent,attr,omitempty"`
}

type eodTickers struct {
	XMLName xml.Name    `xml:"EODTickers"`
	Tickers []eodTicker `xml:"Ticker"`
}

// price prints a price the way the DSE file does: no trailing zeros ("6.1", "77").
func price(v any) string {
	f, _ := v.(float64)
	return strconv.FormatFloat(round2(f), 'f', -1, 64)
}

func num(v any) float64 { f, _ := v.(float64); return f }

// rowToTicker maps a directory row to a DSE EOD ticker. Close is the last traded
// price; Var and VarPercent are measured against the previous close. A scrip that
// has not traded carries only its previous close, like the DSE file's untraded rows.
func rowToTicker(r map[string]any, date string) eodTicker {
	t := eodTicker{
		SecurityCode: r["symbol"].(string), ISIN: r["isin"].(string),
		Category: categoryOf(r["board"].(string)), AssetClass: r["asset_class"].(string),
		Sector: r["sector"].(string), CompulsorySpot: "N", TradeDate: date,
	}
	if t.AssetClass == "DB" {
		t.CompulsorySpot = "Y" // debentures trade on the compulsory spot market
	}
	if t.Category == "" && t.AssetClass != "EQ" {
		t.Category = "A" // funds and bonds are category A unless the master says otherwise
	}
	prev := num(r["prev"])
	trades, _ := r["trades"].(int64)
	if trades == 0 {
		t.Close = price(prev)
		return t
	}
	last := num(r["last"])
	t.Close, t.Open, t.High, t.Low = price(last), price(r["open"]), price(r["high"]), price(r["low"])
	v := last - prev
	t.Var = strconv.FormatFloat(v, 'f', 5, 64)
	pct := 0.0
	if prev != 0 {
		pct = v / prev * 100
	}
	t.VarPercent = strconv.FormatFloat(pct, 'f', 5, 64)
	return t
}

// instrumentsXML renders the directory as a DSE end-of-day ticker file. When
// index is non-zero, the engine's own DSEX index is listed first, as the DSE file
// lists its indices.
func instrumentsXML(rows []map[string]any, index float64) ([]byte, error) {
	date := time.Now().Format("20060102")
	doc := eodTickers{Tickers: make([]eodTicker, 0, len(rows)+1)}
	if index > 0 {
		doc.Tickers = append(doc.Tickers, eodTicker{SecurityCode: "DSEX", ISIN: "DSEX", AssetClass: "IN",
			CompulsorySpot: "N", TradeDate: date, Close: strconv.FormatFloat(index, 'f', 5, 64)})
	}
	for _, r := range rows {
		doc.Tickers = append(doc.Tickers, rowToTicker(r, date))
	}
	b, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	s := strings.ReplaceAll(string(b), "></Ticker>", " />")
	return []byte(`<?xml version="1.0" encoding="utf-8"?>` + "\n" + s), nil
}

func instrumentsCSV(rows []map[string]any) ([]byte, error) {
	var sb strings.Builder
	cw := csv.NewWriter(&sb)
	_ = cw.Write([]string{"symbol", "isin", "name", "asset_class", "board", "sector", "lot", "tick", "prev_close", "last", "volume", "status"})
	for _, r := range rows {
		vol, _ := r["volume"].(int64)
		_ = cw.Write([]string{r["symbol"].(string), r["isin"].(string), r["name"].(string), r["asset_class"].(string),
			r["board"].(string), r["sector"].(string), strconv.Itoa(r["lot"].(int)),
			strconv.FormatFloat(num(r["tick"]), 'f', 2, 64), strconv.FormatFloat(num(r["prev"]), 'f', 2, 64),
			strconv.FormatFloat(num(r["last"]), 'f', 2, 64), strconv.FormatInt(vol, 10), r["status"].(string)})
	}
	cw.Flush()
	return []byte(sb.String()), cw.Error()
}

// persistUniverse writes the engine's whole current universe to the master file
// (normalised JSON). It dumps everything rather than upserting one row because
// the file is the sole source at next boot: a one-row file would shrink the
// universe to a single instrument on restart when none existed before. Delisted
// scrips are omitted, so they are gone after a restart.
func persistUniverse(eng *Engine) error {
	rows := instrumentRows(eng)
	out := make([]masterInstr, 0, len(rows))
	for _, in := range rows {
		out = append(out, masterInstr{
			Symbol: in["symbol"].(string), Name: in["name"].(string), ISIN: in["isin"].(string),
			Board: in["board"].(string), PrevClose: in["prev"].(float64), Tick: in["tick"].(float64),
			Sector: in["sector"].(string), AssetClass: in["asset_class"].(string), Lot: in["lot"].(int), Status: in["status"].(string),
		})
	}
	body, err := json.MarshalIndent(map[string]any{"count": len(out), "instruments": out}, "", " ")
	if err != nil {
		return err
	}
	persistMu.Lock()
	defer persistMu.Unlock()
	return saveInstrumentFile(body)
}

// saveInstrumentFile persists an imported master so it survives a restart. The
// write is atomic (temp file + rename) so a crash mid-write can never leave the
// ME booting from a truncated master.
func saveInstrumentFile(body []byte) error {
	path := strings.TrimSpace(instrumentFile())
	if path == "" {
		return fmt.Errorf("ME_INSTRUMENT_FILE is empty — nowhere to persist")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// instrumentStatus is the universe summary carried on /api/status and the admin
// dashboard, so a stale master is visible without reading the log.
func instrumentStatus(eng *Engine) map[string]any {
	return map[string]any{
		"count":       len(eng.instr()),
		"source":      universeSource(),
		"auto_list":   autoListEnabled(),
		"auto_listed": len(autoListedSymbols()),
	}
}
