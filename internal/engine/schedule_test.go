package engine

import (
	"testing"
	"time"
)

func TestScheduleFollowsTheDhakaClock(t *testing.T) {
	sc, err := ParseSchedule(DefaultSchedule)
	if err != nil {
		t.Fatal(err)
	}
	at := func(day, hhmm string) time.Time {
		ts, _ := time.ParseInLocation("2006-01-02 15:04", day+" "+hhmm, Dhaka)
		return ts
	}
	wed, fri := "2026-10-14", "2026-10-16"
	for _, c := range []struct{ day, at, want string }{
		{wed, "09:00", PhaseClosed}, {wed, "09:30", PhasePreOpen}, {wed, "09:59", PhasePreOpen},
		{wed, "10:00", PhaseOpen}, {wed, "14:29", PhaseOpen}, {wed, "14:30", PhaseClose},
		{wed, "14:45", PhasePostClose}, {wed, "15:00", PhaseClosed}, {fri, "11:00", PhaseClosed},
	} {
		if got := sc.PhaseAt(at(c.day, c.at)); got != c.want {
			t.Errorf("%s %s: %s, want %s", c.day, c.at, got, c.want)
		}
	}
	if _, err := ParseSchedule("10:00,09:00,11:00,12:00,13:00"); err == nil {
		t.Error("a schedule that goes backwards must be refused")
	}
}

// A day from the open to the next morning: close prices, post-close at the
// close price only, nothing accepted when closed, day orders expire, and the
// close becomes the next day's reference price.
func TestTradingDayHousekeeping(t *testing.T) {
	e := newJournalEngine(t) // GP, prev 10
	var expired []*ExecReport
	day := limit(e, "DAY1", Buy, 9.5, 100)
	day.Notify = func(r *ExecReport) {
		if r.Event == "EXPIRE" {
			expired = append(expired, r)
		}
	}
	submit(e, day)
	gtc := limit(e, "GTC1", Buy, 9.4, 100)
	gtc.TIF = GTC
	submit(e, gtc)
	submit(e, limit(e, "S1", Sell, 10.2, 50))
	if r := submit(e, limit(e, "B1", Buy, 10.2, 50)); r.Status != "FILLED" {
		t.Fatalf("trade: %s %s", r.Status, r.Reason)
	}

	e.AdvanceTo(PhasePostClose) // via Close: closing prices are published
	var closes int
	for _, ev := range readAll(t, e) {
		if ev.Kind == EvClose && ev.PriceRaw == 1020 {
			closes++
		}
	}
	if closes != 1 {
		t.Errorf("closing price events = %d, want 1 at 10.20", closes)
	}
	if r := submit(e, limit(e, "PC1", Buy, 10.1, 10)); r.Status != "REJECTED" || r.Reason != ReasonNotClosePrice {
		t.Errorf("post-close order off the close price: %s %q", r.Status, r.Reason)
	}
	if r := submit(e, limit(e, "PC2", Sell, 10.2, 10)); r.Status != "NEW" {
		t.Errorf("post-close order at the close price: %s %q", r.Status, r.Reason)
	}

	e.AdvanceTo(PhaseClosed)
	if r := submit(e, limit(e, "C1", Buy, 10, 10)); r.Status != "REJECTED" || r.Reason != ReasonNotAccepting {
		t.Errorf("order while closed: %s %q", r.Status, r.Reason)
	}
	if len(expired) != 1 || expired[0].ClOrdID != "DAY1" || expired[0].Status != "EXPIRED" {
		t.Fatalf("the day order must expire with a report, got %+v", expired)
	}
	if e.Open["GTC1"] == nil {
		t.Fatal("a GTC order must survive the end of the day")
	}

	e.AdvanceTo(PhasePreOpen) // next morning
	md := e.MdOf("GP")
	if md.Prev != 10.2 || md.Volume != 0 || md.Trades != 0 || md.Last != 10.2 {
		t.Fatalf("new day: prev %.2f last %.2f volume %d trades %d, want prev 10.20 and a fresh day", md.Prev, md.Last, md.Volume, md.Trades)
	}
	var sessions []byte
	for _, ev := range readAll(t, e) {
		if ev.Kind == EvSession {
			sessions = append(sessions, ev.Code)
		}
	}
	if string(sessions) != "MVECS" {
		t.Errorf("session events %q, want MVECS (close, post-close, end, end of messages, pre-open)", sessions)
	}
}
