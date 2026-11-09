package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/mehmetalitilgen/fuelpay/internal/httpjson"
	"github.com/mehmetalitilgen/fuelpay/internal/requestid"
)

// Recover turns a panic in the handler into a 500 internal_error response
// and an ERROR log line with the stack trace. The panic value never reaches
// the client. http.ErrAbortHandler is re-panicked because net/http uses it to
// abort a request on purpose.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &responseWriter{w: w}
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}

				logger.LogAttrs(r.Context(), slog.LevelError, "panic recovered",
					slog.String("request_id", requestid.FromContext(r.Context())),
					slog.Any("panic", v),
					slog.String("stack", string(debug.Stack())),
				)

				// Once the status line is out a second one cannot be sent.
				if !rw.started {
					httpjson.WriteError(rw, r, fmt.Errorf("panic: %v", v))
				}
			}()

			next.ServeHTTP(rw, r)
		})
	}
}
