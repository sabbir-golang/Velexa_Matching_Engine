package admin

import (
	"net/http"
	"strings"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// TradingSession serves /admin/api/trading-session: the market's session phase
// and the DSE clock. Reading it is open to every signed-in user; changing it
// needs an admin and a POST.
func (h *Handler) TradingSession(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	action := r.URL.Query().Get("action")
	if action != "" && r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, `{"ok":false,"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	if action != "" && !middleware.RequireAdminRole(w, r) {
		return
	}
	switch action {
	// Forcing a phase switches the clock off; auto_on hands the phases back to it.
	case "preopen":
		eng.ForcePhase(engine.PhasePreOpen)
	case "open":
		eng.ForcePhase(engine.PhaseOpen)
	case "close":
		eng.ForcePhase(engine.PhaseClose)
	case "postclose":
		eng.ForcePhase(engine.PhasePostClose)
	case "endofday":
		eng.ForcePhase(engine.PhaseClosed)
	case "auto_on":
		eng.SetAutoClock(true)
	case "auto_off":
		eng.SetAutoClock(false)
	default:
		action = "" // nothing changed: nothing to record
	}
	if action != "" { // the console's reason, if any, goes into the audit trail with the new phase
		target := eng.CurrentPhase()
		if note := strings.TrimSpace(r.URL.Query().Get("note")); note != "" {
			if len(note) > 200 {
				note = note[:200]
			}
			target += " · " + note
		}
		audit.Record(middleware.ActorOf(r), "SESSION "+strings.ToUpper(action), target)
	}
	utils.WriteJSON(w, map[string]any{"ok": true, "phase": eng.CurrentPhase(), "auto_clock": eng.AutoClock(), "schedule": eng.ScheduleText()})
}
