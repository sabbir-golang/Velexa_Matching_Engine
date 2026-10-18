package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"velexa-sim-me/internal/database/db/dbtest"
)

func TestFileTrail(t *testing.T) {
	Path = filepath.Join(t.TempDir(), "audit.jsonl")
	Load()
	Record("alice (1.1.1.1)", "ISSUE", "AAA")
	Record("bob (2.2.2.2)", "REVOKE", "BBB")
	Record("Alice (1.1.1.1)", "login.ok", "user")
	if r := Recent(10); len(r) != 3 || r[0].Action != "login.ok" {
		t.Fatalf("recent: %+v", r)
	}
	if r := RecentFor("alice (", 10); len(r) != 2 {
		t.Fatalf("alice's entries: %+v", r)
	}
	Load() // a restart reads the file back
	if len(Recent(10)) != 3 {
		t.Fatal("entries lost across a reload")
	}
}

func TestDatabaseTrail(t *testing.T) {
	pool, _ := dbtest.Open(t)
	dir := t.TempDir()
	Path = filepath.Join(dir, "audit.jsonl")
	// An engine that ran on files leaves a trail behind; it is imported once.
	Load()
	Record("old_user (1.1.1.1)", "ISSUE", "OLD")
	if err := UseDB(pool); err != nil {
		t.Fatal(err)
	}
	defer Load()
	if _, err := os.Stat(Path); !os.IsNotExist(err) {
		t.Fatal("the jsonl file must be renamed after the import")
	}
	time.Sleep(10 * time.Millisecond)
	Record("al_ice (1.1.1.1)", "user.add", "x")
	Record("alxice (1.1.1.1)", "user.add", "y")
	if r := Recent(10); len(r) != 3 || r[0].Target != "y" || r[2].Target != "OLD" {
		t.Fatalf("recent, newest first: %+v", r)
	}
	// "_" is literal in the prefix, not a LIKE wildcard.
	if r := RecentFor("al_ice (", 10); len(r) != 1 || r[0].Target != "x" {
		t.Fatalf("prefix lookup: %+v", r)
	}
	// With the database gone, entries fall back to the file, and the next start imports them.
	pool.Close()
	Record("carol (3.3.3.3)", "REVOKE", "LOST?")
	if entries, _ := readFile(Path); len(entries) != 1 || entries[0].Target != "LOST?" {
		t.Fatalf("fallback file: %+v", entries)
	}
}
