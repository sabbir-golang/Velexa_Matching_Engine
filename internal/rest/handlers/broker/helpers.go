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

// errSave is shown when broker credentials cannot be written to the data folder.
const errSave = "could not save the broker credentials — check that the engine's data folder is writable (see the engine log)"

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

// meEndpoints returns the connection details a broker needs, as shown on the
// broker detail view and the onboarding sheet. Each value comes from the env
// when set (ME_PUB_IP, ME_PUB_HOST, ME_INT_IP, ME_ITCH_IP, the ports…) and
// otherwise from this machine: its LAN IP (also used as the host) and the ports the
// engine really listens on, so a fresh install on any PC shows usable values.
//
// ext_addr is where an outside broker connects: the TLS FIX gateway
// (ME_PUB_IP:ME_FIXS_PORT, ext_tls=1) when one is configured, else the FIX
// acceptor itself.
func meEndpoints() map[string]string {
	lan := config.LocalIP()
	// The port brokers reach: ME_PUB_FIX_PORT when it differs from the listener
	// (a container whose 5001 is published on another host port), else FIX_PORT.
	fixPort := config.EnvOr("ME_PUB_FIX_PORT", config.EnvOr("FIX_PORT", "5001"))
	ep := map[string]string{
		"pub_host":  config.EnvOr("ME_PUB_HOST", lan),
		"pub_ip":    config.EnvOr("ME_PUB_IP", lan),
		"fixs_port": config.EnvOr("ME_FIXS_PORT", ""),
		"fix_port":  fixPort,
		"int_ip":    config.EnvOr("ME_INT_IP", lan),
		"int_port":  config.EnvOr("ME_INT_PORT", fixPort),
		"itch_ip":   config.EnvOr("ME_ITCH_IP", config.EnvOr("ME_PUB_IP", lan)),
		"itch_port": config.EnvOr("ME_ITCH_PORT", config.EnvOr("ITCH_PORT", "5501")),
		"target":    config.EnvOr("FIX_SENDER", "DSEME"),
		"begin":     config.EnvOr("ME_FIX_BEGIN", "FIXT.1.1"),
	}
	if ep["fixs_port"] != "" {
		ep["ext_addr"], ep["ext_tls"] = ep["pub_ip"]+":"+ep["fixs_port"], "1"
	} else {
		ep["ext_addr"] = ep["pub_ip"] + ":" + fixPort
	}
	if ep["int_ip"]+":"+ep["int_port"] != ep["ext_addr"] {
		ep["show_int"] = "1"
	}
	if ep["begin"] == "FIXT.1.1" {
		ep["applver"] = config.EnvOr("ME_FIX_APPLVER", "9")
		ep["applver_name"] = map[string]string{"7": "FIX.5.0", "8": "FIX.5.0 SP1", "9": "FIX.5.0 SP2"}[ep["applver"]]
	}
	return ep
}

// fixVersion is the protocol line for the sheet: "FIXT.1.1 · FIX.5.0 SP2".
func fixVersion(ep map[string]string) string {
	if ep["applver_name"] != "" {
		return ep["begin"] + " · " + ep["applver_name"]
	}
	return ep["begin"]
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
