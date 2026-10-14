package control

import (
	"net/http"

	"velexa-sim-me/internal/rest/middleware"
)

// RegRoutes maps this package's URLs to its handlers.
func (h *Handler) RegRoutes(mux *http.ServeMux, manager *middleware.Manager) {
	mux.Handle("/api/session", manager.With(http.HandlerFunc(h.Session)))
	mux.Handle("/api/session/preopen", manager.With(http.HandlerFunc(h.SessionPreOpen), middleware.PostOnly, h.middlewares.Admin))
	mux.Handle("/api/session/open", manager.With(http.HandlerFunc(h.SessionOpen), middleware.PostOnly, h.middlewares.Admin))
}
