package rest

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"velexa-sim-me/internal/rest/deps"
	"velexa-sim-me/internal/rest/middleware"

	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/itchgw"
)

// wantRoutes is every URL the engine served before the handlers were reorganised
// into internal/rest. A route that is dropped or renamed fails here.
var wantRoutes = []string{
	"/",
	"/terminal",
	"/register",
	"/user",
	"/admin",
	"/admin/api/allowlist",
	"/admin/api/allowlist/add",
	"/admin/api/allowlist/del",
	"/admin/api/broker/add",
	"/admin/api/broker/del",
	"/admin/api/broker/detail",
	"/admin/api/brokers",
	"/admin/api/broker/sheet",
	"/admin/api/fixlog",
	"/admin/api/instrument/add",
	"/admin/api/instrument/delist",
	"/admin/api/instrument/halt",
	"/admin/api/instruments",
	"/admin/api/instruments/export",
	"/admin/api/instruments/import",
	"/admin/api/instruments/reload",
	"/admin/api/instrument/update",
	"/admin/api/session",
	"/admin/api/sim",
	"/admin/api/status",
	"/admin/api/me",
	"/admin/api/me/password",
	"/admin/api/users",
	"/admin/api/user/add",
	"/admin/api/user/update",
	"/admin/api/user/password",
	"/admin/api/user/delete",
	"/admin/login",
	"/admin/logout",
	"/api/bench/orders.csv",
	"/api/bench/start",
	"/api/bench/stats",
	"/api/bench/stop",
	"/api/bench/summary.csv",
	"/api/book",
	"/api/bots",
	"/api/bots/pause",
	"/api/bots/resume",
	"/api/live/cleanup",
	"/api/live/latency",
	"/api/live/start",
	"/api/live/stats",
	"/api/live/stop",
	"/api/loadtest",
	"/api/market",
	"/api/order",
	"/api/session",
	"/api/session/open",
	"/api/session/preopen",
	"/api/status",
	"/bench",
	"/events",
	"/health",
	"/healthz",
	"/index",
	"/market",
	"/marketdepth",
}

func testServer(t *testing.T) *Server {
	t.Helper()
	eng := engine.NewEngine([]engine.Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 331.20}}, engine.NewBus())
	go eng.Run()
	return NewServer(&deps.Deps{Eng: eng, Bus: engine.NewBus(), Itch: itchgw.NewGateway(eng),
		Users: users.Load(filepath.Join(t.TempDir(), "users.json"), "admin", "AdminPass123")})
}

// Registering a route twice panics, so building the mux also proves there are no
// duplicates; each expected URL must resolve to a pattern of its own.
func TestRouteTableIsComplete(t *testing.T) {
	mux := testServer(t).newMux(middleware.NewManager())
	for _, p := range wantRoutes {
		if _, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, p, nil)); pattern != p {
			t.Errorf("%s is not registered (resolves to %q)", p, pattern)
		}
	}
	for _, p := range []string{"/admin/login/x", "/nope", "/api/nothing"} {
		if _, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, p, nil)); pattern == p {
			t.Errorf("%s should not be a route", p)
		}
	}
}

// Every /admin/api route must refuse an unauthenticated request; only the login
// and logout pages (and the public routes) are open.
func TestAdminRoutesRequireLogin(t *testing.T) {
	h := testServer(t).Routes()
	for _, p := range wantRoutes {
		if !strings.HasPrefix(p, "/admin/api/") {
			continue
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without a session = %d, want 401", p, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if rec.Code != http.StatusSeeOther {
		t.Errorf("GET /admin without a session = %d, want a redirect to the login page", rec.Code)
	}
}

func TestPublicRoutesRespond(t *testing.T) {
	h := testServer(t).Routes()
	for path, want := range map[string]int{"/healthz": 200, "/health": 200, "/api/status": 200, "/api/market": 200,
		"/api/book?symbol=GP": 200, "/api/bots": 200, "/nope": 404} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/order", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/order = %d, want 405", rec.Code)
	}
}
