// Package admin serves sign-in, the console pages, status, simulator and session controls and the FIX message log.
package admin

import (
	"velexa-sim-me/internal/rest/deps"
	"velexa-sim-me/internal/rest/middleware"
)

// Handler holds the shared dependencies and the route middlewares; its methods are the HTTP handlers.
type Handler struct {
	*deps.Deps
	middlewares *middleware.Middlewares
}

// NewHandler returns the handlers backed by d.
func NewHandler(d *deps.Deps, middlewares *middleware.Middlewares) *Handler {
	return &Handler{Deps: d, middlewares: middlewares}
}
