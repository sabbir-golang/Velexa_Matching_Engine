package rest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/rest/handlers/admin"
	"velexa-sim-me/internal/rest/handlers/register"
	"velexa-sim-me/internal/rest/middleware"

	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/engine"
)

const (
	adminName, adminPW = "admin", "Control-Room1"
	testIP             = "192.0.2.1" // httptest's default client address
)

// viewerRoutes are the console routes every signed-in role may open. Every other
// console route (except login/logout) needs the admin role.
var viewerRoutes = map[string]bool{
	"/admin": true, "/admin/api/status": true, "/admin/api/me": true, "/admin/api/me/password": true,
	"/admin/api/brokers": true, "/admin/api/fixlog": true,
	"/admin/api/broker/detail": true, "/admin/api/broker/add": true, "/admin/api/broker/del": true, "/admin/api/broker/sheet": true,
	"/admin/api/session":      true,                                                                // a user's own brokers only (checked per broker)
	"/admin/api/report/daily": true, "/admin/report/daily": true, "/admin/api/report/orders": true, // a user: own brokers only
}

func isConsoleRoute(p string) bool {
	return strings.HasPrefix(p, "/admin") && p != "/admin/login" && p != "/admin/logout"
}

// roleEnv is a Server with an admin and a read-only user, and a way to call it.
type roleEnv struct {
	t   *testing.T
	srv *Server
	h   http.Handler
}

func newRoleEnv(t *testing.T) *roleEnv {
	t.Helper()
	middleware.ResetLoginThrottle()
	register.ResetQuota()
	audit.Path = filepath.Join(t.TempDir(), "audit.jsonl")
	srv := testServer(t)
	if err := srv.Users.Add("alice", "Rabbit-Hole9", users.RoleUser); err != nil {
		t.Fatal(err)
	}
	return &roleEnv{t: t, srv: srv, h: srv.Routes()}
}

func (e *roleEnv) do(method, path string, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	var rd *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	} else {
		rd = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if method != http.MethodGet {
		withCSRF(req)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

const testCSRF = "test-csrf-token-0123456789"

// withCSRF makes a request look like it came from the console page: same-origin,
// with the CSRF cookie and its header.
func withCSRF(req *http.Request) {
	req.AddCookie(&http.Cookie{Name: middleware.CSRFCookie, Value: testCSRF})
	req.Header.Set(middleware.CSRFHeader, testCSRF)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
}

// form posts an HTML form the way the welcome page does (token in a hidden field).
func (e *roleEnv) form(path string, v url.Values) *httptest.ResponseRecorder {
	v.Set(middleware.CSRFField, testCSRF)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: middleware.CSRFCookie, Value: testCSRF})
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

// sessionFrom is the session cookie a response set, or nil.
func sessionFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if (c.Name == middleware.SessionCookie || c.Name == middleware.SecureSessionCookie) && c.Value != "" && c.MaxAge >= 0 {
			return c
		}
	}
	return nil
}

// login signs in through the real /admin/login form and returns the session cookie.
func (e *roleEnv) login(user, pass string) *http.Cookie {
	e.t.Helper()
	return sessionFrom(e.form("/admin/login", url.Values{"user": {user}, "pass": {pass}}))
}

func TestLoginSetsASignedSession(t *testing.T) {
	e := newRoleEnv(t)
	c := e.login(adminName, adminPW)
	if c == nil {
		t.Fatal("admin login did not set a session cookie")
	}
	if !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteStrictMode || c.MaxAge <= 0 {
		t.Fatalf("cookie attributes: %+v", c)
	}
	var me map[string]any
	_ = json.Unmarshal(e.do(http.MethodGet, "/admin/api/me", c, nil).Body.Bytes(), &me)
	if me["user"] != adminName {
		t.Fatalf("the session belongs to %v", me["user"])
	}
	if e.login(adminName, "wrong") != nil || e.login("nobody", adminPW) != nil {
		t.Fatal("a bad login must not set a session")
	}
}

