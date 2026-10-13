package broker

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// AllowList serves /admin/api/allowlist.
func (h *Handler) AllowList(w http.ResponseWriter, r *http.Request) {
	m := readAllow()
	list := make([]map[string]string, 0, len(m))
	for ip, label := range m {
		list = append(list, map[string]string{"ip": ip, "label": label})
	}
	utils.WriteJSON(w, map[string]any{"list": list, "count": len(list)})
}

// AllowAdd serves /admin/api/allowlist/add.
func (h *Handler) AllowAdd(w http.ResponseWriter, r *http.Request) {
	var req struct{ IP, Label string }
	json.NewDecoder(r.Body).Decode(&req)
	ip := strings.TrimSpace(req.IP)
	if ip == "" || net.ParseIP(ip) == nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "a valid IP address is required"})
		return
	}
	m := readAllow()
	m[ip] = strings.TrimSpace(req.Label)
	if err := writeAllow(m); err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	audit.Record(middleware.ActorOf(r), "ALLOW+", ip)
	utils.WriteJSON(w, map[string]any{"ok": true})
}

// AllowRemove serves /admin/api/allowlist/del.
func (h *Handler) AllowRemove(w http.ResponseWriter, r *http.Request) {
	var req struct{ IP string }
	json.NewDecoder(r.Body).Decode(&req)
	ip := strings.TrimSpace(req.IP)
	m := readAllow()
	delete(m, ip)
	writeAllow(m)
	audit.Record(middleware.ActorOf(r), "ALLOW-", ip)
	utils.WriteJSON(w, map[string]any{"ok": true})
}
