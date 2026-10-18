package users

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// backend is where accounts are kept: the users.json file, or PostgreSQL when
// ME_DB_URL is set. The Store holds every account in memory and writes each
// change through to its backend before the change counts.
type backend interface {
	// load returns every account (nil, nil when there is nothing stored yet).
	load() ([]*User, error)
	// save persists the account with key k after a change; all[k] == nil means
	// it was deleted. all is the complete set after the change.
	save(all map[string]*User, k string) error
	// describe names the storage for logs.
	describe() string
}

// fileBackend keeps the accounts in one JSON file, rewritten atomically.
type fileBackend struct{ path string }

func (f *fileBackend) describe() string { return f.path }

func (f *fileBackend) load() ([]*User, error) {
	b, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*User
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (f *fileBackend) save(all map[string]*User, _ string) error {
	list := make([]*User, 0, len(all))
	for _, u := range all {
		list = append(list, u)
	}
	sort.Slice(list, func(i, j int) bool { return key(list[i].Username) < key(list[j].Username) })
	b, err := json.MarshalIndent(list, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o750); err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}
