package middleware

// Data scoping for console users. Admins see and manage everything; a normal
// user sees only the brokers they registered, and everything tied to them:
// credentials, FIX sessions, the FIX message log, the dashboard figures.

import (
	"net/http"
	"strings"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/database/users"
)

// IsAdmin reports whether the signed-in user is an admin.
func IsAdmin(r *http.Request) bool {
	u := CurrentUser(r)
	return u != nil && u.Role.Allows(users.RoleAdmin)
}

// Username is the signed-in user's name ("" outside a signed-in request).
func Username(r *http.Request) string {
	if u := CurrentUser(r); u != nil {
		return u.Username
	}
	return ""
}

// CanAccessBroker reports whether the signed-in user may see and manage the
// broker with this SenderCompID.
func CanAccessBroker(r *http.Request, sender string) bool {
	if IsAdmin(r) {
		return true
	}
	return brokers.Meta.Get(sender).OwnedBy(Username(r))
}

// BrokerScope returns the SenderCompIDs the signed-in user may see; all is true
// for an admin, who sees every broker.
func BrokerScope(r *http.Request) (all bool, own map[string]bool) {
	if IsAdmin(r) {
		return true, nil
	}
	own = map[string]bool{}
	for _, rec := range brokers.Meta.ListOwnedBy(Username(r)) {
		own[strings.ToUpper(rec.Sender)] = true
	}
	return false, own
}

// Forbid writes a 403 for data the user may not access. The message does not
// say whether the thing exists.
func Forbid(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{"ok":false,"error":"not found or not yours"}` + "\n"))
}
