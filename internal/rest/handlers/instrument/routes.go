package instrument

import (
	"net/http"

	"velexa-sim-me/internal/rest/middleware"
)

// RegRoutes maps this package's URLs to its handlers.
func (h *Handler) RegRoutes(mux *http.ServeMux, manager *middleware.Manager) {
	mux.Handle("/admin/api/instruments", manager.With(http.HandlerFunc(h.List), h.middlewares.Admin))
	mux.Handle("/admin/api/instrument/add", manager.With(http.HandlerFunc(h.Add), middleware.PostOnly, h.middlewares.Admin))
	mux.Handle("/admin/api/instrument/update", manager.With(http.HandlerFunc(h.Update), middleware.PostOnly, h.middlewares.Admin))
	mux.Handle("/admin/api/instrument/halt", manager.With(http.HandlerFunc(h.Halt), middleware.PostOnly, h.middlewares.Admin))
	mux.Handle("/admin/api/instrument/delist", manager.With(http.HandlerFunc(h.Delist), middleware.PostOnly, h.middlewares.Admin))
	mux.Handle("/admin/api/instruments/export", manager.With(http.HandlerFunc(h.Export), h.middlewares.Admin))
	mux.Handle("/admin/api/instruments/reload", manager.With(http.HandlerFunc(h.Reload), middleware.PostOnly, h.middlewares.Admin))
	mux.Handle("/admin/api/instruments/import", manager.With(http.HandlerFunc(h.Import), middleware.PostOnly, h.middlewares.Admin))
}
