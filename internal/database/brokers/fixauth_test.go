package brokers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Simulator bots log on with temporary logons: accepted while they exist,
// never saved, never listed, and they do not switch the "auto" check on.
func TestSimulatorLogons(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fix_brokers.json")
	t.Setenv("ME_FIX_AUTH", "on")
	t.Setenv("ME_FIX_AUTH_FILE", path)
	a := LoadFix()
	a.Add("JBL", "Right-Pass1")
	a.AddTemp("SIMBOT1", "Sim-Pass1")
	if r := a.Authorize("SIMBOT1", "SIMBOT1", "Sim-Pass1"); r != "" {
		t.Fatalf("bot logon refused: %s", r)
	}
	if a.Authorize("SIMBOT1", "", "wrong") == "" || a.Authorize("SIMBOT2", "", "Sim-Pass1") == "" {
		t.Fatal("bad bot logons accepted")
	}
	if l := a.List(); len(l) != 1 || l[0] != "JBL" {
		t.Fatalf("listed: %v", l)
	}
	a.Add("KBL", "Right-Pass2") // saves the file again
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "SIMBOT") {
		t.Fatalf("a bot logon was saved: %s", b)
	}
	a.RemoveTemp("SIMBOT1")
	if a.Authorize("SIMBOT1", "", "Sim-Pass1") == "" {
		t.Fatal("a removed bot logon still works")
	}
	t.Setenv("ME_FIX_AUTH", "")
	t.Setenv("ME_FIX_AUTH_FILE", filepath.Join(t.TempDir(), "none.json"))
	b := LoadFix()
	b.AddTemp("SIMBOT1", "x")
	if b.Enforced() {
		t.Fatal("a bot logon switched the auto logon check on")
	}
	if !IsSimulator("simbot4") || !IsSimulator("SIMBENCH1") || IsSimulator("SIMBA") || !IsSimBench("SIMBENCH2") || IsSimBench("SIMBOT1") {
		t.Fatal("IsSimulator / IsSimBench")
	}
}

func TestFixAuthModes(t *testing.T) {
	for _, tc := range []struct {
		env             string
		mode            string
		emptyEnforced   bool
		brokersEnforced bool
	}{
		{"", "auto", false, true},
		{"on", "on", true, true},
		{"off", "off", false, false},
	} {
		t.Setenv("ME_FIX_AUTH", tc.env)
		t.Setenv("ME_FIX_AUTH_FILE", filepath.Join(t.TempDir(), "fix_brokers.json"))
		a := LoadFix()
		if a.Mode != tc.mode {
			t.Fatalf("ME_FIX_AUTH=%q: mode %q", tc.env, a.Mode)
		}
		if got := a.Authorize("ANYBRK", "", "x") == ""; got == tc.emptyEnforced {
			t.Errorf("%s with no brokers: logon allowed=%v", tc.mode, got)
		}
		a.Add("JBL", "Right-Pass1")
		if a.Enforced() != tc.brokersEnforced {
			t.Errorf("%s after adding a broker: enforced=%v", tc.mode, a.Enforced())
		}
		if tc.brokersEnforced {
			if a.Authorize("JBL", "JBL", "wrong") == "" || a.Authorize("ANYBRK", "", "x") == "" {
				t.Errorf("%s: bad logons accepted once a broker exists", tc.mode)
			}
			if r := a.Authorize("JBL", "JBL", "Right-Pass1"); r != "" {
				t.Errorf("%s: good logon refused: %s", tc.mode, r)
			}
		}
	}
}
