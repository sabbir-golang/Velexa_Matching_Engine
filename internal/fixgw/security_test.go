package fixgw

import (
	"strings"
	"testing"
)

func TestLogonPasswordsAreNeverStored(t *testing.T) {
	s := &msgLogStore{max: 10}
	s.add("JBL", "IN", []byte("8=FIXT.1.1\x0135=A\x0149=JBL\x01553=JBL\x01554=Secret-99\x01925=NewSecret\x0198=0\x01"))
	got := s.Recent("", "", 10)
	if len(got) != 1 {
		t.Fatalf("logged %d messages", len(got))
	}
	raw := got[0].Raw
	if strings.Contains(raw, "Secret-99") || strings.Contains(raw, "NewSecret") {
		t.Fatalf("password stored: %s", raw)
	}
	if !strings.Contains(raw, "|554=***|") || !strings.Contains(raw, "|925=***|") || !strings.Contains(raw, "|553=JBL|") {
		t.Fatalf("masking changed the wrong fields: %s", raw)
	}
}

func TestLogonCompIDMustBePlain(t *testing.T) {
	for id, ok := range map[string]bool{"JBL": true, "gbl": true, "ALPHA.EQ-1_X": true, "": false,
		"<img src=x onerror=alert(1)>": false, "A B": false, "JBL'": false, strings.Repeat("A", 21): false} {
		if validCompID(id) != ok {
			t.Errorf("validCompID(%q) = %v", id, !ok)
		}
	}
}
