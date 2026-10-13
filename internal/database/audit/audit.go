package audit

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"velexa-sim-me/internal/config"
)

// ── credential audit log ───────────────────────────────────────────────────
// Every credential issue/revoke and session control action is recorded (in a
// ring + appended to admin_audit.jsonl) and shown in the Diagnostics view.
type Entry struct {
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target"`
}

var (
	auditMu   sync.Mutex
	auditRing []Entry
	Path      = config.EnvOr("ME_AUDIT_FILE", config.StateFile("admin_audit.jsonl"))
)

func Record(actor, action, target string) {
	e := Entry{Time: time.Now(), Actor: actor, Action: action, Target: target}
	auditMu.Lock()
	auditRing = append(auditRing, e)
	if len(auditRing) > 300 {
		auditRing = auditRing[len(auditRing)-300:]
	}
	auditMu.Unlock()
	if f, err := os.OpenFile(Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		b, _ := json.Marshal(e)
		f.Write(append(b, '\n'))
		f.Close()
	}
}

func Recent(n int) []Entry {
	auditMu.Lock()
	defer auditMu.Unlock()
	out := []Entry{}
	for i := len(auditRing) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, auditRing[i])
	}
	return out
}

func Load() {
	b, err := os.ReadFile(Path)
	if err != nil {
		return
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var e Entry
		if json.Unmarshal([]byte(line), &e) == nil {
			auditRing = append(auditRing, e)
		}
	}
	if len(auditRing) > 300 {
		auditRing = auditRing[len(auditRing)-300:]
	}
}
