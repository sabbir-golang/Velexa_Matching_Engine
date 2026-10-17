package engine

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
)

// Direct tests of the matching core: priority, fills, order types, cancel and
// amend, and the pre-trade rules. GP has a previous close of 10.00, so its band
// is ±10% (9.00-11.00) and its tick 0.10.

func order(e *Engine, actor, cl string, side Side, typ OrdType, px float64, qty int64) *Order {
	o := e.NewOrder("GP", side, typ, px, qty, actor)
	o.ClOrdID = cl
	o.reply = make(chan *ExecReport, 1)
	return o
}

func bestAsk(e *Engine) (float64, int64) {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	if lv := e.Book("GP").BestAsk(); lv != nil {
		return K2Px(lv.Px), lv.Qty
	}
	return 0, 0
}

func bestBid(e *Engine) (float64, int64) {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	if lv := e.Book("GP").BestBid(); lv != nil {
		return K2Px(lv.Px), lv.Qty
	}
	return 0, 0
}

func isOpen(e *Engine, cl, actor string) bool {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	return e.FindOpen(cl, actor) != nil
}

func TestPriceTimePriority(t *testing.T) {
	e := newJournalEngine(t)
	first := order(e, "FIX:A", "S1", Sell, Limit, 10.2, 100)
	var firstFilled int64
	first.Notify = func(r *ExecReport) {
		if r.Event == "FILL" {
			firstFilled += r.LastQty
		}
	}
	submit(e, first)
	submit(e, limit(e, "S2", Sell, 10.2, 100)) // same price, later
	submit(e, limit(e, "S3", Sell, 10.1, 50))  // better price, latest

	r := submit(e, order(e, "FIX:B", "B1", Buy, Limit, 10.5, 120))
	if r.Status != "FILLED" || len(r.Fills) != 2 {
		t.Fatalf("buy 120: %s fills=%d", r.Status, len(r.Fills))
	}
	// Best price first (10.10 x 50, price improvement for the buyer), then the
	// oldest order at 10.20 — never the newer S2.
	if r.Fills[0].Price != 10.1 || r.Fills[0].Qty != 50 || r.Fills[1].Price != 10.2 || r.Fills[1].Qty != 70 {
		t.Fatalf("fills %+v, want 50@10.10 then 70@10.20", r.Fills)
	}
	if firstFilled != 70 {
		t.Fatalf("the oldest order at 10.20 got %d, want 70 (time priority)", firstFilled)
	}
	if px, q := bestAsk(e); px != 10.2 || q != 130 { // S1 30 left + S2 100
		t.Fatalf("best ask %.2f x %d, want 10.20 x 130", px, q)
	}
	if want := Round2((50*10.1 + 70*10.2) / 120); Round2(r.AvgPx) != want {
		t.Fatalf("avg px %.4f, want %.2f", r.AvgPx, want)
	}
}

func TestPartialFillRestsRemainder(t *testing.T) {
	e := newJournalEngine(t)
	submit(e, limit(e, "S1", Sell, 10, 40))
	r := submit(e, limit(e, "B1", Buy, 10, 100))
	if r.Status != "PARTIALLY_FILLED" || r.FilledQty != 40 || r.LeavesQty != 60 {
		t.Fatalf("%s filled %d leaves %d", r.Status, r.FilledQty, r.LeavesQty)
	}
	if px, q := bestBid(e); px != 10 || q != 60 {
		t.Fatalf("best bid %.2f x %d, want 10.00 x 60", px, q)
	}
	if px, _ := bestAsk(e); px != 0 {
		t.Fatal("the ask side must be empty")
	}
}

func TestIOCAndFOK(t *testing.T) {
	e := newJournalEngine(t)
	submit(e, limit(e, "S1", Sell, 10, 50))

	ioc := limit(e, "IOC1", Buy, 10, 80)
	ioc.TIF = IOC
	if r := submit(e, ioc); r.FilledQty != 50 || isOpen(e, "IOC1", "") {
		t.Fatalf("IOC: filled %d, rests=%v; want 50 filled and nothing resting", r.FilledQty, isOpen(e, "IOC1", ""))
	}
	if px, _ := bestBid(e); px != 0 {
		t.Fatal("an IOC remainder must not rest")
	}

	submit(e, limit(e, "S2", Sell, 10, 50))
	fok := limit(e, "FOK1", Buy, 10, 80)
	fok.TIF = FOK
	if r := submit(e, fok); r.FilledQty != 0 {
		t.Fatalf("FOK with 50 of 80 available filled %d, want 0", r.FilledQty)
	}
	if _, q := bestAsk(e); q != 50 {
		t.Fatalf("FOK must leave the book untouched, ask qty %d", q)
	}
	fok2 := limit(e, "FOK2", Buy, 10, 50)
	fok2.TIF = FOK
	if r := submit(e, fok2); r.Status != "FILLED" {
		t.Fatalf("FOK fully fillable: %s", r.Status)
	}
}

