// Package pgtest gives each test its own empty PostgreSQL database.
//
// The server comes from TEST_DATABASE_URL when it is set (it must allow
// CREATE DATABASE); otherwise a PostgreSQL container is started once per test
// binary with testcontainers. Without either, tests are skipped.
//
// Every call to URL creates a fresh database and drops it when the test
// ends, so tests are isolated and may run in parallel, also across packages.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Image is the PostgreSQL image used when no TEST_DATABASE_URL is given. It
// matches compose.yaml and CI.
const Image = "postgres:18-alpine"

var (
	serverOnce sync.Once
	serverURL  string
	serverErr  error
)

// server returns the admin URL of the shared PostgreSQL server, starting a
// container on first use. The container lives until the test binary exits;
// testcontainers' reaper removes it.
func server(t *testing.T) string {
	t.Helper()
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	if !dockerAvailable() {
		t.Skip("TEST_DATABASE_URL is not set and Docker is not available")
	}
	serverOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		ctr, err := tcpostgres.Run(ctx, Image,
			tcpostgres.WithDatabase("postgres"),
			tcpostgres.WithUsername("fuelpay"),
			tcpostgres.WithPassword("fuelpay"),
			tcpostgres.BasicWaitStrategies(),
		)
		if err != nil {
			serverErr = fmt.Errorf("start postgres container: %w", err)
			return
		}
		serverURL, serverErr = ctr.ConnectionString(ctx, "sslmode=disable")
	})
	if serverErr != nil {
		t.Fatal(serverErr)
	}
	return serverURL
}

// URL creates an empty database for this test and returns its connection
// string. The database is dropped in t.Cleanup.
func URL(t *testing.T) string {
	t.Helper()
	admin := server(t)
	ctx := context.Background()

	var b [6]byte
	_, _ = rand.Read(b[:])
	name := "test_" + hex.EncodeToString(b[:])

	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("connect to test server: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, admin)
		if err != nil {
			t.Logf("drop test database %s: %v", name, err)
			return
		}
		defer func() { _ = conn.Close(ctx) }()
		// FORCE terminates connections a test may have leaked (PostgreSQL 13+).
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop test database %s: %v", name, err)
		}
	})

	u, err := url.Parse(admin)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

var (
	dockerOnce sync.Once
	dockerOK   bool
)

// dockerAvailable reports whether a Docker daemon answers within a few
// seconds. testcontainers' own health check has no deadline and hangs when
// Docker Desktop is installed but not running.
func dockerAvailable() bool {
	dockerOnce.Do(func() {
		done := make(chan bool, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			provider, err := testcontainers.ProviderDocker.GetProvider()
			if err != nil {
				done <- false
				return
			}
			done <- provider.Health(ctx) == nil
		}()
		select {
		case dockerOK = <-done:
		case <-time.After(10 * time.Second):
		}
	})
	return dockerOK
}
