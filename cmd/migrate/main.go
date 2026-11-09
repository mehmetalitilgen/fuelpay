// Command migrate applies or rolls back the database schema migrations that
// are embedded in the binary.
//
// Usage:
//
//	migrate up        apply all pending migrations
//	migrate down      roll back the most recent migration
//	migrate status    list migrations and whether they are applied
//
// The database is taken from DATABASE_URL.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mehmetalitilgen/fuelpay/internal/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger, os.Args[1:]); err != nil {
		logger.Error("migrate failed", "err", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: migrate up|down|status")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is not set")
	}

	pool, err := postgres.Open(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	p, db, err := postgres.NewMigrator(pool)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	switch args[0] {
	case "up":
		results, err := p.Up(ctx)
		for _, r := range results {
			logger.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration.String())
		}
		if err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
	case "down":
		r, err := p.Down(ctx)
		if err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		logger.Info("migration rolled back", "version", r.Source.Version, "file", r.Source.Path)
	case "status":
		statuses, err := p.Status(ctx)
		if err != nil {
			return fmt.Errorf("migrate status: %w", err)
		}
		for _, s := range statuses {
			logger.Info("migration", "version", s.Source.Version, "file", s.Source.Path, "state", string(s.State))
		}
	default:
		return fmt.Errorf("unknown command %q: want up, down or status", args[0])
	}
	return nil
}
