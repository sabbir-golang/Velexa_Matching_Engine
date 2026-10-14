// Package cmd starts the matching engine process.
package cmd

import (
	"log"
	"os"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/fixgw"
	"velexa-sim-me/internal/itchgw"
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
	eng.LoadOrderBookIDs(config.EnvOr("ME_OBID_FILE", config.StateFile("orderbook_ids.json")))
	go eng.Run()
	eng.StartClock(engine.ScheduleFromEnv()) // DSE trading day on the Dhaka clock (ME_SESSION_AUTO=off to drive it by hand)

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

	log.Printf("Velexa Matching Engine on :%s  (%d instruments from %s, auto-list=%v, phase=%s)",
		port, len(eng.Instr()), engine.UniverseSource(), engine.AutoListEnabled(), eng.Phase)
	srv.Start(":" + port)
}

// newServer starts the services the HTTP layer sits on: the admin stores, the
// ITCH gateway and feeds.
func newServer(eng *engine.Engine, bus *engine.Bus) *rest.Server {
	audit.Load()
	brokers.LoadMeta()
	itch := itchgw.NewGateway(eng)
	go itch.Run()
	go itchgw.NewFeed(eng).Run()      // DSE-ITCH market feed (SoupBinTCP) for the OMS itch service
	go itchgw.NewIndexFeed(eng).Run() // DSE-ITCH index feed: 'Z' index values
	go itchgw.NewNewsFeed(eng).Run()  // DSE-ITCH news feed: 'N' announcements
	if config.AdminPass() == "" {
		log.Printf("[auth] ME_ADMIN_PASS is not set (or is the old default): the break-glass environment admin is off")
	}
	config.SessionKey() // create the session key now, not on the first sign-in
	return rest.NewServer(&deps.Deps{Eng: eng, Bus: bus, Itch: itch,
		Users: users.Load(users.Path(), config.AdminUser(), config.AdminPass())})
}
