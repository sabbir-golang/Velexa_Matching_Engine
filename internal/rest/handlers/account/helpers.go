package account

import (
	"encoding/json"
	"io"
	"net/http"

	"velexa-sim-me/internal/utils"
)

// decodeBody reads a small JSON request body into v; it writes the error reply
// itself and reports whether the request may continue.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return false
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(v); err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "bad request"})
		return false
	}
	return true
}

func replyErr(w http.ResponseWriter, err error) {
	utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
}
