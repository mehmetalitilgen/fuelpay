// Package requestid assigns every HTTP request an ID that is returned in the
// X-Request-ID header and carried in the request context.
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// Header is the HTTP header that carries the request ID.
const Header = "X-Request-ID"

const maxLen = 128

type ctxKey struct{}

// NewContext returns a copy of ctx that carries id.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the request ID stored in ctx, or "" if there is none.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// valid reports whether a client-supplied ID can be reused: 1-128 ASCII
// letters, digits, '.', '_' or '-'. Anything else could bloat or forge log
// lines, so it is replaced.
func valid(id string) bool {
	if id == "" || len(id) > maxLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// newID returns 32 lowercase hex characters. crypto/rand.Read never returns
// an error since Go 1.24.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Middleware reuses a valid incoming X-Request-ID or generates a new one,
// sets it on the response and stores it in the request context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(Header)
		if !valid(id) {
			id = newID()
		}
		w.Header().Set(Header, id)
		next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), id)))
	})
}
