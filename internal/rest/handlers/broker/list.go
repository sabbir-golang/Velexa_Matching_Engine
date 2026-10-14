package broker

import (
	"net/http"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// List serves /admin/api/brokers.
// An admin sees every broker (with its owner); a user only their own.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if middleware.IsAdmin(r) {
		utils.WriteJSON(w, map[string]any{
			"brokers":    brokers.Meta.List(),
			"fix_total":  len(brokers.Fix.List()),
			"itch_total": len(brokers.Itch.List()),
			"scope":      "all",
		})
		return
	}
	own := brokers.Meta.ListOwnedBy(middleware.Username(r))
	utils.WriteJSON(w, map[string]any{"brokers": own, "fix_total": len(own), "itch_total": len(own), "scope": "own"})
}
