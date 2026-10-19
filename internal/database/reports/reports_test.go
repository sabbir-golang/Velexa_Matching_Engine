package reports

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"velexa-sim-me/internal/database/db/dbtest"
	"velexa-sim-me/internal/engine"
)

func checkStore(t *testing.T, st Store) {
	t.Helper()
	day1 := map[string]engine.DayCounters{
		engine.ExchangeActor: {BuyOrders: 3, SellOrders: 2, Accepted: 4, Rejected: 1, Trades: 2, Volume: 150, Turnover: 1234.5},
		"FIX:AAA":            {BuyOrders: 3, Accepted: 2, Rejected: 1, Trades: 2, Volume: 150, Turnover: 1234.5},
	}
	if err := st.Save("2026-10-18", day1); err != nil {
		t.Fatal(err)
	}
	if err := st.Save("2026-10-19", map[string]engine.DayCounters{engine.ExchangeActor: {BuyOrders: 1}}); err != nil {
		t.Fatal(err)
	}
	day1["FIX:AAA"] = engine.DayCounters{BuyOrders: 4, Accepted: 3, Rejected: 1, Trades: 2, Volume: 150, Turnover: 1234.5}
	if err := st.Save("2026-10-18", day1); err != nil { // a later save replaces the figures
		t.Fatal(err)
	}
	got, err := st.Load("2026-10-18")
	if err != nil || got["FIX:AAA"] != day1["FIX:AAA"] || got[engine.ExchangeActor] != day1[engine.ExchangeActor] {
		t.Fatalf("load: %+v %v", got, err)
	}
	if got, _ := st.Load("2001-01-01"); len(got) != 0 {
		t.Fatalf("a day without figures: %+v", got)
	}
	if days, _ := st.Days(10); len(days) != 2 || days[0] != "2026-10-19" || days[1] != "2026-10-18" {
		t.Fatalf("days: %v", days)
	}
}

func TestFileStore(t *testing.T) {
	checkStore(t, NewFileStore(filepath.Join(t.TempDir(), "daily_report.json")))
}

func TestPGStore(t *testing.T) {
	pool, _ := dbtest.Open(t)
	checkStore(t, NewPGStore(pool))
}

type flaky struct {
	mu    sync.Mutex
	fail  bool
	saved map[string]map[string]engine.DayCounters
}

func (f *flaky) setFail(v bool) { f.mu.Lock(); f.fail = v; f.mu.Unlock() }

func (f *flaky) Save(day string, a map[string]engine.DayCounters) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("database down")
	}
	f.saved[day] = a
	return nil
}
func (f *flaky) Load(string) (map[string]engine.DayCounters, error) { return nil, nil }
func (f *flaky) Days(int) ([]string, error)                         { return nil, nil }

// Figures that could not be saved are retried, so a short outage loses nothing.
func TestSaverRetries(t *testing.T) {
	e := engine.NewEngine(nil, engine.NewBus())
	st := &flaky{fail: true, saved: map[string]map[string]engine.DayCounters{}}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { Saver(e, st, 20*time.Millisecond, stop, func(string, ...any) {}); close(done) }()
	e.CountReject("FIX:AAA", engine.Buy)
	time.Sleep(80 * time.Millisecond)
	st.setFail(false)
	close(stop) // the final flush happens even though nothing changed since
	<-done
	today := engine.ReportDay(time.Now())
	if st.saved[today][engine.ExchangeActor].Rejected != 1 {
		t.Fatalf("figures lost after a failed save: %+v", st.saved)
	}
}
