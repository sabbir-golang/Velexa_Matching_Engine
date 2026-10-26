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
// (JSON: {"ALPHAEQUITIE":"secret", ...}).
//
// ME_FIX_AUTH picks the mode: "on" always checks, "off" never checks, and unset
// means "auto": logons are checked as soon as at least one broker is
// registered, so a fresh install accepts its first OMS before any credentials
// exist and locks down once the first broker is added in the console.
type FixAuth struct {
	mu    sync.RWMutex
	Mode  string // on | off | auto
	path  string
	creds map[string]string // CompID (upper) → password
	temp  map[string]string // simulator bots: CompID → password, in memory only
}

// Simulator bots log on with CompIDs under these prefixes. They are reserved:
// a broker cannot be registered with one.
const (
	SimBotPrefix   = "SIMBOT"   // market bots
	SimBenchPrefix = "SIMBENCH" // benchmark sessions (not written to the message log)
)

// IsSimulator reports whether comp is one of the simulator's bot CompIDs.
func IsSimulator(comp string) bool {
	c := strings.ToUpper(strings.TrimSpace(comp))
	return strings.HasPrefix(c, SimBotPrefix) || strings.HasPrefix(c, SimBenchPrefix)
}

// IsSimBench reports whether comp is a benchmark session.
func IsSimBench(comp string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(comp)), SimBenchPrefix)
}

// AddTemp lets a simulator bot log on with pass until RemoveTemp. It is kept in
// memory only: never saved, never listed, and it does not switch the "auto"
// logon check on.
func (a *FixAuth) AddTemp(comp, pass string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.temp == nil {
		a.temp = map[string]string{}
	}
	a.temp[strings.ToUpper(strings.TrimSpace(comp))] = pass
}

// RemoveTemp withdraws a simulator bot's logon.
func (a *FixAuth) RemoveTemp(comp string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.temp, strings.ToUpper(strings.TrimSpace(comp)))
}

// Enforced reports whether logons are being checked right now.
func (a *FixAuth) Enforced() bool {
	if a == nil {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.enforcedLocked()
}

func (a *FixAuth) enforcedLocked() bool {
	return a.Mode == "on" || (a.Mode == "auto" && len(a.creds) > 0)
}

// fixAuthMode reads ME_FIX_AUTH: on, off, or (unset / anything else) auto.
func fixAuthMode() string {
	switch strings.ToLower(config.EnvOr("ME_FIX_AUTH", "auto")) {
	case "on", "true", "1", "yes":
		return "on"
	case "off", "false", "0", "no":
		return "off"
	}
	return "auto"
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
		Mode:  fixAuthMode(),
		path:  config.EnvOr("ME_FIX_AUTH_FILE", config.StateFile("fix_brokers.json")),
		creds: map[string]string{},
	}
	if b, err := os.ReadFile(a.path); err == nil {
		var m map[string]string
		if json.Unmarshal(b, &m) == nil {
			for k, v := range m {
				a.creds[strings.ToUpper(strings.TrimSpace(k))] = v
			}
		}
	}
	log.Printf("[FIX] logon auth mode=%s enforced=%v registered_brokers=%d", a.Mode, a.enforcedLocked(), len(a.creds))
	if a.Mode == "auto" && len(a.creds) == 0 {
		log.Printf("[FIX] no brokers registered yet: any CompID may log on until the first broker is added (Broker Credentials)")
	}
	return a
}

// Authorize validates a broker logon. broker = the counterparty CompID (the
// acceptor session's TargetCompID); user/pass come from the Logon (553/554).
// Returns "" when allowed, otherwise the rejection reason.
func (a *FixAuth) Authorize(broker, user, pass string) string {
	a.mu.RLock()
	enforced := a.enforcedLocked()
	want, ok := a.creds[strings.ToUpper(strings.TrimSpace(broker))]
	if !ok {
		want, ok = a.temp[strings.ToUpper(strings.TrimSpace(broker))]
	}
	a.mu.RUnlock()
	if !enforced {
		return ""
	}
	if !ok {
		return "broker not registered: " + broker
	}
	if !secretEqual(pass, want) {
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
	return writeFileAtomic(path, b)
}
