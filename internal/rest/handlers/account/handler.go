// Package account serves the signed-in user's profile and the Users page.
package account

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
