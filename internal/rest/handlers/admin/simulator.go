package admin

import (
	"net/http"
	"sync/atomic"

	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Simulator serves /admin/api/sim.
func (h *Handler) Simulator(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	// Reading the status is open to every signed-in user; any action (pause or
	// resume the bots, pre-open, open) changes the market and needs an admin.
	if r.URL.Query().Get("action") != "" && !middleware.RequireAdminRole(w, r) {
		return
	}
	switch r.URL.Query().Get("action") {
	case "bots_pause":
		atomic.StoreInt32(&engine.BotsPaused, 1)
	case "bots_resume":
		atomic.StoreInt32(&engine.BotsPaused, 0)
	case "preopen":
		eng.SetPhase("Pre-Open")
	case "open":
		eng.SetPhase("Continuous Trading")
	}
	utils.WriteJSON(w, map[string]any{"ok": true, "phase": eng.Phase,
		"bots_paused": atomic.LoadInt32(&engine.BotsPaused) == 1})
}
