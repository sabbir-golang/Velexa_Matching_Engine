package users

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"velexa-sim-me/internal/config"
)

func TestMain(m *testing.M) {
	HashIterations = 1000 // keep the suite fast; the real work factor is checked below
	os.Exit(m.Run())
}

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
	if err := s.Add("alice", "Correct-horse1", RoleUser); err != nil {
		t.Fatal(err)
	}
	if u, ok := s.Authenticate("alice", "Correct-horse1"); !ok || u.Role != RoleUser || u.Hash != "" {
		t.Fatalf("good login: %+v %v", u, ok)
	}
	if u, ok := s.Authenticate("ALICE", "Correct-horse1"); !ok || u.Username != "alice" {
		t.Fatal("usernames are case-insensitive")
	}
	for _, bad := range [][2]string{{"alice", "wrong"}, {"alice", ""}, {"nobody", "Correct-horse1"}, {"", ""}} {
		if _, ok := s.Authenticate(bad[0], bad[1]); ok {
			t.Fatalf("login %v should fail", bad)
		}
	}
	if err := s.SetDisabled("alice", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Authenticate("alice", "Correct-horse1"); ok {
		t.Fatal("a disabled account must not sign in")
	}
	if _, ok := s.Get("alice"); ok {
		t.Fatal("Get must not return a disabled account")
	}
}

