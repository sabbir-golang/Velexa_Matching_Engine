package middleware

import "velexa-sim-me/internal/database/users"

// Middlewares holds what the route middlewares need: the console user store.
type Middlewares struct {
	users *users.Store
}

// NewMiddlewares returns the route middlewares backed by the user store.
func NewMiddlewares(store *users.Store) *Middlewares {
	return &Middlewares{users: store}
}