func TestUserRoleIsReadOnly(t *testing.T) {
	e := newRoleEnv(t)
	c := e.login("alice", "Rabbit-Hole9")
	if c == nil {
		t.Fatal("alice cannot sign in")
	}
	checked := 0
	for _, pattern := range wantRoutes {
		if !isConsoleRoute(pattern) {
			continue
		}
		checked++
		rec := e.do(http.MethodGet, pattern, c, nil)
		code := rec.Code
		roleRefused := code == http.StatusUnauthorized || strings.Contains(rec.Body.String(), "role required")
		if viewerRoutes[pattern] {
			// open to the role; a 403 here can only be the per-broker ownership check
			if roleRefused {
				t.Errorf("user must be able to open %s, got %d %s", pattern, code, rec.Body.String())
			}
		} else if code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "role required") {
			t.Errorf("user must be forbidden from %s, got %d", pattern, code)
		}
	}
	if checked < 25 {
		t.Fatalf("only %d console routes checked", checked)
	}
	// the 403 explains itself
	rec := e.do(http.MethodPost, "/admin/api/allowlist/add", c, map[string]string{"IP": "203.0.113.9"})
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "admin role required") {
		t.Fatalf("forbidden reply: %d %q", rec.Code, rec.Body.String())
	}
}

func TestAdminRoleReachesEverything(t *testing.T) {
	e := newRoleEnv(t)
	c := e.login(adminName, adminPW)
	for _, pattern := range wantRoutes {
		if !isConsoleRoute(pattern) {
			continue
		}
		if code := e.do(http.MethodGet, pattern, c, nil).Code; code == http.StatusUnauthorized || code == http.StatusForbidden {
			t.Errorf("admin must be able to open %s, got %d", pattern, code)
		}
	}
}

func TestTradingSessionControlNeedsAdmin(t *testing.T) {
	e := newRoleEnv(t)
	user, admin := e.login("alice", "Rabbit-Hole9"), e.login(adminName, adminPW)
	e.srv.Eng.SetPhase(engine.PhaseOpen)

	if code := e.do(http.MethodGet, "/admin/api/trading-session", user, nil).Code; code != 403 {
		t.Fatalf("user reading the trading session: %d, want 403", code)
	}
	if code := e.do(http.MethodGet, "/admin/api/trading-session?action=endofday", user, nil).Code; code != 403 {
		t.Fatalf("user ending the day: %d, want 403", code)
	}
	if e.srv.Eng.CurrentPhase() != engine.PhaseOpen {
		t.Fatal("a forbidden request still changed the phase")
	}
	if code := e.do(http.MethodGet, "/admin/api/trading-session?action=preopen", admin, nil).Code; code != http.StatusMethodNotAllowed {
		t.Fatalf("admin changing the phase with GET: %d, want 405", code)
	}
	if e.srv.Eng.CurrentPhase() != engine.PhaseOpen {
		t.Fatal("a GET changed the phase")
	}
	if code := e.do(http.MethodPost, "/admin/api/trading-session?action=preopen", admin, nil).Code; code != 200 {
		t.Fatalf("admin forcing pre-open: %d", code)
	}
	if e.srv.Eng.CurrentPhase() != engine.PhasePreOpen || e.srv.Eng.AutoClock() {
		t.Fatalf("admin request: phase %s, clock %v; want Pre-Open with the clock off", e.srv.Eng.CurrentPhase(), e.srv.Eng.AutoClock())
	}
}

func TestFixLogHidesPasswordsFromTheUserRole(t *testing.T) {
	raw := `{"messages":[{"raw":"8=FIXT.1.1|35=A|49=SAM|553=SAM|554=FIX-secret99|925=newSecret|98=0"}]}`
	got := string(admin.LoginSecretRE.ReplaceAll([]byte(raw), []byte("${1}***")))
	if strings.Contains(got, "FIX-secret99") || strings.Contains(got, "newSecret") {
		t.Fatalf("passwords still visible: %s", got)
	}
	if !strings.Contains(got, "|554=***|") || !strings.Contains(got, "|553=SAM|") {
		t.Fatalf("masking changed the wrong fields: %s", got)
	}
	e := newRoleEnv(t)
	rec := e.do(http.MethodGet, "/admin/api/fixlog?limit=5", e.login("alice", "Rabbit-Hole9"), nil)
	var out map[string]any
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &out) != nil || out["messages"] == nil {
		t.Fatalf("fixlog for the user role: %d %q", rec.Code, rec.Body.String())
	}
}

