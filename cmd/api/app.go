package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/mehmetalitilgen/fuelpay/internal/config"
	"github.com/mehmetalitilgen/fuelpay/internal/postgres"
	"github.com/mehmetalitilgen/fuelpay/internal/server"
	"github.com/mehmetalitilgen/fuelpay/internal/station"
	"github.com/mehmetalitilgen/fuelpay/internal/terminal"
)

// buildAPI wires repositories and services. With DATABASE_URL it uses
// PostgreSQL (the schema must already be migrated); without it, in-memory
// storage for local experiments.
func buildAPI(ctx context.Context, cfg config.Config, logger *slog.Logger) (http.Handler, func(), error) {
	var (
		stationRepo  station.Repository
		terminalRepo terminal.Repository
		cleanup      = func() {}
	)
	if cfg.DatabaseURL == "" {
		logger.Warn("DATABASE_URL is not set: using in-memory storage, data is lost on restart")
		stationRepo = station.NewMemoryRepository()
		terminalRepo = terminal.NewMemoryRepository()
	} else {
		pool, err := postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, nil, fmt.Errorf("open database: %w", err)
		}
		stationRepo = postgres.NewStationRepository(pool)
		terminalRepo = postgres.NewTerminalRepository(pool)
		cleanup = pool.Close
	}

	stations := station.NewService(stationRepo)
	terminals := terminal.NewService(terminalRepo, stations)

	return server.Routes(server.Deps{
		Version:   version,
		Logger:    logger,
		Stations:  stations,
		Terminals: terminals,
	}), cleanup, nil
}
