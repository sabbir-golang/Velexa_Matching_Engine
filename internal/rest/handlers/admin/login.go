package admin

import (
	"net/http"
	"net/url"

	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/rest/middleware"
)

// Login takes the sign-in form posted from the welcome page; any GET goes back to "/".
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	ip := middleware.ClientIP(r)
	if r.Method == http.MethodPost {
		r.ParseForm()
		if loginBlocked(ip) {
			http.Redirect(w, r, "/?e=lock", http.StatusSeeOther)
			return
		}
		as := r.FormValue("as") // set by the welcome page: "user" or "admin"
		if u, ok := middleware.Authenticate(h.Users, r.FormValue("user"), r.FormValue("pass")); ok {
			if as == "admin" && u.Role != users.RoleAdmin {
				http.Redirect(w, r, "/?e=role&as=admin", http.StatusSeeOther)
				return
			}
			if as == "user" && u.Role == users.RoleAdmin {
				http.Redirect(w, r, "/?e=admin&as=user", http.StatusSeeOther)
				return
			}
			ClearLoginFail(ip)
			middleware.SetSessionCookie(w, r, u.Username)
			http.Redirect(w, r, middleware.HomeFor(u), http.StatusSeeOther)
			return
		}
		noteLoginFail(ip)
		if as != "" {
			http.Redirect(w, r, "/?e=1&as="+url.QueryEscape(as), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/?e=1", http.StatusSeeOther)
		return
	}
	// There is no separate login page: sign-in lives on the welcome page at "/".
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout serves /admin/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	for _, p := range []string{"/", "/admin"} { // "/admin" clears cookies set before the console moved to "/"
		http.SetCookie(w, &http.Cookie{Name: middleware.SessionCookie, Value: "", Path: p, MaxAge: -1})
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
