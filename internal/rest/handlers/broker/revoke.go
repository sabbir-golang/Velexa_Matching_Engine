package broker

import (
	"encoding/json"
	"net/http"
	"strings"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Revoke serves /admin/api/broker/del.
// broker/del — revoke a whole broker (FIX + ITCH + metadata) by SenderCompID.
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	var req struct{ Sender string }
	json.NewDecoder(r.Body).Decode(&req)
	sender := strings.ToUpper(strings.TrimSpace(req.Sender))
	if rec := brokers.Meta.Get(sender); rec != nil && rec.ItchUser != "" {
		brokers.Itch.Del(rec.ItchUser)
	}
	brokers.Fix.Del(sender)
	brokers.Meta.Del(sender)
	audit.Record(middleware.ActorOf(r), "REVOKE", sender)
	utils.WriteJSON(w, map[string]any{"ok": true})
}
