package broker

import (
	"net/http"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/utils"
)

// List serves /admin/api/brokers.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, map[string]any{
		"brokers":    brokers.Meta.List(),
		"fix_total":  len(brokers.Fix.List()),
		"itch_total": len(brokers.Itch.List()),
	})
}
