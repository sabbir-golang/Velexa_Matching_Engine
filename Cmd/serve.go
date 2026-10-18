// Package cmd starts the matching engine process.
package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/database/db"
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
	envFile := config.EnvFile
	if envFile == "" {
		envFile = "(none)"
	}
	log.Printf("[config] env file %s · data dir %s · FIX spec dir %s · this PC %s (%s)",
		envFile, config.StateDir(), config.SpecDir(), config.LocalIP(), config.Hostname())
	if err := config.CheckStateDir(); err != nil {
		log.Fatalf("[config] the data folder %s is not writable: %v — broker credentials and settings could not be saved. "+
			"Give the engine's user write access (in Docker: DOCKER_UID must own DOCKER_DATA_DIR; run make docker-setup).", config.StateDir(), err)
	}
	if config.InContainer() && config.EnvOr("ME_PUB_IP", "") == "" {
		log.Printf("[config] running in a container without ME_PUB_IP: broker sheets will show the container's address %s — set ME_PUB_IP to the host's IP (tools/docker-setup.sh does)", config.LocalIP())
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
	fa, err := fixgw.Start(eng, fixPort, fixSender)
	if err != nil {
		log.Printf("[FIX] acceptor start failed: %v (HTTP still serving)", err)
	}
	brokers.Itch = brokers.LoadItch()

	srv, closeStores := newServer(eng, bus)

	log.Printf("Velexa Matching Engine %s on :%s  (%d instruments from %s, auto-list=%v, phase=%s)",
		config.Version, port, len(eng.Instr()), engine.UniverseSource(), engine.AutoListEnabled(), eng.Phase)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(ctx, ":"+port); err != nil {
		log.Fatalf("[http] %v", err)
	}
	// SIGTERM (docker stop, systemctl stop): log brokers out and close the database.
	if fa != nil {
		fa.Stop()
	}
	closeStores()
	log.Printf("stopped")
}

// newServer starts the services the HTTP layer sits on: the admin stores, the
// ITCH gateway and feeds. closeStores releases the database at shutdown.
func newServer(eng *engine.Engine, bus *engine.Bus) (srv *rest.Server, closeStores func()) {
	accounts, closeStores := openAccounts()
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
	return rest.NewServer(&deps.Deps{Eng: eng, Bus: bus, Itch: itch, Users: accounts}), closeStores
}

// openAccounts opens the console accounts and the audit trail: in PostgreSQL
// when ME_DB_URL is set (the engine does not start if it cannot reach it, so
// data never splits between the database and files), else in the data files.
func openAccounts() (*users.Store, func()) {
	dsn := config.DBURL()
	if dsn == "" {
		audit.Load()
		s := users.Load(users.Path(), config.AdminUser(), config.AdminPass())
		log.Printf("[db] ME_DB_URL not set — accounts in %s, audit trail in %s", s.Storage(), audit.Path)
		return s, func() {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := db.Open(ctx, dsn, config.DBMaxConns())
	if err != nil {
		log.Fatalf("[db] %v", err)
	}
	name := db.Describe(dsn)
	s, err := users.LoadDB(pool, name, users.Path(), config.StateDir(), config.AdminUser(), config.AdminPass())
	if err != nil {
		log.Fatalf("[db] accounts: %v", err)
	}
	if err := audit.UseDB(pool); err != nil {
		log.Fatalf("[db] audit trail: %v", err)
	}
	log.Printf("[db] connected to %s (schema v%d): %d accounts; audit trail in the database",
		name, db.SchemaVersion(ctx, pool), s.Count())
	return s, pool.Close
}
