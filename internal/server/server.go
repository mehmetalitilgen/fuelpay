package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/mehmetalitilgen/fuelpay/internal/config"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func healthzHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		err := json.NewEncoder(w).Encode(healthResponse{Status: "ok", Version: version})
		if err != nil {
			slog.Error("healthz: failed to encode response", "err", err)
		}
	}
}

// Routes returns the API's HTTP handler.
func Routes(version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler(version))
	return mux
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
