package public

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

// Order serves POST /api/order: manual order entry from the Trade Terminal by a
// signed-in console user, who is recorded as the order's actor ("USER:<name>").
func (h *Handler) Order(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	middleware.LimitBody(w, r)
	var req struct {
		Symbol  string  `json:"symbol"`
		Side    string  `json:"side"`
		Type    string  `json:"type"`
		Price   float64 `json:"price"`
		Qty     int64   `json:"qty"`
		ClOrdID string  `json:"cl_ord_id"`
		TIF     string  `json:"tif"` // DAY (default), IOC, FOK or GTC
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "malformed order JSON", 400)
		return
	}
	req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	side := engine.Buy
	if req.Side == "SELL" || req.Side == "sell" {
		side = engine.Sell
	}
	actor := "USER:" + middleware.Username(r)
	reject := func(reason string) {
		o := eng.NewOrder(req.Symbol, side, engine.Limit, req.Price, req.Qty, actor)
		o.ClOrdID = req.ClOrdID
		eng.RejectUnsubmitted(o, reason) // counted, and listed in the order report
		http.Error(w, reason, 400)
	}
	if req.Qty <= 0 {
		reject(fmt.Sprintf("invalid quantity %d: must be greater than zero", req.Qty))
		return
	}
	if !eng.Listed(req.Symbol) && !eng.AutoList(req.Symbol, req.Price) {
		reject(fmt.Sprintf("unknown symbol %s: not in this engine's instrument universe (%s)", req.Symbol, engine.UniverseSource()))
		return
	}
	typ := engine.Limit
	if req.Type == "MARKET" || req.Type == "market" || req.Price <= 0 {
		typ = engine.Market
	}
	o := eng.NewOrder(req.Symbol, side, typ, req.Price, req.Qty, actor)
	o.ClOrdID = req.ClOrdID
	switch strings.ToUpper(strings.TrimSpace(req.TIF)) {
	case "IOC":
		o.TIF = engine.IOC
	case "FOK":
		o.TIF = engine.FOK
	case "GTC":
		o.TIF = engine.GTC
	}
	rep := eng.SubmitAndWait(o)
	utils.WriteJSON(w, rep)
}

// CancelOrder serves POST /api/order/cancel {cl_ord_id}: a signed-in console
// user cancels one of their own working Trade Terminal orders.
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	middleware.LimitBody(w, r)
	var req struct {
		ClOrdID string `json:"cl_ord_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.ClOrdID) == "" {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "cl_ord_id is required"})
		return
	}
	cl := strings.TrimSpace(req.ClOrdID)
	rep := h.Eng.CancelAs(cl, cl+"-X", "USER:"+middleware.Username(r)) // only the user's own orders match
	if rep.Status != "CANCELED" {
		msg := "the order is not working (already filled, cancelled or not yours)"
		if rep.Reason != "" {
			msg = rep.Reason
		}
		utils.WriteJSON(w, map[string]any{"ok": false, "error": msg})
		return
	}
	utils.WriteJSON(w, map[string]any{"ok": true, "report": rep})
}
