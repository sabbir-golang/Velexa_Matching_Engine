package engine

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// An orderbook id must never change for a symbol: the ITCH engine and the OMS
// key their order books, prices and depth on it. Ids used to be "base + position
// in the list", which is stable while the list only grows in memory but not
// across a restart: the saved master is sorted by symbol, so a newly listed
// instrument moved into the middle and every later symbol got the next
// instrument's id (wrong prices on the wrong tickers). obIDStore keeps the
// symbol -> id assignment in a file instead.
type obIDStore struct {
	path string
	ids  map[string]int // every id ever handed out, including delisted symbols
	next int            // highest id handed out so far
}

func (s *obIDStore) save() {
	b, err := json.MarshalIndent(s.ids, "", " ")
	if err == nil {
		if err = os.MkdirAll(filepath.Dir(s.path), 0o750); err == nil {
			tmp := s.path + ".tmp"
			if err = os.WriteFile(tmp, b, 0o640); err == nil {
				err = os.Rename(tmp, s.path)
			}
		}
	}
	if err != nil {
		log.Printf("[universe] could not save orderbook ids to %s: %v", s.path, err)
	}
}

// LoadOrderBookIDs makes the symbol -> orderbook id mapping permanent, kept in
// the file at path. Symbols already in the file keep their id; new symbols get
// the next free one. With no file yet, ids are the list position, exactly as
// before, and are saved from then on. Call it once at start-up, before the
// feeds and gateways read OrderBookID.
func (e *Engine) LoadOrderBookIDs(path string) {
	if path == "" {
		return
	}
	st := &obIDStore{path: path, ids: map[string]int{}, next: obIDBase - 1}
	fresh := true
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &st.ids) == nil {
			fresh = false
		} else {
			st.ids = map[string]int{}
		}
	}
	for _, id := range st.ids {
		if id > st.next {
			st.next = id
		}
	}
	obs := map[string]int{}
	for i, in := range e.Instr() {
		id, ok := st.ids[in.Symbol]
		switch {
		case ok:
		case fresh:
			id = obIDBase + i
		default:
			id = st.next + 1
		}
		st.ids[in.Symbol] = id
		if id > st.next {
			st.next = id
		}
		obs[in.Symbol] = id
	}
	e.obStore = st
	e.obIDs.Store(&obs)
	st.save()
}

// newOrderBookID is the id for a symbol being listed now; pos is its position in
// the list, used only when the ids are not kept in a file.
func (e *Engine) newOrderBookID(sym string, pos int) int {
	st := e.obStore
	if st == nil {
		return obIDBase + pos
	}
	if id, ok := st.ids[sym]; ok { // listed before, delisted, listed again
		return id
	}
	st.next++
	st.ids[sym] = st.next
	st.save()
	return st.next
}
