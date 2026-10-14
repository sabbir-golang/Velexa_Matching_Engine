package admin

import (
	"net/http"
	"net/url"
	"strconv"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/rest/middleware"
)

// Login takes the sign-in form posted from the welcome page; any GET goes back to "/".
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// There is no separate login page: sign-in lives on the welcome page at "/".
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	ip := middleware.ClientIP(r)
	name := r.FormValue("user")
	if len(name) > 64 { // no account name is longer; keeps junk out of the audit trail
		name = name[:64]
	}
	as := r.FormValue("as") // set by the welcome page: "user" or "admin"
	back := func(code string) {
		q := "/?e=" + code
		if as != "" {
			q += "&as=" + url.QueryEscape(as)
		}
		http.Redirect(w, r, q, http.StatusSeeOther)
	}
	if middleware.LoginBlocked(ip, name) {
		audit.Record(strconv.Quote(name)+" ("+ip+")", "login.locked", "")
		back("lock")
		return
	}
	u, ok := middleware.Authenticate(h.Users, name, r.FormValue("pass"))
	if !ok {
		middleware.NoteLoginFail(ip, name)
		audit.Record(strconv.Quote(name)+" ("+ip+")", "login.fail", "")
		back("1")
		return
	}
	// The password was right, so naming the account's real role leaks nothing.
	if as == "admin" && u.Role != users.RoleAdmin {
		http.Redirect(w, r, "/?e=role&as=admin", http.StatusSeeOther)
		return
	}
	if as == "user" && u.Role == users.RoleAdmin {
		http.Redirect(w, r, "/?e=admin&as=user", http.StatusSeeOther)
		return
	}
	middleware.ClearLoginFail(ip, name)
	if !middleware.StartSession(w, r, h.Users, u.Username) {
		back("1")
		return
	}
	audit.Record(u.Username+" ("+ip+")", "login.ok", string(u.Role))
	http.Redirect(w, r, middleware.HomeFor(u), http.StatusSeeOther)
}

// Logout serves /admin/logout. A POST (with the CSRF token) ends every session
// of the signed-in account and clears the cookie; a GET only goes home, so a
// link on another site cannot sign anyone out.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if u := middleware.SessionUser(h.Users, r); u != nil {
			_ = h.Users.RevokeSessions(u.Username)
			audit.Record(u.Username+" ("+middleware.ClientIP(r)+")", "logout", "")
		}
		middleware.ClearSession(w)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
