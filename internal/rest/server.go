// Package rest is the HTTP layer: the server, the handlers and the middleware.
package rest

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"velexa-sim-me/internal/rest/deps"
	"velexa-sim-me/internal/rest/handlers/account"
	"velexa-sim-me/internal/rest/handlers/admin"
	"velexa-sim-me/internal/rest/handlers/broker"
	"velexa-sim-me/internal/rest/handlers/control"
	"velexa-sim-me/internal/rest/handlers/instrument"
	"velexa-sim-me/internal/rest/handlers/public"
	"velexa-sim-me/internal/rest/handlers/register"
	"velexa-sim-me/internal/rest/middleware"
)

// Server holds one handler group per feature.
type Server struct {
	*deps.Deps
	publicHandler     *public.Handler
	controlHandler    *control.Handler
	registerHandler   *register.Handler
	accountHandler    *account.Handler
	instrumentHandler *instrument.Handler
	brokerHandler     *broker.Handler
	adminHandler      *admin.Handler
}

// NewServer builds the handler groups on top of the shared dependencies.
func NewServer(d *deps.Deps) *Server {
	mid := middleware.NewMiddlewares(d.Users)
	return &Server{
		Deps:              d,
		publicHandler:     public.NewHandler(d, mid),
		controlHandler:    control.NewHandler(d, mid),
		registerHandler:   register.NewHandler(d, mid),
		accountHandler:    account.NewHandler(d, mid),
		instrumentHandler: instrument.NewHandler(d, mid),
		brokerHandler:     broker.NewHandler(d, mid),
		adminHandler:      admin.NewHandler(d, mid),
	}
}

// Routes returns the HTTP handler for every route the engine serves.
func (server *Server) Routes() http.Handler {
	manager := middleware.NewManager()
	// The last one added runs first: security headers go on every response,
	// including CSRF refusals.
	manager.Use(middleware.CSRF, middleware.SecurityHeaders)
	return manager.WrapMux(server.newMux(manager))
}

// newMux registers every handler group's routes on one mux.
func (server *Server) newMux(manager *middleware.Manager) *http.ServeMux {
	mux := http.NewServeMux()
	server.publicHandler.RegRoutes(mux, manager)
	server.registerHandler.RegRoutes(mux, manager)
	server.controlHandler.RegRoutes(mux, manager)
	server.adminHandler.RegRoutes(mux, manager)
	server.brokerHandler.RegRoutes(mux, manager)
	server.accountHandler.RegRoutes(mux, manager)
	server.instrumentHandler.RegRoutes(mux, manager)
	return mux
}

// Run serves HTTP on addr (":8010") until ctx is cancelled (SIGTERM), then
// stops accepting connections, ends the long-lived streams, and waits up to
// 10 s for requests in flight. It returns an error only if serving failed.
func (server *Server) Run(ctx context.Context, addr string) error {
	// Requests get a context that ends at shutdown, so /events (SSE) and the
	// WebSocket feeds — which otherwise stay open for hours — finish promptly.
	base, endStreams := context.WithCancel(context.Background())
	defer endStreams()
	// No WriteTimeout: /events (SSE) and the WebSocket feeds stay open for hours.
	httpSrv := &http.Server{Addr: addr, Handler: server.Routes(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10,
		BaseContext: func(net.Listener) context.Context { return base }}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.ListenAndServe() }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Printf("[http] shutting down")
	endStreams()
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(sctx); err != nil {
		return httpSrv.Close()
	}
	return nil
}
