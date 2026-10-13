package instrument

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Reload serves /admin/api/instruments/reload.
func (h *Handler) Reload(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	added, updated := eng.ListInstruments(engine.UniverseSet{Instr: engine.LoadUniverse()})
	audit.Record(middleware.ActorOf(r), "instruments.reload", fmt.Sprintf("added=%d updated=%d", added, updated))
	log.Printf("[universe] reload: +%d new, %d refreshed (source: %s)", added, updated, engine.UniverseSource())
	utils.WriteJSON(w, map[string]any{"ok": true, "added": added, "updated": updated,
		"count": len(eng.Instr()), "source": engine.UniverseSource()})
}

// Import serves /admin/api/instruments/import.
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		http.Error(w, `{"error":"read failed"}`, http.StatusBadRequest)
		return
	}
	rows, err := engine.ParseMaster(body)
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	u := engine.BuildUniverse(rows)
	if len(u.Instr) == 0 {
		utils.WriteJSON(w, map[string]any{"ok": false,
			"error": fmt.Sprintf("no tradeable instruments in the %d rows submitted", len(rows))})
		return
	}
	// An import MERGES onto the running universe, but the persisted file is
	// the SOLE source at next boot. So a partial master applied now would
	// silently shrink the universe on restart — apply it, but don't persist
	// it, and say so plainly.
	before := len(eng.Instr())
	partial := len(u.Instr) < before
	saved, warning := "", ""
	if partial {
		warning = fmt.Sprintf("applied to the running engine only: %d instruments is fewer than the %d currently listed, "+
			"so persisting it would shrink the universe on restart. Re-import the FULL security master to make it permanent.",
			len(u.Instr), before)
		log.Printf("[universe] import: %s", warning)
	} else if err := engine.SaveInstrumentFile(body); err != nil {
		warning = fmt.Sprintf("applied, but could not be persisted to %s: %v", engine.InstrumentFile(), err)
		log.Printf("[universe] import: could not persist to %s: %v (applied in memory only)", engine.InstrumentFile(), err)
	} else {
		saved = engine.InstrumentFile()
	}
	added, updated := eng.ListInstruments(u)
	audit.Record(middleware.ActorOf(r), "instruments.import", fmt.Sprintf("rows=%d added=%d updated=%d", len(u.Instr), added, updated))
	if !partial {
		engine.UniverseSrc.Store(fmt.Sprintf("admin import (%d instruments)", len(u.Instr)))
	}
	log.Printf("[universe] admin import: %d instruments (+%d new, %d refreshed), persisted=%q", len(u.Instr), added, updated, saved)
	utils.WriteJSON(w, map[string]any{"ok": true, "instruments": len(u.Instr), "added": added,
		"updated": updated, "count": len(eng.Instr()), "persisted_to": saved, "warning": warning})
}
