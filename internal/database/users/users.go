// Package users holds the admin-console accounts and their roles. Accounts live
// in users.json; passwords are stored only as salted PBKDF2 hashes.
package users

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"velexa-sim-me/internal/config"
)

// Role is what an account may do in the console.
type Role string

const (
	// RoleAdmin can do everything, including managing users and broker credentials.
	RoleAdmin Role = "admin"
	// RoleUser is read-only.
	RoleUser Role = "user"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool { return r == RoleAdmin || r == RoleUser }

// Allows reports whether an account with role r may do something that needs min.
func (r Role) Allows(min Role) bool {
	if min == RoleUser {
		return r.Valid()
	}
	return r == RoleAdmin
}

// User is one console account. The password hash never leaves this package.
type User struct {
	Username  string `json:"username"`
	Role      Role   `json:"role"`
	Disabled  bool   `json:"disabled,omitempty"`
	Created   string `json:"created,omitempty"`
	LastLogin string `json:"last_login,omitempty"`
	Profile
	Hash string `json:"hash,omitempty"`
	// SessionVer is a random token written into every session cookie. Changing
	// it (sign-out, password change, disable) ends all of the account's sessions.
	SessionVer string `json:"session_ver,omitempty"`
}

// Profile is the contact detail collected when someone registers themselves.
// Accounts created by an admin may leave it empty.
type Profile struct {
	FullName string `json:"full_name,omitempty"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Gender   string `json:"gender,omitempty"` // male | female | other
	Country  string `json:"country,omitempty"`
}

// Public returns a copy without the password hash or session token, safe to send to a client.
func (u User) Public() User {
	u.Hash = ""
	u.SessionVer = ""
	return u
}

// HashIterations is the PBKDF2-SHA256 work factor for new password hashes
// (OWASP: 600 000). Older, weaker hashes are upgraded at the next sign-in.
// Tests lower it to stay fast.
var HashIterations = hashIterationsDefault()

func hashIterationsDefault() int { return 600_000 }

const (
	minPasswordLen = 10
	maxPasswordLen = 128
	timeFormat     = "2006-01-02 15:04:05"
)

// Errors returned by Store methods; their text is shown to the operator.
var (
	ErrInvalidUsername = errors.New("username must be 3-32 characters: letters, digits, . _ -")
	ErrWeakPassword    = fmt.Errorf("password must be %d-%d characters, must not contain the username, and must use at least 3 of: lower-case letters, upper-case letters, digits, symbols", minPasswordLen, maxPasswordLen)
	ErrReservedName    = errors.New("that username is reserved")
	ErrWrongPassword   = errors.New("current password is incorrect")
	ErrInvalidRole     = errors.New("role must be admin or user")
	ErrInvalidProfile  = errors.New("invalid profile")
	ErrEmailTaken      = errors.New("that email is already registered")
	ErrExists          = errors.New("that username already exists")
	ErrNotFound        = errors.New("no such user")
	ErrLastAdmin       = errors.New("cannot remove, disable or demote the last enabled admin")
)

// Store is the set of console accounts, persisted to a JSON file.
type Store struct {
	mu    sync.RWMutex
	path  string
	users map[string]*User // keyed by lower-case username

	// The environment account (ME_ADMIN_USER / ME_ADMIN_PASS) is a break-glass
	// admin: it works only while users.json has no enabled admin, so a lost or
	// damaged file can never lock everyone out. Only a hash of its password is kept.
	envUser    string
	envPassSum [32]byte
	envVer     string // its session token; new on every start
}

// Path is where accounts are stored by default.
func Path() string { return config.EnvOr("ME_USERS_FILE", config.StateFile("users.json")) }

// Load reads the store from path and makes sure at least one admin exists: if the
// file has none, the environment admin (envUser/envPass) is added, or — with no
// usable envPass — an admin with a random password written to
// initial-admin-password next to the file. envPass "" turns the break-glass
// environment admin off.
func Load(path, envUser, envPass string) *Store {
	s := &Store{path: path, users: map[string]*User{}, envVer: newToken()}
	if envUser != "" && envPass != "" {
		s.envUser, s.envPassSum = envUser, sha256.Sum256([]byte(envPass))
	}
	dirty := false
	if b, err := os.ReadFile(path); err == nil {
		var list []*User
		if err := json.Unmarshal(b, &list); err != nil {
			log.Printf("[users] %s is unreadable (%v) — starting with the environment admin only", path, err)
		}
		for _, u := range list {
			if u != nil && u.Username != "" && u.Role.Valid() {
				if u.SessionVer == "" {
					u.SessionVer, dirty = newToken(), true
				}
				s.users[key(u.Username)] = u
			}
		}
	}
	// The environment admin's file account may still hold the published default
	// password from an older build: re-key it to ME_ADMIN_PASS, or disable it.
	if u := s.users[key(envUser)]; u != nil && !u.Disabled && verifyPassword(u.Hash, config.DefaultAdminPass) {
		if h, err := hashPassword(envPass); envPass != "" && err == nil {
			u.Hash = h
			log.Printf("[users] %q still had the old default password — it now uses ME_ADMIN_PASS", u.Username)
		} else {
			u.Disabled = true
			log.Printf("[users] %q still had the old default password and ME_ADMIN_PASS is not set — account disabled", u.Username)
		}
		u.SessionVer, dirty = newToken(), true
	}
	if !s.hasEnabledAdmin() {
		dirty = s.bootstrapAdmin(envUser, envPass) || dirty
	}
	if dirty {
		if err := s.saveLocked(); err != nil {
			log.Printf("[users] could not save %s: %v", path, err)
		}
	}
	return s
}

// bootstrapAdmin creates the first admin account. Caller holds s.mu (or owns s).
func (s *Store) bootstrapAdmin(envUser, envPass string) bool {
	if envUser == "" {
		envUser = "admin"
	}
	if old := s.users[key(envUser)]; old != nil && !old.Disabled {
		return false // an enabled non-admin already has the name; keep it
	}
	pass, generated := envPass, false
	if pass == "" {
		pass, generated = "Me-"+newToken(), true
	}
	h, err := hashPassword(pass)
	if err != nil {
		return false
	}
	s.users[key(envUser)] = &User{Username: envUser, Role: RoleAdmin, Created: now(), Hash: h, SessionVer: newToken()}
	if !generated {
		log.Printf("[users] created the admin account %q from the environment", envUser)
		return true
	}
	file := filepath.Join(filepath.Dir(s.path), "initial-admin-password")
	_ = os.MkdirAll(filepath.Dir(file), 0o750)
	if err := os.WriteFile(file, []byte(envUser+" "+pass+"\n"), 0o600); err != nil {
		log.Printf("[users] created the admin account %q; could not save its password (%v): %s", envUser, err, pass)
	} else {
		log.Printf("[users] created the admin account %q with a random password — see %s (delete it after signing in)", envUser, file)
	}
	return true
}

func key(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
func now() string            { return time.Now().Format(timeFormat) }

func (s *Store) hasEnabledAdmin() bool {
	for _, u := range s.users {
		if u.Role == RoleAdmin && !u.Disabled {
			return true
		}
	}
	return false
}

// Authenticate checks a username and password. On success it returns the account
// (a copy without its hash) and records the login time. The password is checked
// without holding the store lock, and an unknown name costs as much time as a
// known one, so timing does not reveal which accounts exist.
func (s *Store) Authenticate(username, password string) (*User, bool) {
	s.mu.RLock()
	hash := ""
	if u := s.users[key(username)]; u != nil && !u.Disabled {
		hash = u.Hash
	}
	glass := s.breakGlassLocked()
	s.mu.RUnlock()

	if hash == "" || !verifyPassword(hash, password) {
		if hash == "" {
			verifyPassword(dummyHash(), password)
		}
		if glass && s.isEnvAdmin(username, password) {
			return &User{Username: s.envUser, Role: RoleAdmin}, true
		}
		return nil, false
	}
	upgraded := ""
	if hashIterations(hash) < HashIterations {
		upgraded, _ = hashPassword(password)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[key(username)]
	if u == nil || u.Disabled || u.Hash != hash { // changed while the password was checked
		return nil, false
	}
	if upgraded != "" {
		u.Hash = upgraded
	}
	u.LastLogin = now()
	_ = s.saveLocked()
	p := u.Public()
	return &p, true
}

// Get returns the enabled account with that name (or the break-glass env admin).
func (s *Store) Get(username string) (*User, bool) {
	u, _, ok := s.Session(username)
	return u, ok
}

// Session returns the enabled account with that name and its current session
// token, as used to check a session cookie on every request.
func (s *Store) Session(username string) (*User, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if u := s.users[key(username)]; u != nil {
		if u.Disabled {
			return nil, "", false
		}
		p := u.Public()
		return &p, u.SessionVer, true
	}
	if s.breakGlassLocked() && key(username) == key(s.envUser) {
		return &User{Username: s.envUser, Role: RoleAdmin}, s.envVer, true
	}
	return nil, "", false
}

// RevokeSessions ends every session of the account (sign out everywhere).
func (s *Store) RevokeSessions(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.users[key(username)]; u != nil {
		u.SessionVer = newToken()
		return s.saveLocked()
	}
	if key(username) == key(s.envUser) {
		s.envVer = newToken()
		return nil
	}
	return ErrNotFound
}

// breakGlassLocked reports whether the environment admin may sign in: it is
// configured and the file has no enabled admin. Caller holds s.mu.
func (s *Store) breakGlassLocked() bool {
	return s.envUser != "" && !s.hasEnabledAdmin()
}

func (s *Store) isEnvAdmin(username, password string) bool {
	sum := sha256.Sum256([]byte(password))
	nameOK := ctEqual(key(username), key(s.envUser))
	return subtle.ConstantTimeCompare(sum[:], s.envPassSum[:]) == 1 && nameOK
}

// List returns every account, sorted by name, without password hashes.
func (s *Store) List() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u.Public())
	}
	sort.Slice(out, func(i, j int) bool { return key(out[i].Username) < key(out[j].Username) })
	return out
}

// Add creates an account.
func (s *Store) Add(username, password string, role Role) error {
	username = strings.TrimSpace(username)
	if !validUsername(username) {
		return ErrInvalidUsername
	}
	if err := CheckPassword(username, password); err != nil {
		return err
	}
	if !role.Valid() {
		return ErrInvalidRole
	}
	h, err := hashPassword(password)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.users[key(username)]; dup {
		return ErrExists
	}
	s.users[key(username)] = &User{Username: username, Role: role, Created: now(), Hash: h, SessionVer: newToken()}
	return s.saveLocked()
}

// Normalize trims the profile and checks every field; the error text names the
// field that is wrong.
func (p Profile) Normalize() (Profile, error) {
	p.FullName = strings.TrimSpace(p.FullName)
	p.Email = strings.ToLower(strings.TrimSpace(p.Email))
	p.Phone = strings.TrimSpace(p.Phone)
	p.Gender = strings.ToLower(strings.TrimSpace(p.Gender))
	p.Country = strings.TrimSpace(p.Country)
	bad := func(field string) (Profile, error) {
		return p, fmt.Errorf("%w: %s", ErrInvalidProfile, field)
	}
	if n := len([]rune(p.FullName)); n < 2 || n > 80 {
		return bad("full name")
	}
	if a, err := mail.ParseAddress(p.Email); err != nil || a.Address != p.Email || len(p.Email) > 120 || !strings.Contains(p.Email[strings.Index(p.Email, "@"):], ".") {
		return bad("email")
	}
	digits := 0
	for i, r := range p.Phone {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '+' && i == 0, r == ' ', r == '-', r == '(', r == ')':
		default:
			return bad("phone")
		}
	}
	if digits < 7 || digits > 15 {
		return bad("phone")
	}
	if p.Gender != "male" && p.Gender != "female" && p.Gender != "other" {
		return bad("gender")
	}
	if n := len([]rune(p.Country)); n < 2 || n > 60 {
		return bad("country")
	}
	return p, nil
}

// Register creates a role-"user" account for a self-registered person. The
// profile is required and the email must not belong to another account.
func (s *Store) Register(username, password string, p Profile) error {
	username = strings.TrimSpace(username)
	if !validUsername(username) {
		return ErrInvalidUsername
	}
	if s.reserved(username) {
		return ErrReservedName
	}
	if err := CheckPassword(username, password); err != nil {
		return err
	}
	p, err := p.Normalize()
	if err != nil {
		return err
	}
	h, err := hashPassword(password)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.users[key(username)]; dup {
		return ErrExists
	}
	for _, u := range s.users {
		if u.Email != "" && u.Email == p.Email {
			return ErrEmailTaken
		}
	}
	s.users[key(username)] = &User{Username: username, Role: RoleUser, Created: now(), Profile: p, Hash: h, SessionVer: newToken()}
	return s.saveLocked()
}

// SetRole changes an account's role.
func (s *Store) SetRole(username string, role Role) error {
	if !role.Valid() {
		return ErrInvalidRole
	}
	return s.mutate(username, func(u *User) error {
		if u.Role == RoleAdmin && role != RoleAdmin && s.otherEnabledAdmins(u) == 0 {
			return ErrLastAdmin
		}
		u.Role = role
		return nil
	})
}

// SetDisabled enables or disables an account.
func (s *Store) SetDisabled(username string, disabled bool) error {
	return s.mutate(username, func(u *User) error {
		if disabled && u.Role == RoleAdmin && !u.Disabled && s.otherEnabledAdmins(u) == 0 {
			return ErrLastAdmin
		}
		if disabled && !u.Disabled {
			u.SessionVer = newToken()
		}
		u.Disabled = disabled
		return nil
	})
}

// SetPassword replaces an account's password and ends all of its sessions.
func (s *Store) SetPassword(username, password string) error {
	if err := CheckPassword(username, password); err != nil {
		return err
	}
	h, err := hashPassword(password)
	if err != nil {
		return err
	}
	return s.mutate(username, func(u *User) error { u.Hash, u.SessionVer = h, newToken(); return nil })
}

// ChangeOwnPassword sets a new password after checking the current one.
func (s *Store) ChangeOwnPassword(username, oldPassword, newPassword string) error {
	s.mu.RLock()
	hash := ""
	if u := s.users[key(username)]; u != nil && !u.Disabled {
		hash = u.Hash
	}
	s.mu.RUnlock()
	if hash == "" || !verifyPassword(hash, oldPassword) {
		return ErrWrongPassword
	}
	return s.SetPassword(username, newPassword)
}

// Delete removes an account.
func (s *Store) Delete(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[key(username)]
	if u == nil {
		return ErrNotFound
	}
	if u.Role == RoleAdmin && !u.Disabled && s.otherEnabledAdmins(u) == 0 {
		return ErrLastAdmin
	}
	delete(s.users, key(username))
	return s.saveLocked()
}

func (s *Store) mutate(username string, fn func(*User) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[key(username)]
	if u == nil {
		return ErrNotFound
	}
	if err := fn(u); err != nil {
		return err
	}
	return s.saveLocked()
}

// otherEnabledAdmins counts enabled admins other than u. Caller holds s.mu.
func (s *Store) otherEnabledAdmins(u *User) int {
	n := 0
	for _, o := range s.users {
		if o != u && o.Role == RoleAdmin && !o.Disabled {
			n++
		}
	}
	return n
}

// saveLocked writes the file atomically. Caller holds s.mu.
func (s *Store) saveLocked() error {
	list := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		list = append(list, u)
	}
	sort.Slice(list, func(i, j int) bool { return key(list[i].Username) < key(list[j].Username) })
	b, err := json.MarshalIndent(list, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// reserved reports whether a self-registered account may not take this name:
// administrative names and the environment admin's. Caller need not hold s.mu.
func (s *Store) reserved(name string) bool {
	switch key(name) {
	case "admin", "administrator", "root", "system", "superuser", "support", "security", key(s.envUser):
		return true
	}
	return false
}

// CheckPassword applies the password rules: 10-128 characters, not containing
// the username, and at least three of lower-case, upper-case, digits, symbols.
func CheckPassword(username, password string) error {
	if n := utf8.RuneCountInString(password); n < minPasswordLen || n > maxPasswordLen {
		return ErrWeakPassword
	}
	if name := key(username); name != "" && strings.Contains(strings.ToLower(password), name) {
		return ErrWeakPassword
	}
	var lower, upper, digit, other int
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			lower = 1
		case unicode.IsUpper(r):
			upper = 1
		case unicode.IsDigit(r):
			digit = 1
		default:
			other = 1
		}
	}
	if lower+upper+digit+other < 3 {
		return ErrWeakPassword
	}
	return nil
}

// newToken is a random URL-safe token (128 bits).
func newToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("users: no randomness: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func validUsername(s string) bool {
	if len(s) < 3 || len(s) > 32 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// ---- password hashing: "pbkdf2-sha256$iterations$salt$hash" (base64) ----

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk, err := pbkdf2.Key(sha256.New, password, salt, HashIterations, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return "pbkdf2-sha256$" + strconv.Itoa(HashIterations) + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(dk), nil
}

// hashIterations is the work factor recorded in a stored hash (0 if unreadable).
func hashIterations(stored string) int {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 {
		return 0
	}
	n, _ := strconv.Atoi(parts[1])
	return n
}

var (
	dummyOnce sync.Once
	dummy     string
)

// dummyHash is checked against when the account does not exist, so a failed
// sign-in takes the same time either way.
func dummyHash() string {
	dummyOnce.Do(func() { dummy, _ = hashPassword(newToken()) })
	return dummy
}

func verifyPassword(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func ctEqual(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
