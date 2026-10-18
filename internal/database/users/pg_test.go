package users

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"velexa-sim-me/internal/database/db/dbtest"
)

func pgStore(t *testing.T) (*Store, func() *Store) {
	t.Helper()
	pool, _ := dbtest.Open(t)
	dir := t.TempDir()
	reopen := func() *Store {
		s, err := LoadDB(pool, "test", "", dir, "root", "RootPass123")
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	return reopen(), reopen
}

func TestPGAccountsSurviveARestart(t *testing.T) {
	s, reopen := pgStore(t)
	p := Profile{FullName: "Rahim Uddin", Email: "rahim@example.com", Phone: "+880 1711-000000", Gender: "male", Country: "Bangladesh"}
	if err := s.Register("rahim", "Password1234", p); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("ops1", "Other-Pass12", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	_ = s.SetDisabled("rahim", true)
	_ = s.SetDisabled("rahim", false)
	_ = s.SetRole("ops1", RoleUser)
	if _, ok := s.Authenticate("rahim", "Password1234"); !ok {
		t.Fatal("login failed")
	}
	_, ver, _ := s.Session("rahim")

	s2 := reopen()
	u, ver2, ok := s2.Session("rahim")
	if !ok || u.Email != "rahim@example.com" || u.Country != "Bangladesh" || u.LastLogin == "" || ver2 != ver {
		t.Fatalf("after restart: %+v ver=%v ok=%v", u, ver2 == ver, ok)
	}
	if _, ok := s2.Authenticate("rahim", "Password1234"); !ok {
		t.Fatal("password hash not kept")
	}
	if o, _ := s2.Get("ops1"); o == nil || o.Role != RoleUser {
		t.Fatalf("role change not kept: %+v", o)
	}
	if err := s2.Delete("ops1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := reopen().Get("ops1"); ok {
		t.Fatal("deleted account came back")
	}
	if len(reopen().List()) != 2 { // root (bootstrap) + rahim
		t.Fatalf("accounts: %+v", reopen().List())
	}
}

func TestPGEmailIsUniqueInTheDatabase(t *testing.T) {
	s, _ := pgStore(t)
	p := Profile{FullName: "A B", Email: "same@example.com", Phone: "+880 1711-000000", Gender: "other", Country: "BD"}
	if err := s.Register("first1", "Password1234", p); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("second2", "Password1234", p); err != ErrEmailTaken {
		t.Fatalf("second account with the same email: %v", err)
	}
}

// A failed write leaves memory as it was, so memory and database never disagree.
func TestPGFailedWriteIsRolledBack(t *testing.T) {
	s, _ := pgStore(t)
	_ = s.Add("keep1", "Keep-Pass123", RoleUser)
	s.be.(*pgBackend).pool.Close() // the database goes away
	if err := s.Add("newbie", "Fresh-Start1", RoleUser); !errors.Is(err, ErrStorage) {
		t.Fatalf("add with no database: %v", err)
	}
	if _, ok := s.Get("newbie"); ok {
		t.Fatal("an account that was not saved is still in memory")
	}
	if err := s.SetRole("keep1", RoleAdmin); !errors.Is(err, ErrStorage) {
		t.Fatalf("role change with no database: %v", err)
	}
	if u, _ := s.Get("keep1"); u.Role != RoleUser {
		t.Fatal("a role change that was not saved is still in memory")
	}
	if err := s.Delete("keep1"); !errors.Is(err, ErrStorage) {
		t.Fatalf("delete with no database: %v", err)
	}
	if _, ok := s.Get("keep1"); !ok {
		t.Fatal("a delete that was not saved removed the account from memory")
	}
}

func TestPGImportsUsersJSONOnce(t *testing.T) {
	pool, _ := dbtest.Open(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "users.json")
	file := Load(path, "root", "RootPass123") // an engine that ran on files
	_ = file.Add("carol", "Hidden-Word1", RoleUser)
	_, oldVer, _ := file.Session("carol")

	s, err := LoadDB(pool, "test", path, dir, "root", "RootPass123")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Authenticate("carol", "Hidden-Word1"); !ok {
		t.Fatal("imported account cannot sign in with its old password")
	}
	if _, ver, _ := s.Session("carol"); ver != oldVer {
		t.Fatal("import must keep session tokens (nobody is signed out)")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("users.json must be renamed after the import")
	}
	matches, _ := filepath.Glob(path + ".imported-*")
	if len(matches) != 1 {
		t.Fatalf("renamed file: %v", matches)
	}
	// A new users.json later is not imported over a non-empty table.
	b, _ := json.Marshal([]*User{{Username: "ghost", Role: RoleAdmin, Hash: "x"}})
	os.WriteFile(path, b, 0o600)
	s2, _ := LoadDB(pool, "test", path, dir, "root", "RootPass123")
	if _, ok := s2.Get("ghost"); ok {
		t.Fatal("a second import must not happen")
	}
	var n int
	pool.QueryRow(context.Background(), "SELECT count(*) FROM users").Scan(&n)
	if n != 2 {
		t.Fatalf("%d rows, want 2 (root, carol)", n)
	}
}
