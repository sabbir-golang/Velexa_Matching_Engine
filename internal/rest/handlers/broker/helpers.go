package broker

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// genPass makes a random broker password (96 bits).
func genPass(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic("broker: no randomness: " + err.Error())
	}
	return prefix + "-" + hex.EncodeToString(b)
}

// decodeJSON reads a small JSON body (at most 64 KB) into v; on a bad body it
// replies itself and reports false.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	middleware.LimitBody(w, r)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "bad request"})
		return false
	}
	return true
}

// meEndpoints returns the reachable connection details a broker needs to fill
// in on their credential (bm_itch) page. IPs are the ORIGIN (not the
// Cloudflare-proxied host, which doesn't pass FIX/ITCH ports). Overridable via
// env so the admin can set the real public IP.
func meEndpoints() map[string]string {
	return map[string]string{
		"pub_host":  config.EnvOr("ME_PUB_HOST", "me.velexa.net"),
		"pub_ip":    config.EnvOr("ME_PUB_IP", "103.169.105.76"),
		"fixs_port": config.EnvOr("ME_FIXS_PORT", "5443"),
		"int_ip":    config.EnvOr("ME_INT_IP", "10.1.1.9"),
		"int_port":  config.EnvOr("ME_INT_PORT", "5001"),
		"itch_ip":   config.EnvOr("ME_ITCH_IP", config.EnvOr("ME_PUB_IP", "103.169.105.76")),
		"itch_port": config.EnvOr("ME_ITCH_PORT", "5501"),
		"target":    config.EnvOr("FIX_SENDER", "DSEME"),
	}
}

// ── External OMS IP allowlist (fix_allow.json, read live by the FIXS forwarder) ──
var allowPath = config.EnvOr("FIXS_ALLOW_FILE", config.StateFile("fix_allow.json"))

func readAllow() map[string]string {
	m := map[string]string{}
	if b, err := os.ReadFile(allowPath); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func writeAllow(m map[string]string) error {
	b, _ := json.MarshalIndent(m, "", " ")
	if err := os.WriteFile(allowPath, b, 0o600); err != nil {
		return err
	}
	return os.Chmod(allowPath, 0o600) // WriteFile keeps the mode of an existing file
}
