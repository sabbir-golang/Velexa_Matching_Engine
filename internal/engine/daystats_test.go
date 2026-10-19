package engine

import (
	"testing"
	"time"
)

func TestDailyReportCounts(t *testing.T) {
	e := newJournalEngine(t) // GP, prev 10
	today := ReportDay(time.Now())

	submit(e, order(e, "FIX:AAA", "B1", Buy, Limit, 9.8, 100))  // accepted, rests
	submit(e, order(e, "FIX:AAA", "B1", Buy, Limit, 9.8, 100))  // rejected: duplicate ClOrdID
	submit(e, order(e, "FIX:AAA", "B2", Buy, Limit, 50, 100))   // rejected: price band
	submit(e, order(e, "FIX:BBB", "S1", Sell, Limit, 9.8, 40))  // accepted, trades 40 @ 9.80
	submit(e, order(e, "FIX:BBB", "S2", Sell, Limit, 10.5, 10)) // accepted, rests
	e.ReplaceAs("B1", "B1a", "FIX:AAA", 60, 9.7)                // amended
	e.ReplaceAs("B1a", "B1b", "FIX:AAA", 60, 99)                // refused amend: not counted
	e.CancelAs("S2", "S2c", "FIX:BBB")                          // cancelled
	e.CountReject("FIX:BBB", Sell)                              // refused by the FIX gateway

	got, live := e.DayReport(today)
	if !live {
		t.Fatal("today's report must be live")
	}
	want := map[string]DayCounters{
		"FIX:AAA":     {BuyOrders: 3, Accepted: 1, Rejected: 2, Amended: 1, Trades: 1, Volume: 40, Turnover: 392},
		"FIX:BBB":     {SellOrders: 3, Accepted: 2, Rejected: 1, Cancelled: 1, Trades: 1, Volume: 40, Turnover: 392},
		ExchangeActor: {BuyOrders: 3, SellOrders: 3, Accepted: 3, Rejected: 3, Cancelled: 1, Amended: 1, Trades: 1, Volume: 40, Turnover: 392},
	}
	for actor, w := range want {
		if got[actor] != w {
			t.Errorf("%s:\n got  %+v\n want %+v", actor, got[actor], w)
		}
	}
	if _, live := e.DayReport("2001-01-01"); live {
		t.Fatal("a past day is not live")
	}

	// End of the day: the resting day order expires.
	e.AdvanceTo(PhaseClose)
	e.AdvanceTo(PhaseClosed)
	if got, _ := e.DayReport(today); got["FIX:AAA"].Expired != 1 || got[ExchangeActor].Expired != 1 {
		t.Fatalf("expired: %+v", got["FIX:AAA"])
	}

	// The saver gets today's figures once, and again only after a change.
	if p := e.PendingDays(); len(p) != 1 || p[0].Day != today || p[0].Actors[ExchangeActor].Trades != 1 {
		t.Fatalf("pending: %+v", p)
	}
	if p := e.PendingDays(); len(p) != 0 {
		t.Fatalf("nothing changed, but %d days pending", len(p))
	}
}

func TestDailyReportRollsOverAndSeeds(t *testing.T) {
	e := NewEngine(nil, NewBus())
	day1 := time.Date(2026, 10, 18, 23, 59, 0, 0, Dhaka)
	clock := day1
	e.days.now = func() time.Time { return clock }
	e.CountReject("FIX:AAA", Buy)
	clock = day1.Add(2 * time.Minute) // next Dhaka day
	e.CountReject("FIX:AAA", Sell)
	p := e.PendingDays()
	if len(p) != 2 || p[0].Day != "2026-10-18" || p[1].Day != "2026-10-19" ||
		p[0].Actors["FIX:AAA"].BuyOrders != 1 || p[1].Actors["FIX:AAA"].SellOrders != 1 {
		t.Fatalf("rollover: %+v", p)
	}

	// After a restart, today's saved figures are carried on.
	e2 := NewEngine(nil, NewBus())
	today := ReportDay(time.Now())
	e2.SeedDay(today, map[string]DayCounters{ExchangeActor: {Trades: 5, Volume: 500, Turnover: 1000}})
	e2.CountReject("FIX:AAA", Buy)
	got, _ := e2.DayReport(today)
	if c := got[ExchangeActor]; c.Trades != 5 || c.Rejected != 1 || c.Volume != 500 {
		t.Fatalf("seeded day: %+v", c)
	}
	e2.SeedDay("2001-01-01", map[string]DayCounters{ExchangeActor: {Trades: 9}})
	if got, _ := e2.DayReport(today); got[ExchangeActor].Trades != 5 {
		t.Fatal("seeding another day must not touch today")
	}
}
