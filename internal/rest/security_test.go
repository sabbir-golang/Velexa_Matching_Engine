package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
)

func TestMain(m *testing.M) {
	users.HashIterations = 1000 // the real work factor is tested in the users package
	os.Exit(m.Run())
}

// loginFrom posts the sign-in form from a given peer, optionally with an
// X-Forwarded-For header, and returns where it was redirected.
func (e *roleEnv) loginFrom(peer, xff, user, pass string) string {
	v := url.Values{"user": {user}, "pass": {pass}, middleware.CSRFField: {testCSRF}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: middleware.CSRFCookie, Value: testCSRF})
	req.RemoteAddr = peer + ":40000"
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec.Header().Get("Location")
}

func TestSpoofedForwardedForDoesNotBypassTheLockout(t *testing.T) {
	t.Setenv("ME_RATE_LIMIT", "on")
	e := newRoleEnv(t)
	for i := 0; i < 5; i++ { // a new fake address every time, from an untrusted peer
		e.loginFrom("198.51.100.7", "10.9.9."+string(rune('1'+i)), "nobody1", "Wrong-Pass1")
	}
	if loc := e.loginFrom("198.51.100.7", "10.1.1.1", "alice", "Rabbit-Hole9"); loc != "/?e=lock" {
		t.Fatalf("the IP must be locked whatever X-Forwarded-For says: %q", loc)
	}
	// A trusted proxy (loopback by default) is believed, right-most untrusted hop first.
	middleware.ResetLoginThrottle()
	if got := middleware.ClientIP(&http.Request{RemoteAddr: "127.0.0.1:1", Header: http.Header{
		"X-Forwarded-For": {"6.6.6.6, 203.0.113.50"}}}); got != "203.0.113.50" {
		t.Fatalf("client behind a trusted proxy = %q", got)
	}
	if got := middleware.ClientIP(&http.Request{RemoteAddr: "198.51.100.7:1", Header: http.Header{
		"X-Forwarded-For": {"203.0.113.50"}, "Cf-Connecting-Ip": {"203.0.113.51"}}}); got != "198.51.100.7" {
		t.Fatalf("forwarding headers from an untrusted peer were believed: %q", got)
	}
}

func TestUsernameLockoutAcrossAddresses(t *testing.T) {
	t.Setenv("ME_RATE_LIMIT", "on")
	e := newRoleEnv(t)
	for i := 0; i < 5; i++ {
		e.loginFrom("198.51.100."+string(rune('1'+i)), "", "alice", "Wrong-Pass1")
	}
	if loc := e.loginFrom("198.51.100.99", "", "alice", "Rabbit-Hole9"); loc != "/?e=lock" {
		t.Fatalf("the account must be locked from every address: %q", loc)
	}
	if loc := e.loginFrom("198.51.100.99", "", adminName, adminPW); loc != "/admin" {
		t.Fatalf("another account from that address must still sign in: %q", loc)
	}
}

func TestRateLimitIsOffByDefault(t *testing.T) {
	e := newRoleEnv(t)
	for i := 0; i < 8; i++ {
		e.loginFrom("198.51.100.30", "", "alice", "Wrong-Pass1")
	}
	if loc := e.loginFrom("198.51.100.30", "", "alice", "Rabbit-Hole9"); loc != "/user" {
		t.Fatalf("without ME_RATE_LIMIT=on nothing is locked: %q", loc)
	}
}

func TestFailedLoginLooksTheSameForUnknownUsers(t *testing.T) {
	e := newRoleEnv(t)
	known := e.loginFrom("198.51.100.20", "", "alice", "Wrong-Pass1")
	unknown := e.loginFrom("198.51.100.21", "", "nobody-here", "Wrong-Pass1")
	if known != unknown || known != "/?e=1" {
		t.Fatalf("known %q vs unknown %q", known, unknown)
	}
}

func TestLogoutEndsTheSession(t *testing.T) {
	e := newRoleEnv(t)
	c, other := e.login("alice", "Rabbit-Hole9"), e.login("alice", "Rabbit-Hole9")
	if e.do(http.MethodGet, "/admin/logout", c, nil); e.do(http.MethodGet, "/admin/api/me", c, nil).Code != 200 {
		t.Fatal("a GET (a link on another site) must not sign anyone out")
	}
	rec := e.do(http.MethodPost, "/admin/logout", c, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout: %d", rec.Code)
	}
	for name, ck := range map[string]*http.Cookie{"this browser": c, "another browser": other} {
		if code := e.do(http.MethodGet, "/admin/api/me", ck, nil).Code; code != http.StatusUnauthorized {
			t.Errorf("after sign-out the old cookie (%s) still works: %d", name, code)
		}
	}
	if e.login("alice", "Rabbit-Hole9") == nil {
		t.Fatal("signing in again must work")
	}
}

func TestDeletedAccountsSessionEnds(t *testing.T) {
	e := newRoleEnv(t)
	c := e.login("alice", "Rabbit-Hole9")
	_ = e.srv.Users.Delete("alice")
	_ = e.srv.Users.Add("alice", "Other-Pass12", users.RoleUser) // same name, new person
	if code := e.do(http.MethodGet, "/admin/api/me", c, nil).Code; code != http.StatusUnauthorized {
		t.Fatalf("the deleted account's cookie works for the new account: %d", code)
	}
}

