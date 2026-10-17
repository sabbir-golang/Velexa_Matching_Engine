package brokers

import (
	"path/filepath"
	"testing"
)

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