func TestPasswordsAreHashedAndSalted(t *testing.T) {
	s, path := newStore(t)
	_ = s.Add("a1user", "SamePassword1", RoleUser)
	_ = s.Add("b2user", "SamePassword1", RoleUser)
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "SamePassword1") || strings.Contains(string(b), "RootPass123") {
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
		{"ab", "LongEnough12", RoleUser, ErrInvalidUsername},
		{"has space", "LongEnough12", RoleUser, ErrInvalidUsername},
		{strings.Repeat("x", 33), "LongEnough12", RoleUser, ErrInvalidUsername},
		{"okname", "short", RoleUser, ErrWeakPassword},
		{"okname", "alllowercase", RoleUser, ErrWeakPassword},
		{"okname", "NoDigitsHere", RoleUser, ErrWeakPassword}, // two classes
		{"okname", "LongEnough12", Role("root"), ErrInvalidRole},
		{"root", "LongEnough12", RoleUser, ErrExists},
		{"ROOT", "LongEnough12", RoleUser, ErrExists},
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
	if err := s.Add("second", "Other-Pass12", RoleAdmin); err != nil {
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
	_ = s.Add("carol", "FirstPassword1", RoleUser)
	if err := s.ChangeOwnPassword("carol", "WrongOld123", "NewPassword12"); err == nil {
		t.Fatal("wrong current password accepted")
	}
	if err := s.ChangeOwnPassword("carol", "FirstPassword1", "tiny"); err != ErrWeakPassword {
		t.Fatalf("weak new password: %v", err)
	}
	if err := s.ChangeOwnPassword("carol", "FirstPassword1", "NewPassword12"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Authenticate("carol", "FirstPassword1"); ok {
		t.Fatal("old password still works")
	}
	if _, ok := s.Authenticate("carol", "NewPassword12"); !ok {
		t.Fatal("new password does not work")
	}
	if err := s.SetPassword("carol", "AdminReset99"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Authenticate("carol", "AdminReset99"); !ok {
		t.Fatal("admin reset does not work")
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	s, path := newStore(t)
	_ = s.Add("dave", "Hidden-Word7", RoleUser)
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
	if err := s.Register("rahim", "Password1234", ok); err != nil {
		t.Fatal(err)
	}
	u, _ := s.Get("rahim")
	if u.Role != RoleUser || u.Email != "rahim@example.com" || u.Country != "Bangladesh" {
		t.Fatalf("stored: %+v", u)
	}
	if err := s.Register("rahim2", "Password1234", ok); err != ErrEmailTaken {
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
		if err := s.Register("someone", "Password1234", p); !errors.Is(err, ErrInvalidProfile) {
			t.Errorf("%+v: %v", p, err)
		}
	}
	if _, found := s.Get("someone"); found {
		t.Fatal("a rejected registration must not create an account")
	}
}

func TestPasswordPolicy(t *testing.T) {
	for pw, ok := range map[string]bool{
		"Short1!":                             false, // too short
		"lowercaseonly":                       false, // one class
		"lowercase1234":                       false, // two classes
		"Lowercase1234":                       true,
		"lower-case-12":                       true,
		"UPPER LOWER x":                       true,  // upper, lower, space
		"Pr" + strings.Repeat("x", 126) + "1": false, // 129 characters
		"MyBobby-Pass1":                       true,  // the username may be part of it
	} {
		if err := CheckPassword("bobby", pw); (err == nil) != ok {
			t.Errorf("CheckPassword(%q) = %v, want ok=%v", pw, err, ok)
		}
	}
	if HashIterations < 1000 {
		t.Fatal("HashIterations lowered too far")
	}
}

func TestDefaultWorkFactorAndUpgrade(t *testing.T) {
	if got := hashIterationsDefault(); got < 600_000 {
		t.Fatalf("default PBKDF2 iterations %d, want >= 600000", got)
	}
	s, _ := newStore(t)
	_ = s.Add("eve", "Secret-Word1", RoleUser)
	old := HashIterations
	HashIterations = 2000
	defer func() { HashIterations = old }()
	if _, ok := s.Authenticate("eve", "Secret-Word1"); !ok {
		t.Fatal("login failed")
	}
	if n := hashIterations(s.users["eve"].Hash); n != 2000 {
		t.Fatalf("hash not upgraded at sign-in: %d iterations", n)
	}
	if _, ok := s.Authenticate("eve", "Secret-Word1"); !ok {
		t.Fatal("login with the upgraded hash failed")
	}
}

func TestSessionTokenChangesOnSecurityEvents(t *testing.T) {
	s, path := newStore(t)
	_ = s.Add("finn", "Hidden-Word1", RoleUser)
	ver := func() string { _, v, _ := s.Session("finn"); return v }
	v0 := ver()
	if v0 == "" {
		t.Fatal("new account has no session token")
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), v0) {
		t.Fatal("session token not persisted")
	}
	for _, l := range s.List() {
		if l.SessionVer != "" || l.Hash != "" {
			t.Fatal("List leaks the hash or session token")
		}
	}
	steps := []struct {
		name string
		do   func() error
	}{
		{"revoke", func() error { return s.RevokeSessions("finn") }},
		{"own password", func() error { return s.ChangeOwnPassword("finn", "Hidden-Word1", "Hidden-Word2") }},
		{"admin reset", func() error { return s.SetPassword("finn", "Hidden-Word3") }},
	}
	for _, st := range steps {
		before := ver()
		if err := st.do(); err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
		if ver() == before {
			t.Fatalf("%s did not change the session token", st.name)
		}
	}
	before := ver()
	_ = s.SetDisabled("finn", true)
	_ = s.SetDisabled("finn", false)
	if ver() == before {
		t.Fatal("disabling did not change the session token")
	}
	before = ver()
	_ = s.Delete("finn")
	_ = s.Add("finn", "Hidden-Word9", RoleUser)
	if ver() == before {
		t.Fatal("a re-created account reuses the old session token")
	}
}

func TestBreakGlassOnlyWithoutAFileAdmin(t *testing.T) {
	s, _ := newStore(t)
	_ = s.SetPassword("root", "Changed-Adm12")
	if _, ok := s.Authenticate("root", "RootPass123"); ok {
		t.Fatal("env password must not work while an enabled file admin exists")
	}
	if _, ok := s.Authenticate("root", "Changed-Adm12"); !ok {
		t.Fatal("file password must work")
	}
	// No env password: no break-glass, and the first admin gets a random password.
	path := filepath.Join(t.TempDir(), "users.json")
	s2 := Load(path, "admin", "")
	b, err := os.ReadFile(filepath.Join(filepath.Dir(path), "initial-admin-password"))
	if err != nil {
		t.Fatalf("initial admin password not written: %v", err)
	}
	user, pass, _ := strings.Cut(strings.TrimSpace(string(b)), " ")
	if _, ok := s2.Authenticate(user, pass); !ok || user != "admin" {
		t.Fatal("generated admin password does not work")
	}
	if _, ok := s2.Authenticate("admin", ""); ok {
		t.Fatal("empty password accepted")
	}
}

func TestDefaultPasswordIsReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	s := Load(path, "admin", config.DefaultAdminPass) // old build: seeded with the default
	if _, ok := s.Authenticate("admin", config.DefaultAdminPass); !ok {
		t.Fatal("setup: seeded admin")
	}
	s = Load(path, "admin", "RealSecret-2026")
	if _, ok := s.Authenticate("admin", config.DefaultAdminPass); ok {
		t.Fatal("the published default password still works")
	}
	if _, ok := s.Authenticate("admin", "RealSecret-2026"); !ok {
		t.Fatal("ME_ADMIN_PASS does not work after re-keying")
	}
	s = Load(path, "admin", "")
	if _, ok := s.Authenticate("admin", "RealSecret-2026"); !ok {
		t.Fatal("a re-keyed admin must keep working without the env")
	}
}

func TestRegisterRefusesReservedNames(t *testing.T) {
	s, _ := newStore(t)
	p := Profile{FullName: "X Y", Email: "x@example.com", Phone: "+880 1711-000000", Gender: "other", Country: "BD"}
	for _, n := range []string{"admin", "Administrator", "ROOT", "system"} {
		if err := s.Register(n, "Password1234", p); err != ErrReservedName {
			t.Errorf("Register(%q) = %v", n, err)
		}
	}
}

func TestUnknownUserCostsAHashCheck(t *testing.T) {
	s, _ := newStore(t)
	if _, ok := s.Authenticate("ghost", "Whatever123"); ok {
		t.Fatal("unknown user signed in")
	}
	if dummyHash() == "" {
		t.Fatal("no dummy hash")
	}
}
