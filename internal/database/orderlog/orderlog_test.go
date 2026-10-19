package orderlog

import (
	"path/filepath"
	"testing"
	"time"

	"velexa-sim-me/internal/database/db/dbtest"
	"velexa-sim-me/internal/engine"
)

func newEngine(t *testing.T, st Store) (*engine.Engine, *Recorder) {
	t.Helper()
	rec, err := NewRecorder(st)
	if err != nil {
		t.Fatal(err)
	}
	e := engine.NewEngine([]engine.Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 10}}, engine.NewBus())
	e.OnOrderEvent = rec.Observe
	e.SetMinSeq(rec.MaxSeq())
	go e.Run()
	return e, rec
}

func place(e *engine.Engine, actor, cl string, side engine.Side, px float64, qty int64, info engine.OrderInfo) *engine.ExecReport {
	o := e.NewOrder("GP", side, engine.Limit, px, qty, actor)
	o.ClOrdID, o.Info = cl, info
	return e.SubmitAndWait(o)
}

func byCl(recs []Record) map[string]Record {
	m := map[string]Record{}
	for _, r := range recs {
		m[r.ClOrdID] = r
	}
	return m
}

func checkLifecycle(t *testing.T, st Store) {
	e, rec := newEngine(t, st)
	info := engine.OrderInfo{Account: "1206500076409418", Client: "00018", Board: "PUBLIC", Trader: "BENTRDR004"}
	place(e, "FIX:AAA", "B1", engine.Buy, 9.8, 100, info)                  // rests: New
	place(e, "USER:alice", "S1", engine.Sell, 9.8, 40, engine.OrderInfo{}) // console order, trades 40: Filled
	place(e, "FIX:AAA", "B2", engine.Buy, 9.5, 50, info)                   // rests, then cancelled
	e.CancelAs("B2", "B2c", "FIX:AAA")
	place(e, "FIX:AAA", "B3", engine.Buy, 9.6, 30, info) // amended to B3a
	e.ReplaceAs("B3", "B3a", "FIX:AAA", 20, 9.7)
	rej := e.NewOrder("GP", engine.Sell, engine.Limit, 9.9, 10, "FIX:AAA")
	rej.ClOrdID, rej.Info = "R1", info
	e.RejectUnsubmitted(rej, "limit order without a price (tag 44)")
	place(e, "FIX:AAA", "B4", engine.Buy, 50, 10, info) // price band reject inside the engine

	day, recs := rec.Today()
	if day != engine.ReportDay(time.Now()) || len(recs) != 6 {
		t.Fatalf("today %s: %d records", day, len(recs))
	}
	m := byCl(recs)
	b1 := m["B1"]
	if b1.Status != "Partial" || b1.Filled != 40 || b1.AvgPx != 9.8 || b1.Qty != 100 || b1.Client != "00018" ||
		b1.Account != "1206500076409418" || b1.Board != "Public" || b1.Trader != "BENTRDR004" ||
		b1.Side != "Buy" || b1.Type != "LIMIT" || b1.OrderID == "" || b1.TIF != "DAY" {
		t.Fatalf("B1 (partly filled resting order): %+v", b1)
	}
	if s1 := m["S1"]; s1.Status != "Filled" || s1.Filled != 40 || s1.Side != "Sell" {
		t.Fatalf("S1 (console order, filled): %+v", s1)
	}
	if b2 := m["B2"]; b2.Status != "Cancelled" || b2.OrderID == "" { // a cancel keeps the order's own ClOrdID
		t.Fatalf("B2 cancelled: %+v", b2)
	}
	if b3 := m["B3a"]; b3.Status != "New" || b3.Qty != 20 || b3.Price != 9.7 {
		t.Fatalf("B3 amended: %+v", b3)
	}
	if r1 := m["R1"]; r1.Status != "Rejected" || r1.Reason == "" || r1.OrderID != "" || r1.Qty != 10 {
		t.Fatalf("R1 gateway reject: %+v", r1)
	}
	if b4 := m["B4"]; b4.Status != "Rejected" || b4.Reason == "" {
		t.Fatalf("B4 band reject: %+v", b4)
	}

	// Saved, and a restarted engine continues the day: same records, no reused order numbers.
	if err := rec.flush(); err != nil {
		t.Fatal(err)
	}
	e2, rec2 := newEngine(t, st)
	if _, recs2 := rec2.Today(); len(recs2) != 6 {
		t.Fatalf("after restart: %d records", len(recs2))
	}
	r := place(e2, "FIX:AAA", "N1", engine.Buy, 9.0, 10, info)
	if r.OrderID <= rec.MaxSeq() {
		t.Fatalf("restarted engine reused order number %d (max before %d)", r.OrderID, rec.MaxSeq())
	}
	if got, err := rec2.List("2001-01-01"); err != nil || len(got) != 0 {
		t.Fatalf("a day without orders: %v %v", got, err)
	}
}

