// Package live serves the live load test through the OMS.
package live

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
