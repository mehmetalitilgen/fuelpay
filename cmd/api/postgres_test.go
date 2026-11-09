package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mehmetalitilgen/fuelpay/internal/config"
	"github.com/mehmetalitilgen/fuelpay/internal/postgres"
	"github.com/mehmetalitilgen/fuelpay/internal/postgres/pgtest"
)

// TestAPIWithPostgres drives the real API wiring (buildAPI) against a
// migrated PostgreSQL database: the same path production takes.
func TestAPIWithPostgres(t *testing.T) {
	ctx := context.Background()
	url := pgtest.URL(t)
	pool, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	pool.Close()

	logger, _ := newTestLogger()
	h, cleanup, err := buildAPI(ctx, config.Config{DatabaseURL: url}, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	srv := httptest.NewServer(h)
	defer srv.Close()

	do := func(method, path, body string, want int) map[string]any {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		if res.StatusCode != want {
			t.Fatalf("%s %s: want %d, got %d %v", method, path, want, res.StatusCode, out)
		}
		return out
	}

	st := do(http.MethodPost, "/v1/stations", `{"code":"IST-1","name":"n","city":"c"}`, http.StatusCreated)
	do(http.MethodPost, "/v1/stations", `{"code":"IST-1","name":"n","city":"c"}`, http.StatusConflict)
	term := do(http.MethodPost, "/v1/stations/"+st["id"].(string)+"/terminals", `{"serial_number":"S-000001","model":"m"}`, http.StatusCreated)
	id := term["id"].(string)

	do(http.MethodPost, "/v1/terminals/"+id+"/activate", "", http.StatusOK)
	do(http.MethodPost, "/v1/terminals/"+id+"/suspend", "", http.StatusOK)
	do(http.MethodPost, "/v1/terminals/"+id+"/decommission", "", http.StatusOK)
	do(http.MethodPost, "/v1/terminals/"+id+"/activate", "", http.StatusConflict)

	evs := do(http.MethodGet, "/v1/terminals/"+id+"/events", "", http.StatusOK)
	if items, _ := evs["items"].([]any); len(items) != 4 {
		t.Errorf("want 4 events (registered + 3 changes), got %d", len(items))
	}
	list := do(http.MethodGet, "/v1/stations/"+st["id"].(string)+"/terminals", "", http.StatusOK)
	if items, _ := list["items"].([]any); len(items) != 1 {
		t.Errorf("want 1 terminal, got %d", len(items))
	}
	do(http.MethodGet, "/v1/terminals/not-a-uuid", "", http.StatusNotFound)
}
