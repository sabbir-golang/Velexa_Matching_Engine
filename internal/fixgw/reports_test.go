package fixgw

import (
	"strings"
	"testing"
	"time"

	"github.com/quickfixgo/quickfix"

	"velexa-sim-me/internal/engine"
)

// er is a decoded execution report, enough to check the fields the OMS reads.
type er map[quickfix.Tag]string

func decode(m *quickfix.Message) er {
	out := er{}
	for _, t := range []quickfix.Tag{tOrderID, tClOrdID, tOrigClOrdID, tExecID, tExecType, tOrdStatus, tSymbol, tSide,
		tOrderQty, tOrdType, tPrice, tTimeInForce, tAccount, tCumQty, tLeavesQty, tAvgPx, tLastPx, tLastQty,
		tTrdMatchID, tTradeDate, tSettlDate, tGrossTradeAmt, tAggressor, tOrdRejReason, tText, tSecSubType, tOrderRestrict} {
		if v, err := m.Body.GetString(t); err == nil {
			out[t] = v
		}
	}
	if v, err := m.Header.GetString(tTargetSubID); err == nil {
		out[tTargetSubID] = v
	}
	return out
}

func kindsOf(rs []er) string {
	var b []string
	for _, r := range rs {
		b = append(b, r[tExecType]+"/"+r[tOrdStatus])
	}
	return strings.Join(b, " ")
}

// broker collects what the gateway would send to one session.
type broker struct {
	fa  *Acceptor
	ctx *orderCtx
	got chan *engine.ExecReport
}

func newBroker(fa *Acceptor, account string) *broker {
	return &broker{fa: fa, got: make(chan *engine.ExecReport, 64),
		ctx: &orderCtx{account: account, ordType: "2", tif: "0", subType: "PUBLIC", restrictions: "N", senderSub: "TRDR1",
			parties: []party{{"C123", "C", "5"}}}}
}

func (b *broker) order(t *testing.T, cl string, side engine.Side, typ engine.OrdType, px float64, qty int64, tif engine.TIF) {
	t.Helper()
	o := b.fa.eng.NewOrder("GP", side, typ, px, qty, "FIX:"+b.ctx.account)
	o.ClOrdID, o.TIF, o.Ctx = cl, tif, b.ctx
	o.Notify = func(rep *engine.ExecReport) { b.got <- rep }
	b.fa.eng.SubmitAndWait(o)
}

// reports drains what arrived and expands it like the sender goroutine does.
func (b *broker) reports() []er {
	var out []er
	for {
		select {
		case rep := <-b.got:
			for _, m := range b.fa.reports(rep, b.ctx) {
				out = append(out, decode(m))
			}
		case <-time.After(100 * time.Millisecond):
			return out
		}
	}
}

