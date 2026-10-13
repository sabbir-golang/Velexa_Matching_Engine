package control

import (
	"net/http"

	"velexa-sim-me/internal/rest/middleware"
)

// RegRoutes maps this package's URLs to its handlers.
func (h *Handler) RegRoutes(mux *http.ServeMux, manager *middleware.Manager) {
	mux.Handle("/api/bots", manager.With(http.HandlerFunc(h.BotsStatus)))
	mux.Handle("/api/bots/pause", manager.With(http.HandlerFunc(h.BotsPause)))
	mux.Handle("/api/bots/resume", manager.With(http.HandlerFunc(h.BotsResume)))
	mux.Handle("/api/session", manager.With(http.HandlerFunc(h.Session)))
	mux.Handle("/api/session/preopen", manager.With(http.HandlerFunc(h.SessionPreOpen)))
	mux.Handle("/api/session/open", manager.With(http.HandlerFunc(h.SessionOpen)))
}
