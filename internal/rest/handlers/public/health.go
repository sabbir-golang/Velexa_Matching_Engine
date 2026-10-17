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

// favicon is the console's "M" mark, served for the browser tab.
const favicon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="7" fill="#0fa585"/>` +
	`<text x="16" y="23" font-family="Arial,sans-serif" font-size="19" font-weight="700" text-anchor="middle" fill="#fff">M</text></svg>`

// Favicon serves /favicon.ico.
func (h *Handler) Favicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprint(w, favicon)
}
