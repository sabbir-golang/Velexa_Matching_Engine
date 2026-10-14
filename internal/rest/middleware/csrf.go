package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
)

// CSRF protection for every state-changing request (double-submit token plus an
// origin check). Each browser gets a random token in the readable me_csrf
// cookie; the console sends it back in the X-CSRF-Token header (fetch) or a
// "csrf" field (HTML forms). A page on another site can neither read the cookie
// nor set the header, so it cannot forge a request.

const (
	// CSRFCookie holds this browser's token. It is readable by the page's own
	// scripts on purpose; it grants nothing by itself.
	CSRFCookie = "me_csrf"
	// CSRFHeader and CSRFField carry the token back.
	CSRFHeader = "X-CSRF-Token"
	CSRFField  = "csrf"
)

// CSRF is the global middleware: it hands out the token cookie and refuses
// POST/PUT/PATCH/DELETE requests that come from another site or lack the token.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if ck, err := r.Cookie(CSRFCookie); err == nil && len(ck.Value) >= 20 {
			token = ck.Value
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			if token == "" && !isUpgrade(r) {
				http.SetCookie(w, &http.Cookie{Name: CSRFCookie, Value: newCSRFToken(), Path: "/",
					SameSite: http.SameSiteLaxMode, Secure: IsHTTPS(r)})
			}
			next.ServeHTTP(w, r)
			return
		}
		if !sameOrigin(r) || token == "" || !tokenMatches(token, sentToken(w, r)) {
			csrfRefused(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sentToken is the token the request carries: the header, else the form field.
func sentToken(w http.ResponseWriter, r *http.Request) string {
	if h := r.Header.Get(CSRFHeader); h != "" {
		return h
	}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") || strings.HasPrefix(ct, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		return r.FormValue(CSRFField)
	}
	return ""
}

func tokenMatches(want, got string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// sameOrigin rejects requests a browser marks as coming from another site.
// Sec-Fetch-Site is sent by every current browser; Origin covers older ones.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
	default: // same-site, cross-site
		return false
	}
	o := r.Header.Get("Origin")
	if o == "" || o == "null" {
		return o == "" // no Origin: not a browser cross-site request
	}
	u, err := url.Parse(o)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func isUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func newCSRFToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic("csrf: no randomness: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// csrfRefused answers a forged or stale request: JSON for API calls, the
// welcome page with a notice for form posts.
func csrfRefused(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/admin/api/") || strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"ok":false,"error":"request refused (CSRF check failed) — reload the page and try again"}` + "\n"))
		return
	}
	http.Redirect(w, r, "/?e=csrf", http.StatusSeeOther)
}
