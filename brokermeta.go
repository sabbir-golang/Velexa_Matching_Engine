package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
)

// brokermeta.go — per-broker records for the admin panel: the human/company
// identity that ties a FIX SenderCompID to its ITCH user, so credentials are
// traceable and each broker gets a dynamic detail + onboarding view. The actual
// secrets stay in fix_brokers.json / itch_brokers.json (the auth allowlists);
// this store only holds the linking metadata.

type BrokerRec struct {
	Company  string `json:"company"`
	Sender   string `json:"sender"`    // FIX SenderCompID (key)
	ItchUser string `json:"itch_user"` // linked ITCH username
	Contact  string `json:"contact"`   // optional technical contact
	Created  string `json:"created"`   // human timestamp
}

type brokerMeta struct {
	mu   sync.RWMutex
	path string
	recs map[string]*BrokerRec // sender (upper) → rec
}

var meBrokers = &brokerMeta{recs: map[string]*BrokerRec{}}

func loadBrokerMeta() {
	meBrokers.path = envOr("ME_BROKER_META_FILE", "/opt/velexa-sim-me/broker_meta.json")
	if b, err := os.ReadFile(meBrokers.path); err == nil {
		var m map[string]*BrokerRec
		if json.Unmarshal(b, &m) == nil {
			meBrokers.mu.Lock()
			for k, v := range m {
				meBrokers.recs[strings.ToUpper(strings.TrimSpace(k))] = v
			}
			meBrokers.mu.Unlock()
		}
	}
}

func (m *brokerMeta) get(sender string) *BrokerRec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.recs[strings.ToUpper(strings.TrimSpace(sender))]
}

func (m *brokerMeta) add(r *BrokerRec) {
	r.Sender = strings.ToUpper(strings.TrimSpace(r.Sender))
	m.mu.Lock()
	m.recs[r.Sender] = r
	m.mu.Unlock()
	m.save()
}

func (m *brokerMeta) del(sender string) {
	m.mu.Lock()
	delete(m.recs, strings.ToUpper(strings.TrimSpace(sender)))
	m.mu.Unlock()
	m.save()
}

// list returns the managed broker records, newest-created first.
func (m *brokerMeta) list() []*BrokerRec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*BrokerRec, 0, len(m.recs))
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
