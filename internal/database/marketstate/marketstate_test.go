package marketstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"velexa-sim-me/internal/database/db/dbtest"
	"velexa-sim-me/internal/engine"
)

func trade(id uint64, at time.Time) engine.TradeRecord {
	return engine.TradeRecord{
		Trade: engine.Trade{ID: id, Symbol: "GP", Price: 370, Qty: 10, Aggressor: engine.Buy, Time: at},
		Buy:   engine.TradeSide{Actor: "FIX:A", OrderID: id * 2, ClOrdID: "B"},
		Sell:  engine.TradeSide{Actor: "FIX:B", OrderID: id*2 + 1, ClOrdID: "S"},
	}
}

// storeContract is what every Store must do.
func storeContract(t *testing.T, st Store) {
	if b, err := st.LoadState(Market); err != nil || b != nil {
		t.Fatalf("empty store: %q %v", b, err)
	}
	if err := st.SaveState(Market, []byte(`{"version":1,"phase":"A"}`)); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveState(Market, []byte(`{"version":1,"phase":"B"}`)); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSnapshot(st)
	if err != nil || s == nil || s.Phase != "B" {
		t.Fatalf("load: %+v %v", s, err)
	}

	// Unreadable: reported as corrupt, and set aside so the engine can start.
	if err := st.SaveState(Market, []byte(`{"version":99}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSnapshot(st); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("another layout version: %v, want ErrCorrupt", err)
	}
	if _, err := st.SetAside(Market); err != nil {
		t.Fatal(err)
	}
	if s, err := LoadSnapshot(st); s != nil || err != nil {
		t.Fatalf("after set aside: %+v %v", s, err)
	}

	// Trade log: appends, ignores repeats, lists per day in order.
	day1 := time.Date(2026, 10, 18, 10, 0, 0, 0, engine.Dhaka)
	day2 := day1.AddDate(0, 0, 1)
	if err := st.AppendTrades([]engine.TradeRecord{trade(1, day1), trade(2, day1), trade(3, day2)}); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendTrades([]engine.TradeRecord{trade(2, day1), trade(4, day2)}); err != nil { // 2 is a retry
		t.Fatal(err)
	}
	got, err := st.Trades("2026-10-18")
	if err != nil || len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("day 1 trades %+v %v", got, err)
	}
	got, err = st.Trades("2026-10-19")
	if err != nil || len(got) != 2 || got[1].ID != 4 || got[1].Sell.Actor != "FIX:B" || got[1].Buy.OrderID != 8 ||
		got[1].Price != 370 || got[1].Aggressor != engine.Buy || !got[1].Time.Equal(day2) {
		t.Fatalf("day 2 trades %+v %v", got, err)
	}
	if n, err := st.MaxTradeID(); err != nil || n != 4 {
		t.Fatalf("max trade id %d %v", n, err)
	}
}

func TestFileStore(t *testing.T) {
	dir := t.TempDir()
	st := NewFileStore(dir, filepath.Join(dir, "trades"))
	storeContract(t, st)
	if bad, _ := filepath.Glob(filepath.Join(dir, "engine_state.json.bad-*")); len(bad) != 1 {
		t.Fatalf("the unreadable state was not kept: %v", bad)
	}
}

// A crash while appending leaves a torn last line: it is skipped, and the
// next append still works.
func TestFileTradeLogSurvivesATornLine(t *testing.T) {
	dir := t.TempDir()
	st := NewFileStore(dir, dir)
	at := time.Date(2026, 10, 19, 11, 0, 0, 0, engine.Dhaka)
	if err := st.AppendTrades([]engine.TradeRecord{trade(1, at)}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(filepath.Join(dir, "trades-2026-10-19.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"id":2,"sym`)
	f.Close()
	st = NewFileStore(dir, dir) // a restart
	if err := st.AppendTrades([]engine.TradeRecord{trade(2, at)}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Trades("2026-10-19")
	if err != nil || len(got) != 2 || got[1].ID != 2 {
		t.Fatalf("trades after a torn line %+v %v", got, err)
	}
}

func TestPGStore(t *testing.T) {
	pool, _ := dbtest.Open(t)
	storeContract(t, NewPGStore(pool))
}

// flakyStore fails until told otherwise.
type flakyStore struct {
	*FileStore
	mu   sync.Mutex
	fail bool
}

func (f *flakyStore) failing() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.fail }

func (f *flakyStore) SaveState(name string, b []byte) error {
	if f.failing() {
		return errors.New("disk full")
	}
	return f.FileStore.SaveState(name, b)
}

func (f *flakyStore) AppendTrades(tr []engine.TradeRecord) error {
	if f.failing() {
		return errors.New("disk full")
	}
	return f.FileStore.AppendTrades(tr)
}

func submit(e *engine.Engine, o *engine.Order) *engine.ExecReport { return e.SubmitAndWait(o) }

// The saver writes the market and its trades; what fails is written later.
func TestSaverRetries(t *testing.T) {
	dir := t.TempDir()
	st := &flakyStore{FileStore: NewFileStore(dir, dir), fail: true}
	e := engine.NewEngine([]engine.Instrument{{Symbol: "GP", Prev: 10}}, engine.NewBus())
	sv := NewSaver(e, st)
	e.OnTrade = sv.OnTrade
	box, boxChanged := json.RawMessage(`[{"broker":"A"}]`), true
	sv.Outbox = func() (json.RawMessage, bool) { c := boxChanged; boxChanged = false; return box, c }
	go e.Run()

	sell := e.NewOrder("GP", engine.Sell, engine.Limit, 10, 100, "FIX:A")
	sell.ClOrdID = "S1"
	submit(e, sell)
	buy := e.NewOrder("GP", engine.Buy, engine.Limit, 10, 40, "FIX:B")
	buy.ClOrdID = "B1"
	submit(e, buy)

	if err := sv.Flush(false); err == nil {
		t.Fatal("a failing store reported no error")
	}
	if sv.Pending() != 1 || !sv.LastSaved().IsZero() {
		t.Fatalf("after a failed save: pending %d, saved %v", sv.Pending(), sv.LastSaved())
	}
	st.mu.Lock()
	st.fail = false
	st.mu.Unlock()
	if err := sv.Flush(false); err != nil { // nothing changed in the engine: the failed snapshot is retried anyway
		t.Fatal(err)
	}
	if sv.Pending() != 0 || sv.LastSaved().IsZero() {
		t.Fatalf("after the retry: pending %d, saved %v", sv.Pending(), sv.LastSaved())
	}
	snap, err := LoadSnapshot(st)
	if err != nil || snap == nil || len(snap.Orders) != 1 || snap.Orders[0].Qty != 60 || snap.TradeSeq != 1 {
		t.Fatalf("saved snapshot %+v %v", snap, err)
	}
	trades, _ := st.Trades(engine.ReportDay(time.Now()))
	if len(trades) != 1 || trades[0].Buy.ClOrdID != "B1" || trades[0].Sell.ClOrdID != "S1" {
		t.Fatalf("trade log %+v", trades)
	}
	if b, _ := st.LoadState(Outbox); string(b) != string(box) {
		t.Fatalf("outbox %s", b)
	}
	// A quiet market is not written again, but the saved one is still current.
	before := sv.LastSaved()
	time.Sleep(5 * time.Millisecond)
	if err := sv.Flush(false); err != nil {
		t.Fatal(err)
	}
	if !sv.LastSaved().After(before) {
		t.Fatalf("an unchanged market looks stale: last saved %v, before %v", sv.LastSaved(), before)
	}
}
