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

func routes(version string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler(version))
	return mux
}

func Server(version string, cfg config.Config) *http.Server {
	return &http.Server{
		Handler:           routes(version),
		Addr:              cfg.HTTP_ADDR,
		ReadHeaderTimeout: cfg.HTTP_READ_HEADER_TIMEOUT,
		ReadTimeout:       cfg.HTTP_READ_TIMEOUT,
		WriteTimeout:      cfg.HTTP_WRITE_TIMEOUT,
		IdleTimeout:       cfg.HTTP_IDLE_TIMEOUT,
		MaxHeaderBytes:    1 << 20,
	}
}
