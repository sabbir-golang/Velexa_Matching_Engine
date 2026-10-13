package bench

import (
	"net/http"
	"velexa-sim-me/internal/loadtest"
	"velexa-sim-me/internal/rest/middleware"
)

// RegRoutes maps this package's URLs to its handlers.
func (h *Handler) RegRoutes(mux *http.ServeMux, manager *middleware.Manager) {
	mux.Handle("/bench", manager.With(http.HandlerFunc(h.Page)))
	mux.Handle("/api/bench/start", manager.With(http.HandlerFunc(h.Start)))
	mux.Handle("/api/bench/stop", manager.With(http.HandlerFunc(h.Stop)))
	mux.Handle("/api/bench/stats", manager.With(http.HandlerFunc(h.Stats)))
	mux.Handle("/api/bench/summary.csv", manager.With(http.HandlerFunc(h.SummaryCSV)))
	mux.Handle("/api/bench/orders.csv", manager.With(loadtest.OrdersCSVHandler(h.Pool)))
}
