// Package deps holds what the HTTP handlers share: the engine and the services built around it.
package deps

import (
	"velexa-sim-me/internal/database/orderlog"
	"velexa-sim-me/internal/database/reports"
	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/itchgw"
)

// Deps is embedded in every handler, so a handler method reads h.Eng, h.Users and so on.
type Deps struct {
	Eng     *engine.Engine
	Bus     *engine.Bus        // SSE fan-out for /events
	Itch    *itchgw.Gateway    // WebSocket market-data gateway (/market, /marketdepth, /index, /events)
	Users   *users.Store       // console accounts and their roles
	Reports reports.Store      // saved daily report figures (past days)
	Orders  *orderlog.Recorder // the order log (order report); nil in tests that do not need it
}