func TestMarketOrders(t *testing.T) {
	e := newJournalEngine(t)
	if r := submit(e, order(e, "FIX:A", "M1", Buy, Market, 0, 10)); r.Status != "REJECTED" {
		t.Fatalf("market order on an empty book: %s, want REJECTED", r.Status)
	}
	submit(e, limit(e, "S1", Sell, 10.1, 30))
	submit(e, limit(e, "S2", Sell, 10.3, 30))
	r := submit(e, order(e, "FIX:A", "M2", Buy, Market, 0, 100))
	if r.FilledQty != 60 || isOpen(e, "M2", "") {
		t.Fatalf("market sweep: filled %d, rests=%v; want 60 filled, rest never rests", r.FilledQty, isOpen(e, "M2", ""))
	}
}

func TestIcebergShowsOnlyItsPeak(t *testing.T) {
	e := newJournalEngine(t)
	ice := limit(e, "ICE", Sell, 10, 100)
	ice.Display = 30
	submit(e, ice)
	if _, q := bestAsk(e); q != 30 {
		t.Fatalf("visible %d, want 30", q)
	}
	submit(e, limit(e, "S2", Sell, 10, 20)) // behind the iceberg's first slice
	r := submit(e, limit(e, "B1", Buy, 10, 40))
	// 30 from the iceberg's slice, then the queue moves on to S2 (the refreshed
	// slice goes to the back), so 10 from S2.
	if r.FilledQty != 40 || len(r.Fills) != 2 {
		t.Fatalf("fills %+v", r.Fills)
	}
	if _, q := bestAsk(e); q != 40 { // S2's 10 left + the iceberg's next 30
		t.Fatalf("visible after refresh %d, want 40", q)
	}
}

func TestCancelAndAmend(t *testing.T) {
	e := newJournalEngine(t)
	submit(e, limit(e, "B1", Buy, 9.8, 100))
	if r := e.CancelAs("B1", "C1", "FIX:other"); r.Status != "REJECT_CXL" {
		t.Fatalf("another broker cancelled the order: %s", r.Status)
	}
	if r := e.ReplaceAs("B1", "B1a", "FIX:test", 60, 9.9); r.Status != "REPLACED" || !isOpen(e, "B1a", "FIX:test") || isOpen(e, "B1", "FIX:test") {
		t.Fatalf("amend: %s", r.Status)
	}
	if px, q := bestBid(e); px != 9.9 || q != 60 {
		t.Fatalf("after amend %.2f x %d, want 9.90 x 60", px, q)
	}
	// An amend without a quantity keeps the order's quantity.
	if r := e.ReplaceAs("B1a", "B1b", "FIX:test", 0, 9.7); r.Status != "REPLACED" {
		t.Fatalf("amend without qty: %s %q", r.Status, r.Reason)
	}
	if _, q := bestBid(e); q != 60 {
		t.Fatalf("amend without qty changed the quantity to %d", q)
	}
	// Amending to a marketable price trades.
	submit(e, limit(e, "S1", Sell, 10, 25))
	if r := e.ReplaceAs("B1b", "B1c", "FIX:test", 60, 10); r.FilledQty != 25 {
		t.Fatalf("marketable amend filled %d, want 25", r.FilledQty)
	}
	if r := e.CancelAs("B1c", "C2", "FIX:test"); r.Status != "CANCELED" || r.FilledQty != 25 {
		t.Fatalf("cancel: %s filled %d", r.Status, r.FilledQty)
	}
	if r := e.CancelAs("B1c", "C3", "FIX:test"); r.Status != "REJECT_CXL" {
		t.Fatalf("second cancel: %s, want REJECT_CXL", r.Status)
	}
	if px, _ := bestBid(e); px != 0 {
		t.Fatal("the book must be empty after the cancel")
	}
}

