package middleware

import "net/http"

// Middleware wraps a handler with extra behaviour (a login check, for example).
type Middleware func(http.Handler) http.Handler

// Manager holds the middlewares applied to every request, and wraps single routes.
type Manager struct {
	globalMiddlewares []Middleware
}

// NewManager returns a Manager with no global middlewares.
func NewManager() *Manager {
	return &Manager{globalMiddlewares: make([]Middleware, 0)}
}

// Use adds middlewares that WrapMux applies to the whole mux.
func (mngr *Manager) Use(middleware ...Middleware) {
	mngr.globalMiddlewares = append(mngr.globalMiddlewares, middleware...)
}

// With wraps next in the given middlewares, for one route.
func (mngr *Manager) With(next http.Handler, middlewares ...Middleware) http.Handler {
	n := next
	for _, middleware := range middlewares {
		n = middleware(n)
	}
	return n
}

// WrapMux applies the global middlewares to the whole mux.
func (mngr *Manager) WrapMux(handler http.Handler) http.Handler {
	h := handler
	for _, middleware := range mngr.globalMiddlewares {
		h = middleware(h)
	}
	return h
}
