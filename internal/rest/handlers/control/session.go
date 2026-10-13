package control

import (
	"net/http"

	"velexa-sim-me/internal/utils"
)

// Session serves /api/session.
// Session control: pre-open call auction → continuous trading.
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	utils.WriteJSON(w, map[string]any{"phase": eng.Phase})
}

// SessionPreOpen serves /api/session/preopen.
func (h *Handler) SessionPreOpen(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	utils.WriteJSON(w, map[string]any{"phase": eng.SetPhase("Pre-Open")})
}

// SessionOpen serves /api/session/open.
func (h *Handler) SessionOpen(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	utils.WriteJSON(w, map[string]any{"phase": eng.SetPhase("Continuous Trading")})
}