// Two brokers may use the same ClOrdID; each must still be able to cancel and
// amend its own order, and only its own.
func TestSameClOrdIDOnTwoBrokers(t *testing.T) {
	e := newJournalEngine(t)
	if r := submit(e, order(e, "FIX:AAA", "1", Buy, Limit, 9.5, 100)); r.Status != "NEW" {
		t.Fatalf("AAA: %s %q", r.Status, r.Reason)
	}
	if r := submit(e, order(e, "FIX:BBB", "1", Buy, Limit, 9.6, 200)); r.Status != "NEW" {
		t.Fatalf("BBB: %s %q", r.Status, r.Reason)
	}
	if r := e.CancelAs("1", "c1", "FIX:AAA"); r.Status != "CANCELED" || r.OrderQty != 100 {
		t.Fatalf("AAA cancelling its order 1: %s qty %d", r.Status, r.OrderQty)
	}
	if !isOpen(e, "1", "FIX:BBB") {
		t.Fatal("cancelling AAA's order 1 removed BBB's order 1")
	}
	if r := e.ReplaceAs("1", "2", "FIX:BBB", 200, 9.7); r.Status != "REPLACED" {
		t.Fatalf("BBB amending its order 1: %s %q", r.Status, r.Reason)
	}
	if px, q := bestBid(e); px != 9.7 || q != 200 {
		t.Fatalf("book %.2f x %d, want only BBB's 9.70 x 200", px, q)
	}
}

func TestDuplicateClOrdIDIsRejected(t *testing.T) {
	e := newJournalEngine(t)
	submit(e, limit(e, "D1", Buy, 9.5, 100))
	if r := submit(e, limit(e, "D1", Buy, 9.6, 100)); r.Status != "REJECTED" || !strings.HasPrefix(r.Reason, ReasonDuplicateClOrdID) {
		t.Fatalf("duplicate of a working order: %s %q", r.Status, r.Reason)
	}
	if _, q := bestBid(e); q != 100 {
		t.Fatal("the duplicate must not reach the book")
	}
	submit(e, limit(e, "D2", Buy, 9.4, 10))
	if r := e.ReplaceAs("D2", "D1", "FIX:test", 10, 9.4); r.Status != "REJECT_CXL" || !strings.HasPrefix(r.Reason, ReasonDuplicateClOrdID) {
		t.Fatalf("amend onto a working ClOrdID: %s %q", r.Status, r.Reason)
	}
	e.CancelAs("D1", "x", "FIX:test")
	if r := submit(e, limit(e, "D1", Buy, 9.6, 100)); r.Status != "NEW" {
		t.Fatalf("a ClOrdID whose order is gone may be used again: %s %q", r.Status, r.Reason)
	}
}

func TestAmendIsCheckedLikeANewOrder(t *testing.T) {
	e := newJournalEngine(t)
	submit(e, limit(e, "A1", Buy, 9.5, 100))
	for _, c := range []struct {
		name string
		px   float64
		want string
	}{
		{"off tick", 9.55, "off-tick"},
		{"above the band", 11.5, "price band"},
		{"below the band", 8.9, "price band"},
	} {
		r := e.ReplaceAs("A1", "A1-"+c.name, "FIX:test", 100, c.px)
		if r.Status != "REJECT_CXL" || !strings.HasPrefix(r.Reason, c.want) {
			t.Errorf("%s: %s %q", c.name, r.Status, r.Reason)
		}
	}
	if !isOpen(e, "A1", "FIX:test") {
		t.Fatal("a refused amend must leave the order untouched")
	}
	e.ForcePhase(PhaseClose) // the market has closed; orders still rest until the end of the day
	if r := e.ReplaceAs("A1", "A2", "FIX:test", 100, 9.6); r.Status != "REJECT_CXL" || r.Reason != ReasonNotAccepting {
		t.Fatalf("amend after the close: %s %q", r.Status, r.Reason)
	}
}

