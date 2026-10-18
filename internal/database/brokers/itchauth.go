package brokers

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"log"
	"os"
	"sort"
	"strings"
	"sync"

	"velexa-sim-me/internal/config"
)

// itchAuth is the ITCH market-data feed credential store (username → password),
// managed from the admin panel and, when ME_ITCH_AUTH=on, enforced at SoupBinTCP
// login. Kept parallel to fixAuth so a broker gets both a FIX and an ITCH set.
type itchAuth struct {
	mu      sync.RWMutex
	Enabled bool
	path    string
	creds   map[string]string // username → password
}

var Itch = &itchAuth{creds: map[string]string{}}

func LoadItch() *itchAuth {
	a := &itchAuth{
		Enabled: strings.EqualFold(config.EnvOr("ME_ITCH_AUTH", "off"), "on"),
		path:    config.EnvOr("ME_ITCH_AUTH_FILE", config.StateFile("itch_brokers.json")),
		creds:   map[string]string{},
	}
	if b, err := os.ReadFile(a.path); err == nil {
		var m map[string]string
		if json.Unmarshal(b, &m) == nil {
			for k, v := range m {
				a.creds[strings.TrimSpace(k)] = v
			}
		}
	}
	log.Printf("[ITCH] feed auth enabled=%v registered_users=%d", a.Enabled, len(a.creds))
	return a
}

// Authorize validates an ITCH feed login. Returns "" when allowed.
func (a *itchAuth) Authorize(user, pass string) string {
	if !a.Enabled {
		return ""
	}
	a.mu.RLock()
	want, ok := a.creds[strings.TrimSpace(user)]
	a.mu.RUnlock()
	if !ok {
		return "itch user not registered: " + user
	}
	if !secretEqual(pass, want) {
		return "invalid itch password for " + user
	}
	return ""
}

// Password returns the registered password for an ITCH user ("" if none).
func (a *itchAuth) Password(user string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.creds[strings.TrimSpace(user)]
}

func (a *itchAuth) List() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]string, 0, len(a.creds))
	for k := range a.creds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (a *itchAuth) Add(user, pass string) error {
	a.mu.Lock()
	a.creds[strings.TrimSpace(user)] = pass
	a.mu.Unlock()
	return a.save()
}

func (a *itchAuth) Del(user string) error {
	a.mu.Lock()
	delete(a.creds, strings.TrimSpace(user))
	a.mu.Unlock()
	return a.save()
}

func (a *itchAuth) save() error {
	a.mu.RLock()
	b, _ := json.MarshalIndent(a.creds, "", " ")
	path := a.path
	a.mu.RUnlock()
	return writeFileAtomic(path, b)
}

// secretEqual compares passwords in constant time (hashing first, so the
// length of the stored one does not leak either).
func secretEqual(got, want string) bool {
	a, b := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
