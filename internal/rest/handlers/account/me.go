package account

import (
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
	if err := h.Users.ChangeOwnPassword(u.Username, req.Old, req.New); err != nil {
		replyErr(w, err)
		return
	}
	audit.Record(middleware.ActorOf(r), "user.password.self", u.Username)
	utils.WriteJSON(w, map[string]any{"ok": true})
}
