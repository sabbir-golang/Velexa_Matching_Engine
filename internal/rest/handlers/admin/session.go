package admin

import (
	"fmt"
	"net/http"
	"strings"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/fixgw"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Session serves /admin/api/session.
// Per-broker session controls: force-disconnect or reset sequence numbers.
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	broker := strings.TrimSpace(r.URL.Query().Get("broker"))
	if broker != "" && !middleware.CanAccessBroker(r, broker) {
		middleware.Forbid(w)
		return
	}
	if fixgw.Active == nil || broker == "" {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "broker required"})
		return
	}
	var err error
	switch r.URL.Query().Get("action") {
	case "disconnect":
		err = fixgw.Active.Disconnect(broker)
	case "reset":
		err = fixgw.Active.ResetSeq(broker)
	default:
		err = fmt.Errorf("unknown action")
	}
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	audit.Record(middleware.ActorOf(r), "SESSION "+strings.ToUpper(r.URL.Query().Get("action")), broker)
	utils.WriteJSON(w, map[string]any{"ok": true})
}
