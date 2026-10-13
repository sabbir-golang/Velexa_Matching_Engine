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

func ClientIP(r *http.Request) string {
	if h := r.Header.Get("CF-Connecting-IP"); h != "" {
		return h
	}
	if h := r.Header.Get("X-Forwarded-For"); h != "" {
		return strings.TrimSpace(strings.Split(h, ",")[0])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

const (
	SessionCookie = "me_admin"
	SessionTTL    = 8 * time.Hour
)

func SignSession(username string, exp time.Time) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(username + "|" + strconv.FormatInt(exp.Unix(), 10)))
	return payload + "." + sessionMAC(payload)
}

func sessionMAC(payload string) string {
	mac := hmac.New(sha256.New, []byte(config.AdminSecret()))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySession returns the user a session cookie value was issued to, if the
// signature is genuine and the session has not expired.
func VerifySession(value string) (string, bool) {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(sessionMAC(payload))) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", false
	}
	name, exp, ok := strings.Cut(string(raw), "|")
	if !ok {
		return "", false
	}
	unix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > unix {
		return "", false
	}
	return name, true
}

func SetSessionCookie(w http.ResponseWriter, r *http.Request, username string) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: SignSession(username, time.Now().Add(SessionTTL)),
		Path: "/", HttpOnly: true, MaxAge: int(SessionTTL.Seconds()), SameSite: http.SameSiteLaxMode,
		Secure: r.Header.Get("X-Forwarded-Proto") == "https"})
}

// HomeFor is the console address for a role: admins use /admin, everyone else /user.
func HomeFor(u *users.User) string {
	if u != nil && u.Role == users.RoleAdmin {
		return "/admin"
	}
	return "/user"
}

// SessionUser is the signed-in, enabled user behind the request's cookie, or nil.
func SessionUser(store *users.Store, r *http.Request) *users.User {
	if store == nil {
		return nil
	}
	ck, err := r.Cookie(SessionCookie)
	if err != nil {
		return nil
	}
	name, ok := VerifySession(ck.Value)
	if !ok {
		return nil
	}
	if u, ok := store.Get(name); ok {
		return u
	}
	return nil
}
