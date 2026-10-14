package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/database/users"
)

// ClientIP is the address of the client behind the request. Forwarding headers
// (X-Forwarded-For, X-Real-IP) are believed only when the direct peer is a
// trusted proxy (ME_TRUSTED_PROXIES); otherwise anyone could pick their own
// address and dodge the sign-in lockout. CF-Connecting-IP is never used: a
// client that reaches nginx without going through Cloudflare could set it. Behind
// Cloudflare, list its address ranges in ME_TRUSTED_PROXIES instead.
func ClientIP(r *http.Request) string {
	peer := remoteHost(r)
	if !trustedProxy(peer) {
		return peer
	}
	// The right-most hop that is not one of our proxies is the real client; the
	// hops to its left were written by the client and prove nothing.
	if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
		hops := strings.Split(strings.Join(xff, ","), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			h := strings.TrimSpace(hops[i])
			if net.ParseIP(h) == nil {
				break
			}
			if !trustedProxy(h) {
				return h
			}
		}
	}
	if h := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(h) != nil {
		return h
	}
	return peer
}

func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// trustedProxy reports whether ip is listed in ME_TRUSTED_PROXIES.
func trustedProxy(ip string) bool {
	addr := net.ParseIP(ip)
	if addr == nil {
		return false
	}
	for _, p := range config.TrustedProxies() {
		if strings.Contains(p, "/") {
			if _, n, err := net.ParseCIDR(p); err == nil && n.Contains(addr) {
				return true
			}
		} else if pip := net.ParseIP(p); pip != nil && pip.Equal(addr) {
			return true
		}
	}
	return false
}

// IsHTTPS reports whether the browser reached us over HTTPS: directly, through
// a trusted proxy that says so, or because ME_COOKIE_SECURE=on.
func IsHTTPS(r *http.Request) bool {
	if r.TLS != nil || config.CookieSecure() {
		return true
	}
	return trustedProxy(remoteHost(r)) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

const (
	// SessionCookie is the console session cookie on plain HTTP; over HTTPS it is
	// SecureSessionCookie, whose __Host- prefix makes the browser refuse it unless
	// it is Secure, host-only and Path=/.
	SessionCookie       = "me_admin"
	SecureSessionCookie = "__Host-me_admin"
	// SessionTTL is the longest a sign-in lasts; SessionIdle ends it sooner when
	// the console is not used.
	SessionTTL  = 8 * time.Hour
	SessionIdle = 30 * time.Minute
	// sessionRefresh is how often an active session's cookie is re-issued to
	// move its idle deadline.
	sessionRefresh = time.Minute
	sessionFormat  = "v2"
)

// session is the content of a session cookie.
type session struct {
	name, ver   string
	issued, act int64 // sign-in time and last activity (unix seconds)
}

// signSession encodes and signs a session: base64url("v2|name|ver|iat|act").hmac.
func signSession(s session) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(strings.Join([]string{sessionFormat, s.name, s.ver,
		strconv.FormatInt(s.issued, 10), strconv.FormatInt(s.act, 10)}, "|")))
	return payload + "." + sessionMAC(payload)
}

func sessionMAC(payload string) string {
	mac := hmac.New(sha256.New, config.SessionKey())
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignSession returns a signed session cookie value (used by tests).
func SignSession(name, ver string, issued, active time.Time) string {
	return signSession(session{name: name, ver: ver, issued: issued.Unix(), act: active.Unix()})
}

// parseSession checks a cookie value's signature and lifetime.
func parseSession(value string, now time.Time) (session, bool) {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(sessionMAC(payload))) {
		return session{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return session{}, false
	}
	f := strings.Split(string(raw), "|")
	if len(f) != 5 || f[0] != sessionFormat || f[1] == "" || f[2] == "" {
		return session{}, false
	}
	iat, err1 := strconv.ParseInt(f[3], 10, 64)
	act, err2 := strconv.ParseInt(f[4], 10, 64)
	if err1 != nil || err2 != nil {
		return session{}, false
	}
	t := now.Unix()
	if iat > t+60 || act > t+60 || t >= iat+int64(SessionTTL.Seconds()) || t >= act+int64(SessionIdle.Seconds()) {
		return session{}, false
	}
	return session{name: f[1], ver: f[2], issued: iat, act: act}, true
}

// writeSession sets the session cookie; MaxAge follows the idle deadline.
func writeSession(w http.ResponseWriter, r *http.Request, s session) {
	name, secure := SessionCookie, IsHTTPS(r)
	if secure {
		name = SecureSessionCookie
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: signSession(s), Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: int(SessionIdle.Seconds())})
}

// StartSession signs the user in on this browser. It reports false when the
// account cannot hold a session (disabled or deleted meanwhile).
func StartSession(w http.ResponseWriter, r *http.Request, store *users.Store, username string) bool {
	u, ver, ok := store.Session(username)
	if !ok {
		return false
	}
	now := time.Now().Unix()
	writeSession(w, r, session{name: u.Username, ver: ver, issued: now, act: now})
	return true
}

// ClearSession removes the session cookie from this browser (both names, and
// the "/admin" path older builds used).
func ClearSession(w http.ResponseWriter) {
	for _, c := range []*http.Cookie{
		{Name: SessionCookie, Path: "/"}, {Name: SessionCookie, Path: "/admin"},
		{Name: SecureSessionCookie, Path: "/", Secure: true},
	} {
		c.MaxAge, c.HttpOnly, c.SameSite = -1, true, http.SameSiteStrictMode
		http.SetCookie(w, c)
	}
}

// HomeFor is the console address for a role: admins use /admin, everyone else /user.
func HomeFor(u *users.User) string {
	if u != nil && u.Role == users.RoleAdmin {
		return "/admin"
	}
	return "/user"
}

// sessionOf is the signed-in, enabled user behind the request's cookie, with the
// decoded session. The cookie must carry the account's current session token,
// so a sign-out, password change, disable or delete ends it at once.
func sessionOf(store *users.Store, r *http.Request) (*users.User, session, bool) {
	if store == nil {
		return nil, session{}, false
	}
	now := time.Now()
	for _, name := range []string{SecureSessionCookie, SessionCookie} {
		ck, err := r.Cookie(name)
		if err != nil {
			continue
		}
		s, ok := parseSession(ck.Value, now)
		if !ok {
			continue
		}
		u, ver, ok := store.Session(s.name)
		if ok && hmac.Equal([]byte(ver), []byte(s.ver)) {
			return u, s, true
		}
	}
	return nil, session{}, false
}

// SessionUser is the signed-in, enabled user behind the request's cookie, or nil.
func SessionUser(store *users.Store, r *http.Request) *users.User {
	u, _, _ := sessionOf(store, r)
	return u
}

// touchSession re-issues an active session's cookie now and then, so the idle
// deadline moves while the console is in use.
func touchSession(w http.ResponseWriter, r *http.Request, s session) {
	now := time.Now().Unix()
	if now-s.act >= int64(sessionRefresh.Seconds()) {
		s.act = now
		writeSession(w, r, s)
	}
}
