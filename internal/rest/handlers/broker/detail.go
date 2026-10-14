package broker

import (
	"net/http"
	"strings"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Detail serves /admin/api/broker/detail.
// broker/detail — full dynamic credential + linkage for one broker.
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	sender := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("sender")))
	if !middleware.CanAccessBroker(r, sender) {
		middleware.Forbid(w)
		return
	}
	rec := brokers.Meta.Get(sender)
	fixPass := brokers.Fix.Password(sender)
	itchUser, company, created, contact := "", "", "", ""
	if rec != nil {
		itchUser, company, created, contact = rec.ItchUser, rec.Company, rec.Created, rec.Contact
	}
	itchPass := brokers.Itch.Password(itchUser)
	utils.WriteJSON(w, map[string]any{
		"sender": sender, "company": company, "contact": contact, "created": created,
		"fix_password": fixPass, "itch_user": itchUser, "itch_password": itchPass,
		"endpoints":  meEndpoints(),
		"fix_active": fixPass != "", "itch_active": itchPass != "",
	})
}