func TestReportsLikeTheDSE(t *testing.T) {
	fa := &Acceptor{eng: newTestEngine(t)}
	seller, buyer := newBroker(fa, "SELLER"), newBroker(fa, "BUYER")

	seller.order(t, "S1", engine.Sell, engine.Limit, 331.20, 100, engine.Day)
	if rs := seller.reports(); kindsOf(rs) != "0/0" || rs[0][tLeavesQty] != "100" || rs[0][tCumQty] != "0" {
		t.Fatalf("resting order: want one ack 0/0 with leaves 100, got %s %v", kindsOf(rs), rs)
	}

	buyer.order(t, "B1", engine.Buy, engine.Limit, 331.20, 60, engine.Day)
	b := buyer.reports()
	if kindsOf(b) != "0/0 F/2" {
		t.Fatalf("aggressor: want ack then one fill, got %s", kindsOf(b))
	}
	s := seller.reports()
	if kindsOf(s) != "F/1" {
		t.Fatalf("resting side: want one partial fill, got %s", kindsOf(s))
	}
	bf, sf := b[1], s[0]
	if bf[tTrdMatchID] == "" || bf[tTrdMatchID] != sf[tTrdMatchID] {
		t.Errorf("both sides must carry the same TrdMatchID: %q vs %q", bf[tTrdMatchID], sf[tTrdMatchID])
	}
	if bf[tAggressor] != "Y" || sf[tAggressor] != "N" {
		t.Errorf("aggressor flags %q/%q, want Y/N", bf[tAggressor], sf[tAggressor])
	}
	for _, c := range []struct {
		r          er
		cum, leave string
	}{{bf, "60", "0"}, {sf, "60", "40"}} {
		if c.r[tCumQty] != c.cum || c.r[tLeavesQty] != c.leave || c.r[tLastQty] != "60" || c.r[tLastPx] != "331.20" {
			t.Errorf("fill: cum %s leaves %s last %s@%s, want cum %s leaves %s 60@331.20",
				c.r[tCumQty], c.r[tLeavesQty], c.r[tLastQty], c.r[tLastPx], c.cum, c.leave)
		}
		if c.r[tGrossTradeAmt] != "19872.00" || c.r[tTradeDate] == "" || c.r[tSettlDate] <= c.r[tTradeDate] {
			t.Errorf("trade fields: gross %s trade %s settle %s", c.r[tGrossTradeAmt], c.r[tTradeDate], c.r[tSettlDate])
		}
	}
	if bf[tExecID] == sf[tExecID] || bf[tExecID] == b[0][tExecID] {
		t.Error("ExecIDs must be unique")
	}
	if !strings.HasPrefix(b[0][tOrderID], time.Now().In(dhakaTZ).Format("20060102")) || bf[tOrderID] != b[0][tOrderID] {
		t.Errorf("OrderID must be DSE style and stable: %q / %q", b[0][tOrderID], bf[tOrderID])
	}
	// echoed order fields
	for tag, want := range map[quickfix.Tag]string{tAccount: "BUYER", tOrdType: "2", tTimeInForce: "0", tPrice: "331.20",
		tSecSubType: "PUBLIC", tOrderRestrict: "N", tTargetSubID: "TRDR1", tSide: "1", tOrderQty: "60"} {
		if bf[tag] != want {
			t.Errorf("tag %d = %q, want %q", tag, bf[tag], want)
		}
	}

	// IOC that cannot fully fill: ack, fill, cancel of the rest
	buyer.order(t, "B2", engine.Buy, engine.Limit, 331.20, 100, engine.IOC)
	if got := kindsOf(buyer.reports()); got != "0/0 F/1 4/4" {
		t.Fatalf("IOC remainder: got %s, want 0/0 F/1 4/4", got)
	}
	seller.reports()

	// cancel confirmation with 11 = cancel id and 41 = original
	seller.order(t, "S2", engine.Sell, engine.Limit, 340, 10, engine.Day)
	seller.reports()
	rep := fa.eng.CancelAs("S2", "S2-CXL", "FIX:SELLER")
	if rep.Status != "CANCELED" {
		t.Fatalf("cancel: %s", rep.Status)
	}
	c := seller.reports()
	if kindsOf(c) != "4/4" || c[0][tClOrdID] != "S2-CXL" || c[0][tOrigClOrdID] != "S2" {
		t.Fatalf("cancel confirmation: %s %v", kindsOf(c), c)
	}
	// another broker cannot cancel it
	seller.order(t, "S3", engine.Sell, engine.Limit, 340, 10, engine.Day)
	seller.reports()
	if rep := fa.eng.CancelAs("S3", "X", "FIX:BUYER"); rep.Status != "REJECT_CXL" {
		t.Fatalf("a cancel from another broker must be refused, got %s", rep.Status)
	}

	// replace: 150=5 with 41 = original
	fa.eng.ReplaceAs("S3", "S3-R", "FIX:SELLER", 20, 341)
	r := seller.reports()
	if kindsOf(r) != "5/0" || r[0][tOrigClOrdID] != "S3" || r[0][tClOrdID] != "S3-R" || r[0][tOrderQty] != "20" {
		t.Fatalf("replace: %s %v", kindsOf(r), r)
	}
}

func TestRejectsUseTheDSEForm(t *testing.T) {
	fa := &Acceptor{eng: newTestEngine(t)}
	ctx := &orderCtx{ordType: "2"}
	for reason, want := range map[string]string{
		"Minimum quantity step: 1000": "(792): Minimum quantity step: 1000",
		engine.ReasonNotAccepting:     "(293): Orders are not being accepted at this time",
		engine.ReasonNotClosePrice:    "(1116): Order price is not equal to close price",
		"price band: 6.00 outside +/-10.00% limit [4.86-5.94] of prev 5.40": "price band: 6.00 outside +/-10.00% limit [4.86-5.94] of prev 5.40",
	} {
		ms := fa.reports(&engine.ExecReport{ClOrdID: "X", Symbol: "GP", Status: "REJECTED", Reason: reason, OrderQty: 10}, ctx)
		r := decode(ms[0])
		if len(ms) != 1 || r[tExecType] != "8" || r[tOrdRejReason] != "99" || r[tText] != want || r[tOrderID] != "NONE" {
			t.Errorf("%q -> 150=%s 103=%s 37=%s 58=%q, want 8/99/NONE %q", reason, r[tExecType], r[tOrdRejReason], r[tOrderID], r[tText], want)
		}
	}
	cr := cxlReject("C1", "O1", "1")
	for tag, want := range map[quickfix.Tag]string{tOrderID: "NONE", tOrdStatus: "8", tCxlRejReason: "99", tText: "(226): Invalid order number", tOrigClOrdID: "O1"} {
		if v, _ := cr.Body.GetString(tag); v != want {
			t.Errorf("cancel reject tag %d = %q, want %q", tag, v, want)
		}
	}
}

func TestSettlementSkipsTheWeekend(t *testing.T) {
	thu := time.Date(2026, 10, 15, 11, 0, 0, 0, dhakaTZ)                 // Thursday
	if got := settlDate(thu).Format("2006-01-02"); got != "2026-10-19" { // Fri/Sat off -> Sun, Mon
		t.Fatalf("T+2 from Thursday = %s, want 2026-10-19", got)
	}
}
