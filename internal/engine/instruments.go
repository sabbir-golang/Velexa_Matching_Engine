package engine

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"velexa-sim-me/internal/config"
)

// ---- auto-listing ---------------------------------------------------------

// AutoListEnabled reports whether an order for an unlisted symbol should open a
// book instead of being rejected. On by default: this is a simulator standing in
// for the exchange, and an OMS whose security master is ahead of the ME's should
// still be able to trade. Set ME_AUTO_LIST=off to enforce the master strictly.
func AutoListEnabled() bool {
	switch strings.ToLower(config.EnvOr("ME_AUTO_LIST", "on")) {
	case "off", "0", "false", "no":
		return false
	}
	return true
}

var (
	autoListMu   sync.Mutex
	autoListedAt = map[string]time.Time{}
)

func noteAutoListed(sym string) {
	autoListMu.Lock()
	autoListedAt[sym] = time.Now()
	autoListMu.Unlock()
}

// AutoListedSymbols returns the symbols this process opened books for on the
// fly. A non-empty list means the loaded security master is behind the OMS's —
// the admin console surfaces it as exactly that.
func AutoListedSymbols() []string {
	autoListMu.Lock()
	defer autoListMu.Unlock()
	out := make([]string, 0, len(autoListedAt))
	for s := range autoListedAt {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// AutoList opens a book for a symbol that is not in the loaded master, seeding
// its reference price from the order that asked for it (so the price band is
// anchored somewhere sane rather than rejecting the very order that listed it).
// Returns false if auto-listing is off or the symbol is not a plausible ticker.
func (e *Engine) AutoList(sym string, seedPx float64) bool {
	if !AutoListEnabled() || !ValidSymbol(sym) {
		return false
	}
	if seedPx <= 0 {
		seedPx = 100
	}
	seedPx = Round2(seedPx)
	in := Instrument{Symbol: sym, Name: sym, Prev: seedPx}
	added, _ := e.ListInstruments(UniverseSet{
		Instr: []Instrument{in},
		Meta:  map[string]InstrMeta{sym: {Board: "MAIN", Tick: DefaultTick(seedPx)}},
	})
	if added == 0 {
		return e.Listed(sym) // lost a race with a concurrent listing — still fine
	}
	noteAutoListed(sym)
	log.Printf("[universe] AUTO-LISTED %s at %.2f — it is not in the loaded security master (%s). Import the real master to get its true previous close and tick.",
		sym, seedPx, UniverseSource())
	return true
}

// ---- applying a master to a running engine --------------------------------

// ListInstruments applies a security master to the running engine and returns
// how many instruments it added and refreshed. Safe to call at any time.
func (e *Engine) ListInstruments(u UniverseSet) (added, updated int) {
	u.apply() // metadata first: the pre-trade checks read it the moment a book exists
	type result struct {
		added, updated int
		fresh          []string
	}
	done := make(chan result, 1)
	e.ctrl <- func() {
		a, up, fresh := e.listInstruments(u.Instr)
		done <- result{a, up, fresh}
	}
	r := <-done
	e.startBotsFor(r.fresh)
	return r.added, r.updated
}

// listInstruments rebuilds the books/market-data/instrument snapshots with the
// given instruments folded in. Runs on the engine's control goroutine with the
// engine lock held; the containers are swapped copy-on-write so the lock-free
// readers (ITCH feed, gateway, bench) never observe a half-built map.
func (e *Engine) listInstruments(ins []Instrument) (added, updated int, fresh []string) {
	books := *e.booksMap.Load()
	md := *e.mdMap.Load()
	list := e.Instr()

	nb := make(map[string]*Book, len(books)+len(ins))
	for k, v := range books {
		nb[k] = v
	}
	nm := make(map[string]*MarketData, len(md)+len(ins))
	for k, v := range md {
		nm[k] = v
	}
	nl := append(make([]Instrument, 0, len(list)+len(ins)), list...)
	pos := make(map[string]int, len(nl))
	for i, in := range nl {
		pos[in.Symbol] = i
	}
	obs := make(map[string]int, len(nl)+len(ins))
	for k, v := range *e.obIDs.Load() {
		obs[k] = v
	}

	for _, in := range ins {
		in.Symbol = strings.ToUpper(strings.TrimSpace(in.Symbol))
		if !ValidSymbol(in.Symbol) {
			continue
		}
		if in.Prev <= 0 {
			in.Prev = 100
		}
		if in.Name == "" {
			in.Name = in.Symbol
		}
		cur := nm[in.Symbol]
		if cur == nil { // newly listed scrip
			nb[in.Symbol] = NewBook(in.Symbol)
			nm[in.Symbol] = newMarketData(in)
			obs[in.Symbol] = e.newOrderBookID(in.Symbol, len(nl))
			pos[in.Symbol] = len(nl)
			nl = append(nl, in)
			fresh = append(fresh, in.Symbol)
			added++
			continue
		}
		// Already listed: refresh the reference data in place. An instrument
		// that has not traded yet is re-seeded to the new previous close; one
		// that has is left on its traded prices, but still re-anchors its band.
		cur.Name = in.Name
		if cur.Prev != in.Prev {
			cur.Prev = in.Prev
			if cur.Trades == 0 {
				cur.Open, cur.Last, cur.High, cur.Low, cur.Ref = in.Prev, in.Prev, in.Prev, in.Prev, in.Prev
			}
		}
		nl[pos[in.Symbol]] = in
		updated++
	}
	if added == 0 && updated == 0 {
		return 0, 0, nil
	}
	// Order matters: instrList is published last, so a lock-free reader that
	// iterates the instrument list and then looks a symbol up always finds the
	// books and market data for it (both maps only ever grow).
	e.booksMap.Store(&nb)
	e.mdMap.Store(&nm)
	e.obIDs.Store(&obs)
	e.instrList.Store(&nl)
	return added, updated, fresh
}

// startBotsFor gives newly listed instruments the same market-maker + noise
// trader every boot-time instrument gets, so a new book is quotable immediately
// instead of sitting empty (which would reject every market order against it).
func (e *Engine) startBotsFor(syms []string) {
	for i, sym := range syms {
		seed := time.Now().UnixNano() + int64(i)
		go e.marketMaker(sym, rand.New(rand.NewSource(seed)))
		go e.noiseTrader(sym, rand.New(rand.NewSource(seed+1000)))
	}
}

// SaveInstrumentFile persists an imported master so it survives a restart. The
// write is atomic (temp file + rename) so a crash mid-write can never leave the
// ME booting from a truncated master.
func SaveInstrumentFile(body []byte) error {
	path := strings.TrimSpace(InstrumentFile())
	if path == "" {
		return fmt.Errorf("ME_INSTRUMENT_FILE is empty — nowhere to persist")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// InstrumentStatus is the universe summary carried on /api/status and the admin
// dashboard, so a stale master is visible without reading the log.
func InstrumentStatus(eng *Engine) map[string]any {
	return map[string]any{
		"count":       len(eng.Instr()),
		"source":      UniverseSource(),
		"auto_list":   AutoListEnabled(),
		"auto_listed": len(AutoListedSymbols()),
	}
}
