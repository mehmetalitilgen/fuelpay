package middleware

import "net/http"

// responseWriter wraps an http.ResponseWriter and records the status code and
// the number of body bytes written, for use by logging and metrics middleware.
type responseWriter struct {
	w       http.ResponseWriter
	status  int
	bytes   int64
	started bool
}

func (rw *responseWriter) Header() http.Header {
	return rw.w.Header()
}

// WriteHeader records only the first status code but always forwards the call,
// so net/http can still warn about superfluous WriteHeader calls.
func (rw *responseWriter) WriteHeader(status int) {
	if !rw.started {
		rw.status = status
		rw.started = true
	}
	rw.w.WriteHeader(status)
}

// Write mirrors net/http: a Write before WriteHeader implies 200 OK.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.started {
		rw.status = http.StatusOK
		rw.started = true
	}
	n, err := rw.w.Write(b)
	rw.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer's optional
// capabilities (Flusher, Hijacker, deadlines) through this wrapper.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.w
}

// Status returns the status code sent to the client. If the handler wrote
// nothing, net/http sends 200, so that is reported.
func (rw *responseWriter) Status() int {
	if !rw.started {
		return http.StatusOK
	}
	return rw.status
}

// BytesWritten returns the number of body bytes actually written.
func (rw *responseWriter) BytesWritten() int64 {
	return rw.bytes
}
