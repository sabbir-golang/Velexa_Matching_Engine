package broker

import (
	"net/http"

	"velexa-sim-me/internal/rest/middleware"
)

// RegRoutes maps this package's URLs to its handlers.
func (h *Handler) RegRoutes(mux *http.ServeMux, manager *middleware.Manager) {
	mux.Handle("/admin/api/brokers", manager.With(http.HandlerFunc(h.List), h.middlewares.Viewer))
	mux.Handle("/admin/api/broker/detail", manager.With(http.HandlerFunc(h.Detail), h.middlewares.Viewer)) // broker credentials: same access for every role
	mux.Handle("/admin/api/broker/add", manager.With(http.HandlerFunc(h.Add), middleware.PostOnly, h.middlewares.Viewer))
	mux.Handle("/admin/api/broker/del", manager.With(http.HandlerFunc(h.Revoke), middleware.PostOnly, h.middlewares.Viewer))
	mux.Handle("/admin/api/broker/sheet", manager.With(http.HandlerFunc(h.Sheet), middleware.PostOnly, h.middlewares.Viewer))
	mux.Handle("/admin/api/allowlist", manager.With(http.HandlerFunc(h.AllowList), h.middlewares.Admin))
	mux.Handle("/admin/api/allowlist/add", manager.With(http.HandlerFunc(h.AllowAdd), middleware.PostOnly, h.middlewares.Admin))
	mux.Handle("/admin/api/allowlist/del", manager.With(http.HandlerFunc(h.AllowRemove), middleware.PostOnly, h.middlewares.Admin))
}
