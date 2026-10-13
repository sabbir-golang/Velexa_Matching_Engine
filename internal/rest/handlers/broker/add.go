package broker

import (
	"encoding/json"
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
	json.NewDecoder(r.Body).Decode(&req)
	sender := strings.ToUpper(strings.TrimSpace(req.Sender))
	if sender == "" {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "SenderCompID is required"})
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
	itchPass := strings.TrimSpace(req.ItchPass)
	if itchPass == "" {
		itchPass = genPass("ITCH")
	}
	company := strings.TrimSpace(req.Company)
	brokers.Fix.Add(sender, fixPass)
	brokers.Itch.Add(itchUser, itchPass)
	brokers.Meta.Add(&brokers.Record{Company: company, Sender: sender, ItchUser: itchUser,
		Contact: strings.TrimSpace(req.Contact), Created: time.Now().Format("2006-01-02 15:04")})
	label := sender
	if company != "" {
		label = company + " (" + sender + ")"
	}
	audit.Record(middleware.ActorOf(r), "ISSUE", label)
	utils.WriteJSON(w, map[string]any{"ok": true, "sender": sender, "company": company,
		"fix_password": fixPass, "itch_user": itchUser, "itch_password": itchPass})
}
