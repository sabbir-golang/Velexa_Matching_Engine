package control

import (
	"net/http"

	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/utils"
)

// Session serves /api/session.
// Session control: pre-open call auction → continuous trading. Forcing a phase
// switches the session clock off (the admin console switches it back on).
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	utils.WriteJSON(w, map[string]any{"phase": eng.CurrentPhase(), "auto_clock": eng.AutoClock()})
}

// SessionPreOpen serves /api/session/preopen.
func (h *Handler) SessionPreOpen(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	utils.WriteJSON(w, map[string]any{"phase": eng.ForcePhase(engine.PhasePreOpen)})
}

// SessionOpen serves /api/session/open.
func (h *Handler) SessionOpen(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	utils.WriteJSON(w, map[string]any{"phase": eng.ForcePhase(engine.PhaseOpen)})
}
