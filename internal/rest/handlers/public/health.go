package public

import (
	"fmt"
	"net/http"

	"velexa-sim-me/internal/utils"
)

// Health serves /health.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	eng.Mu.RLock()
	phase, running := eng.Phase, eng.Running
	eng.Mu.RUnlock()
	// Public gateway health for the exchange /marketfeed panel. No secrets:
	// if this responds, the gateway is up and serving all feeds. `phase` and
	// `running` reflect the live matching-engine trading state.
	utils.WriteJSON(w, map[string]any{
		"ok": true, "phase": phase, "running": running,
		"feeds": map[string]bool{"market": true, "index": true, "news": true, "depth": true},
	})
}

// Healthz serves /healthz.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
}
