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

// Halt serves /admin/api/instrument/halt.
func (h *Handler) Halt(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Symbol string
		Halted bool
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "bad request"})
		return
	}
	sym := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if !eng.Listed(sym) {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "unknown symbol " + sym})
		return
	}
	if engine.MetaFor(sym).Delisted {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": sym + " is delisted — list it again to trade"})
		return
	}
	// Activating (Halted=false) also lifts a Pending listing.
	if req.Halted {
		engine.SetStatus(sym, "halted")
	} else {
		engine.SetStatus(sym, "active")
	}
	eng.PublishTradingState(sym)
	action := "instrument.resume"
	if req.Halted {
		action = "instrument.halt"
	}
	audit.Record(middleware.ActorOf(r), action, sym)
	out := map[string]any{"ok": true, "symbol": sym, "halted": req.Halted}
	if err := persistUniverse(eng); err != nil {
		out["warning"] = fmt.Sprintf("applied, but could not be persisted to %s: %v", engine.InstrumentFile(), err)
	}
	utils.WriteJSON(w, out)
}

// Delist serves /admin/api/instrument/delist.
func (h *Handler) Delist(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct{ Symbol string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "bad request"})
		return
	}
	sym := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if !eng.Listed(sym) || engine.MetaFor(sym).Delisted {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "unknown symbol " + sym})
		return
	}
	// The book and ITCH id stay (the engine's maps only grow); the scrip
	// rejects orders, drops out of the directory and is not saved to the master.
	engine.SetStatus(sym, "delisted")
	eng.PublishTradingState(sym)
	audit.Record(middleware.ActorOf(r), "instrument.delist", sym)
	log.Printf("[universe] admin delisted %s", sym)
	out := map[string]any{"ok": true, "symbol": sym, "count": len(instrumentRows(eng))}
	if err := persistUniverse(eng); err != nil {
		out["warning"] = fmt.Sprintf("applied, but could not be persisted to %s: %v", engine.InstrumentFile(), err)
	}
	utils.WriteJSON(w, out)
}