func TestSessionsCannotBeForgedOrReused(t *testing.T) {
	e := newRoleEnv(t)
	good := e.login("alice", "Rabbit-Hole9")
	if e.do(http.MethodGet, "/admin/api/status", good, nil).Code != 200 {
		t.Fatal("a genuine session was refused")
	}
	_, ver, _ := e.srv.Users.Session("alice")
	now := time.Now()
	// The format older builds used, signed with the key they derived from the admin password.
	oldPayload := base64.RawURLEncoding.EncodeToString([]byte(adminName + "|9999999999"))
	oldMAC := hmac.New(sha256.New, []byte("velexa-me/Velexa@ME2026"))
	oldMAC.Write([]byte(oldPayload))
	bad := map[string]string{
		"tampered payload": "YWRtaW58OTk5OTk5OTk5OQ." + strings.SplitN(good.Value, ".", 2)[1], // "admin|9999999999" with alice's signature
		"no signature":     strings.SplitN(good.Value, ".", 2)[0],
		"garbage":          "not-a-session",
		"old bare HMAC":    strings.Repeat("ab", 32),
		"old format+key":   oldPayload + "." + hex.EncodeToString(oldMAC.Sum(nil)),
		"past 8 hours":     middleware.SignSession("alice", ver, now.Add(-9*time.Hour), now),
		"idle 31 minutes":  middleware.SignSession("alice", ver, now.Add(-time.Hour), now.Add(-31*time.Minute)),
		"stale version":    middleware.SignSession("alice", "old-version-token", now, now),
		"unknown user":     middleware.SignSession("mallory", ver, now, now),
	}
	for name, v := range bad {
		rec := e.do(http.MethodGet, "/admin/api/status", &http.Cookie{Name: middleware.SessionCookie, Value: v}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: got %d, want 401", name, rec.Code)
		}
	}
}

