// Package middleware provides the HTTP middleware shared by all routes.
package middleware

import "net/http"

// Middleware wraps an http.Handler with additional behavior.
type Middleware func(http.Handler) http.Handler

// Chain wraps h with the given middlewares so that the first one listed is the
// outermost: Chain(h, A, B, C) handles a request as A -> B -> C -> h.
// With no middlewares, h is returned unchanged.
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
