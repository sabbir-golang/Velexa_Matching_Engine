package engine

import (
	"encoding/json"
	"testing"
	"time"
)

// Saving and restoring the live market (snapshot.go): a restarted engine must
// carry on exactly where the old one stopped.

// testCtx stands in for a gateway's order context.
type testCtx struct{ Session string }

func (c *testCtx) CtxKind() string                   { return "TEST" }
func (c *testCtx) SaveCtx() (json.RawMessage, error) { return json.Marshal(c) }

func init() {
	RegisterCtxDecoder("TEST", func(raw json.RawMessage) (any, error) {
		var c testCtx
		err := json.Unmarshal(raw, &c)
		return &c, err
	})
}

// roundTrip saves e's market as JSON and restores it into a fresh engine.
func roundTrip(t *testing.T, e *Engine, instr ...Instrument) (*Engine, RestoreResult) {
	t.Helper()
	snap := e.TakeSnapshot(true)
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var back Snapshot
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(instr) == 0 {
		instr = []Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 10}}
	}
	e2 := NewEngine(instr, NewBus())
	r, err := e2.Restore(&back)
	if err != nil {
		t.Fatal(err)
	}
	go e2.Run()
	return e2, r
}

func depth(e *Engine) map[string]any {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	return e.SnapshotBook("GP")
}