func TestDisablingOrDemotingTakesEffectImmediately(t *testing.T) {
	e := newRoleEnv(t)
	c := e.login("alice", "Rabbit-Hole9")
	// promote: the same cookie now reaches admin routes
	if e.do(http.MethodGet, "/admin/api/users", c, nil).Code != 403 {
		t.Fatal("alice should start as a plain user")
	}
	if err := e.srv.Users.SetRole("alice", users.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if code := e.do(http.MethodGet, "/admin/api/users", c, nil).Code; code != 200 {
		t.Fatalf("after promotion the open session should reach /admin/api/users, got %d", code)
	}
	if err := e.srv.Users.SetRole("alice", users.RoleUser); err != nil {
		t.Fatal(err)
	}
	if code := e.do(http.MethodGet, "/admin/api/users", c, nil).Code; code != 403 {
		t.Fatalf("after demotion: %d, want 403", code)
	}
	// disable: the open session is rejected at once
	if err := e.srv.Users.SetDisabled("alice", true); err != nil {
		t.Fatal(err)
	}
	if code := e.do(http.MethodGet, "/admin/api/status", c, nil).Code; code != 401 {
		t.Fatalf("after disabling: %d, want 401", code)
	}
	if e.login("alice", "Rabbit-Hole9") != nil {
		t.Fatal("a disabled account must not sign in")
	}
}

func TestUsersAPI(t *testing.T) {
	e := newRoleEnv(t)
	admin, alice := e.login(adminName, adminPW), e.login("alice", "Rabbit-Hole9")
	post := func(path string, c *http.Cookie, body any) (int, map[string]any) {
		rec := e.do(http.MethodPost, path, c, body)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	ok := func(code int, out map[string]any) bool { return code == 200 && out["ok"] == true }

	// a plain user cannot manage accounts
	if code, _ := post("/admin/api/user/add", alice, map[string]string{"Username": "evil", "Password": "Quiet-Lake4", "Role": "admin"}); code != 403 {
		t.Fatalf("user adding an admin: %d", code)
	}
	// admin adds, validates, lists (no hashes), updates, resets, deletes
	if c, o := post("/admin/api/user/add", admin, map[string]string{"Username": "bob", "Password": "Quiet-Lake1", "Role": "user"}); !ok(c, o) {
		t.Fatalf("add bob: %v", o)
	}
	for _, bad := range []map[string]string{
		{"Username": "bob", "Password": "Quiet-Lake1", "Role": "user"}, // duplicate
		{"Username": "x", "Password": "Quiet-Lake1", "Role": "user"},   // short name
		{"Username": "carl", "Password": "short", "Role": "user"},      // weak password
		{"Username": "carl", "Password": "Quiet-Lake3", "Role": "root"},
	} {
		if _, o := post("/admin/api/user/add", admin, bad); o["ok"] != false || o["error"] == "" {
			t.Fatalf("add %v should be refused: %v", bad, o)
		}
	}
	rec := e.do(http.MethodGet, "/admin/api/users", admin, nil)
	if strings.Contains(rec.Body.String(), "pbkdf2") || strings.Contains(rec.Body.String(), "hash") {
		t.Fatalf("the user list leaks password hashes: %s", rec.Body.String())
	}
	var list struct{ Users []users.User }
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Users) != 3 { // admin, alice, bob
		t.Fatalf("listed %d users: %+v", len(list.Users), list.Users)
	}
	if c, o := post("/admin/api/user/update", admin, map[string]any{"Username": "bob", "Role": "admin"}); !ok(c, o) {
		t.Fatalf("promote bob: %v", o)
	}
	if c, o := post("/admin/api/user/update", admin, map[string]any{"Username": "bob", "Disabled": true}); !ok(c, o) {
		t.Fatalf("disable bob: %v", o)
	}
	if c, o := post("/admin/api/user/password", admin, map[string]string{"Username": "bob", "Password": "resetByAdmin1"}); !ok(c, o) {
		t.Fatalf("reset password: %v", o)
	}
	_ = e.srv.Users.SetDisabled("bob", false)
	if e.login("bob", "resetByAdmin1") == nil {
		t.Fatal("bob cannot sign in with the reset password")
	}
	if c, o := post("/admin/api/user/delete", admin, map[string]string{"Username": "bob"}); !ok(c, o) {
		t.Fatalf("delete bob: %v", o)
	}
	// the last admin is protected through the API too
	if _, o := post("/admin/api/user/delete", admin, map[string]string{"Username": adminName}); o["ok"] != false {
		t.Fatalf("deleting the last admin must be refused: %v", o)
	}
	if _, o := post("/admin/api/user/update", admin, map[string]any{"Username": adminName, "Role": "user"}); o["ok"] != false {
		t.Fatalf("demoting the last admin must be refused: %v", o)
	}
	// self-service password change, for any role
	if c, o := post("/admin/api/me/password", alice, map[string]string{"Old": "wrong", "New": "Rabbit-Hole10"}); ok(c, o) {
		t.Fatal("changing a password needs the current one")
	}
	changed := e.do(http.MethodPost, "/admin/api/me/password", alice, map[string]string{"Old": "Rabbit-Hole9", "New": "Rabbit-Hole10"})
	if !strings.Contains(changed.Body.String(), `"ok":true`) {
		t.Fatalf("self password change: %s", changed.Body.String())
	}
	if e.do(http.MethodGet, "/admin/api/me", alice, nil).Code != http.StatusUnauthorized {
		t.Fatal("a password change must end the account's other sessions")
	}
	if alice = sessionFrom(changed); alice == nil || e.do(http.MethodGet, "/admin/api/me", alice, nil).Code != 200 {
		t.Fatal("the browser that changed the password must stay signed in")
	}
	if e.login("alice", "Rabbit-Hole9") != nil || e.login("alice", "Rabbit-Hole10") == nil {
		t.Fatal("the old password must stop working and the new one work")
	}
	// who am I
	rec = e.do(http.MethodGet, "/admin/api/me", alice, nil)
	var me map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &me)
	if me["user"] != "alice" || me["role"] != "user" || me["can_write"] != false {
		t.Fatalf("/admin/api/me for a user: %v", me)
	}
}

func TestAuditNamesTheUser(t *testing.T) {
	e := newRoleEnv(t)
	admin := e.login(adminName, adminPW)
	e.do(http.MethodPost, "/admin/api/user/add", admin, map[string]string{"Username": "dora", "Password": "Quiet-Lake2", "Role": "user"})
	got := audit.Recent(1)
	if len(got) != 1 || got[0].Action != "user.add" || !strings.HasPrefix(got[0].Actor, adminName+" (") {
		t.Fatalf("audit entry should name the acting user: %+v", got)
	}
}

