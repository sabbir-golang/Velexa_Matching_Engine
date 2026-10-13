package account

import (
	"net/http"

	"velexa-sim-me/internal/rest/middleware"
)

// RegRoutes maps this package's URLs to its handlers.
func (h *Handler) RegRoutes(mux *http.ServeMux, manager *middleware.Manager) {
	mux.Handle("/admin/api/me", manager.With(http.HandlerFunc(h.Me), h.middlewares.Viewer))
	mux.Handle("/admin/api/me/password", manager.With(http.HandlerFunc(h.MePassword), h.middlewares.Viewer))
	mux.Handle("/admin/api/users", manager.With(http.HandlerFunc(h.List), h.middlewares.Admin))
	mux.Handle("/admin/api/user/add", manager.With(http.HandlerFunc(h.Add), h.middlewares.Admin))
	mux.Handle("/admin/api/user/update", manager.With(http.HandlerFunc(h.Update), h.middlewares.Admin))
	mux.Handle("/admin/api/user/password", manager.With(http.HandlerFunc(h.ResetPassword), h.middlewares.Admin))
	mux.Handle("/admin/api/user/delete", manager.With(http.HandlerFunc(h.Delete), h.middlewares.Admin))
}