func TestOrderLogFile(t *testing.T) {
	checkLifecycle(t, NewFileStore(filepath.Join(t.TempDir(), "orders")))
}

func TestOrderLogPostgres(t *testing.T) {
	pool, _ := dbtest.Open(t)
	checkLifecycle(t, NewPGStore(pool))
}

// Orders restored with the market after a restart keep their row: a fill
// after the restart updates it instead of adding a second one.
func TestRestoredOrdersKeepTheirRow(t *testing.T) {
	st := NewFileStore(filepath.Join(t.TempDir(), "orders"))
	e, rec := newEngine(t, st)
	info := engine.OrderInfo{Client: "00018"}
	place(e, "FIX:AAA", "B1", engine.Buy, 9.8, 100, info)
	if err := rec.flush(); err != nil {
		t.Fatal(err)
	}
	snap := e.TakeSnapshot(true)

	rec2, err := NewRecorder(st) // the restart
	if err != nil {
		t.Fatal(err)
	}
	e2 := engine.NewEngine([]engine.Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 10}}, engine.NewBus())
	e2.OnOrderEvent = rec2.Observe
	r, err := e2.Restore(snap)
	if err != nil {
		t.Fatal(err)
	}
	if n := rec2.Adopt(r.Restored); n != 1 {
		t.Fatalf("adopted %d of %d", n, len(r.Restored))
	}
	go e2.Run()
	place(e2, "USER:alice", "S1", engine.Sell, 9.8, 30, engine.OrderInfo{})
	_, recs := rec2.Today()
	if len(recs) != 2 {
		t.Fatalf("%d records after the fill, want 2 (B1 and S1): %+v", len(recs), recs)
	}
	if b1 := byCl(recs)["B1"]; b1.Status != "Partial" || b1.Filled != 30 || b1.Client != "00018" {
		t.Fatalf("B1 after a fill following the restart: %+v", b1)
	}
}

// An order the log shows as working, but that did not come back with the
// market, is closed at start-up instead of showing as working forever.
func TestLostOrdersAreClosed(t *testing.T) {
	st := NewFileStore(filepath.Join(t.TempDir(), "orders"))
	e, rec := newEngine(t, st)
	place(e, "FIX:AAA", "KEEP", engine.Buy, 9.8, 100, engine.OrderInfo{})
	place(e, "FIX:AAA", "LOST", engine.Buy, 9.7, 100, engine.OrderInfo{})
	place(e, "FIX:AAA", "DONE", engine.Sell, 50, 10, engine.OrderInfo{}) // rejected: stays as it is
	if err := rec.flush(); err != nil {
		t.Fatal(err)
	}
	snap := e.TakeSnapshot(true)
	keep := snap.Orders[:0]
	for _, o := range snap.Orders {
		if o.ClOrdID != "LOST" { // LOST did not survive the restart
			keep = append(keep, o)
		}
	}
	snap.Orders = keep

	rec2, err := NewRecorder(st)
	if err != nil {
		t.Fatal(err)
	}
	e2 := engine.NewEngine([]engine.Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 10}}, engine.NewBus())
	r, err := e2.Restore(snap)
	if err != nil {
		t.Fatal(err)
	}
	rec2.Adopt(r.Restored)
	lost := rec2.CloseLost("not on the book after an engine restart")
	if len(lost) != 1 || lost[0].ClOrdID != "LOST" {
		t.Fatalf("closed %+v, want only LOST", lost)
	}
	_, recs := rec2.Today()
	m := byCl(recs)
	if m["LOST"].Status != "Cancelled" || m["LOST"].Reason == "" || m["KEEP"].Status != "New" || m["DONE"].Status != "Rejected" {
		t.Fatalf("after start-up: LOST %+v KEEP %+v DONE %+v", m["LOST"], m["KEEP"], m["DONE"])
	}
	if err := rec2.flush(); err != nil {
		t.Fatal(err)
	}
	saved, _ := st.List(engine.ReportDay(time.Now()))
	if byCl(saved)["LOST"].Status != "Cancelled" {
		t.Fatal("the closed order was not saved")
	}
}
