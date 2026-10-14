package broker

import (
	"net/http"
	"strings"
	"time"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Add serves /admin/api/broker/add.
func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	var req struct{ Sender, Company, Contact, FixPass, ItchUser, ItchPass string }
	if !decodeJSON(w, r, &req) {
		return
	}
	sender := strings.ToUpper(strings.TrimSpace(req.Sender))
	if sender == "" {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "SenderCompID is required"})
		return
	}
	if !validCompID(sender) {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "SenderCompID must be 1-20 letters, digits, '.', '-' or '_'"})
		return
	}
	// A CompID identifies one broker on the exchange: never hand out one that is
	// already registered (to anyone), and never overwrite its credentials.
	if brokers.Meta.Get(sender) != nil || brokers.Fix.Password(sender) != "" {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "SenderCompID " + sender + " is already registered"})
		return
	}
	fixPass := strings.TrimSpace(req.FixPass)
	if fixPass == "" {
		fixPass = genPass("FIX")
	}
	itchUser := strings.TrimSpace(req.ItchUser)
	if itchUser == "" {
		itchUser = sender
		if len(itchUser) > 6 {
			itchUser = itchUser[:6]
		}
	}
	if brokers.Meta.ItchUserTaken(itchUser) || brokers.Itch.Password(itchUser) != "" {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "ITCH username " + itchUser + " is already in use — choose another"})
		return
	}
	itchPass := strings.TrimSpace(req.ItchPass)
	if itchPass == "" {
		itchPass = genPass("ITCH")
	}
	company := strings.TrimSpace(req.Company)
	brokers.Fix.Add(sender, fixPass)
	brokers.Itch.Add(itchUser, itchPass)
	owner := "" // registered by an admin: belongs to the exchange
	if !middleware.IsAdmin(r) {
		owner = middleware.Username(r)
	}
	brokers.Meta.Add(&brokers.Record{Company: company, Sender: sender, ItchUser: itchUser,
		Contact: strings.TrimSpace(req.Contact), Created: time.Now().Format("2006-01-02 15:04"), Owner: owner})
	label := sender
	if company != "" {
		label = company + " (" + sender + ")"
	}
	audit.Record(middleware.ActorOf(r), "ISSUE", label)
	utils.WriteJSON(w, map[string]any{"ok": true, "sender": sender, "company": company,
		"fix_password": fixPass, "itch_user": itchUser, "itch_password": itchPass})
}

// validCompID is a FIX CompID the exchange accepts.
func validCompID(s string) bool {
	if len(s) == 0 || len(s) > 20 {
		return false
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
