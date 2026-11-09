package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/mehmetalitilgen/fuelpay/internal/requestid"
)

// AccessLog returns a middleware that writes one "http request" log line per
// request after the handler returns. It reads the request ID from the context,
// so it must run inside requestid.Middleware.
func AccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{w: w}

			next.ServeHTTP(rw, r)

			duration := time.Since(start)
			status := rw.Status()

			logger.LogAttrs(r.Context(), accessLogLevel(r, status), "http request",
				slog.String("request_id", requestid.FromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int64("bytes", rw.BytesWritten()),
				slog.Float64("duration_ms", float64(duration)/float64(time.Millisecond)),
				slog.String("remote_addr", remoteHost(r.RemoteAddr)),
			)
		})
	}
}

// accessLogLevel picks the log level: server errors are ERROR, health checks
// are DEBUG so they don't flood the logs, everything else is INFO.
func accessLogLevel(r *http.Request, status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case r.URL.Path == "/healthz":
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// remoteHost strips the port from addr, falling back to the raw value if it
// isn't in host:port form.
func remoteHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