func TestSnapshotRestoresTheMarket(t *testing.T) {
	e := newJournalEngine(t)
	s1 := order(e, "FIX:A", "S1", Sell, Limit, 10.2, 100)
	s1.Ctx = &testCtx{Session: "A-session"}
	submit(e, s1)
	submit(e, order(e, "FIX:B", "S2", Sell, Limit, 10.2, 100)) // same price, behind S1
	submit(e, order(e, "FIX:B", "S3", Sell, Limit, 10.3, 50))
	ice := order(e, "FIX:B", "ICE", Sell, Limit, 10.4, 100)
	ice.Display = 30
	submit(e, ice)
	gtc := order(e, "FIX:A", "B1", Buy, Limit, 9.8, 100)
	gtc.TIF = GTC
	submit(e, gtc)
	stop := order(e, "FIX:A", "ST1", Buy, StopMkt, 0, 10)
	stop.StopPx = 10.6
	submit(e, stop)
	submit(e, order(e, "FIX:C", "T1", Buy, Limit, 10.2, 40)) // trades 40 of S1
	e.SetAutoClock(false)

	before := depth(e)
	e.Mu.RLock()
	md := *e.MdOf("GP")
	seq, tradeSeq := e.Seq, e.TradeSeq
	e.Mu.RUnlock()

	e2, r := roundTrip(t, e)
	if r.Orders != 5 || r.Stops != 1 || r.Trades != 1 || r.Dropped != 0 || r.Stale {
		t.Fatalf("restore result %+v", r)
	}
	after := depth(e2)
	if a, b := mustJSON(t, before), mustJSON(t, after); a != b {
		t.Fatalf("depth differs after restore:\nbefore %s\nafter  %s", a, b)
	}
	e2.Mu.RLock()
	md2 := *e2.MdOf("GP")
	e2.Mu.RUnlock()
	if md2.Last != 10.2 || md2.Volume != 40 || md2.Trades != 1 || md2.Bid != md.Bid || md2.Ask != md.Ask ||
		md2.BidQty != md.BidQty || md2.AskQty != md.AskQty || md2.High != md.High || md2.Low != md.Low ||
		md2.Open != md.Open || md2.Change != md.Change || md2.ChangePct != md.ChangePct {
		t.Fatalf("market data after restore %+v, before %+v", md2, md)
	}
	if e2.Phase != e.Phase || *e2.restoredAuto != false {
		t.Fatalf("phase %q auto %v", e2.Phase, *e2.restoredAuto)
	}

	// The restored S1 keeps its fill so far and its gateway context.
	e2.Mu.RLock()
	r1 := e2.FindOpen("S1", "FIX:A")
	e2.Mu.RUnlock()
	if r1 == nil || r1.CumQty != 40 || r1.Qty != 60 || r1.OrigQty != 100 {
		t.Fatalf("S1 after restore %+v", r1)
	}
	if c, ok := r1.Ctx.(*testCtx); !ok || c.Session != "A-session" {
		t.Fatalf("S1 context after restore %#v", r1.Ctx)
	}

	// Time priority: S1's 60 trade before S2. Counters continue.
	var trades []TradeRecord
	e2.OnTrade = func(tr TradeRecord) { trades = append(trades, tr) }
	rep := submit(e2, order(e2, "FIX:C", "T2", Buy, Limit, 10.2, 70))
	if rep.FilledQty != 70 || len(trades) != 2 {
		t.Fatalf("after restore: filled %d, %d trades", rep.FilledQty, len(trades))
	}
	if trades[0].Sell.ClOrdID != "S1" || trades[0].Qty != 60 || trades[1].Sell.ClOrdID != "S2" || trades[1].Qty != 10 {
		t.Fatalf("queue order lost: %+v", trades)
	}
	if trades[0].ID != tradeSeq+1 || rep.OrderID != seq+1 {
		t.Fatalf("counters restart: trade %d (want %d), order %d (want %d)", trades[0].ID, tradeSeq+1, rep.OrderID, seq+1)
	}
	if trades[0].Buy.Actor != "FIX:C" || trades[0].Buy.ClOrdID != "T2" || trades[0].Aggressor != Buy {
		t.Fatalf("trade sides %+v", trades[0])
	}

	// Cancel and amend by ClOrdID work on restored orders.
	if r := e2.CancelAs("S3", "X1", "FIX:B"); r.Status != "CANCELED" {
		t.Fatalf("cancel restored S3: %+v", r)
	}
	if r := e2.ReplaceAs("B1", "B1A", "FIX:A", 100, 9.9); r.Status != "REPLACED" {
		t.Fatalf("amend restored B1: %+v", r)
	}
	// The iceberg kept its reserve: 30 shown, 70 hidden.
	e2.Mu.RLock()
	ri := e2.FindOpen("ICE", "FIX:B")
	e2.Mu.RUnlock()
	if ri == nil || ri.Qty != 30 || ri.Hidden != 70 || ri.Display != 30 {
		t.Fatalf("iceberg after restore %+v", ri)
	}
	// The stop still triggers once a trade prints at 10.60.
	submit(e2, order(e2, "FIX:B", "S9", Sell, Limit, 10.6, 10))
	submit(e2, order(e2, "FIX:C", "T3", Buy, Limit, 10.6, 300)) // lifts every offer up to 10.60
	e2.Mu.RLock()
	stops := len(e2.stops["GP"])
	e2.Mu.RUnlock()
	if stops != 0 {
		t.Fatalf("restored stop did not trigger")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSnapshotOnlyWhenChanged(t *testing.T) {
	e := newJournalEngine(t)
	e.TakeSnapshot(true)
	if s := e.TakeSnapshot(false); s != nil {
		t.Fatal("an unchanged market was saved again")
	}
	submit(e, limit(e, "S1", Sell, 10.2, 10))
	if s := e.TakeSnapshot(false); s == nil || len(s.Orders) != 1 {
		t.Fatalf("a new order did not mark the market changed: %+v", s)
	}
}

func TestRestoreDropsUnlistedInstruments(t *testing.T) {
	e := NewEngine([]Instrument{{Symbol: "GP", Prev: 10}, {Symbol: "OLD", Prev: 20}}, NewBus())
	go e.Run()
	submit(e, limit(e, "S1", Sell, 10.2, 10))
	o := e.NewOrder("OLD", Buy, Limit, 19.5, 10, "FIX:test")
	o.ClOrdID, o.reply = "O1", make(chan *ExecReport, 1)
	submit(e, o)
	_, r := roundTrip(t, e, Instrument{Symbol: "GP", Prev: 10})
	if r.Orders != 1 || r.Dropped != 1 {
		t.Fatalf("restore %+v, want 1 order and 1 dropped", r)
	}
}

// A market saved on an earlier day: the restart closes that day — day orders
// expire, GTC orders stay — and the next day opens with the right reference.
func TestStaleSnapshotClosesTheDay(t *testing.T) {
	e := newJournalEngine(t)
	submit(e, limit(e, "DAY1", Buy, 9.5, 100))
	gtc := limit(e, "GTC1", Buy, 9.4, 100)
	gtc.TIF = GTC
	submit(e, gtc)
	submit(e, limit(e, "S1", Sell, 10.2, 50))
	submit(e, limit(e, "B1", Buy, 10.2, 50))
	snap := e.TakeSnapshot(true)
	snap.Taken = snap.Taken.Add(-24 * time.Hour)

	e2 := NewEngine([]Instrument{{Symbol: "GP", Prev: 10}}, NewBus())
	r, err := e2.Restore(snap)
	if err != nil || !r.Stale {
		t.Fatalf("restore %+v %v: want stale", r, err)
	}
	go e2.Run()
	var expired []string
	e2.AttachNotify("FIX:", func(rep *ExecReport) {
		if rep.Event == "EXPIRE" {
			expired = append(expired, rep.ClOrdID)
		}
	})
	if !e2.FinishStaleDay() {
		t.Fatal("FinishStaleDay did nothing")
	}
	if e2.FinishStaleDay() {
		t.Fatal("FinishStaleDay ran twice")
	}
	if e2.CurrentPhase() != PhaseClosed || len(expired) != 1 || expired[0] != "DAY1" {
		t.Fatalf("phase %s, expired %v", e2.CurrentPhase(), expired)
	}
	e2.Mu.RLock()
	gtcLeft := e2.FindOpen("GTC1", "FIX:test") != nil
	e2.Mu.RUnlock()
	if !gtcLeft {
		t.Fatal("the GTC order must survive the end of the day")
	}
	e2.AdvanceTo(PhasePreOpen) // the new day
	e2.Mu.RLock()
	md := *e2.MdOf("GP")
	e2.Mu.RUnlock()
	if md.Prev != 10.2 || md.Volume != 0 || md.Trades != 0 {
		t.Fatalf("new day market data %+v: want prev 10.20 and the day's figures reset", md)
	}
}

// A restart on the same day changes nothing, not even in Pre-Open.
func TestSameDaySnapshotKeepsThePhase(t *testing.T) {
	e := newJournalEngine(t)
	e.SetPhase(PhasePreOpen)
	submit(e, limit(e, "DAY1", Buy, 9.5, 100))
	e2, r := roundTrip(t, e)
	if r.Stale || e2.FinishStaleDay() {
		t.Fatal("a same-day snapshot is not stale")
	}
	if e2.CurrentPhase() != PhasePreOpen || !isOpen(e2, "DAY1", "FIX:test") {
		t.Fatalf("phase %s, DAY1 open %v", e2.CurrentPhase(), isOpen(e2, "DAY1", "FIX:test"))
	}
}

func TestRestoreRefusesAnotherVersion(t *testing.T) {
	e := NewEngine(nil, NewBus())
	if _, err := e.Restore(&Snapshot{Version: SnapshotVersion + 1}); err == nil {
		t.Fatal("a snapshot of another layout was accepted")
	}
}

func TestTradeRecordSidesWhenTheSellerAggresses(t *testing.T) {
	e := newJournalEngine(t)
	var got []TradeRecord
	e.OnTrade = func(tr TradeRecord) { got = append(got, tr) }
	submit(e, order(e, "FIX:A", "B1", Buy, Limit, 10, 10))
	submit(e, order(e, "FIX:B", "S1", Sell, Limit, 10, 10))
	if len(got) != 1 || got[0].Buy.Actor != "FIX:A" || got[0].Sell.Actor != "FIX:B" || got[0].Aggressor != Sell ||
		got[0].Buy.OrderID == 0 || got[0].Sell.ClOrdID != "S1" {
		t.Fatalf("trade record %+v", got)
	}
}
