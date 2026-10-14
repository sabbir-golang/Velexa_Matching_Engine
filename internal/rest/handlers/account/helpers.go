package account

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"velexa-sim-me/internal/database/users"
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

// replyErr shows the account errors written for people (bad input, last
// admin…); anything else (a disk error, say) is logged and answered generically.
func replyErr(w http.ResponseWriter, err error) {
	for _, known := range []error{users.ErrInvalidUsername, users.ErrWeakPassword, users.ErrInvalidRole,
		users.ErrInvalidProfile, users.ErrEmailTaken, users.ErrExists, users.ErrNotFound, users.ErrLastAdmin,
		users.ErrReservedName, users.ErrWrongPassword} {
		if errors.Is(err, known) {
			utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	log.Printf("[users] %v", err)
	utils.WriteJSON(w, map[string]any{"ok": false, "error": "could not save the change"})
}
