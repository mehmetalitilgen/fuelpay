package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

type healthResponse struct{
	Status string `json:"status"`
	Version string `json:"version"`

}

var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	addr := os.Getenv("HTTP_ADDR")

	if strings.TrimSpace(addr) == "" {
		addr = "localhost:8080"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
		err := json.NewEncoder(w).Encode(healthResponse{Status: "ok",Version: version})
		if err != nil {
			slog.Error("healthz: failed to encode response:","error", err)
			return
		}
    })

	s := &http.Server{
		Addr:           addr,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	slog.Info("Server is starting...","Address",addr)

	err := s.ListenAndServe()

	if err != nil {
		slog.Error("Server Error","Error",err)
	}
}