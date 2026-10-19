package fixgw

// Execution reports in the shape the DSE sends them, which is what the OMS
// books cash and positions from:
//   - a new order is acknowledged (150=0) before it trades;
//   - every trade is its own report (150=F) to BOTH sides, with TrdMatchID (880),
//     trade and settlement dates, gross amount and the aggressor flag;
//   - an IOC / FOK / market remainder that does not rest is cancelled with 150=4;
//   - cancel and replace confirmations carry the request's ClOrdID (11) and the
//     original (41); an order that expires is reported with 150=C;
//   - rejects carry OrdRejReason 103=99 and the DSE "(nnn): text" form;
//   - the order's own fields (1, 40, 59, 44, parties, 762, 529) are echoed.
// Reports are queued and sent by one goroutine, in the order the engine made them.

import (
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/quickfixgo/quickfix"

	"velexa-sim-me/internal/engine"
)

const (
	tAccount        quickfix.Tag = 1
	tExecInst       quickfix.Tag = 18
	tSenderSubID    quickfix.Tag = 50
	tTargetSubID    quickfix.Tag = 57
	tSettlDate      quickfix.Tag = 64
	tTradeDate      quickfix.Tag = 75
	tOrdRejReason   quickfix.Tag = 103
	tMinQty         quickfix.Tag = 110
	tGrossTradeAmt  quickfix.Tag = 381
	tNoPartyIDs     quickfix.Tag = 453
	tPartyIDSource  quickfix.Tag = 447
	tPartyID        quickfix.Tag = 448
	tPartyRole      quickfix.Tag = 452
	tOrderRestrict  quickfix.Tag = 529
	tSecSubType     quickfix.Tag = 762
	tOrdStatusReqID quickfix.Tag = 790
	tTrdMatchID     quickfix.Tag = 880
	tAggressor      quickfix.Tag = 1057
	tDisplayQty     quickfix.Tag = 1138
)

var dhakaTZ = time.FixedZone("Asia/Dhaka", 6*3600)

type party struct{ id, src, role string }

// orderCtx is what the gateway remembers about an order to echo it on every report.
type orderCtx struct {
	sid          quickfix.SessionID
	account      string
	ordType      string // tag 40 as sent
	tif          string // tag 59 as sent
	subType      string // tag 762
	restrictions string // tag 529
	senderSub    string // tag 50 on the request, echoed as 57
	parties      []party
	orderID      string // DSE-style OrderID, fixed by the first report
}

func partiesTemplate() quickfix.GroupTemplate {
	return quickfix.GroupTemplate{quickfix.GroupElement(tPartyID), quickfix.GroupElement(tPartyIDSource), quickfix.GroupElement(tPartyRole)}
}

// info is what the order says about whom it is for, for the order report: the
// BO account (1), the client (448 with role 452=5, else the first party), the
// board (762) and the trader (50).
func (c *orderCtx) info() engine.OrderInfo {
	client := ""
	for _, p := range c.parties {
		if p.role == "5" {
			client = p.id
			break
		}
	}
	if client == "" && len(c.parties) > 0 {
		client = c.parties[0].id
	}
	return engine.OrderInfo{Account: c.account, Client: client, Board: c.subType, Trader: c.senderSub}
}

// newOrderCtx reads the fields of an order message that the reports echo.
func newOrderCtx(m *quickfix.Message, sid quickfix.SessionID) *orderCtx {
	get := func(t quickfix.Tag) string { v, _ := m.Body.GetString(t); return v }
	c := &orderCtx{sid: sid, account: get(tAccount), ordType: get(tOrdType), tif: get(tTimeInForce),
		subType: get(tSecSubType), restrictions: get(tOrderRestrict)}
	c.senderSub, _ = m.Header.GetString(tSenderSubID)
	rg := quickfix.NewRepeatingGroup(tNoPartyIDs, partiesTemplate())
	if m.Body.GetGroup(rg) == nil {
		for i := 0; i < rg.Len(); i++ {
			g := rg.Get(i)
			id, _ := g.GetString(tPartyID)
			src, _ := g.GetString(tPartyIDSource)
			role, _ := g.GetString(tPartyRole)
			c.parties = append(c.parties, party{id, src, role})
		}
	}
	return c
}

