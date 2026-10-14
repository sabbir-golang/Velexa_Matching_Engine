package engine

import (
	"testing"
	"time"
)

func newJournalEngine(t *testing.T) *Engine {
	t.Helper()
	e := NewEngine([]Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 10}}, NewBus())
	go e.Run()
	return e
}

func limit(e *Engine, cl string, side Side, px float64, qty int64) *Order {
	o := e.NewOrder("GP", side, Limit, px, qty, "FIX:test")
	o.ClOrdID = cl
	o.reply = make(chan *ExecReport, 1)
	return o
}

func submit(e *Engine, o *Order) *ExecReport {
	e.submit <- o
	return <-o.reply
}

func readAll(t *testing.T, e *Engine) []Event {
	t.Helper()
	evs, _, ok := e.Journal().Read(1, 1<<20)
	if !ok {
		t.Fatal("journal lost its start")
	}
	return evs
}

// Replaying the journal must rebuild exactly the book the engine holds.
func TestJournalRebuildsTheBook(t *testing.T) {
	e := newJournalEngine(t)
	b1 := limit(e, "B1", Buy, 10, 100)
	submit(e, b1)
	submit(e, limit(e, "B2", Buy, 9.9, 50))
	submit(e, limit(e, "S1", Sell, 10, 40)) // trades 40 against B1, nothing rests
	e.Cancel("B2")
	e.Replace("B1", "B1b", 80, 10.1) // 40 filled, 40 left, now at 10.1

	evs := readAll(t, e)
	var execs, adds, dels int
	book := map[uint64]int64{}
	for _, ev := range evs {
		switch ev.Kind {
		case EvAdd:
			adds++
			book[ev.OrderID] = ev.Qty
		case EvExec, EvExecAuction:
			execs++
			book[ev.OrderID] -= ev.Qty
			if book[ev.OrderID] <= 0 {
				delete(book, ev.OrderID)
			}
			if ev.Match == 0 {
				t.Error("an execution without a match number")
			}
		case EvDelete:
			dels++
			delete(book, ev.OrderID)
		}
	}
	if execs != 1 {
		t.Errorf("one trade should be one execution on the resting order, got %d", execs)
	}
	snap, _, _ := e.SnapshotOrders()
	if len(snap) != len(book) {
		t.Fatalf("snapshot has %d orders, journal replay has %d", len(snap), len(book))
	}
	for _, o := range snap {
		if book[o.ID] != o.Qty {
			t.Errorf("order %d: snapshot qty %d, replay qty %d", o.ID, o.Qty, book[o.ID])
		}
	}
	if len(snap) != 1 || snap[0].Qty != 40 || snap[0].PriceRaw != 1010 {
		t.Fatalf("expected one resting order of 40 @ 10.10, got %+v", snap)
	}
}

func TestJournalRingKeepsTheNewestEvents(t *testing.T) {
	j := NewJournal(1024)
	for i := 0; i < 3000; i++ {
		j.Append(Event{Kind: EvAdd, OrderID: uint64(i)})
	}
	if _, _, ok := j.Read(1, 10); ok {
		t.Fatal("an overwritten sequence must not be readable")
	}
	evs, next, ok := j.Read(j.Next()-5, 100)
	if !ok || len(evs) != 5 || next != j.Next() || evs[4].OrderID != 2999 {
		t.Fatalf("tail read: ok=%v n=%d next=%d", ok, len(evs), next)
	}
	select {
	case <-j.Wait():
		t.Fatal("Wait fired with nothing new")
	case <-time.After(10 * time.Millisecond):
	}
	ch := j.Wait()
	go j.Append(Event{Kind: EvAdd})
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("Wait did not fire on an append")
	}
}

func TestStatsPerOrderSource(t *testing.T) {
	e := newJournalEngine(t)
	s := e.NewOrder("GP", Sell, Limit, 10, 100, "FIX:SELLER")
	s.reply = make(chan *ExecReport, 1)
	submit(e, s)
	b := e.NewOrder("GP", Buy, Limit, 10, 40, "FIX:BUYER")
	b.reply = make(chan *ExecReport, 1)
	submit(e, b)
	if st := e.StatsFor([]string{"FIX:SELLER"}); st.Orders != 1 || st.Trades != 1 || st.Volume != 40 {
		t.Fatalf("seller stats %+v", st)
	}
	if st := e.StatsFor([]string{"FIX:NOBODY"}); st.Orders != 0 || st.Trades != 0 {
		t.Fatalf("an unrelated source has activity: %+v", st)
	}
}
