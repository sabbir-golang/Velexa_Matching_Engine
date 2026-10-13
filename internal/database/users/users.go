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

// Public returns a copy with the password hash removed, safe to send to a client.
func (u User) Public() User {
	u.Hash = ""
	return u
}

const (
	pbkdf2Iterations = 210_000
	minPasswordLen   = 8
	timeFormat       = "2006-01-02 15:04:05"
)

// Errors returned by Store methods; their text is shown to the operator.
var (
	ErrInvalidUsername = errors.New("username must be 3-32 characters: letters, digits, . _ -")
	ErrWeakPassword    = fmt.Errorf("password must be at least %d characters", minPasswordLen)
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

	// The environment account (ME_ADMIN_USER / ME_ADMIN_PASS) always works as an
	// admin, so a lost or damaged users.json can never lock everyone out.
	envUser, envPass string
}

// Path is where accounts are stored by default.
func Path() string { return config.EnvOr("ME_USERS_FILE", config.StateFile("users.json")) }

// Load reads the store from path and makes sure at least one admin exists: if the
// file has none, the environment admin is added. envUser/envPass is also kept as
// a break-glass admin that does not depend on the file.
func Load(path, envUser, envPass string) *Store {
	s := &Store{path: path, users: map[string]*User{}, envUser: envUser, envPass: envPass}
	if b, err := os.ReadFile(path); err == nil {
		var list []*User
		if err := json.Unmarshal(b, &list); err != nil {
			log.Printf("[users] %s is unreadable (%v) — starting with the environment admin only", path, err)
		}
		for _, u := range list {
			if u != nil && u.Username != "" && u.Role.Valid() {
				s.users[key(u.Username)] = u
			}
		}
	}
	if !s.hasEnabledAdmin() && envUser != "" && envPass != "" {
		if _, exists := s.users[key(envUser)]; !exists {
			if h, err := hashPassword(envPass); err == nil {
				s.users[key(envUser)] = &User{Username: envUser, Role: RoleAdmin, Created: now(), Hash: h}
				if err := s.saveLocked(); err != nil {
					log.Printf("[users] could not save %s: %v", path, err)
				}
				log.Printf("[users] created the admin account %q from the environment", envUser)
			}
		}
	}
	return s
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
// (a copy without its hash) and records the login time.
func (s *Store) Authenticate(username, password string) (*User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.users[key(username)]; u != nil {
		if !u.Disabled && verifyPassword(u.Hash, password) {
			u.LastLogin = now()
			_ = s.saveLocked()
			p := u.Public()
			return &p, true
		}
		// A disabled or mistyped account does not fall through to the env admin
		// unless the name really is the env admin's.
		if key(username) != key(s.envUser) {
			return nil, false
		}
	}
	if s.envUser != "" && ctEqual(username, s.envUser) && ctEqual(password, s.envPass) {
		return &User{Username: s.envUser, Role: RoleAdmin}, true
	}
	return nil, false
}

// Get returns the enabled account with that name (or the env admin), as used to
// re-check a session on every request.
func (s *Store) Get(username string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if u := s.users[key(username)]; u != nil {
		if u.Disabled {
			return nil, false
		}
		p := u.Public()
		return &p, true
	}
	if s.envUser != "" && ctEqual(username, s.envUser) {
		return &User{Username: s.envUser, Role: RoleAdmin}, true
	}
	return nil, false
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
	if len(password) < minPasswordLen {
		return ErrWeakPassword
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
	s.users[key(username)] = &User{Username: username, Role: role, Created: now(), Hash: h}
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
	if len(password) < minPasswordLen {
		return ErrWeakPassword
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
	s.users[key(username)] = &User{Username: username, Role: RoleUser, Created: now(), Profile: p, Hash: h}
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
		u.Disabled = disabled
		return nil
	})
}

// SetPassword replaces an account's password.
func (s *Store) SetPassword(username, password string) error {
	if len(password) < minPasswordLen {
		return ErrWeakPassword
	}
	h, err := hashPassword(password)
	if err != nil {
		return err
	}
	return s.mutate(username, func(u *User) error { u.Hash = h; return nil })
}

// ChangeOwnPassword sets a new password after checking the current one.
func (s *Store) ChangeOwnPassword(username, oldPassword, newPassword string) error {
	s.mu.RLock()
	u := s.users[key(username)]
	ok := u != nil && !u.Disabled && verifyPassword(u.Hash, oldPassword)
	s.mu.RUnlock()
	if !ok {
		return errors.New("current password is incorrect")
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
	dk, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return "pbkdf2-sha256$" + strconv.Itoa(pbkdf2Iterations) + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(dk), nil
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