func TestPreOpenChecksPricesAndUncrosses(t *testing.T) {
	e := newJournalEngine(t)
	e.SetPhase(PhasePreOpen)
	if r := submit(e, limit(e, "P0", Buy, 12, 10)); r.Status != "REJECTED" || !strings.HasPrefix(r.Reason, "price band") {
		t.Fatalf("pre-open order outside the band: %s %q", r.Status, r.Reason)
	}
	if r := submit(e, order(e, "FIX:test", "PM", Buy, Market, 0, 10)); r.Status != "REJECTED" {
		t.Fatal("market orders are refused in the call auction")
	}
	submit(e, limit(e, "PB1", Buy, 10.4, 100))
	submit(e, limit(e, "PB2", Buy, 10.0, 100))
	submit(e, limit(e, "PS1", Sell, 9.8, 150))
	if px, _ := bestAsk(e); px != 9.8 {
		t.Fatal("orders must rest without matching during pre-open")
	}
	e.SetPhase(PhaseOpen) // uncross: one equilibrium price for every fill
	e.Mu.RLock()
	trades := append([]Trade(nil), e.Trades...)
	e.Mu.RUnlock()
	var vol int64
	for _, tr := range trades {
		if tr.Price != trades[0].Price {
			t.Fatalf("auction fills at several prices: %+v", trades)
		}
		vol += tr.Qty
	}
	if vol != 150 {
		t.Fatalf("auction volume %d, want 150 (all the sell side)", vol)
	}
}

func TestStopTriggersOnLastPrice(t *testing.T) {
	e := newJournalEngine(t)
	stop := order(e, "FIX:test", "ST1", Buy, StopMkt, 0, 10)
	stop.StopPx = 10.3
	if r := submit(e, stop); r.Status != "PENDING" {
		t.Fatalf("stop: %s", r.Status)
	}
	submit(e, limit(e, "S1", Sell, 10.3, 10))
	submit(e, limit(e, "S2", Sell, 10.4, 10))
	submit(e, limit(e, "B1", Buy, 10.3, 10)) // trades at 10.30 → triggers the stop
	if _, q := bestAsk(e); q != 0 {
		t.Fatalf("the triggered stop should have bought the 10.40 offer, ask qty left %d", q)
	}
}

// Many brokers trading at once: the book is never left crossed, every fill has
// a matching counter-fill, and the ClOrdID index holds exactly the resting orders.
func TestConcurrentTradingKeepsTheBookConsistent(t *testing.T) {
	e := newJournalEngine(t)
	const brokers, perBroker = 8, 400
	var mu sync.Mutex
	var bought, sold int64
	var wg sync.WaitGroup
	for b := 0; b < brokers; b++ {
		wg.Add(1)
		go func(b int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(b)))
			actor := fmt.Sprintf("FIX:B%d", b)
			for i := 0; i < perBroker; i++ {
				side := Buy
				if rng.Intn(2) == 1 {
					side = Sell
				}
				px := Round2(9.5 + float64(rng.Intn(11))/10) // 9.50 - 10.50
				o := order(e, actor, fmt.Sprintf("%d", i), side, Limit, px, int64(1+rng.Intn(20))*10)
				o.Notify = func(r *ExecReport) {
					if r.Event == "FILL" {
						mu.Lock()
						if r.Side == Buy {
							bought += r.LastQty
						} else {
							sold += r.LastQty
						}
						mu.Unlock()
					}
				}
				r := e.SubmitAndWait(o)
				mu.Lock()
				for _, f := range r.Fills {
					if side == Buy {
						bought += f.Qty
					} else {
						sold += f.Qty
					}
				}
				mu.Unlock()
				if i%7 == 0 {
					e.CancelAs(fmt.Sprintf("%d", i-3), "x", actor)
				}
			}
		}(b)
	}
	wg.Wait()
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	b := e.Book("GP")
	if bid, ask := b.BestBid(), b.BestAsk(); bid != nil && ask != nil && bid.Px >= ask.Px {
		t.Fatalf("book left crossed: bid %d >= ask %d", bid.Px, ask.Px)
	}
	if bought != sold {
		t.Fatalf("bought %d != sold %d", bought, sold)
	}
	resting := 0
	for _, side := range [][]Level{b.DepthSide(true, 1<<20), b.DepthSide(false, 1<<20)} {
		for _, lv := range side {
			resting += lv.Orders
		}
	}
	if resting != len(e.Open) {
		t.Fatalf("%d resting orders but %d in the ClOrdID index", resting, len(e.Open))
	}
	var traded int64
	for _, tr := range e.Trades {
		traded += tr.Qty
	}
	if md := e.MdOf("GP"); md.Volume < traded {
		t.Fatalf("volume %d below the traded total %d", md.Volume, traded)
	}
}
