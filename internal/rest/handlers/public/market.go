package public

import (
	"net/http"

	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/utils"
)

// Status serves /api/status.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	eng.Mu.RLock()
	st := map[string]any{"phase": eng.Phase, "index": eng.Index, "instruments": len(eng.Instr()), "orders": eng.Seq, "trades": eng.TradeSeq, "running": eng.Running, "universe": engine.InstrumentStatus(eng)}
	eng.Mu.RUnlock()
	utils.WriteJSON(w, st)
}

// Market serves /api/market.
func (h *Handler) Market(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	utils.WriteJSON(w, map[string]any{"index": eng.Index, "phase": eng.Phase, "rows": eng.MarketRows()})
}

// Book serves /api/book.
func (h *Handler) Book(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	utils.WriteJSON(w, eng.SnapshotBook(r.URL.Query().Get("symbol")))
}

// LoadTest serves /api/loadtest.
func (h *Handler) LoadTest(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	n := utils.AtoiDefault(r.URL.Query().Get("n"), 100000)
	if n > 2000000 {
		n = 2000000
	}
	utils.WriteJSON(w, eng.LoadTest(n, r.URL.Query().Get("symbol")))
}
