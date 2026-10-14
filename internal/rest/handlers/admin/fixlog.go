package admin

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/fixgw"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// FixLog serves /admin/api/fixlog.
// FIX message log — every inbound/outbound message, filterable by broker + ClOrdID.
func (h *Handler) FixLog(w http.ResponseWriter, r *http.Request) {
	limit := 250
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 1000 {
		limit = n
	}
	broker := strings.TrimSpace(r.URL.Query().Get("broker"))
	clordid := strings.TrimSpace(r.URL.Query().Get("clordid"))
	all, own := middleware.BrokerScope(r)
	var resp map[string]any
	if all {
		resp = map[string]any{"messages": fixgw.MsgLog.Recent(broker, clordid, limit)}
	} else { // a user: only their own brokers' sessions
		resp = map[string]any{"messages": fixgw.MsgLog.RecentWhere(func(session string) bool {
			s := strings.ToUpper(session)
			return own[s] && (broker == "" || strings.EqualFold(session, broker))
		}, clordid, limit)}
	}
	// The raw log holds broker logon passwords (tag 554, and 925 for a new one):
	// only admins see them.
	if u := middleware.CurrentUser(r); u != nil && !u.Role.Allows(users.RoleAdmin) {
		b, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(append(LoginSecretRE.ReplaceAll(b, []byte("${1}***")), '\n'))
		return
	}
	utils.WriteJSON(w, resp)
}

// LoginSecretRE finds password tags in a raw FIX message shown with | separators.
var LoginSecretRE = regexp.MustCompile(`(\|(?:554|925)=)[^|"]*`)
