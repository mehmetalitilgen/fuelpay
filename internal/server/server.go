package server

import (
	"log/slog"
	"net/http"

	"github.com/mehmetalitilgen/fuelpay/internal/config"
	"github.com/mehmetalitilgen/fuelpay/internal/httpjson"
	"github.com/mehmetalitilgen/fuelpay/internal/middleware"
	"github.com/mehmetalitilgen/fuelpay/internal/requestid"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func healthzHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpjson.WriteJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: version})
	}
}

// Routes returns the API's HTTP handler, wrapped in the middleware chain.
func Routes(version string, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler(version))
	return withMiddleware(withJSONErrors(mux), logger)
}

// withMiddleware applies the request-wide middleware in a fixed order. The
// request ID is outermost so every other layer can read it; AccessLog wraps
// Recover so a recovered panic is still logged with its 500 status.
func withMiddleware(h http.Handler, logger *slog.Logger) http.Handler {
	return middleware.Chain(h,
		requestid.Middleware,
		middleware.AccessLog(logger),
		middleware.Recover(logger),
	)
}

// statusRecorder is an http.ResponseWriter that records the status code and
// headers a handler writes, without sending anything to the client.
type statusRecorder struct {
	header http.Header
	status int
}

func newStatusRecorder() *statusRecorder {
	return &statusRecorder{header: make(http.Header)}
}

func (rec *statusRecorder) Header() http.Header {
	return rec.header
}

func (rec *statusRecorder) WriteHeader(code int) {
	if rec.status == 0 {
		rec.status = code
	}
}

// Write discards the body; only the status matters here. Like the real
// ResponseWriter, a Write without a prior WriteHeader implies 200.
func (rec *statusRecorder) Write(b []byte) (int, error) {
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	return len(b), nil
}

// withJSONErrors replaces the mux's plain-text 404 and 405 responses with
// JSON ones.
func withJSONErrors(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}

		rec := newStatusRecorder()
		h.ServeHTTP(rec, r)

		switch rec.status {
		case http.StatusMethodNotAllowed:
			w.Header().Set("Allow", rec.header.Get("Allow"))
			httpjson.WriteError(w, r, &httpjson.Error{
				Status:  http.StatusMethodNotAllowed,
				Code:    "method_not_allowed",
				Message: "The request method is not allowed for this resource.",
			})
		case http.StatusNotFound:
			httpjson.WriteError(w, r, &httpjson.Error{
				Status:  http.StatusNotFound,
				Code:    "not_found",
				Message: "The requested resource was not found.",
			})
		default:
			// Not expected for an unmatched route; let the mux answer as usual.
			mux.ServeHTTP(w, r)
		}
	})
}

// New returns an HTTP server for handler configured from cfg. The caller
// supplies the listener, so cfg.HTTPAddr is not used here.
func New(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}
}
