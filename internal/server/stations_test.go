package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestCreateStation(t *testing.T) {
	srv := httptest.NewServer(Routes(testDeps()))
	defer srv.Close()

	res := call(t, srv, http.MethodPost, "/v1/stations",
		`{"code":"IST-0001","name":"  Kadıköy  ","city":"İstanbul","address":"Rıhtım Cd. 1"}`)

	if res.Status != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", res.Status, res.Raw)
	}
	id := res.str("id")
	if res.Header.Get("Location") != "/v1/stations/"+id {
		t.Errorf("Location: want /v1/stations/%s, got %q", id, res.Header.Get("Location"))
	}
	if res.str("name") != "Kadıköy" {
		t.Errorf("name should be trimmed, got %q", res.str("name"))
	}
	if res.str("created_at") == "" {
		t.Error("created_at missing")
	}

	got := call(t, srv, http.MethodGet, "/v1/stations/"+id, "")
	if got.Status != http.StatusOK || got.str("code") != "IST-0001" || got.str("id") != id {
		t.Errorf("get: want 200 with the created station, got %d %s", got.Status, got.Raw)
	}
}

func TestCreateStationValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"missing fields", `{}`, []string{"code:required", "name:required", "city:required"}},
		{"blank name", `{"code":"IST-1","name":"   ","city":"x"}`, []string{"name:required"}},
		{"lower-case code", `{"code":"ist-1","name":"a","city":"x"}`, []string{"code:invalid_format"}},
		{"code too short", `{"code":"AB","name":"a","city":"x"}`, []string{"code:too_short"}},
		{"code too long", `{"code":"` + strings.Repeat("A", 33) + `","name":"a","city":"x"}`, []string{"code:too_long"}},
		{"name too long", `{"code":"IST-1","name":"` + strings.Repeat("ş", 101) + `","city":"x"}`, []string{"name:too_long"}},
		{"address too long", `{"code":"IST-1","name":"a","city":"x","address":"` + strings.Repeat("a", 201) + `"}`, []string{"address:too_long"}},
	}

	srv := httptest.NewServer(Routes(testDeps()))
	defer srv.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := call(t, srv, http.MethodPost, "/v1/stations", tt.body)
			if res.Status != http.StatusBadRequest || res.errorCode() != "validation_failed" {
				t.Fatalf("want 400 validation_failed, got %d %s", res.Status, res.Raw)
			}
			if got := res.details(); !slices.Equal(got, tt.want) {
				t.Errorf("details: want %v, got %v", tt.want, got)
			}
		})
	}
}

func TestCreateStationErrors(t *testing.T) {
	srv := httptest.NewServer(Routes(testDeps()))
	defer srv.Close()

	body := `{"code":"IST-0001","name":"a","city":"b"}`
	if res := call(t, srv, http.MethodPost, "/v1/stations", body); res.Status != http.StatusCreated {
		t.Fatalf("first create: %d %s", res.Status, res.Raw)
	}

	dup := call(t, srv, http.MethodPost, "/v1/stations", body)
	if dup.Status != http.StatusConflict || dup.errorCode() != "conflict" ||
		!slices.Equal(dup.details(), []string{"code:already_exists"}) {
		t.Errorf("duplicate: want 409 conflict code:already_exists, got %d %s", dup.Status, dup.Raw)
	}

	unknown := call(t, srv, http.MethodPost, "/v1/stations", `{"code":"IST-2","name":"a","city":"b","owner":"x"}`)
	if unknown.Status != http.StatusBadRequest || unknown.errorCode() != "invalid_json" {
		t.Errorf("unknown field: want 400 invalid_json, got %d %s", unknown.Status, unknown.Raw)
	}

	missing := call(t, srv, http.MethodGet, "/v1/stations/does-not-exist", "")
	if missing.Status != http.StatusNotFound || missing.errorCode() != "not_found" {
		t.Errorf("get missing: want 404 not_found, got %d %s", missing.Status, missing.Raw)
	}
}
