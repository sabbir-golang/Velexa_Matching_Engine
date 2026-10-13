// Package cmd starts the matching engine process.
package cmd

import (
	"log"
	"os"
	"time"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/fixgw"
	"velexa-sim-me/internal/itchgw"
	"velexa-sim-me/internal/loadtest"
	"velexa-sim-me/internal/rest"
	"velexa-sim-me/internal/rest/deps"
)

// Serve starts the matching engine: the FIX acceptor, the ITCH feed and gateway,
// and the HTTP server (terminal, admin console, benchmark UI, APIs). It blocks
// until the HTTP server stops.
func Serve() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8010"
	}
	bus := engine.NewBus()
	eng := engine.NewEngine(engine.LoadUniverse(), bus)
	go eng.Run()
	eng.StartBots(time.Now().UnixNano())

	// FIX acceptor — brokers' fix_initiator_go connect here (real FIX round-trip)
	fixPort := os.Getenv("FIX_PORT")
	if fixPort == "" {
		fixPort = "5001"
	}
	fixSender := os.Getenv("FIX_SENDER")
	if fixSender == "" {
		fixSender = "DSEME"
	}
	if _, err := fixgw.Start(eng, fixPort, fixSender); err != nil {
		log.Printf("[FIX] acceptor start failed: %v (HTTP still serving)", err)
	}
	brokers.Itch = brokers.LoadItch()

	srv := newServer(eng, bus)

	log.Printf("Velexa Simulator Matching Engine on :%s  (%d instruments from %s, auto-list=%v, phase=%s)",
		port, len(eng.Instr()), engine.UniverseSource(), engine.AutoListEnabled(), eng.Phase)
	srv.Start(":" + port)
}

// newServer starts the services the HTTP layer sits on: the admin stores, the
// ITCH gateway and feed, and the benchmark and load-test runners.
func newServer(eng *engine.Engine, bus *engine.Bus) *rest.Server {
	audit.Load()
	brokers.LoadMeta()
	itch := itchgw.NewGateway(eng)
	go itch.Run()
	go itchgw.NewFeed(eng).Run() // native DSE-ITCH SoupBinTCP feed for the C++ itch service
	pool, poolErr := loadtest.MustPool()
	if poolErr != nil {
		log.Printf("[bench] pool init: %v", poolErr)
	}
	bench := loadtest.NewBench(pool, eng)
	live := loadtest.NewLiveBench(eng)
	return rest.NewServer(&deps.Deps{Eng: eng, Bus: bus, Itch: itch, Bench: bench, Live: live, Pool: pool,
		Users: users.Load(users.Path(), config.AdminUser(), config.AdminPass())})
}
