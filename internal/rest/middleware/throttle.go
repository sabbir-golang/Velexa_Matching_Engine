package middleware

import (
	"strings"

	"sync"
	"time"
	"velexa-sim-me/internal/config"
)

// ── sign-in brute-force throttle ───────────────────────────────────────────
// Failed password checks are counted per client IP and per username. After
// maxLoginFails failures inside loginWindow the IP (or the username, from any
// IP) is locked for loginLock. All of it is off unless ME_RATE_LIMIT=on. The maps are swept and capped so a flood of
// random names cannot grow them without bound.

type failRec struct {
	count int
	last  time.Time
	until time.Time // locked until
}

var (
	loginMu    sync.Mutex
	loginFails = map[string]*failRec{} // "ip:<addr>" or "user:<name>"
	lastSweep  time.Time
)

const (
	maxLoginFails  = 5
	loginWindow    = 15 * time.Minute
	loginLock      = 15 * time.Minute
	maxFailRecords = 50_000
)

func ipKey(ip string) string     { return "ip:" + ip }
func userKey(name string) string { return "user:" + strings.ToLower(strings.TrimSpace(name)) }

// LoginBlocked reports whether sign-in (or a password check) is locked for this
// IP or this username.
func LoginBlocked(ip, username string) bool {
	if !config.RateLimit() {
		return false
	}
	loginMu.Lock()
	defer loginMu.Unlock()
	now := time.Now()
	for _, k := range []string{ipKey(ip), userKey(username)} {
		if r := loginFails[k]; r != nil && now.Before(r.until) {
			return true
		}
	}
	return false
}

// NoteLoginFail counts one failed password check for the IP and the username.
func NoteLoginFail(ip, username string) {
	if !config.RateLimit() {
		return
	}
	loginMu.Lock()
	defer loginMu.Unlock()
	now := time.Now()
	sweepLocked(now)
	keys := []string{ipKey(ip)}
	if strings.TrimSpace(username) != "" {
		keys = append(keys, userKey(username))
	}
	for _, k := range keys {
		r := loginFails[k]
		if r == nil || now.Sub(r.last) > loginWindow {
			if r == nil && len(loginFails) >= maxFailRecords {
				continue // full: the IP lock still applies, as it is checked first
			}
			r = &failRec{}
			loginFails[k] = r
		}
		r.count++
		r.last = now
		if r.count >= maxLoginFails {
			r.until = now.Add(loginLock)
		}
	}
}

// ClearLoginFail forgets the failures of an IP and a username after a good sign-in.
func ClearLoginFail(ip, username string) {
	loginMu.Lock()
	delete(loginFails, ipKey(ip))
	if username != "" {
		delete(loginFails, userKey(username))
	}
	loginMu.Unlock()
}

// ResetLoginThrottle forgets every failure (used by tests).
func ResetLoginThrottle() {
	loginMu.Lock()
	loginFails = map[string]*failRec{}
	loginMu.Unlock()
}

// sweepLocked drops records that are neither locked nor inside the window.
func sweepLocked(now time.Time) {
	if now.Sub(lastSweep) < time.Minute && len(loginFails) < maxFailRecords {
		return
	}
	lastSweep = now
	for k, r := range loginFails {
		if now.After(r.until) && now.Sub(r.last) > loginWindow {
			delete(loginFails, k)
		}
	}
}
