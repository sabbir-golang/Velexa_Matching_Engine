package users

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "users.json")
	return Load(path, "root", "RootPass123"), path
}

func TestBootstrapCreatesTheEnvAdmin(t *testing.T) {
	s, path := newStore(t)
	list := s.List()
	if len(list) != 1 || list[0].Username != "root" || list[0].Role != RoleAdmin || list[0].Hash != "" {
		t.Fatalf("bootstrap: %+v", list)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("users.json not written: %v", err)
	}
	if u, ok := s.Authenticate("root", "RootPass123"); !ok || u.Role != RoleAdmin {
		t.Fatalf("bootstrap admin cannot sign in: %v %v", u, ok)
	}
}

func TestAuthenticate(t *testing.T) {
	s, _ := newStore(t)
	if err := s.Add("alice", "correct-horse", RoleUser); err != nil {
		t.Fatal(err)
	}
	if u, ok := s.Authenticate("alice", "correct-horse"); !ok || u.Role != RoleUser || u.Hash != "" {
		t.Fatalf("good login: %+v %v", u, ok)
	}
	if u, ok := s.Authenticate("ALICE", "correct-horse"); !ok || u.Username != "alice" {
		t.Fatal("usernames are case-insensitive")
	}
	for _, bad := range [][2]string{{"alice", "wrong"}, {"alice", ""}, {"nobody", "correct-horse"}, {"", ""}} {
		if _, ok := s.Authenticate(bad[0], bad[1]); ok {
			t.Fatalf("login %v should fail", bad)
		}
	}
	if err := s.SetDisabled("alice", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Authenticate("alice", "correct-horse"); ok {
		t.Fatal("a disabled account must not sign in")
	}
	if _, ok := s.Get("alice"); ok {
		t.Fatal("Get must not return a disabled account")
	}
}

func TestPasswordsAreHashedAndSalted(t *testing.T) {
	s, path := newStore(t)
	_ = s.Add("a1user", "samePassword1", RoleUser)
	_ = s.Add("b2user", "samePassword1", RoleUser)
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "samePassword1") || strings.Contains(string(b), "RootPass123") {
		t.Fatal("plain-text password found in users.json")
	}
	if s.users["a1user"].Hash == s.users["b2user"].Hash {
		t.Fatal("identical passwords must hash differently (salt)")
	}
	if !strings.HasPrefix(s.users["a1user"].Hash, "pbkdf2-sha256$") {
		t.Fatalf("unexpected hash format %q", s.users["a1user"].Hash)
	}
}

func TestValidation(t *testing.T) {
	s, _ := newStore(t)
	cases := []struct {
		name, pass string
		role       Role
		want       error
	}{
		{"ab", "longenough1", RoleUser, ErrInvalidUsername},
		{"has space", "longenough1", RoleUser, ErrInvalidUsername},
		{strings.Repeat("x", 33), "longenough1", RoleUser, ErrInvalidUsername},
		{"okname", "short", RoleUser, ErrWeakPassword},
		{"okname", "longenough1", Role("root"), ErrInvalidRole},
		{"root", "longenough1", RoleUser, ErrExists},
		{"ROOT", "longenough1", RoleUser, ErrExists},
	}
	for _, c := range cases {
		if err := s.Add(c.name, c.pass, c.role); err != c.want {
			t.Errorf("Add(%q,%q,%q) = %v, want %v", c.name, c.pass, c.role, err, c.want)
		}
	}
}

