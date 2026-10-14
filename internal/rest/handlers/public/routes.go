package public

import (
	"net/http"
	"velexa-sim-me/internal/itchgw"
	"velexa-sim-me/internal/rest/middleware"
)

// RegRoutes maps this package's URLs to its handlers.
func (h *Handler) RegRoutes(mux *http.ServeMux, manager *middleware.Manager) {
	mux.Handle("/", manager.With(http.HandlerFunc(h.Index)))
	mux.Handle("/terminal", manager.With(http.HandlerFunc(h.Terminal)))
	mux.Handle("/health", manager.With(http.HandlerFunc(h.Health)))
	mux.Handle("/healthz", manager.With(http.HandlerFunc(h.Healthz)))
	mux.Handle("/api/status", manager.With(http.HandlerFunc(h.Status)))
	mux.Handle("/api/market", manager.With(http.HandlerFunc(h.Market)))
	mux.Handle("/api/book", manager.With(http.HandlerFunc(h.Book)))
	mux.Handle("/api/order", manager.With(http.HandlerFunc(h.Order), middleware.PostOnly, h.middlewares.Viewer)) // the signed-in user is the order's actor
	mux.Handle("/events", manager.With(http.HandlerFunc(h.Events)))                                              // SSE market stream, or the news WebSocket on an Upgrade

	// ITCH WebSocket gateway for the trader SPA
	mux.Handle("/market", manager.With(h.Itch.Serve(h.Itch.Market, itchgw.SelMarket)))
	mux.Handle("/marketdepth", manager.With(h.Itch.Serve(h.Itch.Depth, itchgw.SelDepth)))
	mux.Handle("/index", manager.With(h.Itch.Serve(h.Itch.Index, itchgw.SelIndex)))
}