// outItem is one queued outbound message: an execution report to expand, or a
// ready-made message.
type outItem struct {
	rep *engine.ExecReport
	msg *quickfix.Message
	sid quickfix.SessionID
}

var execBase = strconv.FormatInt(time.Now().Unix(), 16)

func (fa *Acceptor) nextExecID() string {
	return strings.ToUpper(execBase + strconv.FormatUint(atomic.AddUint64(&fa.execSeq, 1), 16))
}

// startSender starts the goroutine that sends queued messages in order.
func (fa *Acceptor) startSender() {
	fa.out = make(chan outItem, 1<<16)
	go func() {
		for it := range fa.out {
			fa.sendNow(it)
		}
	}()
}

func (fa *Acceptor) sendNow(it outItem) {
	if it.msg != nil {
		if err := quickfix.SendToTarget(it.msg, it.sid); err != nil {
			log.Printf("[FIX] send failed: %v", err)
		}
		return
	}
	ctx, _ := it.rep.Ctx.(*orderCtx)
	if ctx == nil {
		return
	}
	for _, m := range fa.reports(it.rep, ctx) {
		if err := quickfix.SendToTarget(m, ctx.sid); err != nil {
			log.Printf("[FIX] send exec report failed: %v", err)
		}
	}
}

// deliver queues a report from the engine (it is the orders' Notify; it must not block for long).
func (fa *Acceptor) deliver(rep *engine.ExecReport) {
	if fa.out == nil {
		fa.sendNow(outItem{rep: rep})
		return
	}
	fa.out <- outItem{rep: rep}
}

func (fa *Acceptor) queueMsg(m *quickfix.Message, sid quickfix.SessionID) {
	if fa.out == nil {
		fa.sendNow(outItem{msg: m, sid: sid})
		return
	}
	fa.out <- outItem{msg: m, sid: sid}
}

// dseRejectCodes are the DSE error numbers for the reject texts we share with it.
var dseRejectCodes = []struct{ prefix, code string }{
	{"Minimum quantity step", "792"},
	{engine.ReasonNotAccepting, "293"},
	{engine.ReasonNotClosePrice, "1116"},
}

// dseText formats a reject reason the way the DSE does: "(nnn): text" when we
// know the DSE number, otherwise the text alone (no invented numbers).
func dseText(reason string) string {
	if reason == "" {
		reason = "rejected"
	}
	for _, c := range dseRejectCodes {
		if strings.HasPrefix(reason, c.prefix) {
			return "(" + c.code + "): " + reason
		}
	}
	return reason
}

// settlDate is the settlement date: two Dhaka business days (Sunday to Thursday) after d.
func settlDate(d time.Time) time.Time {
	for n := 0; n < 2; {
		d = d.AddDate(0, 0, 1)
		if wd := d.Weekday(); wd != time.Friday && wd != time.Saturday {
			n++
		}
	}
	return d
}