func TestWelcomePageAndRoleChosenLogin(t *testing.T) {
	e := newRoleEnv(t)
	body := func(path string) string { return e.do(http.MethodGet, path, nil, nil).Body.String() }
	if w := body("/"); !strings.Contains(w, "User login") || !strings.Contains(w, "Admin login") {
		t.Fatal("/ should be the welcome board with both sign-in options")
	}
	if b := body("/?embed=1"); strings.Contains(b, "Admin login") || !strings.Contains(b, "Market Watch") {
		t.Fatal("/?embed=1 must still be the Trade Terminal")
	}
	if b := body("/terminal"); !strings.Contains(b, "Market Watch") {
		t.Fatal("/terminal must be the Trade Terminal")
	}
	post := func(user, pass, as string) *httptest.ResponseRecorder {
		return e.form("/admin/login", url.Values{"user": {user}, "pass": {pass}, "as": {as}})
	}
	if r := post("alice", "Rabbit-Hole9", "admin"); r.Header().Get("Location") != "/?e=role&as=admin" || sessionFrom(r) != nil {
		t.Fatalf("a user account must not get in through Admin login: %v", r.Header())
	}
	if r := post(adminName, adminPW, "user"); r.Header().Get("Location") != "/?e=admin&as=user" || sessionFrom(r) != nil {
		t.Fatalf("an admin account must not get in through User login: %v", r.Header())
	}
	if r := post("alice", "Rabbit-Hole9", "user"); r.Header().Get("Location") != "/user" {
		t.Fatalf("user login: %v", r.Header())
	}
	if r := post(adminName, adminPW, "admin"); r.Header().Get("Location") != "/admin" {
		t.Fatalf("admin login: %v", r.Header())
	}
	if r := post("alice", "nope", "user"); !strings.HasPrefix(r.Header().Get("Location"), "/?e=1&as=user") {
		t.Fatalf("bad password should return to the welcome page: %v", r.Header())
	}
}

func TestSelfRegistrationMakesReadOnlyUsers(t *testing.T) {
	e := newRoleEnv(t)
	register.ResetQuota()
	reg := func(user, pass, pass2 string, more ...map[string]string) *httptest.ResponseRecorder {
		extra := map[string]string{}
		for _, m := range more {
			for k, v := range m {
				extra[k] = v
			}
		}
		form := url.Values{"user": {user}, "pass": {pass}, "pass2": {pass2}, "full_name": {"New Person"},
			"email": {user + "@example.com"}, "phone": {"+880 1711-000000"}, "gender": {"female"}, "country": {"Bangladesh"}}
		for k, v := range extra {
			form[k] = []string{v}
		}
		register.ResetQuota() // every attempt counts against the IP; this test makes many
		return e.form("/register", form)
	}
	rec := reg("newbie", "Fresh-Start1", "Fresh-Start1")
	if rec.Header().Get("Location") != "/?e=registered&as=user" || sessionFrom(rec) != nil {
		t.Fatalf("registering should go to the login form without signing in: %v", rec.Header())
	}
	if !strings.Contains(e.do(http.MethodGet, "/?e=registered&as=user", nil, nil).Body.String(), "Account created") {
		t.Fatal("the login form should confirm the account was created")
	}
	c := e.login("newbie", "Fresh-Start1")
	if c == nil {
		t.Fatal("the new account cannot sign in")
	}
	if me, _ := e.srv.Users.Get("newbie"); me == nil || me.Role != users.RoleUser {
		t.Fatalf("self-registered accounts must be role user: %+v", me)
	}
	if e.do(http.MethodGet, "/admin/api/users", c, nil).Code != 403 {
		t.Fatal("a registered user must not reach admin routes")
	}
	for _, c := range []struct{ name, got string }{
		{"reg_taken", reg("newbie", "Fresh-Start1", "Fresh-Start1").Header().Get("Location")},
		{"reg_taken", reg("pers6", "Fresh-Start1", "Fresh-Start1", map[string]string{"email": "newbie@example.com"}).Header().Get("Location")},
		{"reg_taken", reg("admin", "Fresh-Start1", "Fresh-Start1", map[string]string{"email": "adm@example.com"}).Header().Get("Location")},
		{"reg_mismatch", reg("other1", "Fresh-Start1", "Different-12").Header().Get("Location")},
		{"reg_weak", reg("other2", "short", "short").Header().Get("Location")},
		{"reg_weak", reg("other3", "other3-Pass1", "other3-Pass1").Header().Get("Location")}, // contains the username
		{"reg_name", reg("a b", "Fresh-Start1", "Fresh-Start1").Header().Get("Location")},
		{"reg_email", reg("pers1", "Fresh-Start1", "Fresh-Start1", map[string]string{"email": "not-an-email"}).Header().Get("Location")},
		{"reg_phone", reg("pers2", "Fresh-Start1", "Fresh-Start1", map[string]string{"phone": "abc"}).Header().Get("Location")},
		{"reg_gender", reg("pers3", "Fresh-Start1", "Fresh-Start1", map[string]string{"gender": ""}).Header().Get("Location")},
		{"reg_country", reg("pers4", "Fresh-Start1", "Fresh-Start1", map[string]string{"country": ""}).Header().Get("Location")},
		{"reg_fullname", reg("pers5", "Fresh-Start1", "Fresh-Start1", map[string]string{"full_name": ""}).Header().Get("Location")},
	} {
		if !strings.Contains(c.got, "e="+c.name+"&") {
			t.Errorf("%s: redirected to %q", c.name, c.got)
		}
	}
	t.Setenv("ME_REGISTRATION", "off")
	if loc := reg("late1", "Fresh-Start1", "Fresh-Start1").Header().Get("Location"); !strings.Contains(loc, "reg_off") {
		t.Fatalf("registration off: %q", loc)
	}
	if _, ok := e.srv.Users.Get("late1"); ok {
		t.Fatal("no account may be created while registration is off")
	}
}

