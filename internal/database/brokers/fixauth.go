package brokers

import (
	"encoding/json"
	"log"
	"os"
	"sort"
	"strings"
	"sync"

	"velexa-sim-me/internal/config"
)

// ── FIX Logon authentication ───────────────────────────────────────────────
// External brokers authenticate at logon with their assigned SenderCompID plus
// a password (FIX tag 554; tag 553 Username, if sent, must equal the CompID).
// The allowlist maps broker CompID → password, loaded from ME_FIX_AUTH_FILE
// (JSON: {"ALPHAEQUITIE":"secret", ...}). Enforcement is gated by ME_FIX_AUTH=on
// so the internal single-box setup keeps working unauthenticated by default.
type FixAuth struct {
	mu      sync.RWMutex
	Enabled bool
	path    string
	creds   map[string]string // CompID (upper) → password
}

// Fix is the process-wide allowlist, set by StartFixAcceptor and also read
// by the Live self-heal logon and the admin panel (add/remove brokers live).
var Fix = &FixAuth{creds: map[string]string{}}

// Password returns the registered password for a CompID ("" if none).
func (a *FixAuth) Password(comp string) string {
	if a == nil {
		return ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.creds[strings.ToUpper(strings.TrimSpace(comp))]
}

func LoadFix() *FixAuth {
	a := &FixAuth{
		Enabled: strings.EqualFold(config.EnvOr("ME_FIX_AUTH", "off"), "on"),
		path:    config.EnvOr("ME_FIX_AUTH_FILE", config.StateFile("fix_brokers.json")),
		creds:   map[string]string{},
	}
	if b, err := os.ReadFile(a.path); err == nil {
		var m map[string]string
		if json.Unmarshal(b, &m) == nil {
			for k, v := range m {
				a.creds[strings.ToUpper(strings.TrimSpace(k))] = v
			}
		}
	}
	log.Printf("[FIX] logon auth enabled=%v registered_brokers=%d", a.Enabled, len(a.creds))
	return a
}

// Authorize validates a broker logon. broker = the counterparty CompID (the
// acceptor session's TargetCompID); user/pass come from the Logon (553/554).
// Returns "" when allowed, otherwise the rejection reason.
func (a *FixAuth) Authorize(broker, user, pass string) string {
	if !a.Enabled {
		return ""
	}
	a.mu.RLock()
	want, ok := a.creds[strings.ToUpper(strings.TrimSpace(broker))]
	a.mu.RUnlock()
	if !ok {
		return "broker not registered: " + broker
	}
	if pass != want {
		return "invalid credentials for " + broker
	}
	if user != "" && !strings.EqualFold(strings.TrimSpace(user), broker) {
		return "username does not match SenderCompID"
	}
	return ""
}

// List returns the registered CompIDs (sorted). Passwords are not returned.
func (a *FixAuth) List() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]string, 0, len(a.creds))
	for k := range a.creds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (a *FixAuth) has(comp string) bool { return a.Password(comp) != "" }

// Add registers or updates a broker credential and persists the allowlist.
func (a *FixAuth) Add(comp, pass string) error {
	comp = strings.ToUpper(strings.TrimSpace(comp))
	a.mu.Lock()
	a.creds[comp] = pass
	a.mu.Unlock()
	return a.save()
}

// Del removes a broker credential and persists.
func (a *FixAuth) Del(comp string) error {
	a.mu.Lock()
	delete(a.creds, strings.ToUpper(strings.TrimSpace(comp)))
	a.mu.Unlock()
	return a.save()
}

func (a *FixAuth) save() error {
	a.mu.RLock()
	b, _ := json.MarshalIndent(a.creds, "", " ")
	path := a.path
	a.mu.RUnlock()
	if path == "" {
		return nil
	}
	return os.WriteFile(path, b, 0o600)
}