func px2(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

// erBase starts an execution report with the fields every report carries.
func (fa *Acceptor) erBase(rep *engine.ExecReport, ctx *orderCtx, execType, ordStatus string, cum, leaves int64, avg float64) *quickfix.Message {
	m := quickfix.NewMessage()
	m.Header.SetString(tMsgType, "8")
	if ctx.senderSub != "" {
		m.Header.SetString(tTargetSubID, ctx.senderSub)
	}
	oid := "NONE"
	if rep.OrderID != 0 {
		if ctx.orderID == "" {
			ctx.orderID = time.Now().In(dhakaTZ).Format("20060102") + strconv.FormatUint(rep.OrderID, 10)
		}
		oid = ctx.orderID
	}
	b := &m.Body
	b.SetString(tOrderID, oid)
	b.SetString(tClOrdID, rep.ClOrdID)
	if rep.OrigClOrdID != "" {
		b.SetString(tOrigClOrdID, rep.OrigClOrdID)
	}
	b.SetString(tExecID, fa.nextExecID())
	b.SetString(tExecType, execType)
	b.SetString(tOrdStatus, ordStatus)
	if ctx.account != "" {
		b.SetString(tAccount, ctx.account)
	}
	b.SetString(tSymbol, rep.Symbol)
	side := "1"
	if rep.Side == engine.Sell {
		side = "2"
	}
	b.SetString(tSide, side)
	ordQty := rep.OrderQty
	if ordQty <= 0 {
		ordQty = rep.FilledQty + rep.LeavesQty
	}
	b.SetString(tOrderQty, strconv.FormatInt(ordQty, 10))
	if ctx.ordType != "" {
		b.SetString(tOrdType, ctx.ordType)
	}
	if rep.Price > 0 && ctx.ordType != "1" {
		b.SetString(tPrice, px2(rep.Price))
	}
	if ctx.tif != "" {
		b.SetString(tTimeInForce, ctx.tif)
	}
	b.SetString(tCumQty, strconv.FormatInt(cum, 10))
	b.SetString(tLeavesQty, strconv.FormatInt(leaves, 10))
	b.SetString(tAvgPx, px2(avg))
	if ctx.subType != "" {
		b.SetString(tSecSubType, ctx.subType)
	}
	if ctx.restrictions != "" {
		b.SetString(tOrderRestrict, ctx.restrictions)
	}
	if len(ctx.parties) > 0 {
		rg := quickfix.NewRepeatingGroup(tNoPartyIDs, partiesTemplate())
		for _, p := range ctx.parties {
			g := rg.Add()
			g.SetString(tPartyID, p.id)
			if p.src != "" {
				g.SetString(tPartyIDSource, p.src)
			}
			if p.role != "" {
				g.SetString(tPartyRole, p.role)
			}
		}
		b.SetGroup(rg)
	}
	b.SetString(tTransactTime, time.Now().UTC().Format("20060102-15:04:05.000"))
	return m
}

// workingStatus is OrdStatus for a report that is not a fill.
func workingStatus(cum, ordQty int64) string {
	switch {
	case cum <= 0:
		return "0"
	case cum >= ordQty:
		return "2"
	}
	return "1"
}

// reports expands one engine report into the execution reports the DSE would send.
func (fa *Acceptor) reports(rep *engine.ExecReport, ctx *orderCtx) []*quickfix.Message {
	ordQty := rep.OrderQty
	if ordQty <= 0 {
		ordQty = rep.FilledQty + rep.LeavesQty
	}
	switch {
	case rep.Status == "REJECTED":
		r := *rep
		r.OrderID = 0 // the DSE gives a rejected order no OrderID: 37=NONE
		m := fa.erBase(&r, ctx, "8", "8", 0, 0, 0)
		m.Body.SetString(tOrdRejReason, "99")
		m.Body.SetString(tText, dseText(rep.Reason))
		return []*quickfix.Message{m}
	case rep.Event == "STATUS":
		m := fa.erBase(rep, ctx, "I", workingStatus(rep.FilledQty, ordQty), rep.FilledQty, rep.LeavesQty, rep.AvgPx)
		if rep.ReqID != "" {
			m.Body.SetString(tOrdStatusReqID, rep.ReqID)
		}
		return []*quickfix.Message{m}
	case rep.Event == "CANCEL":
		return []*quickfix.Message{fa.erBase(rep, ctx, "4", "4", rep.FilledQty, 0, rep.AvgPx)}
	case rep.Event == "EXPIRE":
		return []*quickfix.Message{fa.erBase(rep, ctx, "C", "C", rep.FilledQty, 0, rep.AvgPx)}
	case rep.Status == "PENDING": // a stop accepted and parked until it triggers
		return []*quickfix.Message{fa.erBase(rep, ctx, "A", "A", 0, ordQty, 0)}
	}

	// A new order, a replace or a triggered stop: acknowledge, then one report per
	// trade, then cancel whatever did not rest.
	var filledNow int64
	var valueNow float64
	for _, f := range rep.Fills {
		filledNow += f.Qty
		valueNow += float64(f.Qty) * f.Price
	}
	cum := rep.FilledQty - filledNow
	value := rep.CumValue - valueNow
	avg := func() float64 {
		if cum <= 0 {
			return 0
		}
		return engine.Round2(value / float64(cum))
	}
	var out []*quickfix.Message
	switch rep.Event {
	case "FILL": // resting order: the trades only
	case "REPLACE":
		if rep.Status == "CANCELED" && len(rep.Fills) == 0 { // amended down to what had already traded
			return []*quickfix.Message{fa.erBase(rep, ctx, "4", "4", rep.FilledQty, 0, rep.AvgPx)}
		}
		out = append(out, fa.erBase(rep, ctx, "5", workingStatus(cum, ordQty), cum, ordQty-cum, avg()))
	default:
		out = append(out, fa.erBase(rep, ctx, "0", workingStatus(cum, ordQty), cum, ordQty-cum, avg()))
	}
	now := time.Now().In(dhakaTZ)
	aggr := "N"
	if rep.Aggressor {
		aggr = "Y"
	}
	for _, f := range rep.Fills {
		cum += f.Qty
		value += float64(f.Qty) * f.Price
		status := "1"
		if cum >= ordQty {
			status = "2"
		}
		m := fa.erBase(rep, ctx, "F", status, cum, ordQty-cum, avg())
		b := &m.Body
		b.SetString(tLastPx, px2(f.Price))
		b.SetString(tLastQty, strconv.FormatInt(f.Qty, 10))
		b.SetString(tTrdMatchID, strconv.FormatUint(f.ID, 10))
		b.SetString(tTradeDate, now.Format("20060102"))
		b.SetString(tSettlDate, settlDate(now).Format("20060102"))
		b.SetString(tGrossTradeAmt, px2(float64(f.Qty)*f.Price))
		b.SetString(tAggressor, aggr)
		out = append(out, m)
	}
	if rep.Event != "FILL" && rep.LeavesQty == 0 && cum < ordQty {
		out = append(out, fa.erBase(rep, ctx, "4", "4", cum, 0, avg()))
	}
	return out
}

// cxlReject builds an OrderCancelReject (35=9) the way the DSE sends it.
func cxlReject(cl, orig, respTo string) *quickfix.Message {
	return cxlRejectFor(&engine.ExecReport{}, cl, orig, respTo)
}

// cxlRejectFor is the OrderCancelReject for a cancel/amend the engine refused.
// An unknown order gets the DSE's "(226): Invalid order number"; a known order
// whose amend broke a rule (band, tick, lot, session, duplicate ClOrdID) gets
// that reason, its OrderID and its current status.
func cxlRejectFor(rep *engine.ExecReport, cl, orig, respTo string) *quickfix.Message {
	m := quickfix.NewMessage()
	m.Header.SetString(tMsgType, "9")
	oid, status, text := "NONE", "8", "(226): Invalid order number"
	if rep.Reason != "" {
		text = dseText(rep.Reason)
		if ctx, ok := rep.Ctx.(*orderCtx); ok && ctx.orderID != "" {
			oid, status = ctx.orderID, "0"
		}
	}
	m.Body.SetString(tOrderID, oid)
	m.Body.SetString(tClOrdID, cl)
	m.Body.SetString(tOrigClOrdID, orig)
	m.Body.SetString(tOrdStatus, status)
	m.Body.SetString(tCxlRejRespTo, respTo)
	m.Body.SetString(tCxlRejReason, "99")
	m.Body.SetString(tText, text)
	return m
}

// statusUnknown is the reply to a status request for an order that is not working.
func (fa *Acceptor) statusUnknown(m *quickfix.Message) *quickfix.Message {
	get := func(t quickfix.Tag) string { v, _ := m.Body.GetString(t); return v }
	r := quickfix.NewMessage()
	r.Header.SetString(tMsgType, "8")
	b := &r.Body
	b.SetString(tOrderID, "NONE")
	b.SetString(tClOrdID, get(tClOrdID))
	b.SetString(tExecID, fa.nextExecID())
	b.SetString(tExecType, "I")
	b.SetString(tOrdStatus, "8")
	if v := get(tOrdStatusReqID); v != "" {
		b.SetString(tOrdStatusReqID, v)
	}
	b.SetString(tSymbol, get(tSymbol))
	side := get(tSide)
	if side == "" {
		side = "1"
	}
	b.SetString(tSide, side)
	b.SetString(tLeavesQty, "0")
	b.SetString(tCumQty, "0")
	b.SetString(tAvgPx, "0.00")
	b.SetString(tText, "(226): Invalid order number")
	b.SetString(tTransactTime, time.Now().UTC().Format("20060102-15:04:05.000"))
	return r
}
