package admin

import (
	"net/http"

	"velexa-sim-me/internal/rest/middleware"
)

// RegRoutes maps this package's URLs to its handlers.
func (h *Handler) RegRoutes(mux *http.ServeMux, manager *middleware.Manager) {
	mux.Handle("/admin/login", manager.With(http.HandlerFunc(h.Login)))
	mux.Handle("/admin/logout", manager.With(http.HandlerFunc(h.Logout)))                      // POST signs out; GET just goes home
	mux.Handle("/admin", manager.With(http.HandlerFunc(h.Dashboard), h.middlewares.Viewer))    // admins; other roles are sent to /user
	mux.Handle("/user", manager.With(http.HandlerFunc(h.UserDashboard), h.middlewares.Viewer)) // the console for the user role; admins are sent to /admin
	mux.Handle("/admin/api/status", manager.With(http.HandlerFunc(h.Status), h.middlewares.Viewer))
	mux.Handle("/admin/api/trading-session", manager.With(http.HandlerFunc(h.TradingSession), h.middlewares.Admin))        // actions need POST
	mux.Handle("/admin/api/session", manager.With(http.HandlerFunc(h.Session), middleware.PostOnly, h.middlewares.Viewer)) // own brokers only, for a user
	mux.Handle("/admin/api/overview", manager.With(http.HandlerFunc(h.Overview), h.middlewares.Viewer))                    // the dashboard; a user: own brokers only
	mux.Handle("/admin/api/report/daily", manager.With(http.HandlerFunc(h.DailyReport), h.middlewares.Viewer))             // a user: own brokers only
	mux.Handle("/admin/api/report/orders", manager.With(http.HandlerFunc(h.OrderReport), h.middlewares.Viewer))            // a user: own brokers only
	mux.Handle("/admin/api/report/order-history", manager.With(http.HandlerFunc(h.OrderHistory), h.middlewares.Viewer))    // a user: own brokers only
	mux.Handle("/admin/api/diagnostics", manager.With(http.HandlerFunc(h.Diagnostics), h.middlewares.Admin))
	mux.Handle("/admin/report/daily", manager.With(http.HandlerFunc(h.DailyReportPage), h.middlewares.Viewer))
	mux.Handle("/admin/api/fixlog", manager.With(http.HandlerFunc(h.FixLog), h.middlewares.Viewer)) // logon passwords masked for the user role
}
