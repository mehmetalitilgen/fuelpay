package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool connects to TEST_DATABASE_URL or skips the test. The database
// must be disposable: tests drop and recreate the schema.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	pool, err := Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	err := pool.QueryRow(context.Background(), "SELECT to_regclass($1) IS NOT NULL", name).Scan(&ok)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

// Migrations must apply cleanly, roll back completely and apply again.
func TestMigrationsUpDownUp(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	p, db, err := NewMigrator(pool)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}
	for _, table := range []string{"stations", "terminals", "terminal_events"} {
		if !tableExists(t, pool, table) {
			t.Errorf("table %s missing after up", table)
		}
	}

	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("down: %v", err)
	}
	for _, table := range []string{"stations", "terminals", "terminal_events"} {
		if tableExists(t, pool, table) {
			t.Errorf("table %s still exists after down", table)
		}
	}

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if v, err := p.GetDBVersion(ctx); err != nil || v < 1 {
		t.Errorf("want schema version >= 1, got %d (%v)", v, err)
	}
}

// The schema itself must reject what the application must never write.
func TestSchemaConstraints(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE terminal_events, terminals, stations"); err != nil {
		t.Fatal(err)
	}

	exec := func(sql string, args ...any) error {
		_, err := pool.Exec(ctx, sql, args...)
		return err
	}
	stationID := "019a6b2e-0000-7000-8000-000000000001"
	if err := exec(`INSERT INTO stations (id, code, name, city, created_at) VALUES ($1, 'IST-1', 'n', 'c', now())`, stationID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		sql  string
		args []any
	}{
		{"duplicate station code", `INSERT INTO stations (id, code, name, city, created_at) VALUES (gen_random_uuid(), 'IST-1', 'n', 'c', now())`, nil},
		{"unknown station", `INSERT INTO terminals (id, station_id, serial_number, model, status, created_at, updated_at) VALUES (gen_random_uuid(), gen_random_uuid(), 'S-1', 'm', 'pending', now(), now())`, nil},
		{"invalid status", `INSERT INTO terminals (id, station_id, serial_number, model, status, created_at, updated_at) VALUES (gen_random_uuid(), $1, 'S-2', 'm', 'broken', now(), now())`, []any{stationID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := exec(tt.sql, tt.args...); err == nil {
				t.Error("want the database to reject the row")
			}
		})
	}
}
