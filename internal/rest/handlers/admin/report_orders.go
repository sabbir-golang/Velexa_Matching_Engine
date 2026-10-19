package admin

import (
	"encoding/csv"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/database/orderlog"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Order report: every order of a trading day — time, broker, OrderID, client,
// symbol, side, type, quantity, price, filled, status, board, and more — with
// filters, and as an Excel (.xlsx) or CSV download. An admin sees every
// broker; a user only their own brokers and their own Trade Terminal orders.

// orderRow is one report line, as the page and the downloads show it.
type orderRow struct {
	Time     string  `json:"time"`
	Broker   string  `json:"broker"`
	BrokerID string  `json:"broker_id"`
	OrderID  string  `json:"order_id"`
	Client   string  `json:"client"`
	Symbol   string  `json:"symbol"`
	Side     string  `json:"side"`
	Type     string  `json:"type"`
	Qty      int64   `json:"qty"`
	Price    float64 `json:"price"`
	Filled   int64   `json:"filled"`
	Status   string  `json:"status"`
	Board    string  `json:"board"`
	AvgPx    float64 `json:"avg_px"`
	Account  string  `json:"account"`
	ClOrdID  string  `json:"cl_ord_id"`
	Trader   string  `json:"trader"`
	TIF      string  `json:"tif"`
	Reason   string  `json:"reason"`
}

// brokerLabel names an order's source: the broker's company (or CompID) for
// FIX orders, the console user for Trade Terminal orders.
func brokerLabel(actor string) (id, label string) {
	if sender, ok := strings.CutPrefix(actor, "FIX:"); ok {
		if rec := brokers.Meta.Get(sender); rec != nil && rec.Company != "" {
			return sender, rec.Company
		}
		return sender, sender
	}
	if user, ok := strings.CutPrefix(actor, "USER:"); ok {
		return actor, "Console (" + user + ")"
	}
	return actor, actor
}

func toRow(r orderlog.Record) orderRow {
	id, label := brokerLabel(r.Actor)
	return orderRow{Time: r.Entered.In(engine.Dhaka).Format("2006-01-02 15:04:05"), Broker: label, BrokerID: id,
		OrderID: r.OrderID, Client: r.Client, Symbol: r.Symbol, Side: r.Side, Type: r.Type, Qty: r.Qty,
		Price: r.Price, Filled: r.Filled, Status: r.Status, Board: r.Board, AvgPx: r.AvgPx, Account: r.Account,
		ClOrdID: r.ClOrdID, Trader: r.Trader, TIF: r.TIF, Reason: r.Reason}
}

// OrderReport serves /admin/api/report/orders: ?date= (default today) and the
// filters broker, symbol, side, status and q (OrderID, ClOrdID, client or
// account); &format=xlsx or csv downloads every matching order.
func (h *Handler) OrderReport(w http.ResponseWriter, r *http.Request) {
	day, err := reportDay(r)
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if h.Orders == nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "the order log is not available"})
		return
	}
	recs, err := h.Orders.List(day)
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "the orders of " + day + " could not be read"})
		return
	}
	q := r.URL.Query()
	fBroker, fSymbol := strings.TrimSpace(q.Get("broker")), strings.ToUpper(strings.TrimSpace(q.Get("symbol")))
	fSide, fStatus := strings.ToLower(q.Get("side")), strings.ToLower(q.Get("status"))
	fText := strings.ToLower(strings.TrimSpace(q.Get("q")))

	all, own := middleware.BrokerScope(r)
	me := "USER:" + middleware.Username(r)
	visible := func(actor string) bool {
		if all || actor == me {
			return true
		}
		s, ok := strings.CutPrefix(actor, "FIX:")
		return ok && own[strings.ToUpper(s)]
	}
	rows := []orderRow{}
	brokerSet := map[string]string{}
	statusCount := map[string]int{}
	var filledQty int64
	var turnover float64
	for _, rec := range recs {
		if !visible(rec.Actor) {
			continue
		}
		row := toRow(rec)
		brokerSet[row.BrokerID] = row.Broker
		if (fBroker != "" && !strings.EqualFold(row.BrokerID, fBroker)) ||
			(fSymbol != "" && row.Symbol != fSymbol) ||
			(fSide != "" && strings.ToLower(row.Side) != fSide) ||
			(fStatus != "" && strings.ToLower(row.Status) != fStatus) ||
			(fText != "" && !strings.Contains(strings.ToLower(row.OrderID+" "+row.ClOrdID+" "+row.Client+" "+row.Account), fText)) {
			continue
		}
		rows = append(rows, row)
		statusCount[row.Status]++
		filledQty += row.Filled
		turnover += float64(row.Filled) * row.AvgPx
	}

	switch q.Get("format") {
	case "xlsx":
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="orders-`+day+`.xlsx"`)
		cells := make([][]utils.Cell, 0, len(rows))
		for _, x := range rows {
			cells = append(cells, []utils.Cell{utils.Text(x.Time), utils.Text(x.Broker), utils.Text(x.OrderID),
				utils.Text(x.Client), utils.Text(x.Symbol), utils.Text(x.Side), utils.Text(x.Type), utils.Int(x.Qty),
				utils.Dec2(x.Price), utils.Int(x.Filled), utils.Text(x.Status), utils.Text(x.Board), utils.Dec2(x.AvgPx),
				utils.Text(x.Account), utils.Text(x.ClOrdID), utils.Text(x.Trader), utils.Text(x.TIF), utils.Text(x.Reason)})
		}
		utils.WriteXLSX(w, "Orders "+day, orderColumns, cells)
		return
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="orders-`+day+`.csv"`)
		cw := csv.NewWriter(w)
		head := make([]string, len(orderColumns))
		for i, c := range orderColumns {
			head[i] = c.Header
		}
		cw.Write(head)
		f2 := func(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }
		for _, x := range rows {
			cw.Write([]string{x.Time, x.Broker, x.OrderID, x.Client, x.Symbol, x.Side, x.Type, strconv.FormatInt(x.Qty, 10),
				f2(x.Price), strconv.FormatInt(x.Filled, 10), x.Status, x.Board, f2(x.AvgPx), x.Account, x.ClOrdID,
				x.Trader, x.TIF, x.Reason})
		}
		cw.Flush()
		return
	}

	type opt struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	opts := []opt{}
	for id, label := range brokerSet {
		opts = append(opts, opt{id, label})
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].Label < opts[j].Label })
	total := len(rows)
	const pageMax = 2000 // the page shows the newest; downloads have all
	if len(rows) > pageMax {
		rows = rows[:pageMax]
	}
	utils.WriteJSON(w, map[string]any{"ok": true, "date": day, "today": day == engine.ReportDay(timeNow()),
		"scope": map[bool]string{true: "exchange", false: "own"}[all], "total": total, "shown": len(rows),
		"rows": rows, "brokers": opts, "by_status": statusCount, "filled_qty": filledQty,
		"turnover": engine.Round2(turnover)})
}

// orderColumns are the download columns: the DSE order sheet, then extras.
var orderColumns = []utils.XLSXColumn{
	{Header: "Time", Width: 20}, {Header: "Broker", Width: 24}, {Header: "OrderID", Width: 22},
	{Header: "Client", Width: 10}, {Header: "Symbol", Width: 13}, {Header: "Side", Width: 7},
	{Header: "Type", Width: 9}, {Header: "Qty", Width: 10}, {Header: "Price", Width: 10},
	{Header: "Filled", Width: 10}, {Header: "Status", Width: 10}, {Header: "Board", Width: 11},
	{Header: "Avg price", Width: 11}, {Header: "BO account", Width: 19}, {Header: "ClOrdID", Width: 24},
	{Header: "Trader", Width: 13}, {Header: "Validity", Width: 9}, {Header: "Reject reason", Width: 48},
}
