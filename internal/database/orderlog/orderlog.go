// Package orderlog keeps one row per order — who sent it, for which client,
// what it was, and its latest state (filled, cancelled, rejected, …) — for the
// order report. The engine reports every order event to Recorder.Observe; the
// rows are saved every second to PostgreSQL (order_log) or, without a
// database, to one JSON file per day in the data folder.
package orderlog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velexa-sim-me/internal/engine"
)

// Record is one order as the report shows it.
type Record struct {
	Key     string    `json:"key"`
	Day     string    `json:"day"`
	Entered time.Time `json:"time"`
	Updated time.Time `json:"updated"`
	Seq     uint64    `json:"seq"`
	OrderID string    `json:"order_id"`
	Actor   string    `json:"actor"`
	ClOrdID string    `json:"cl_ord_id"`
	Client  string    `json:"client"`
	Account string    `json:"account"`
	Trader  string    `json:"trader"`
	Board   string    `json:"board"`
	Symbol  string    `json:"symbol"`
	Side    string    `json:"side"`
	Type    string    `json:"type"`
	TIF     string    `json:"tif"`
	Qty     int64     `json:"qty"`
	Price   float64   `json:"price"`
	Filled  int64     `json:"filled"`
	AvgPx   float64   `json:"avg_px"`
	Status  string    `json:"status"`
	Reason  string    `json:"reason,omitempty"`
}

// Store keeps the records.
type Store interface {
	// Save inserts or replaces records (by Key).
	Save(recs []Record) error
	// List returns a day's records, newest first.
	List(day string) ([]Record, error)
}

// Recorder turns engine order events into records. Today's records are kept in
// memory (the report reads them from there); changes are saved by Run.
type Recorder struct {
	mu      sync.Mutex
	store   Store
	run     string                    // this engine run, part of every key
	byOrder map[*engine.Order]*Record // live orders → their record
	today   map[string]*Record        // today's records by key
	day     string
	dirty   map[string]*Record
	rejects int
}

// NewRecorder loads today's records from st (so the report survives a restart).
func NewRecorder(st Store) (*Recorder, error) {
	r := &Recorder{store: st, run: strconv.FormatInt(time.Now().UnixNano(), 36),
		byOrder: map[*engine.Order]*Record{}, today: map[string]*Record{}, dirty: map[string]*Record{},
		day: engine.ReportDay(time.Now())}
	recs, err := st.List(r.day)
	if err != nil {
		return nil, err
	}
	for i := range recs {
		rec := recs[i]
		r.today[rec.Key] = &rec
	}
	return r, nil
}

// MaxSeq is the largest engine order number used today, so a restarted engine
// continues after it (engine.SetMinSeq) instead of reusing OrderIDs.
func (r *Recorder) MaxSeq() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n uint64
	for _, rec := range r.today {
		if rec.Seq > n {
			n = rec.Seq
		}
	}
	return n
}

// statusText maps an engine status to the report's wording.
func statusText(rep *engine.ExecReport, cum int64) string {
	switch rep.Status {
	case "NEW", "PENDING":
		if rep.Status == "PENDING" {
			return "Pending"
		}
		return "New"
	case "PARTIALLY_FILLED":
		return "Partial"
	case "FILLED":
		return "Filled"
	case "CANCELED":
		return "Cancelled"
	case "REJECTED":
		return "Rejected"
	case "EXPIRED":
		return "Expired"
	case "REPLACED":
		if cum > 0 {
			return "Partial"
		}
		return "New"
	}
	return ""
}

// boardNames are the readable names of the DSE board groups (tag 762).
var boardNames = map[string]string{
	"PUBLIC": "Public", "SPUBLIC": "SME Public", "ATBPUB": "ATB Public", "BLOCK": "Block", "SBLOCK": "SME Block",
	"BUYIN": "Buy-in", "SBUYIN": "SME Buy-in", "ATBBUYIN": "ATB Buy-in", "DEBT": "Debt", "SDEBT": "SME Debt",
	"ATBDEBT": "ATB Debt", "ALTDBT": "Alternative Debt", "BUYDBT": "Debt Buy-in", "YIELDDBT": "Yield Debt",
}

func boardName(v string) string {
	if n, ok := boardNames[strings.ToUpper(strings.TrimSpace(v))]; ok {
		return n
	}
	return v
}

