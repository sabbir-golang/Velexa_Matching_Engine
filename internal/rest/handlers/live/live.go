package live

import (
	"encoding/json"
	"net/http"

	"velexa-sim-me/internal/loadtest"
	"velexa-sim-me/internal/utils"
)

// Start serves /api/live/start.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	live := h.Live
	var cfg loadtest.LiveConfig
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&cfg)
	}
	if err := live.Start(cfg); err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	utils.WriteJSON(w, map[string]any{"ok": true, "cfg": cfg})
}

// Stop serves /api/live/stop.
func (h *Handler) Stop(w http.ResponseWriter, r *http.Request) {
	live := h.Live
	live.Stop()
	utils.WriteJSON(w, map[string]any{"ok": true})
}

// Stats serves /api/live/stats.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	live := h.Live
	utils.WriteJSON(w, live.Stats())
}

// Latency serves /api/live/latency.
func (h *Handler) Latency(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, loadtest.PipelineLatency())
}

// Cleanup serves /api/live/cleanup.
func (h *Handler) Cleanup(w http.ResponseWriter, r *http.Request) {
	live := h.Live
	utils.WriteJSON(w, live.Cleanup())
}
