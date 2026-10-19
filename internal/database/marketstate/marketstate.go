// Package marketstate keeps the live market across restarts: the engine's
// snapshot (the order books in queue order, prices, the tape, counters and
// phase), the FIX reports queued for brokers that are logged off, and the
// permanent trade log with both sides of every execution. Saver writes
// changes every second and once more at shutdown; on start the server loads
// the snapshot and restores it before matching begins.
//
// With a database (ME_DB_URL) everything is in PostgreSQL (engine_state,
// trade_log); without one, in the data folder: engine_state.json,
// fix_outbox.json and trades/trades-YYYY-MM-DD.jsonl.
package marketstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"velexa-sim-me/internal/engine"
)

// Names of the saved states.
const (
	Market = "market"     // the engine snapshot
	Outbox = "fix_outbox" // FIX reports waiting for a broker to log on
)

// ErrCorrupt is returned by LoadSnapshot when the saved state can't be read.
var ErrCorrupt = errors.New("the saved market state is unreadable")

// Store keeps the saved states and the trade log.
type Store interface {
	// SaveState replaces a saved state (Market, Outbox).
	SaveState(name string, payload []byte) error
	// LoadState returns a saved state, or nil when there is none.
	LoadState(name string) ([]byte, error)
	// SetAside keeps an unreadable state under another name and removes it,
	// so the engine can start; it returns where it went.
	SetAside(name string) (string, error)
	// AppendTrades adds executions to the trade log. Adding one that is
	// already there (a retry) has no effect.
	AppendTrades(trades []engine.TradeRecord) error
	// Trades returns a day's executions, oldest first.
	Trades(day string) ([]engine.TradeRecord, error)
	// MaxTradeID is the largest trade number in the log.
	MaxTradeID() (uint64, error)
}

// LoadSnapshot reads the saved market. It returns nil, nil when there is
// none, and an error wrapping ErrCorrupt when it can't be used.
func LoadSnapshot(st Store) (*engine.Snapshot, error) {
	b, err := st.LoadState(Market)
	if err != nil || b == nil {
		return nil, err
	}
	var s engine.Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if s.Version != engine.SnapshotVersion {
		return nil, fmt.Errorf("%w: layout version %d, this engine reads %d", ErrCorrupt, s.Version, engine.SnapshotVersion)
	}
	return &s, nil
}

// Saver writes the market state, the outbox and new trades every interval.
type Saver struct {
	eng   *engine.Engine
	store Store

	mu     sync.Mutex
	trades []engine.TradeRecord // executions not yet in the trade log
	day    string               // today's trades, kept in memory for the dashboard
	today  []engine.TradeRecord

	// Outbox, when set, returns the FIX outbox to save and whether it changed
	// since the last call.
	Outbox func() (json.RawMessage, bool)
	outbox json.RawMessage // an outbox whose save failed, tried again

	saved atomic.Int64 // unix nanoseconds when the saved market was last known current
}

// NewSaver saves eng's state to st. Attach OnTrade to the engine before Run.
func NewSaver(eng *engine.Engine, st Store) *Saver { return &Saver{eng: eng, store: st} }

// OnTrade is the engine's OnTrade hook: it queues an execution for the trade
// log. It runs on the matching goroutine and only appends to memory.
func (s *Saver) OnTrade(tr engine.TradeRecord) {
	s.mu.Lock()
	s.trades = append(s.trades, tr)
	if d := engine.ReportDay(tr.Time); d != s.day {
		s.day, s.today = d, nil
	}
	s.today = append(s.today, tr)
	s.mu.Unlock()
}

// SeedToday loads today's executions from the trade log into memory, so
// Trades answers for today without reading the log. Call it before the
// engine runs.
func (s *Saver) SeedToday() error {
	day := engine.ReportDay(time.Now())
	saved, err := s.store.Trades(day)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.day, s.today = day, saved
	s.mu.Unlock()
	return nil
}

// Trades returns a day's executions, oldest first: today's from memory,
// other days from the trade log.
func (s *Saver) Trades(day string) ([]engine.TradeRecord, error) {
	s.mu.Lock()
	if day == s.day {
		out := append([]engine.TradeRecord(nil), s.today...)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()
	saved, err := s.store.Trades(day)
	if err != nil {
		return nil, err
	}
	seen := make(map[uint64]bool, len(saved))
	for _, t := range saved {
		seen[t.ID] = true
	}
	s.mu.Lock()
	for _, t := range s.trades { // not written yet
		if !seen[t.ID] && engine.ReportDay(t.Time) == day {
			saved = append(saved, t)
		}
	}
	s.mu.Unlock()
	sort.Slice(saved, func(i, j int) bool { return saved[i].ID < saved[j].ID })
	return saved, nil
}

// LastSaved is when the saved market was last known to match the live one:
// its last save, or the last check that found nothing changed (zero: not yet).
func (s *Saver) LastSaved() time.Time {
	if n := s.saved.Load(); n != 0 {
		return time.Unix(0, n)
	}
	return time.Time{}
}

// Pending is the number of executions waiting for the trade log.
func (s *Saver) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.trades)
}

// Flush saves what changed: the snapshot (force: even when unchanged), the
// executions in it and since, and the outbox. Whatever fails is tried again
// on the next call.
func (s *Saver) Flush(force bool) error {
	var errs []error
	// The snapshot is taken first: every trade it shows was queued before it,
	// so the trade log written next always covers it.
	checked := time.Now()
	if snap := s.eng.TakeSnapshot(force); snap == nil {
		if s.saved.Load() != 0 { // unchanged since a successful save: still current
			s.saved.Store(checked.UnixNano())
		}
	} else {
		b, err := json.Marshal(snap)
		if err == nil {
			err = s.store.SaveState(Market, b)
		}
		if err != nil {
			s.eng.MarkDirty() // take and save it again next time
			errs = append(errs, fmt.Errorf("market state: %w", err))
		} else {
			s.saved.Store(snap.Taken.UnixNano())
		}
	}
	s.mu.Lock()
	batch := s.trades
	s.trades = nil
	s.mu.Unlock()
	if len(batch) > 0 {
		if err := s.store.AppendTrades(batch); err != nil {
			s.mu.Lock()
			s.trades = append(batch, s.trades...)
			s.mu.Unlock()
			errs = append(errs, fmt.Errorf("trade log: %w", err))
		}
	}
	if s.Outbox != nil {
		if b, changed := s.Outbox(); changed {
			s.outbox = b
		}
		if s.outbox != nil {
			if err := s.store.SaveState(Outbox, s.outbox); err != nil {
				errs = append(errs, fmt.Errorf("FIX outbox: %w", err))
			} else {
				s.outbox = nil
			}
		}
	}
	return errors.Join(errs...)
}

// Run saves every interval until stop is closed, then once more in full.
func (s *Saver) Run(interval time.Duration, stop <-chan struct{}, logf func(string, ...any)) {
	failing := false
	flush := func(force bool) {
		if err := s.Flush(force); err != nil {
			if !failing {
				logf("[state] could not save: %v (retrying every %s)", err, interval)
			}
			failing = true
		} else if failing {
			logf("[state] saving works again")
			failing = false
		}
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			flush(false)
		case <-stop:
			flush(true)
			if failing { // one more go: this is the last chance before the process exits
				time.Sleep(500 * time.Millisecond)
				flush(true)
			}
			return
		}
	}
}
