package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// newStation creates a station through the API and returns its ID.
func newStation(t *testing.T, srv *httptest.Server, code string) string {
	t.Helper()
	res := call(t, srv, http.MethodPost, "/v1/stations", `{"code":"`+code+`","name":"n","city":"c"}`)
	if res.Status != http.StatusCreated {
		t.Fatalf("create station: %d %s", res.Status, res.Raw)
	}
	return res.str("id")
}

func TestRegisterTerminal(t *testing.T) {
	srv := httptest.NewServer(Routes(testDeps()))
	defer srv.Close()
	stationID := newStation(t, srv, "IST-1")

	res := call(t, srv, http.MethodPost, "/v1/stations/"+stationID+"/terminals",
		`{"serial_number":"PAX-A920-0001","model":"PAX A920"}`)
	if res.Status != http.StatusCreated {
		t.Fatalf("want 201, got %d %s", res.Status, res.Raw)
	}
	id := res.str("id")
	if res.Header.Get("Location") != "/v1/terminals/"+id {
		t.Errorf("Location: got %q", res.Header.Get("Location"))
	}
	if res.str("status") != "pending" || res.str("station_id") != stationID {
		t.Errorf("want a pending terminal of the station, got %s", res.Raw)
	}
	if _, ok := res.Body["activated_at"]; ok {
		t.Errorf("activated_at must be omitted for a pending terminal: %s", res.Raw)
	}

	got := call(t, srv, http.MethodGet, "/v1/terminals/"+id, "")
	if got.Status != http.StatusOK || got.str("serial_number") != "PAX-A920-0001" {
		t.Errorf("get: got %d %s", got.Status, got.Raw)
	}

	call(t, srv, http.MethodPost, "/v1/stations/"+stationID+"/terminals", `{"serial_number":"PAX-A920-0002","model":"PAX A920"}`)
	other := newStation(t, srv, "IST-2")
	call(t, srv, http.MethodPost, "/v1/stations/"+other+"/terminals", `{"serial_number":"PAX-A920-0003","model":"PAX A920"}`)

	list := call(t, srv, http.MethodGet, "/v1/stations/"+stationID+"/terminals", "")
	items, _ := list.Body["items"].([]any)
	if list.Status != http.StatusOK || len(items) != 2 {
		t.Fatalf("list: want 2 terminals of the station, got %d %s", list.Status, list.Raw)
	}
	if first, _ := items[0].(map[string]any); first["serial_number"] != "PAX-A920-0001" {
		t.Errorf("list must be oldest first, got %v", first["serial_number"])
	}

	empty := call(t, srv, http.MethodGet, "/v1/stations/"+newStation(t, srv, "IST-3")+"/terminals", "")
	if empty.Raw != "{\"items\":[]}\n" {
		t.Errorf("empty list: want items:[], got %q", empty.Raw)
	}
}

func TestRegisterTerminalErrors(t *testing.T) {
	srv := httptest.NewServer(Routes(testDeps()))
	defer srv.Close()
	stationID := newStation(t, srv, "IST-1")
	path := "/v1/stations/" + stationID + "/terminals"

	tests := []struct {
		name       string
		path, body string
		wantStatus int
		wantCode   string
		wantDetail []string
	}{
		{"missing fields", path, `{}`, 400, "validation_failed", []string{"serial_number:required", "model:required"}},
		{"serial too short", path, `{"serial_number":"S-1","model":"m"}`, 400, "validation_failed", []string{"serial_number:too_short"}},
		{"serial lower case", path, `{"serial_number":"pax-0001","model":"m"}`, 400, "validation_failed", []string{"serial_number:invalid_format"}},
		{"unknown station", "/v1/stations/missing/terminals", `{"serial_number":"S-000001","model":"m"}`, 404, "not_found", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := call(t, srv, http.MethodPost, tt.path, tt.body)
			if res.Status != tt.wantStatus || res.errorCode() != tt.wantCode || !slices.Equal(res.details(), tt.wantDetail) {
				t.Errorf("want %d %s %v, got %d %s", tt.wantStatus, tt.wantCode, tt.wantDetail, res.Status, res.Raw)
			}
		})
	}

	body := `{"serial_number":"S-000001","model":"m"}`
	call(t, srv, http.MethodPost, path, body)
	// The serial is unique across all stations, not just within one.
	dup := call(t, srv, http.MethodPost, "/v1/stations/"+newStation(t, srv, "IST-2")+"/terminals", body)
	if dup.Status != http.StatusConflict || !slices.Equal(dup.details(), []string{"serial_number:already_exists"}) {
		t.Errorf("duplicate serial: want 409 serial_number:already_exists, got %d %s", dup.Status, dup.Raw)
	}

	for _, p := range []string{"/v1/terminals/missing", "/v1/stations/missing/terminals"} {
		if res := call(t, srv, http.MethodGet, p, ""); res.Status != http.StatusNotFound {
			t.Errorf("GET %s: want 404, got %d", p, res.Status)
		}
	}
}