func titleCase(s string) string {
	s = strings.ToLower(strings.ReplaceAll(string(s), "_", " "))
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// OrderID is the OrderID (37) the FIX gateway sends for an order: the Dhaka
// date followed by the engine's order number.
func OrderID(o *engine.Order) string {
	if o.ID == 0 {
		return ""
	}
	return o.Time.In(engine.Dhaka).Format("20060102") + strconv.FormatUint(o.ID, 10)
}

// Observe records one report about an order. It is the engine's OnOrderEvent:
// it runs on the matching goroutine and only updates memory.
func (r *Recorder) Observe(o *engine.Order, rep *engine.ExecReport) {
	if rep.Event == "STATUS" { // an order-status request changes nothing
		return
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if day := engine.ReportDay(now); day != r.day { // a new trading day
		r.day, r.today = day, map[string]*Record{}
	}
	rec := r.byOrder[o]
	if rec == nil {
		key := r.run + "-" + strconv.FormatUint(o.ID, 10)
		if o.ID == 0 {
			r.rejects++
			key = r.run + "-r" + strconv.Itoa(r.rejects)
		}
		entered := o.Time
		if entered.IsZero() {
			entered = now
		}
		rec = &Record{Key: key, Day: engine.ReportDay(entered), Entered: entered, Seq: o.ID, OrderID: OrderID(o),
			Actor: o.Actor, Client: o.Info.Client, Account: o.Info.Account, Trader: o.Info.Trader,
			Board: boardName(o.Info.Board), Symbol: o.Symbol, Side: titleCase(string(o.Side)), TIF: string(o.TIF)}
		r.byOrder[o] = rec
		r.today[key] = rec
	}
	if rec.Seq == 0 && o.ID != 0 {
		rec.Seq, rec.OrderID = o.ID, OrderID(o)
	}
	rec.Type = strings.ReplaceAll(string(o.Type), "_", " ") // LIMIT, MARKET, STOP LIMIT, … as on DSE screens
	if o.ClOrdID != "" {
		rec.ClOrdID = o.ClOrdID // follows amends (the newest ClOrdID)
	}
	rec.Qty, rec.Price = o.OrigQty, o.Price
	rec.Filled, rec.AvgPx = o.CumQty, 0
	if o.CumQty > 0 {
		rec.AvgPx = engine.Round2(o.CumValue / float64(o.CumQty))
	}
	if st := statusText(rep, o.CumQty); st != "" {
		rec.Status = st
	}
	if rep.Status == "REJECTED" {
		rec.Reason = rep.Reason
		rec.Qty = rep.OrderQty
	}
	rec.Updated = now
	r.dirty[rec.Key] = rec
	switch rec.Status { // finished: forget the order pointer (the record stays)
	case "Filled", "Cancelled", "Rejected", "Expired":
		delete(r.byOrder, o)
	}
}

// Today returns a copy of today's records, newest first, with the date.
func (r *Recorder) Today() (string, []Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Record, 0, len(r.today))
	for _, rec := range r.today {
		out = append(out, *rec)
	}
	sortNewestFirst(out)
	return r.day, out
}

// List returns a day's records: today's from memory, others from the store.
func (r *Recorder) List(day string) ([]Record, error) {
	if d, recs := r.Today(); d == day {
		return recs, nil
	}
	return r.store.List(day)
}

func sortNewestFirst(recs []Record) {
	sort.Slice(recs, func(i, j int) bool {
		if !recs[i].Entered.Equal(recs[j].Entered) {
			return recs[i].Entered.After(recs[j].Entered)
		}
		return recs[i].Seq > recs[j].Seq
	})
}

// flush saves the records changed since the last call; on failure they are
// kept and tried again.
func (r *Recorder) flush() error {
	r.mu.Lock()
	if len(r.dirty) == 0 {
		r.mu.Unlock()
		return nil
	}
	batch := make([]Record, 0, len(r.dirty))
	for _, rec := range r.dirty {
		batch = append(batch, *rec)
	}
	r.dirty = map[string]*Record{}
	r.mu.Unlock()
	if err := r.store.Save(batch); err != nil {
		r.mu.Lock()
		for i := range batch {
			if _, newer := r.dirty[batch[i].Key]; !newer {
				rec := batch[i]
				r.dirty[rec.Key] = &rec
			}
		}
		r.mu.Unlock()
		return err
	}
	return nil
}

// Run saves changes every interval until stop is closed, then once more.
func (r *Recorder) Run(interval time.Duration, stop <-chan struct{}, logf func(string, ...any)) {
	failing := false
	flush := func() {
		if err := r.flush(); err != nil {
			if !failing {
				logf("[orders] could not save the order log: %v (retrying)", err)
			}
			failing = true
		} else if failing {
			logf("[orders] saving the order log works again")
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

// ── file store: one JSON file per day ───────────────────────────────────────

// FileStore keeps each day's records in <dir>/orders-YYYY-MM-DD.json.
type FileStore struct {
	mu  sync.Mutex
	dir string
}

// NewFileStore keeps the files in dir.
func NewFileStore(dir string) *FileStore { return &FileStore{dir: dir} }

func (f *FileStore) path(day string) string { return filepath.Join(f.dir, "orders-"+day+".json") }

func (f *FileStore) read(day string) (map[string]Record, error) {
	m := map[string]Record{}
	b, err := os.ReadFile(f.path(day))
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Record
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("%s: %w", f.path(day), err)
	}
	for _, rec := range list {
		m[rec.Key] = rec
	}
	return m, nil
}

func (f *FileStore) Save(recs []Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	byDay := map[string][]Record{}
	for _, rec := range recs {
		byDay[rec.Day] = append(byDay[rec.Day], rec)
	}
	for day, list := range byDay {
		m, err := f.read(day)
		if err != nil {
			return err
		}
		for _, rec := range list {
			m[rec.Key] = rec
		}
		all := make([]Record, 0, len(m))
		for _, rec := range m {
			all = append(all, rec)
		}
		sortNewestFirst(all)
		b, err := json.Marshal(all)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(f.dir, 0o750); err != nil {
			return err
		}
		tmp := f.path(day) + ".tmp"
		if err := os.WriteFile(tmp, b, 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmp, f.path(day)); err != nil {
			return err
		}
	}
	return nil
}

func (f *FileStore) List(day string) ([]Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.read(day)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(m))
	for _, rec := range m {
		out = append(out, rec)
	}
	sortNewestFirst(out)
	return out, nil
}

// ── PostgreSQL store ────────────────────────────────────────────────────────

// PGStore keeps the records in the order_log table.
type PGStore struct{ pool *pgxpool.Pool }

// NewPGStore uses pool's order_log table.
func NewPGStore(pool *pgxpool.Pool) *PGStore { return &PGStore{pool: pool} }

const cols = `key, day, entered_at, updated_at, seq, order_id, actor, cl_ord_id, client, account, trader,
	board, symbol, side, ord_type, tif, qty, price, filled, avg_px, status, reason`

func (p *PGStore) Save(recs []Record) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b := &pgx.Batch{}
	for _, r := range recs {
		b.Queue(`INSERT INTO order_log (`+cols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
			ON CONFLICT (key) DO UPDATE SET updated_at = EXCLUDED.updated_at, seq = EXCLUDED.seq,
				order_id = EXCLUDED.order_id, cl_ord_id = EXCLUDED.cl_ord_id, ord_type = EXCLUDED.ord_type,
				qty = EXCLUDED.qty, price = EXCLUDED.price, filled = EXCLUDED.filled, avg_px = EXCLUDED.avg_px,
				status = EXCLUDED.status, reason = EXCLUDED.reason`,
			r.Key, r.Day, r.Entered, r.Updated, int64(r.Seq), r.OrderID, r.Actor, r.ClOrdID, r.Client, r.Account,
			r.Trader, r.Board, r.Symbol, r.Side, r.Type, r.TIF, r.Qty, r.Price, r.Filled, r.AvgPx, r.Status, r.Reason)
	}
	return p.pool.SendBatch(ctx, b).Close()
}

func (p *PGStore) List(day string) ([]Record, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := p.pool.Query(ctx, `SELECT key, to_char(day, 'YYYY-MM-DD'), entered_at, updated_at, seq, order_id,
		actor, cl_ord_id, client, account, trader, board, symbol, side, ord_type, tif, qty, price::float8,
		filled, avg_px::float8, status, reason FROM order_log WHERE day = $1 ORDER BY entered_at DESC, seq DESC`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		var seq int64
		if err := rows.Scan(&r.Key, &r.Day, &r.Entered, &r.Updated, &seq, &r.OrderID, &r.Actor, &r.ClOrdID,
			&r.Client, &r.Account, &r.Trader, &r.Board, &r.Symbol, &r.Side, &r.Type, &r.TIF, &r.Qty, &r.Price,
			&r.Filled, &r.AvgPx, &r.Status, &r.Reason); err != nil {
			return nil, err
		}
		r.Seq = uint64(seq)
		out = append(out, r)
	}
	return out, rows.Err()
}
