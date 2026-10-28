package config

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

var envKeys = []string{
	"HTTP_ADDR",
	"HTTP_READ_HEADER_TIMEOUT",
	"HTTP_READ_TIMEOUT",
	"HTTP_WRITE_TIMEOUT",
	"HTTP_IDLE_TIMEOUT",
	"SHUTDOWN_TIMEOUT",
	"LOG_LEVEL",
}

// setEnv clears every config variable so values from the developer's shell
// don't leak into the test, then applies the given overrides.
func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, k := range envKeys {
		t.Setenv(k, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func defaultConfig() Config {
	return Config{
		HTTPAddr:          ":8080",
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ShutdownTimeout:   15 * time.Second,
		LogLevel:          slog.LevelInfo,
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{
			name: "defaults",
			env:  nil,
			want: defaultConfig(),
		},
		{
			name: "all overrides",
			env: map[string]string{
				"HTTP_ADDR":                "127.0.0.1:9090",
				"HTTP_READ_HEADER_TIMEOUT": "2s",
				"HTTP_READ_TIMEOUT":        "3s",
				"HTTP_WRITE_TIMEOUT":       "4s",
				"HTTP_IDLE_TIMEOUT":        "1m",
				"SHUTDOWN_TIMEOUT":         "500ms",
				"LOG_LEVEL":                "debug",
			},
			want: Config{
				HTTPAddr:          "127.0.0.1:9090",
				ReadHeaderTimeout: 2 * time.Second,
				ReadTimeout:       3 * time.Second,
				WriteTimeout:      4 * time.Second,
				IdleTimeout:       time.Minute,
				ShutdownTimeout:   500 * time.Millisecond,
				LogLevel:          slog.LevelDebug,
			},
		},
		{
			name: "port zero",
			env:  map[string]string{"HTTP_ADDR": "127.0.0.1:0"},
			want: func() Config { c := defaultConfig(); c.HTTPAddr = "127.0.0.1:0"; return c }(),
		},
		{
			name: "log level is case insensitive",
			env:  map[string]string{"LOG_LEVEL": "WARN"},
			want: func() Config { c := defaultConfig(); c.LogLevel = slog.LevelWarn; return c }(),
		},
		{
			name: "read header timeout equal to read timeout",
			env: map[string]string{
				"HTTP_READ_HEADER_TIMEOUT": "10s",
				"HTTP_READ_TIMEOUT":        "10s",
			},
			want: func() Config { c := defaultConfig(); c.ReadHeaderTimeout = 10 * time.Second; return c }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.env)

			got, err := Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("unexpected config:\nwant %+v\ngot  %+v", tt.want, got)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		// wantKeys lists the variables expected to be reported, in order.
		wantKeys []string
	}{
		{
			name:     "invalid duration",
			env:      map[string]string{"SHUTDOWN_TIMEOUT": "abc"},
			wantKeys: []string{"SHUTDOWN_TIMEOUT"},
		},
		{
			name:     "duration without unit",
			env:      map[string]string{"HTTP_WRITE_TIMEOUT": "10"},
			wantKeys: []string{"HTTP_WRITE_TIMEOUT"},
		},
		{
			name:     "zero duration",
			env:      map[string]string{"HTTP_IDLE_TIMEOUT": "0s"},
			wantKeys: []string{"HTTP_IDLE_TIMEOUT"},
		},
		{
			name:     "negative duration",
			env:      map[string]string{"HTTP_READ_TIMEOUT": "-1s"},
			wantKeys: []string{"HTTP_READ_TIMEOUT"},
		},
		{
			name:     "invalid log level",
			env:      map[string]string{"LOG_LEVEL": "loud"},
			wantKeys: []string{"LOG_LEVEL"},
		},
		{
			name:     "address without port",
			env:      map[string]string{"HTTP_ADDR": "localhost"},
			wantKeys: []string{"HTTP_ADDR"},
		},
		{
			name:     "empty port",
			env:      map[string]string{"HTTP_ADDR": "localhost:"},
			wantKeys: []string{"HTTP_ADDR"},
		},
		{
			name:     "non-numeric port",
			env:      map[string]string{"HTTP_ADDR": ":http"},
			wantKeys: []string{"HTTP_ADDR"},
		},
		{
			name:     "port out of range",
			env:      map[string]string{"HTTP_ADDR": ":65536"},
			wantKeys: []string{"HTTP_ADDR"},
		},
		{
			name:     "negative port",
			env:      map[string]string{"HTTP_ADDR": ":-1"},
			wantKeys: []string{"HTTP_ADDR"},
		},
		{
			name: "read header timeout greater than read timeout",
			env: map[string]string{
				"HTTP_READ_HEADER_TIMEOUT": "11s",
				"HTTP_READ_TIMEOUT":        "10s",
			},
			wantKeys: []string{"HTTP_READ_TIMEOUT"},
		},
		{
			name: "unparsable read timeout skips the cross-field check",
			env: map[string]string{
				"HTTP_READ_HEADER_TIMEOUT": "20s",
				"HTTP_READ_TIMEOUT":        "abc",
			},
			wantKeys: []string{"HTTP_READ_TIMEOUT"},
		},
		{
			name: "all errors are reported together",
			env: map[string]string{
				"HTTP_ADDR":        "nope",
				"SHUTDOWN_TIMEOUT": "abc",
				"LOG_LEVEL":        "loud",
			},
			wantKeys: []string{"SHUTDOWN_TIMEOUT", "LOG_LEVEL", "HTTP_ADDR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.env)

			_, err := Load()
			if err == nil {
				t.Fatal("expected an error, got nil")
			}

			var joined interface{ Unwrap() []error }
			if !errors.As(err, &joined) {
				t.Fatalf("expected a joined error, got %T: %v", err, err)
			}

			errs := joined.Unwrap()
			if len(errs) != len(tt.wantKeys) {
				t.Fatalf("unexpected number of errors: want %d, got %d (%v)", len(tt.wantKeys), len(errs), err)
			}
			for i, key := range tt.wantKeys {
				if !strings.HasPrefix(errs[i].Error(), key+": ") {
					t.Errorf("error %d: want prefix %q, got %q", i, key+": ", errs[i])
				}
			}
		})
	}
}

func TestLoadInvalidDurationFallsBackToDefault(t *testing.T) {
	setEnv(t, map[string]string{"SHUTDOWN_TIMEOUT": "abc"})

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("unexpected ShutdownTimeout: want %s, got %s", 15*time.Second, cfg.ShutdownTimeout)
	}
}
