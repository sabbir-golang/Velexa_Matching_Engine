package middleware

import (
	"context"
	"net/http"
	"strings"

	"velexa-sim-me/internal/database/users"
)

type ctxKey struct{}

// RequireRole wraps a handler so it runs only for a signed-in user whose role
// allows min. Not signed in: 401 for API calls, a redirect to the welcome page
// otherwise. Signed in with too low a role: 403.
func (m *Middlewares) RequireRole(min users.Role) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := SessionUser(m.users, r)
			if u == nil {
				if strings.HasPrefix(r.URL.Path, "/admin/api/") {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				} else {
					http.Redirect(w, r, "/", http.StatusSeeOther)
				}
				return
			}
			if !u.Role.Allows(min) {
				forbidden(w, min)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
		})
	}
}

// Viewer lets any signed-in user through (read-only pages).
func (m *Middlewares) Viewer(next http.Handler) http.Handler {
	return m.RequireRole(users.RoleUser)(next)
}

// Admin lets only admins through (anything that changes state or shows secrets).
func (m *Middlewares) Admin(next http.Handler) http.Handler {
	return m.RequireRole(users.RoleAdmin)(next)
}

func forbidden(w http.ResponseWriter, min users.Role) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{"ok":false,"error":"forbidden: ` + string(min) + ` role required"}` + "\n"))
}

// CurrentUser is the user RequireRole put on the request, or nil outside it.
func CurrentUser(r *http.Request) *users.User {
	u, _ := r.Context().Value(ctxKey{}).(*users.User)
	return u
}

// RequireAdminRole is for handlers that are open to every signed-in user for some
// requests (reading) but need an admin for others (changing). It writes the 403
// itself and reports whether the request may continue.
func RequireAdminRole(w http.ResponseWriter, r *http.Request) bool {
	if u := CurrentUser(r); u != nil && u.Role.Allows(users.RoleAdmin) {
		return true
	}
	forbidden(w, users.RoleAdmin)
	return false
}

// ActorOf names who made a request in the audit log: "alice (10.0.0.5)".
func ActorOf(r *http.Request) string {
	if u := CurrentUser(r); u != nil {
		return u.Username + " (" + ClientIP(r) + ")"
	}
	return ClientIP(r)
}

// Authenticate checks a username and password against the user store.
func Authenticate(store *users.Store, username, password string) (*users.User, bool) {
	if store == nil {
		return nil, false
	}
	return store.Authenticate(username, password)
}
