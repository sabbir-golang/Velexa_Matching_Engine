// Package rest is the HTTP layer: the server, the handlers and the middleware.
package rest

import (
	"log"
	"net/http"
	"time"

	"velexa-sim-me/internal/rest/deps"
	"velexa-sim-me/internal/rest/handlers/account"
	"velexa-sim-me/internal/rest/handlers/admin"
	"velexa-sim-me/internal/rest/handlers/bench"
	"velexa-sim-me/internal/rest/handlers/broker"
	"velexa-sim-me/internal/rest/handlers/control"
	"velexa-sim-me/internal/rest/handlers/instrument"
	"velexa-sim-me/internal/rest/handlers/live"
	"velexa-sim-me/internal/rest/handlers/public"
	"velexa-sim-me/internal/rest/handlers/register"
	"velexa-sim-me/internal/rest/middleware"
)

// Server holds one handler group per feature.
type Server struct {
	*deps.Deps
	publicHandler     *public.Handler
	controlHandler    *control.Handler
	benchHandler      *bench.Handler
	liveHandler       *live.Handler
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
		benchHandler:      bench.NewHandler(d, mid),
		liveHandler:       live.NewHandler(d, mid),
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
	return manager.WrapMux(server.newMux(manager))
}

// newMux registers every handler group's routes on one mux.
func (server *Server) newMux(manager *middleware.Manager) *http.ServeMux {
	mux := http.NewServeMux()
	server.publicHandler.RegRoutes(mux, manager)
	server.registerHandler.RegRoutes(mux, manager)
	server.controlHandler.RegRoutes(mux, manager)
	server.benchHandler.RegRoutes(mux, manager)
	server.liveHandler.RegRoutes(mux, manager)
	server.adminHandler.RegRoutes(mux, manager)
	server.brokerHandler.RegRoutes(mux, manager)
	server.accountHandler.RegRoutes(mux, manager)
	server.instrumentHandler.RegRoutes(mux, manager)
	return mux
}

// Start serves HTTP on addr (":8010") and blocks until the server stops.
func (server *Server) Start(addr string) {
	httpSrv := &http.Server{Addr: addr, Handler: server.Routes(), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(httpSrv.ListenAndServe())
}
