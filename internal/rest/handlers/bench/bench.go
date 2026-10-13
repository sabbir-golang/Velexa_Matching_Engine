package bench

import (
	"encoding/json"
	"net/http"

	"velexa-sim-me/internal/loadtest"
	"velexa-sim-me/internal/utils"
)

// Start serves /api/bench/start.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	bench := h.Bench
	cfg := loadtest.BenchConfig{RMS: true, Persist: true}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&cfg)
	}
	if err := bench.Start(cfg); err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	utils.WriteJSON(w, map[string]any{"ok": true, "cfg": cfg})
}

// Stop serves /api/bench/stop.
func (h *Handler) Stop(w http.ResponseWriter, r *http.Request) {
	bench := h.Bench
	bench.Stop()
	utils.WriteJSON(w, map[string]any{"ok": true})
}

// Stats serves /api/bench/stats.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	bench := h.Bench
	utils.WriteJSON(w, bench.Stats())
}

// SummaryCSV serves /api/bench/summary.csv.
func (h *Handler) SummaryCSV(w http.ResponseWriter, r *http.Request) {
	bench := h.Bench
	bench.WriteSummaryCSV(w)
}
