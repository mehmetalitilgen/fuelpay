package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mehmetalitilgen/fuelpay/internal/config"
	"github.com/mehmetalitilgen/fuelpay/internal/server"
)

var version = "dev"

func main() {
	// The logger is created before the config is loaded so that config
	// errors are logged as JSON too; the level is applied once it is known.
	level := new(slog.LevelVar)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	if err := run(signalContext(), logger, level, server.Routes(version), nil); err != nil {
		logger.Error("application terminated", "err", err)
		os.Exit(1)
	}
}

// signalContext returns a context that is canceled on the first SIGINT or
// SIGTERM, with the signal as its cause. Later signals get the default
// behavior, so a second Ctrl+C terminates the process immediately.
func signalContext() context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		signal.Stop(sigs)
		cancel(fmt.Errorf("received signal %s", sig))
	}()
	return ctx
}

// run serves handler until ctx is canceled, then shuts the server down
// gracefully. onListen, if not nil, is called with the bound address once
// the server accepts connections.
func run(ctx context.Context, logger *slog.Logger, level *slog.LevelVar, handler http.Handler, onListen func(net.Addr)) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	level.Set(cfg.LogLevel)

	srv := server.New(cfg, handler)

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	logger.Info("http server listening",
		"addr", ln.Addr().String(),
		"version", version,
		"log_level", cfg.LogLevel.String(),
		"read_header_timeout", cfg.ReadHeaderTimeout.String(),
		"read_timeout", cfg.ReadTimeout.String(),
		"write_timeout", cfg.WriteTimeout.String(),
		"idle_timeout", cfg.IdleTimeout.String(),
		"shutdown_timeout", cfg.ShutdownTimeout.String(),
	)
	if onListen != nil {
		onListen(ln.Addr())
	}

	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.Serve(ln) }()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutdown started", "reason", context.Cause(ctx).Error(), "timeout", cfg.ShutdownTimeout.String())

	// ctx is already canceled; the shutdown deadline must not inherit that.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Force-close the connections that did not finish in time.
		_ = srv.Close()
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-serverErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}

	logger.Info("server stopped")
	return nil
}
