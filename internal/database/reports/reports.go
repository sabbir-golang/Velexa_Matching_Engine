// Package reports keeps the daily report figures (see engine/daystats.go): in
// PostgreSQL (the daily_stats table) when the engine has a database, otherwise
// in daily_report.json in the data folder.
package reports

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"velexa-sim-me/internal/engine"
)

// Store saves and reads one day's counters per actor.
type Store interface {
	// Save replaces the stored counters of day with actors.
	Save(day string, actors map[string]engine.DayCounters) error
	// Load returns the stored counters of day (empty when nothing was stored).
	Load(day string) (map[string]engine.DayCounters, error)
	// Days returns up to limit days that have figures, newest first.
	Days(limit int) ([]string, error)
}

// ── file store ──────────────────────────────────────────────────────────────

// keepDays is how many days the file store keeps.
const keepDays = 400

// FileStore keeps every day in one JSON file, rewritten atomically.
type FileStore struct {
	mu   sync.Mutex
	path string
}

// NewFileStore uses the JSON file at path.
func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

func (f *FileStore) read() (map[string]map[string]engine.DayCounters, error) {
	all := map[string]map[string]engine.DayCounters{}
	b, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return all, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	return all, nil
}

func (f *FileStore) Save(day string, actors map[string]engine.DayCounters) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	all, err := f.read()
	if err != nil {
		return err
	}
	all[day] = actors
	if len(all) > keepDays {
		days := make([]string, 0, len(all))
		for d := range all {
			days = append(days, d)
		}
		sort.Strings(days)
		for _, d := range days[:len(days)-keepDays] {
			delete(all, d)
		}
	}
	b, err := json.MarshalIndent(all, "", " ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

func (f *FileStore) Load(day string) (map[string]engine.DayCounters, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all, err := f.read()
	if err != nil {
		return nil, err
	}
	if m := all[day]; m != nil {
		return m, nil
	}
	return map[string]engine.DayCounters{}, nil
}

func (f *FileStore) Days(limit int) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all, err := f.read()
	if err != nil {
		return nil, err
	}
	days := make([]string, 0, len(all))
	for d := range all {
		days = append(days, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	if len(days) > limit {
		days = days[:limit]
	}
	return days, nil
}

// ── PostgreSQL store ────────────────────────────────────────────────────────

// PGStore keeps the figures in the daily_stats table.
type PGStore struct{ pool *pgxpool.Pool }

// NewPGStore uses the daily_stats table of pool's database.
func NewPGStore(pool *pgxpool.Pool) *PGStore { return &PGStore{pool: pool} }

const pgTimeout = 5 * time.Second

func (p *PGStore) Save(day string, actors map[string]engine.DayCounters) error {
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for actor, c := range actors {
		if _, err := tx.Exec(ctx, `INSERT INTO daily_stats
			(day, actor, buy_orders, sell_orders, accepted, rejected, cancelled, amended, expired, trades, volume, turnover)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (day, actor) DO UPDATE SET
				buy_orders = EXCLUDED.buy_orders, sell_orders = EXCLUDED.sell_orders,
				accepted = EXCLUDED.accepted, rejected = EXCLUDED.rejected, cancelled = EXCLUDED.cancelled,
				amended = EXCLUDED.amended, expired = EXCLUDED.expired, trades = EXCLUDED.trades,
				volume = EXCLUDED.volume, turnover = EXCLUDED.turnover, updated_at = now()`,
			day, actor, c.BuyOrders, c.SellOrders, c.Accepted, c.Rejected, c.Cancelled, c.Amended,
			c.Expired, c.Trades, c.Volume, c.Turnover); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *PGStore) Load(day string) (map[string]engine.DayCounters, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, `SELECT actor, buy_orders, sell_orders, accepted, rejected, cancelled,
		amended, expired, trades, volume, turnover::float8 FROM daily_stats WHERE day = $1`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]engine.DayCounters{}
	for rows.Next() {
		var actor string
		var c engine.DayCounters
		if err := rows.Scan(&actor, &c.BuyOrders, &c.SellOrders, &c.Accepted, &c.Rejected, &c.Cancelled,
			&c.Amended, &c.Expired, &c.Trades, &c.Volume, &c.Turnover); err != nil {
			return nil, err
		}
		out[actor] = c
	}
	return out, rows.Err()
}

func (p *PGStore) Days(limit int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, `SELECT DISTINCT to_char(day, 'YYYY-MM-DD') AS d FROM daily_stats ORDER BY d DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Saver writes the engine's changed days to the store every interval until
// stop is closed, and once more at the end. A day that cannot be saved is kept
// and tried again on the next tick (with its newest figures).
func Saver(eng *engine.Engine, st Store, interval time.Duration, stop <-chan struct{}, logf func(string, ...any)) {
	pending := map[string]map[string]engine.DayCounters{}
	failing := false
	flush := func() {
		for _, d := range eng.PendingDays() {
			pending[d.Day] = d.Actors
		}
		for day, actors := range pending {
			if err := st.Save(day, actors); err != nil {
				if !failing {
					logf("[report] could not save the figures of %s: %v (retrying)", day, err)
				}
				failing = true
				continue
			}
			delete(pending, day)
		}
		if len(pending) == 0 && failing {
			logf("[report] saving works again")
			failing = false
		}
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			flush()
		case <-stop:
			flush()
			return
		}
	}
}
