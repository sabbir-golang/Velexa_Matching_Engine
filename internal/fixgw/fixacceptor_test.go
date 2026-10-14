package fixgw

import (
	"path/filepath"
	"strconv"
	"testing"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/engine"
)

// newTestEngine boots an engine on a tiny universe with its matching goroutine
// running, so control ops (listing) and order submission both work.
func newTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	e := engine.NewEngine([]engine.Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 331.20}}, engine.NewBus())
	go e.Run()
	return e
}

func TestFixValidateNewReasons(t *testing.T) {
	t.Setenv("ME_AUTO_LIST", "off")
	fa := &Acceptor{eng: newTestEngine(t)}
	cases := []struct {
		name   string
		sym    string
		qty    float64
		typ    engine.OrdType
		px     float64
		stopPx float64
	}{
		{"no symbol", "", 10, engine.Limit, 50, 0},
		{"bad symbol", "not a ticker", 10, engine.Limit, 50, 0},
		{"zero qty", "GP", 0, engine.Limit, 50, 0},
		{"fractional qty", "GP", 1.5, engine.Limit, 50, 0},
		{"stop with no trigger", "GP", 10, engine.StopMkt, 0, 0},
		{"unlisted", "UNITEDINS", 10, engine.Limit, 50, 0},
	}
	for _, c := range cases {
		if got := fa.validateNew(c.sym, c.qty, c.typ, c.px, c.stopPx); got == "" {
			t.Errorf("%s: accepted, want a reject reason", c.name)
		}
	}
	if got := fa.validateNew("GP", 100, engine.Limit, 331.20, 0); got != "" {
		t.Errorf("valid order rejected: %s", got)
	}
}

// On a machine without /opt/velexa-sim-me (a developer laptop) the FIX acceptor
// must still start — it falls back to the dictionaries shipped in ./spec — and its
// data files must default to somewhere writable.
func TestFixAcceptorStartsWithDefaultPaths(t *testing.T) {
	if config.OnServer() {
		t.Skip("running on a server with /opt/velexa-sim-me")
	}
	t.Chdir(repoRoot(t)) // the defaults ("spec", "data/…") are relative to where the engine is started
	t.Setenv("ME_FIX_SPEC_DIR", "")
	t.Setenv("ME_FIX_MSGLOG_FILE", filepath.Join(t.TempDir(), "m.log"))
	if config.SpecDir() != "spec" {
		t.Fatalf("specDir = %q", config.SpecDir())
	}
	if got := config.StateFile("fix_brokers.json"); got != "data/fix_brokers.json" {
		t.Fatalf("stateFile = %q", got)
	}
	eng := newTestEngine(t)
	fa, err := Start(eng, strconv.Itoa(freePort(t)), "DSEME")
	if err != nil {
		t.Fatalf("acceptor failed to start with default paths: %v", err)
	}
	_ = fa // not stopped: quickfix's Acceptor.Stop races on a session that never logged on
}

// A user's message log only holds their own brokers' sessions.
func TestMsgLogRecentWhere(t *testing.T) {
	s := &msgLogStore{max: 100}
	s.add("ALICE", "IN", []byte("8=FIXT.1.1\x0135=D\x0111=A1\x01"))
	s.add("BOB", "IN", []byte("8=FIXT.1.1\x0135=D\x0111=B1\x01"))
	s.add("ALICE", "OUT", []byte("8=FIXT.1.1\x0135=8\x0111=A1\x01"))
	got := s.RecentWhere(func(sess string) bool { return sess == "ALICE" }, "", 10)
	if len(got) != 2 {
		t.Fatalf("got %d messages, want alice's 2", len(got))
	}
	for _, m := range got {
		if m.Session != "ALICE" {
			t.Fatalf("someone else's message leaked: %+v", m)
		}
	}
}
