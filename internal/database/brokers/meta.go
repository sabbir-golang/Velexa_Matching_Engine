// Package brokers holds the broker stores: the FIX and ITCH logon allowlists and the broker metadata (company, CompID and ITCH user) shown in the admin console.
package brokers

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"

	"velexa-sim-me/internal/config"
)

// brokermeta.go — per-broker records for the admin panel: the human/company
// identity that ties a FIX SenderCompID to its ITCH user, so credentials are
// traceable and each broker gets a dynamic detail + onboarding view. The actual
// secrets stay in fix_brokers.json / itch_brokers.json (the auth allowlists);
// this store only holds the linking metadata.

type Record struct {
	Company  string `json:"company"`
	Sender   string `json:"sender"`    // FIX SenderCompID (key)
	ItchUser string `json:"itch_user"` // linked ITCH username
	Contact  string `json:"contact"`   // optional technical contact
	Created  string `json:"created"`   // human timestamp
	// Owner is the console user who registered the broker. Empty means it
	// belongs to the exchange (admins): normal users never see it.
	Owner string `json:"owner,omitempty"`
}

// OwnedBy reports whether the record belongs to the console user username.
func (r *Record) OwnedBy(username string) bool {
	return r != nil && r.Owner != "" && strings.EqualFold(r.Owner, username)
}

type brokerMeta struct {
	mu   sync.RWMutex
	path string
	recs map[string]*Record // sender (upper) → rec
}

var Meta = &brokerMeta{recs: map[string]*Record{}}

func LoadMeta() {
	Meta.path = config.EnvOr("ME_BROKER_META_FILE", config.StateFile("broker_meta.json"))
	if b, err := os.ReadFile(Meta.path); err == nil {
		var m map[string]*Record
		if json.Unmarshal(b, &m) == nil {
			Meta.mu.Lock()
			for k, v := range m {
				Meta.recs[strings.ToUpper(strings.TrimSpace(k))] = v
			}
			Meta.mu.Unlock()
		}
	}
}

func (m *brokerMeta) Get(sender string) *Record {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.recs[strings.ToUpper(strings.TrimSpace(sender))]
}

func (m *brokerMeta) Add(r *Record) {
	r.Sender = strings.ToUpper(strings.TrimSpace(r.Sender))
	m.mu.Lock()
	m.recs[r.Sender] = r
	m.mu.Unlock()
	m.save()
}

func (m *brokerMeta) Del(sender string) {
	m.mu.Lock()
	delete(m.recs, strings.ToUpper(strings.TrimSpace(sender)))
	m.mu.Unlock()
	m.save()
}

// ReleaseOwner hands every broker owned by username back to the exchange (no
// owner), so a later account with the same name cannot inherit them. It returns
// how many brokers were released.
func (m *brokerMeta) ReleaseOwner(username string) int {
	m.mu.Lock()
	n := 0
	for _, r := range m.recs {
		if r.OwnedBy(username) {
			r.Owner = ""
			n++
		}
	}
	m.mu.Unlock()
	if n > 0 {
		m.save()
	}
	return n
}

// ItchUserTaken reports whether an ITCH username is already linked to a broker.
func (m *brokerMeta) ItchUserTaken(user string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.recs {
		if strings.EqualFold(r.ItchUser, user) {
			return true
		}
	}
	return false
}

// ListOwnedBy returns the records belonging to username, newest first.
func (m *brokerMeta) ListOwnedBy(username string) []*Record {
	out := []*Record{}
	for _, r := range m.List() {
		if r.OwnedBy(username) {
			out = append(out, r)
		}
	}
	return out
}

// List returns the managed broker records, newest-created first.
func (m *brokerMeta) List() []*Record {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Record, 0, len(m.recs))
	for _, r := range m.recs {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out
}

func (m *brokerMeta) save() {
	m.mu.RLock()
	b, _ := json.MarshalIndent(m.recs, "", " ")
	path := m.path
	m.mu.RUnlock()
	if path != "" {
		os.WriteFile(path, b, 0o600)
	}
}
