package admin

import (
	"net/http"

	"velexa-sim-me/internal/rest/middleware"
)

// RegRoutes maps this package's URLs to its handlers.
func (h *Handler) RegRoutes(mux *http.ServeMux, manager *middleware.Manager) {
	mux.Handle("/admin/login", manager.With(http.HandlerFunc(h.Login)))
	mux.Handle("/admin/logout", manager.With(http.HandlerFunc(h.Logout)))
	mux.Handle("/admin", manager.With(http.HandlerFunc(h.Dashboard), h.middlewares.Viewer))    // admins; other roles are sent to /user
	mux.Handle("/user", manager.With(http.HandlerFunc(h.UserDashboard), h.middlewares.Viewer)) // the console for the user role; admins are sent to /admin
	mux.Handle("/admin/api/status", manager.With(http.HandlerFunc(h.Status), h.middlewares.Viewer))
	mux.Handle("/admin/api/sim", manager.With(http.HandlerFunc(h.Simulator), h.middlewares.Admin))
	mux.Handle("/admin/api/session", manager.With(http.HandlerFunc(h.Session), h.middlewares.Admin))
	mux.Handle("/admin/api/fixlog", manager.With(http.HandlerFunc(h.FixLog), h.middlewares.Viewer)) // logon passwords masked for the user role
}
