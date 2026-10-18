package fixgw

import (
	"fmt"
	"os"
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

func TestMessageLogRotatesAndReadsOnlyItsTail(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/fix.log"
	s := &msgLogStore{max: 10, path: path, maxBytes: 600, keep: 2}
	s.mu.Lock()
	s.openLocked()
	s.mu.Unlock()
	for i := 0; i < 40; i++ {
		s.add("JBL", "IN", []byte(fmt.Sprintf("8=FIXT.1.1\x0135=D\x0111=CL%03d\x01", i)))
	}
	if st, err := os.Stat(path); err != nil || st.Size() > 600 {
		t.Fatalf("current log not bounded: %v", st.Size())
	}
	for _, f := range []string{path + ".1", path + ".2"} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("rotated file %s missing", f)
		}
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Fatal("more rotated files kept than configured")
	}
	if r := s.Recent("", "", 100); len(r) != 10 || r[0].ClOrdID != "CL039" {
		t.Fatalf("ring: %d messages, newest %q", len(r), r[0].ClOrdID)
	}
	// The tail reader returns whole lines from the end only.
	lines := tailLines(path, 150)
	if len(lines) == 0 || !strings.Contains(lines[len(lines)-1], "11=CL039") {
		t.Fatalf("tail: %q", lines)
	}
	for _, ln := range lines {
		if !strings.HasPrefix(ln, "20") {
			t.Fatalf("partial line in tail: %q", ln)
		}
	}
}
