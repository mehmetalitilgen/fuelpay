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
	HTTP_ADDR                string
	HTTP_READ_HEADER_TIMEOUT time.Duration
	HTTP_READ_TIMEOUT        time.Duration
	HTTP_WRITE_TIMEOUT       time.Duration
	HTTP_IDLE_TIMEOUT        time.Duration
	SHUTDOWN_TIMEOUT         time.Duration
	LOG_LEVEL                slog.Level
}

type envReader struct {
	errs []error
}

func (r *envReader) fail(key string, err error) {
	r.errs = append(r.errs, fmt.Errorf("%s: %w", key, err))
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
		HTTP_ADDR:                r.string("HTTP_ADDR", ":8080"),
		HTTP_READ_HEADER_TIMEOUT: r.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		HTTP_READ_TIMEOUT:        r.duration("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTP_WRITE_TIMEOUT:       r.duration("HTTP_WRITE_TIMEOUT", 10*time.Second),
		HTTP_IDLE_TIMEOUT:        r.duration("HTTP_IDLE_TIMEOUT", 120*time.Second),
		SHUTDOWN_TIMEOUT:         r.duration("SHUTDOWN_TIMEOUT", 15*time.Second),
	}

	if err := cfg.LOG_LEVEL.UnmarshalText([]byte(r.string("LOG_LEVEL", "info"))); err != nil {
		r.fail("LOG_LEVEL", errors.New("must be one of: debug, info, warn, error"))
	}

	if _, portStr, err := net.SplitHostPort(cfg.HTTP_ADDR); err != nil {
		r.fail("HTTP_ADDR", err)
	} else if port, err := strconv.Atoi(portStr); err != nil || port < 0 || port > 65535 {
		r.fail("HTTP_ADDR", fmt.Errorf("port must be in 0-65535, got %q", portStr))
	}

	if cfg.HTTP_READ_HEADER_TIMEOUT > cfg.HTTP_READ_TIMEOUT {
		r.fail("HTTP_READ_TIMEOUT", fmt.Errorf(
			"must be >= HTTP_READ_HEADER_TIMEOUT (%s), got %s",
			cfg.HTTP_READ_HEADER_TIMEOUT, cfg.HTTP_READ_TIMEOUT,
		))
	}

	return cfg, errors.Join(r.errs...)
}
