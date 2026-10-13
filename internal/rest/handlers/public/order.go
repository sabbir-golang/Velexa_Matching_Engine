package public

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/utils"
)

// Order serves /api/order.
// external order entry (this is where the OMS / fix acceptor will feed orders)
func (h *Handler) Order(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		Symbol  string  `json:"symbol"`
		Side    string  `json:"side"`
		Type    string  `json:"type"`
		Price   float64 `json:"price"`
		Qty     int64   `json:"qty"`
		Actor   string  `json:"actor"`
		ClOrdID string  `json:"cl_ord_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "malformed order JSON", 400)
		return
	}
	req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	if req.Qty <= 0 {
		http.Error(w, fmt.Sprintf("invalid quantity %d: must be greater than zero", req.Qty), 400)
		return
	}
	if !eng.Listed(req.Symbol) && !eng.AutoList(req.Symbol, req.Price) {
		http.Error(w, fmt.Sprintf("unknown symbol %s: not in this engine's instrument universe (%s)",
			req.Symbol, engine.UniverseSource()), 400)
		return
	}
	side := engine.Buy
	if req.Side == "SELL" || req.Side == "sell" {
		side = engine.Sell
	}
	typ := engine.Limit
	if req.Type == "MARKET" || req.Type == "market" || req.Price <= 0 {
		typ = engine.Market
	}
	actor := req.Actor
	if actor == "" {
		actor = "API"
	}
	o := eng.NewOrder(req.Symbol, side, typ, req.Price, req.Qty, actor)
	o.ClOrdID = req.ClOrdID
	rep := eng.SubmitAndWait(o)
	utils.WriteJSON(w, rep)
}
