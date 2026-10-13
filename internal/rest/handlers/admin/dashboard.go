package admin

import (
	"net/http"

	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/web"
)

// Dashboard serves /admin.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" {
		http.NotFound(w, r)
		return
	}
	if u := middleware.CurrentUser(r); u != nil && u.Role != users.RoleAdmin {
		http.Redirect(w, r, middleware.HomeFor(u), http.StatusSeeOther) // /admin is for admins
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(web.AdminDashboardHTML))
}

// UserDashboard serves /user: the same console, opened by non-admin accounts.
func (h *Handler) UserDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/user" {
		http.NotFound(w, r)
		return
	}
	if u := middleware.CurrentUser(r); u != nil && u.Role == users.RoleAdmin {
		http.Redirect(w, r, middleware.HomeFor(u), http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(web.AdminDashboardHTML))
}