func TestCSRFIsEnforced(t *testing.T) {
	e := newRoleEnv(t)
	adm := e.login(adminName, adminPW)
	body := `{"IP":"203.0.113.9"}`
	send := func(mod func(*http.Request)) int {
		req := httptest.NewRequest(http.MethodPost, "/admin/api/allowlist/add", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(adm)
		withCSRF(req)
		mod(req)
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec.Code
	}
	for name, mod := range map[string]func(*http.Request){
		"no header":    func(r *http.Request) { r.Header.Del(middleware.CSRFHeader) },
		"wrong header": func(r *http.Request) { r.Header.Set(middleware.CSRFHeader, "x"+testCSRF) },
		"cross-site":   func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"foreign Origin": func(r *http.Request) {
			r.Header.Del("Sec-Fetch-Site")
			r.Header.Set("Origin", "https://evil.example")
		},
	} {
		if code := send(mod); code != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", name, code)
		}
	}
	if code := send(func(*http.Request) {}); code != 200 {
		t.Fatalf("a genuine console request was refused: %d", code)
	}
	// a form without the token goes back to the welcome page, unsigned-in
	v := url.Values{"user": {adminName}, "pass": {adminPW}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Header().Get("Location") != "/?e=csrf" || sessionFrom(rec) != nil {
		t.Fatalf("login without a CSRF token: %v", rec.Header())
	}
	// a first visit hands out the token cookie
	rec = e.do(http.MethodGet, "/", nil, nil)
	found := false
	for _, c := range rec.Result().Cookies() {
		found = found || (c.Name == middleware.CSRFCookie && len(c.Value) >= 20)
	}
	if !found {
		t.Fatal("the welcome page did not set the CSRF cookie")
	}
}

func TestStateChangesNeedPost(t *testing.T) {
	e := newRoleEnv(t)
	adm := e.login(adminName, adminPW)
	for _, p := range []string{"/admin/api/user/add", "/admin/api/instrument/add", "/admin/api/broker/add",
		"/admin/api/allowlist/del", "/admin/api/session?broker=X&action=disconnect", "/admin/api/instruments/reload"} {
		if code := e.do(http.MethodGet, p, adm, nil).Code; code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s = %d, want 405", p, code)
		}
	}
}

func TestOrderEntryAndPhaseControlNeedSignIn(t *testing.T) {
	e := newRoleEnv(t)
	order := map[string]any{"symbol": "GP", "side": "BUY", "type": "LIMIT", "price": 331.2, "qty": 10, "actor": "FIX:JBL"}
	if code := e.do(http.MethodPost, "/api/order", nil, order).Code; code != http.StatusUnauthorized {
		t.Fatalf("anonymous order: %d, want 401", code)
	}
	alice, adm := e.login("alice", "Rabbit-Hole9"), e.login(adminName, adminPW)
	e.srv.Eng.SetPhase(engine.PhaseOpen)
	rec := e.do(http.MethodPost, "/api/order", alice, order)
	if rec.Code != 200 {
		t.Fatalf("signed-in order: %d %s", rec.Code, rec.Body.String())
	}
	if st := e.srv.Eng.StatsFor([]string{"USER:alice"}); st.Orders != 1 {
		t.Fatalf("the order must be placed as USER:alice (the client's actor field is ignored): %+v", st)
	}
	for _, p := range []string{"/api/session/open", "/api/session/preopen"} {
		if code := e.do(http.MethodPost, p, nil, nil).Code; code != http.StatusUnauthorized {
			t.Errorf("anonymous POST %s = %d, want 401", p, code)
		}
		if code := e.do(http.MethodPost, p, alice, nil).Code; code != http.StatusForbidden {
			t.Errorf("user POST %s = %d, want 403", p, code)
		}
		if code := e.do(http.MethodGet, p, adm, nil).Code; code != http.StatusMethodNotAllowed {
			t.Errorf("admin GET %s = %d, want 405", p, code)
		}
	}
	if code := e.do(http.MethodPost, "/api/session/preopen", adm, nil).Code; code != 200 || e.srv.Eng.CurrentPhase() != engine.PhasePreOpen {
		t.Fatalf("admin pre-open: %d, phase %s", code, e.srv.Eng.CurrentPhase())
	}
	var st map[string]any
	if json.Unmarshal(e.do(http.MethodGet, "/api/session", nil, nil).Body.Bytes(), &st); st["phase"] == nil {
		t.Fatal("the public session status must stay readable")
	}
}

func TestSecurityHeaders(t *testing.T) {
	e := newRoleEnv(t)
	for _, p := range []string{"/", "/terminal", "/api/status", "/admin"} {
		h := e.do(http.MethodGet, p, nil, nil).Header()
		for k, want := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "SAMEORIGIN",
			"Referrer-Policy": "same-origin"} {
			if h.Get(k) != want {
				t.Errorf("%s: %s = %q", p, k, h.Get(k))
			}
		}
		if !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'self'") {
			t.Errorf("%s: CSP %q", p, h.Get("Content-Security-Policy"))
		}
		if h.Get("Strict-Transport-Security") != "" {
			t.Errorf("%s: HSTS on plain HTTP", p)
		}
	}
	if e.do(http.MethodGet, "/admin", nil, nil).Header().Get("Cache-Control") != "no-store" {
		t.Error("console pages must not be cached")
	}
	t.Setenv("ME_COOKIE_SECURE", "on")
	rec := e.form("/admin/login", url.Values{"user": {adminName}, "pass": {adminPW}})
	c := sessionFrom(rec)
	if c == nil || c.Name != middleware.SecureSessionCookie || !c.Secure || rec.Header().Get("Strict-Transport-Security") == "" {
		t.Fatalf("HTTPS mode: cookie %+v, HSTS %q", c, rec.Header().Get("Strict-Transport-Security"))
	}
}
