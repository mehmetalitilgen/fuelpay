package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr          string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	LogLevel          slog.Level
}

type envReader struct {
	errs   []error
	failed map[string]bool
}

func (r *envReader) fail(key string, err error) {
	r.errs = append(r.errs, fmt.Errorf("%s: %w", key, err))
	if r.failed == nil {
		r.failed = make(map[string]bool)
	}
	r.failed[key] = true
}

// ok reports whether all given keys were read without errors.
func (r *envReader) ok(keys ...string) bool {
	for _, k := range keys {
		if r.failed[k] {
			return false
		}
	}
	return true
}

func (r *envReader) string(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func (r *envReader) duration(key string, defaultValue time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	n, err := time.ParseDuration(v)
	if err != nil {
		r.fail(key, err)
		return defaultValue
	}
	if n <= 0 {
		r.fail(key, fmt.Errorf("must be > 0, got %s", n))
		return defaultValue
	}
	return n
}

func Load() (Config, error) {
	r := &envReader{}
	cfg := Config{
		HTTPAddr:          r.string("HTTP_ADDR", ":8080"),
		ReadHeaderTimeout: r.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       r.duration("HTTP_READ_TIMEOUT", 10*time.Second),
		WriteTimeout:      r.duration("HTTP_WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:       r.duration("HTTP_IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout:   r.duration("SHUTDOWN_TIMEOUT", 15*time.Second),
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(r.string("LOG_LEVEL", "info"))); err != nil {
		r.fail("LOG_LEVEL", errors.New("must be one of: debug, info, warn, error"))
	}

	if _, portStr, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		r.fail("HTTP_ADDR", err)
	} else if port, err := strconv.Atoi(portStr); err != nil || port < 0 || port > 65535 {
		r.fail("HTTP_ADDR", fmt.Errorf("port must be in 0-65535, got %q", portStr))
	}

	// Skip the cross-field check when either value fell back to its default;
	// comparing against a value the user never set would be misleading.
	if r.ok("HTTP_READ_HEADER_TIMEOUT", "HTTP_READ_TIMEOUT") && cfg.ReadHeaderTimeout > cfg.ReadTimeout {
		r.fail("HTTP_READ_TIMEOUT", fmt.Errorf(
			"must be >= HTTP_READ_HEADER_TIMEOUT (%s), got %s",
			cfg.ReadHeaderTimeout, cfg.ReadTimeout,
		))
	}

	return cfg, errors.Join(r.errs...)
}
