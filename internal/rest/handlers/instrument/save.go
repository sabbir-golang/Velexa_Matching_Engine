package instrument

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Add serves /admin/api/instrument/add.
func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	saveInstrument(w, r, eng, true)
}

// Update serves /admin/api/instrument/update.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	saveInstrument(w, r, eng, false)
}

// saveInstrument handles list (create=true) and edit (create=false). Both go
// through Engine.ListInstruments, which creates the book/market data/ITCH id for
// a new scrip (so it shows in the Trade Terminal on the next push) or refreshes
// an existing one in place. Listing a delisted symbol again re-lists it.
func saveInstrument(w http.ResponseWriter, r *http.Request, eng *engine.Engine, create bool) {
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
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "bad request"})
		return
	}
	fail := func(msg string) { utils.WriteJSON(w, map[string]any{"ok": false, "error": msg}) }

	sym := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if !engine.ValidSymbol(sym) {
		fail("invalid symbol: use up to 20 characters from A-Z, 0-9, . - _")
		return
	}
	cur := engine.MetaFor(sym)
	exists := eng.Listed(sym) && !cur.Delisted
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
	if md := eng.MdOf(sym); md != nil {
		curPrev = md.Prev
	}
	for _, in := range eng.Instr() {
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
		if assetClass = engine.NormAssetClass(assetClass); assetClass == "" {
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
		tick = engine.DefaultTick(prev)
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
			status = cur.StatusOf()
		}
	default:
		fail("status must be active, halted or pending")
		return
	}

	meta := engine.InstrMeta{Board: board, ISIN: isin, Tick: tick, Sector: sector, Lot: lot, AssetClass: assetClass,
		Halted: status == "halted", Pending: status == "pending"}
	added, updated := eng.ListInstruments(engine.UniverseSet{
		Instr: []engine.Instrument{{Symbol: sym, Name: name, Prev: engine.Round2(prev)}},
		Meta:  map[string]engine.InstrMeta{sym: meta},
	})
	action := "instrument.update"
	if create {
		action = "instrument.add"
	}
	audit.Record(middleware.ActorOf(r), action, fmt.Sprintf("%s board=%s lot=%d prev=%.2f tick=%.2f status=%s class=%s", sym, board, lot, prev, tick, status, assetClass))
	log.Printf("[universe] admin %s %s (+%d new, %d refreshed)", action, sym, added, updated)
	out := map[string]any{"ok": true, "symbol": sym, "added": added, "updated": updated, "count": len(instrumentRows(eng))}
	if err := persistUniverse(eng); err != nil {
		out["warning"] = fmt.Sprintf("applied, but could not be persisted to %s: %v", engine.InstrumentFile(), err)
	}
	utils.WriteJSON(w, out)
}