func TestConsoleAddressDependsOnRole(t *testing.T) {
	e := newRoleEnv(t)
	admin, user := e.login(adminName, adminPW), e.login("alice", "Rabbit-Hole9")
	loc := func(path string, c *http.Cookie) string {
		rec := e.do(http.MethodGet, path, c, nil)
		if rec.Code == http.StatusOK {
			return "200"
		}
		return rec.Header().Get("Location")
	}
	for _, tc := range []struct {
		path string
		c    *http.Cookie
		want string
	}{
		{"/admin", admin, "200"}, {"/user", admin, "/admin"},
		{"/user", user, "200"}, {"/admin", user, "/user"},
		{"/admin", nil, "/"}, {"/user", nil, "/"},
		{"/", admin, "/admin"}, {"/", user, "/user"},
	} {
		if got := loc(tc.path, tc.c); got != tc.want {
			t.Errorf("GET %s: %s, want %s", tc.path, got, tc.want)
		}
	}
}

// Every user has their own workspace: brokers they register are theirs alone;
// other users can neither see nor touch them; admins see everything.
func TestUsersOnlySeeTheirOwnBrokers(t *testing.T) {
	e := newRoleEnv(t)
	if err := e.srv.Users.Add("bobby", "Blue-Sky-77", users.RoleUser); err != nil {
		t.Fatal(err)
	}
	alice, bobby, admin := e.login("alice", "Rabbit-Hole9"), e.login("bobby", "Blue-Sky-77"), e.login(adminName, adminPW)
	post := func(c *http.Cookie, path string, body any) map[string]any {
		var out map[string]any
		json.Unmarshal(e.do(http.MethodPost, path, c, body).Body.Bytes(), &out)
		return out
	}
	list := func(c *http.Cookie) []string {
		var out struct {
			Brokers []struct{ Sender, Owner string }
		}
		json.Unmarshal(e.do(http.MethodGet, "/admin/api/brokers", c, nil).Body.Bytes(), &out)
		var s []string
		for _, b := range out.Brokers {
			s = append(s, b.Sender+"/"+b.Owner)
		}
		return s
	}
	has := func(l []string, want string) bool {
		for _, x := range l {
			if x == want {
				return true
			}
		}
		return false
	}

	// a fresh account starts with nothing
	if l := list(bobby); len(l) != 0 {
		t.Fatalf("a new user must start with no brokers, got %v", l)
	}
	if out := post(alice, "/admin/api/broker/add", map[string]string{"Sender": "ALICEBRK", "Company": "Alice Securities"}); out["ok"] != true {
		t.Fatalf("alice registering a broker: %v", out)
	}
	if out := post(admin, "/admin/api/broker/add", map[string]string{"Sender": "EXCHBRK", "ItchUser": "EXCHB1"}); out["ok"] != true {
		t.Fatalf("admin registering a broker: %v", out)
	}
	if l := list(alice); len(l) != 1 || !has(l, "ALICEBRK/alice") {
		t.Fatalf("alice sees %v, want only her own broker", l)
	}
	if l := list(bobby); len(l) != 0 {
		t.Fatalf("bobby must not see other users' or the exchange's brokers: %v", l)
	}
	if l := list(admin); !has(l, "ALICEBRK/alice") || !has(l, "EXCHBRK/") {
		t.Fatalf("admin sees %v, want every broker with its owner", l)
	}

	// bobby cannot read, print, revoke or control alice's broker, nor take its CompID
	for _, rq := range []struct{ method, path string }{
		{http.MethodGet, "/admin/api/broker/detail?sender=ALICEBRK"},
		{http.MethodPost, "/admin/api/session?broker=ALICEBRK&action=disconnect"},
	} {
		if code := e.do(rq.method, rq.path, bobby, nil).Code; code != http.StatusForbidden {
			t.Errorf("bobby %s %s: %d, want 403", rq.method, rq.path, code)
		}
	}
	if code := e.do(http.MethodPost, "/admin/api/broker/del", bobby, map[string]string{"Sender": "ALICEBRK"}).Code; code != http.StatusForbidden {
		t.Errorf("bobby revoking alice's broker: %d, want 403", code)
	}
	if out := post(bobby, "/admin/api/broker/add", map[string]string{"Sender": "ALICEBRK"}); out["ok"] != false {
		t.Fatalf("taking an existing SenderCompID must be refused: %v", out)
	}
	if out := post(bobby, "/admin/api/broker/add", map[string]string{"Sender": "BOBBRK", "ItchUser": "EXCHB1"}); out["ok"] != false {
		t.Fatalf("taking an existing ITCH username must be refused: %v", out)
	}
	// alice can see her credentials; admin can see anyone's
	if code := e.do(http.MethodGet, "/admin/api/broker/detail?sender=ALICEBRK", alice, nil).Code; code != 200 {
		t.Errorf("alice reading her own broker: %d", code)
	}
	if code := e.do(http.MethodGet, "/admin/api/broker/detail?sender=ALICEBRK", admin, nil).Code; code != 200 {
		t.Errorf("admin reading alice's broker: %d", code)
	}
	if code := e.do(http.MethodGet, "/admin/api/broker/detail?sender=EXCHBRK", alice, nil).Code; code != http.StatusForbidden {
		t.Errorf("alice reading an exchange broker: %d, want 403", code)
	}

	// the dashboard is scoped too
	var st map[string]any
	json.Unmarshal(e.do(http.MethodGet, "/admin/api/status", bobby, nil).Body.Bytes(), &st)
	if st["scope"] != "own" || st["my_brokers"] != float64(0) || st["orders"] != float64(0) {
		t.Fatalf("bobby's dashboard: scope %v brokers %v orders %v", st["scope"], st["my_brokers"], st["orders"])
	}
	for _, a := range st["audit"].([]any) {
		if !strings.HasPrefix(a.(map[string]any)["actor"].(string), "bobby (") {
			t.Fatalf("bobby sees someone else's audit entry: %v", a)
		}
	}

	// deleting alice gives her brokers back to the exchange: a new "alice" starts empty
	if out := post(admin, "/admin/api/user/delete", map[string]string{"Username": "alice"}); out["ok"] != true {
		t.Fatalf("delete alice: %v", out)
	}
	if err := e.srv.Users.Add("alice", "Rabbit-Hole11", users.RoleUser); err != nil {
		t.Fatal(err)
	}
	if l := list(e.login("alice", "Rabbit-Hole11")); len(l) != 0 {
		t.Fatalf("a new account reusing the name inherited brokers: %v", l)
	}
	post(admin, "/admin/api/broker/del", map[string]string{"Sender": "ALICEBRK"})
	post(admin, "/admin/api/broker/del", map[string]string{"Sender": "EXCHBRK"})
}
