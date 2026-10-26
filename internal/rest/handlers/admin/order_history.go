package admin

import (
	"net/http"
	"strconv"
	"strings"

	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/utils"
)

// OrderHistory serves /admin/api/report/order-history?date=&order_id=&broker_id=:
// the fills of one order from the trade log, for the Order Report's detail
// panel. broker_id is as the order report gives it (a FIX CompID, or
// "USER:<name>" for a Trade Terminal order); a user sees only their own.
func (h *Handler) OrderHistory(w http.ResponseWriter, r *http.Request) {
	day, err := reportDay(r)
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	q := r.URL.Query()
	actor := strings.TrimSpace(q.Get("broker_id"))
	if actor != "" && !strings.Contains(actor, ":") {
		actor = "FIX:" + actor
	}
	oid := strings.TrimSpace(q.Get("order_id"))
	if len(oid) <= 8 || actor == "" {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "order_id and broker_id are required"})
		return
	}
	seq, err := strconv.ParseUint(oid[8:], 10, 64) // OrderID = Dhaka date + the engine's order number
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "not an engine OrderID"})
		return
	}
	if !visibleActor(r)(actor) {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "not your broker's order"})
		return
	}
	type fill struct {
		Time    string  `json:"time"`
		TradeID uint64  `json:"trade_id"`
		Qty     int64   `json:"qty"`
		Price   float64 `json:"price"`
		Side    string  `json:"side"`
		Against string  `json:"against"`
	}
	fills := []fill{}
	for _, t := range h.dayTrades(day) {
		var side, other string
		switch {
		case t.Buy.Actor == actor && t.Buy.OrderID == seq:
			side, other = "Buy", t.Sell.Actor
		case t.Sell.Actor == actor && t.Sell.OrderID == seq:
			side, other = "Sell", t.Buy.Actor
		default:
			continue
		}
		label := "not recorded"
		if other != "" {
			_, label = brokerLabel(other)
		}
		fills = append(fills, fill{Time: t.Time.In(engine.Dhaka).Format("15:04:05"), TradeID: t.ID, Qty: t.Qty,
			Price: t.Price, Side: side, Against: label})
	}
	utils.WriteJSON(w, map[string]any{"ok": true, "fills": fills})
}
