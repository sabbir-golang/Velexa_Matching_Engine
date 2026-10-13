package control

import (
	"net/http"
	"sync/atomic"

	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/utils"
)

// BotsStatus serves /api/bots.
func (h *Handler) BotsStatus(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, map[string]any{"paused": atomic.LoadInt32(&engine.BotsPaused) == 1})
}

// BotsPause serves /api/bots/pause.
func (h *Handler) BotsPause(w http.ResponseWriter, r *http.Request) {
	atomic.StoreInt32(&engine.BotsPaused, 1)
	utils.WriteJSON(w, map[string]any{"paused": true})
}

// BotsResume serves /api/bots/resume.
func (h *Handler) BotsResume(w http.ResponseWriter, r *http.Request) {
	atomic.StoreInt32(&engine.BotsPaused, 0)
	utils.WriteJSON(w, map[string]any{"paused": false})
}
