// Package deps holds what the HTTP handlers share: the engine and the services built around it.
package deps

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/itchgw"
	"velexa-sim-me/internal/loadtest"
)

// Deps is embedded in every handler, so a handler method reads h.Eng, h.Users and so on.
type Deps struct {
	Eng   *engine.Engine
	Bus   *engine.Bus         // SSE fan-out for /events
	Itch  *itchgw.Gateway     // WebSocket market-data gateway (/market, /marketdepth, /index, /events)
	Bench *loadtest.Bench     // synthetic pipeline benchmark
	Live  *loadtest.LiveBench // live load test through the OMS
	Pool  *pgxpool.Pool
	Users *users.Store // console accounts and their roles
}
