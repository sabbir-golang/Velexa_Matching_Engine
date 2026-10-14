package account

import (
	"errors"
	"net/http"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Me serves /admin/api/me: the signed-in user and what they may do.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	u := middleware.CurrentUser(r)
	utils.WriteJSON(w, map[string]any{"ok": true, "user": u.Username, "role": u.Role,
		"can_write": u.Role.Allows(users.RoleAdmin)})
}

// MePassword serves /admin/api/me/password: change one's own password.
func (h *Handler) MePassword(w http.ResponseWriter, r *http.Request) {
	var req struct{ Old, New string }
	if !decodeBody(w, r, &req) {
		return
	}
	u := middleware.CurrentUser(r)
	ip := middleware.ClientIP(r)
	// The current password is checked, so this form is throttled like sign-in.
	if middleware.LoginBlocked(ip, u.Username) {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "too many attempts — try again in a few minutes"})
		return
	}
	if err := h.Users.ChangeOwnPassword(u.Username, req.Old, req.New); err != nil {
		if errors.Is(err, users.ErrWrongPassword) {
			middleware.NoteLoginFail(ip, u.Username)
		}
		replyErr(w, err)
		return
	}
	// Every other session of the account has ended; keep this browser signed in.
	middleware.StartSession(w, r, h.Users, u.Username)
	audit.Record(middleware.ActorOf(r), "user.password.self", u.Username)
	utils.WriteJSON(w, map[string]any{"ok": true})
}