func TestLastAdminCannotBeRemoved(t *testing.T) {
	s, _ := newStore(t)
	if err := s.Delete("root"); err != ErrLastAdmin {
		t.Fatalf("delete last admin: %v", err)
	}
	if err := s.SetDisabled("root", true); err != ErrLastAdmin {
		t.Fatalf("disable last admin: %v", err)
	}
	if err := s.SetRole("root", RoleUser); err != ErrLastAdmin {
		t.Fatalf("demote last admin: %v", err)
	}
	// with a second admin the first can go
	if err := s.Add("second", "secondPass1", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRole("root", RoleUser); err != nil {
		t.Fatalf("demote with a second admin present: %v", err)
	}
	if err := s.Delete("second"); err != ErrLastAdmin {
		t.Fatalf("second is now the last admin: %v", err)
	}
	if err := s.Delete("nobody"); err != ErrNotFound {
		t.Fatalf("delete unknown: %v", err)
	}
}

func TestPasswordChanges(t *testing.T) {
	s, _ := newStore(t)
	_ = s.Add("carol", "firstPassword", RoleUser)
	if err := s.ChangeOwnPassword("carol", "wrongOld", "newPassword1"); err == nil {
		t.Fatal("wrong current password accepted")
	}
	if err := s.ChangeOwnPassword("carol", "firstPassword", "tiny"); err != ErrWeakPassword {
		t.Fatalf("weak new password: %v", err)
	}
	if err := s.ChangeOwnPassword("carol", "firstPassword", "newPassword1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Authenticate("carol", "firstPassword"); ok {
		t.Fatal("old password still works")
	}
	if _, ok := s.Authenticate("carol", "newPassword1"); !ok {
		t.Fatal("new password does not work")
	}
	if err := s.SetPassword("carol", "adminReset99"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Authenticate("carol", "adminReset99"); !ok {
		t.Fatal("admin reset does not work")
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	s, path := newStore(t)
	_ = s.Add("dave", "davePassword1", RoleUser)
	_ = s.SetDisabled("dave", true)
	s2 := Load(path, "root", "RootPass123")
	l := s2.List()
	if len(l) != 2 {
		t.Fatalf("reloaded %d accounts: %+v", len(l), l)
	}
	if u, ok := s2.users["dave"]; !ok || !u.Disabled || u.Role != RoleUser {
		t.Fatalf("dave not restored: %+v", u)
	}
	if _, ok := s2.Authenticate("root", "RootPass123"); !ok {
		t.Fatal("admin cannot sign in after reload")
	}
}

func TestEnvAdminIsBreakGlass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(path, "root", "RootPass123")
	if u, ok := s.Authenticate("root", "RootPass123"); !ok || u.Role != RoleAdmin {
		t.Fatalf("env admin must work even when users.json is damaged: %v %v", u, ok)
	}
	if _, ok := s.Authenticate("root", "nope"); ok {
		t.Fatal("wrong password accepted")
	}
	if u, ok := s.Get("root"); !ok || u.Role != RoleAdmin {
		t.Fatal("Get must resolve the env admin")
	}
}

func TestRoleAllows(t *testing.T) {
	if !RoleAdmin.Allows(RoleAdmin) || !RoleAdmin.Allows(RoleUser) || !RoleUser.Allows(RoleUser) {
		t.Fatal("expected allows")
	}
	if RoleUser.Allows(RoleAdmin) || Role("").Allows(RoleUser) || Role("x").Allows(RoleAdmin) {
		t.Fatal("expected denies")
	}
}

func TestRegisterValidatesProfileAndEmail(t *testing.T) {
	s := Load(filepath.Join(t.TempDir(), "users.json"), "admin", "AdminPass123")
	ok := Profile{FullName: "Rahim Uddin", Email: "Rahim@Example.com", Phone: "+880 1711-000000", Gender: "male", Country: "Bangladesh"}
	if err := s.Register("rahim", "password123", ok); err != nil {
		t.Fatal(err)
	}
	u, _ := s.Get("rahim")
	if u.Role != RoleUser || u.Email != "rahim@example.com" || u.Country != "Bangladesh" {
		t.Fatalf("stored: %+v", u)
	}
	if err := s.Register("rahim2", "password123", ok); err != ErrEmailTaken {
		t.Fatalf("duplicate email: %v", err)
	}
	for _, mod := range []func(*Profile){
		func(p *Profile) { p.FullName = "" },
		func(p *Profile) { p.Email = "nope" },
		func(p *Profile) { p.Email = "a@b" },
		func(p *Profile) { p.Phone = "12" },
		func(p *Profile) { p.Phone = "12ab5678" },
		func(p *Profile) { p.Gender = "x" },
		func(p *Profile) { p.Country = "" },
	} {
		p := ok
		p.Email = "other@example.com"
		mod(&p)
		if err := s.Register("someone", "password123", p); !errors.Is(err, ErrInvalidProfile) {
			t.Errorf("%+v: %v", p, err)
		}
	}
	if _, found := s.Get("someone"); found {
		t.Fatal("a rejected registration must not create an account")
	}
}
