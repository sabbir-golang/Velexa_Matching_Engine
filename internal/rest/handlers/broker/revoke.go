package broker

import (
	"errors"
	"log"
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
	if !decodeJSON(w, r, &req) {
		return
	}
	sender := strings.ToUpper(strings.TrimSpace(req.Sender))
	if !middleware.CanAccessBroker(r, sender) {
		middleware.Forbid(w)
		return
	}
	rec := brokers.Meta.Get(sender)
	fixPass := brokers.Fix.Password(sender)
	var itchUser, itchPass string
	if rec != nil && rec.ItchUser != "" {
		itchUser, itchPass = rec.ItchUser, brokers.Itch.Password(rec.ItchUser)
	}
	errs := []error{brokers.Fix.Del(sender), brokers.Meta.Del(sender)}
	if itchUser != "" {
		errs = append(errs, brokers.Itch.Del(itchUser))
	}
	if err := errors.Join(errs...); err != nil {
		// Put it back: a revocation that is not saved would undo itself at the next restart.
		if fixPass != "" {
			brokers.Fix.Add(sender, fixPass)
		}
		if itchUser != "" && itchPass != "" {
			brokers.Itch.Add(itchUser, itchPass)
		}
		if rec != nil {
			brokers.Meta.Add(rec)
		}
		log.Printf("[brokers] revoking %s failed: %v", sender, err)
		utils.WriteJSON(w, map[string]any{"ok": false, "error": errSave})
		return
	}
	audit.Record(middleware.ActorOf(r), "REVOKE", sender)
	utils.WriteJSON(w, map[string]any{"ok": true})
}
