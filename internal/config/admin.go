package config

import (
	"crypto/rand"
	"log"
	"os"
	"strings"
	"sync"
)

// DefaultAdminPass is the password older builds used when ME_ADMIN_PASS was not
// set. It is published, so it is never accepted any more: an account still
// holding it is re-keyed or refused at startup.
const DefaultAdminPass = "Velexa@ME2026"

// AdminUser is the console username of the environment admin (ME_ADMIN_USER).
func AdminUser() string { return EnvOr("ME_ADMIN_USER", "admin") }

// AdminPass is the environment admin's password (ME_ADMIN_PASS). It is "" when
// unset or still the old published default, which turns the environment admin off.
func AdminPass() string {
	p := os.Getenv("ME_ADMIN_PASS")
	if p == DefaultAdminPass {
		return ""
	}
	return p
}

var (
	sessionKeyOnce sync.Once
	sessionKey     []byte
)

// SessionKey is the HMAC key that signs console session cookies. ME_ADMIN_SECRET
// is used when it is at least 32 bytes; otherwise a random key is generated once
// and kept in the state directory (session.key, mode 0600). It is never derived
// from a password.
func SessionKey() []byte {
	sessionKeyOnce.Do(func() {
		if s := os.Getenv("ME_ADMIN_SECRET"); len(s) >= 32 {
			sessionKey = []byte(s)
			return
		} else if s != "" {
			log.Printf("[auth] ME_ADMIN_SECRET is shorter than 32 bytes — ignored, using %s", StateFile("session.key"))
		}
		path := StateFile("session.key")
		if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
			sessionKey = b
			return
		}
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("[auth] cannot generate a session key: %v", err)
		}
		if err := os.WriteFile(path, b, 0o600); err != nil {
			log.Printf("[auth] cannot save %s (%v) — sessions end when the engine restarts", path, err)
		}
		sessionKey = b
	})
	return sessionKey
}

// CookieSecure reports whether session cookies must carry the Secure flag even
// when the request itself arrived over plain HTTP (TLS ends at a proxy):
// ME_COOKIE_SECURE=on. It also turns on HSTS.
func CookieSecure() bool { return strings.EqualFold(os.Getenv("ME_COOKIE_SECURE"), "on") }

// TrustedProxies lists the peers whose X-Forwarded-For / X-Real-IP /
// CF-Connecting-IP headers are believed (ME_TRUSTED_PROXIES, comma separated
// IPs or CIDRs; default: this machine only).
func TrustedProxies() []string {
	var out []string
	for _, p := range strings.Split(EnvOr("ME_TRUSTED_PROXIES", "127.0.0.1,::1"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// RateLimit reports whether sign-in lockouts and the sign-up quota are on
// (ME_RATE_LIMIT=on). Off by default for now.
func RateLimit() bool { return strings.EqualFold(os.Getenv("ME_RATE_LIMIT"), "on") }

// RegistrationOpen reports whether self-registration is on (ME_REGISTRATION=off turns it off).
func RegistrationOpen() bool {
	return strings.ToLower(os.Getenv("ME_REGISTRATION")) != "off"
}
