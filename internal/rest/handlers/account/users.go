package account

import (
	"net/http"
	"strings"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// List serves /admin/api/users.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, map[string]any{"users": h.Users.List()})
}

// Add serves /admin/api/user/add.
func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password, Role string }
	if !decodeBody(w, r, &req) {
		return
	}
	role := users.Role(strings.ToLower(strings.TrimSpace(req.Role)))
	if err := h.Users.Add(req.Username, req.Password, role); err != nil {
		replyErr(w, err)
		return
	}
	audit.Record(middleware.ActorOf(r), "user.add", strings.TrimSpace(req.Username)+" role="+string(role))
	utils.WriteJSON(w, map[string]any{"ok": true})
}

// Update serves /admin/api/user/update: change a role and/or
// enable or disable an account. Omitted fields are left alone.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string
		Role     *string
		Disabled *bool
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Role != nil {
		role := users.Role(strings.ToLower(strings.TrimSpace(*req.Role)))
		if err := h.Users.SetRole(req.Username, role); err != nil {
			replyErr(w, err)
			return
		}
		audit.Record(middleware.ActorOf(r), "user.role", req.Username+" -> "+string(role))
	}
	if req.Disabled != nil {
		if err := h.Users.SetDisabled(req.Username, *req.Disabled); err != nil {
			replyErr(w, err)
			return
		}
		action := "user.enable"
		if *req.Disabled {
			action = "user.disable"
		}
		audit.Record(middleware.ActorOf(r), action, req.Username)
	}
	utils.WriteJSON(w, map[string]any{"ok": true})
}

// ResetPassword serves /admin/api/user/password: an admin resets
// another account's password.
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password string }
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.Users.SetPassword(req.Username, req.Password); err != nil {
		replyErr(w, err)
		return
	}
	audit.Record(middleware.ActorOf(r), "user.password.reset", req.Username)
	utils.WriteJSON(w, map[string]any{"ok": true})
}

// Delete serves /admin/api/user/delete.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username string }
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.Users.Delete(req.Username); err != nil {
		replyErr(w, err)
		return
	}
	audit.Record(middleware.ActorOf(r), "user.delete", req.Username)
	utils.WriteJSON(w, map[string]any{"ok": true})
}
