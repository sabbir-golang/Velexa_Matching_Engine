package me

import (
	"encoding/json"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
)

// itchAuth is the ITCH market-data feed credential store (username → password),
// managed from the admin panel and, when ME_ITCH_AUTH=on, enforced at SoupBinTCP
// login. Kept parallel to fixAuth so a broker gets both a FIX and an ITCH set.
type itchAuth struct {
	mu      sync.RWMutex
	enabled bool
	path    string
	creds   map[string]string // username → password
}

var meItchAuth = &itchAuth{creds: map[string]string{}}

func loadItchAuth() *itchAuth {
	a := &itchAuth{
		enabled: strings.EqualFold(envOr("ME_ITCH_AUTH", "off"), "on"),
		path:    envOr("ME_ITCH_AUTH_FILE", stateFile("itch_brokers.json")),
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
	log.Printf("[ITCH] feed auth enabled=%v registered_users=%d", a.enabled, len(a.creds))
	return a
}

// authorize validates an ITCH feed login. Returns "" when allowed.
func (a *itchAuth) authorize(user, pass string) string {
	if !a.enabled {
		return ""
	}
	a.mu.RLock()
	want, ok := a.creds[strings.TrimSpace(user)]
	a.mu.RUnlock()
	if !ok {
		return "itch user not registered: " + user
	}
	if pass != want {
		return "invalid itch password for " + user
	}
	return ""
}

// password returns the registered password for an ITCH user ("" if none).
func (a *itchAuth) password(user string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.creds[strings.TrimSpace(user)]
}

func (a *itchAuth) list() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]string, 0, len(a.creds))
	for k := range a.creds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (a *itchAuth) add(user, pass string) error {
	a.mu.Lock()
	a.creds[strings.TrimSpace(user)] = pass
	a.mu.Unlock()
	return a.save()
}

func (a *itchAuth) del(user string) error {
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
	if path == "" {
		return nil
	}
	return os.WriteFile(path, b, 0o600)
}
