package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mehmetalitilgen/fuelpay/internal/config"
	"github.com/mehmetalitilgen/fuelpay/internal/server"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("application terminated", "err", err)
	}
}

func run() error {
	cfg, err := config.Load()

	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LOG_LEVEL}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.Server(version, cfg)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTP_ADDR, "read header timeout",
			cfg.HTTP_READ_HEADER_TIMEOUT.String(), "log level", cfg.LOG_LEVEL, "read timeout",
			cfg.HTTP_READ_TIMEOUT.String(), "write time", cfg.HTTP_WRITE_TIMEOUT.String(), "idle timeout", cfg.HTTP_IDLE_TIMEOUT.String())
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		stop()
	}

	logger.Info("shutting down", "timeout", cfg.SHUTDOWN_TIMEOUT)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.SHUTDOWN_TIMEOUT)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil

}
