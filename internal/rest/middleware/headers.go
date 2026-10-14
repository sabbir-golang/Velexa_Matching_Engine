package middleware

import (
	"net/http"
	"strings"
)

// contentSecurityPolicy limits where pages may load code and data from. The
// console pages use inline scripts, styles and handlers, so 'unsafe-inline' is
// allowed; everything else is restricted to this origin (plus Google Fonts).
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline'; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src 'self' https://fonts.gstatic.com data:; " +
	"img-src 'self' data:; " +
	"connect-src 'self' ws: wss:; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'self'"

// SecurityHeaders is the global middleware that adds the standard browser
// protections to every response, and keeps signed-in pages out of caches.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		if IsHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if p := r.URL.Path; p == "/" || p == "/user" || p == "/admin" || strings.HasPrefix(p, "/admin/") || p == "/register" {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
