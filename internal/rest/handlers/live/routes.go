package live

import (
	"net/http"

	"velexa-sim-me/internal/rest/middleware"
)

// RegRoutes maps this package's URLs to its handlers.
func (h *Handler) RegRoutes(mux *http.ServeMux, manager *middleware.Manager) {
	mux.Handle("/api/live/start", manager.With(http.HandlerFunc(h.Start)))
	mux.Handle("/api/live/stop", manager.With(http.HandlerFunc(h.Stop)))
	mux.Handle("/api/live/stats", manager.With(http.HandlerFunc(h.Stats)))
	mux.Handle("/api/live/latency", manager.With(http.HandlerFunc(h.Latency)))
	mux.Handle("/api/live/cleanup", manager.With(http.HandlerFunc(h.Cleanup)))
}
