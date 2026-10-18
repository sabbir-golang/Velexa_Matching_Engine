// Package audit is the console's audit trail: who issued or revoked broker
// credentials, changed accounts, controlled sessions, signed in, and so on.
// Entries go to PostgreSQL (the audit_log table) when the engine has a
// database, otherwise to admin_audit.jsonl with the newest 300 kept in memory.
package audit

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"velexa-sim-me/internal/config"
)

// Entry is one audited action.
type Entry struct {
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"`  // "alice (10.0.0.5)"
	Action string    `json:"action"` // ISSUE, REVOKE, user.add, login.ok, ...
	Target string    `json:"target"`
}

// sink is where entries are kept.
type sink interface {
	record(e Entry)
	recent(n int) []Entry
	recentFor(actorPrefix string, n int) []Entry
}

var (
	mu  sync.RWMutex
	cur sink = newFileSink()

	// Path is the JSON-lines file used without a database, and as the fallback
	// when a database insert fails (so no entry is lost).
	Path = config.EnvOr("ME_AUDIT_FILE", config.StateFile("admin_audit.jsonl"))
)

func current() sink {
	mu.RLock()
	defer mu.RUnlock()
	return cur
}

func use(s sink) {
	mu.Lock()
	cur = s
	mu.Unlock()
}

// Record adds an entry.
func Record(actor, action, target string) {
	current().record(Entry{Time: time.Now(), Actor: actor, Action: action, Target: target})
}

// Recent returns up to n entries, newest first.
func Recent(n int) []Entry { return current().recent(n) }

// RecentFor returns up to n entries, newest first, whose actor starts with
// actorPrefix (case-insensitive): a user's own entries are "alice (".
func RecentFor(actorPrefix string, n int) []Entry { return current().recentFor(actorPrefix, n) }

// Load switches to the JSON-lines file at Path and reads its newest entries
// (the store used when no database is configured).
func Load() {
	f := newFileSink()
	f.load(Path)
	use(f)
}

// ── file sink ───────────────────────────────────────────────────────────────

const ringSize = 300

type fileSink struct {
	mu   sync.Mutex
	ring []Entry
}

func newFileSink() *fileSink { return &fileSink{} }

func (f *fileSink) record(e Entry) {
	f.mu.Lock()
	f.ring = append(f.ring, e)
	if len(f.ring) > ringSize {
		f.ring = f.ring[len(f.ring)-ringSize:]
	}
	f.mu.Unlock()
	appendFile(Path, e)
}

func (f *fileSink) recent(n int) []Entry { return f.recentFor("", n) }

func (f *fileSink) recentFor(prefix string, n int) []Entry {
	prefix = strings.ToLower(prefix)
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []Entry{}
	for i := len(f.ring) - 1; i >= 0 && len(out) < n; i-- {
		if prefix == "" || strings.HasPrefix(strings.ToLower(f.ring[i].Actor), prefix) {
			out = append(out, f.ring[i])
		}
	}
	return out
}

func (f *fileSink) load(path string) {
	entries, _ := readFile(path)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ring = entries
	if len(f.ring) > ringSize {
		f.ring = f.ring[len(f.ring)-ringSize:]
	}
}

// appendFile adds one entry to a JSON-lines file (mode 0600).
func appendFile(path string, e Entry) {
	if fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		b, _ := json.Marshal(e)
		fh.Write(append(b, '\n'))
		fh.Close()
	}
}

// readFile reads every entry of a JSON-lines file, oldest first.
func readFile(path string) ([]Entry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var e Entry
		if line != "" && json.Unmarshal([]byte(line), &e) == nil {
			out = append(out, e)
		}
	}
	return out, nil
}
