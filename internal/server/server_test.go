package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testVersion intentionally differs from the "dev" default so the test
// fails if the handler ignores the injected version.
const testVersion = "v9.9.9-test"

// errorEnvelope mirrors the JSON error body httpjson writes, which is
// unexported there.
// errorEnvelope mirrors the JSON error body httpjson writes, which is
// unexported there. Details is decoded as a raw value so the test can tell an
// absent key from an empty list.
type errorEnvelope struct {
	Error struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	} `json:"error"`
}

func TestRoutes(t *testing.T) {
	srv := httptest.NewServer(Routes(testVersion))
	defer srv.Close()

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		expectedAllow  string
		expectedCode   string // error envelope "code"; empty means no error expected
		expectJSON     bool
		expectEmpty    bool
	}{
		{
			name:           "GET healthz",
			method:         http.MethodGet,
			path:           "/healthz",
			expectedStatus: http.StatusOK,
			expectJSON:     true,
		},
		{
			name:           "HEAD healthz",
			method:         http.MethodHead,
			path:           "/healthz",
			expectedStatus: http.StatusOK,
			expectEmpty:    true,
		},
		{
			name:           "POST healthz",
			method:         http.MethodPost,
			path:           "/healthz",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedAllow:  "GET, HEAD",
			expectedCode:   "method_not_allowed",
		},
		{
			name:           "PUT healthz",
			method:         http.MethodPut,
			path:           "/healthz",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedAllow:  "GET, HEAD",
			expectedCode:   "method_not_allowed",
		},
		{
			name:           "PATCH healthz",
			method:         http.MethodPatch,
			path:           "/healthz",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedAllow:  "GET, HEAD",
			expectedCode:   "method_not_allowed",
		},
		{
			name:           "DELETE healthz",
			method:         http.MethodDelete,
			path:           "/healthz",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedAllow:  "GET, HEAD",
			expectedCode:   "method_not_allowed",
		},
		{
			name:           "unknown path",
			method:         http.MethodGet,
			path:           "/does-not-exist",
			expectedStatus: http.StatusNotFound,
			expectedCode:   "not_found",
		},
		{
			name:           "root path",
			method:         http.MethodGet,
			path:           "/",
			expectedStatus: http.StatusNotFound,
			expectedCode:   "not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, srv.URL+tt.path, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			res, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("failed to send request: %v", err)
			}
			defer res.Body.Close()

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("unexpected status code: want %d, got %d", tt.expectedStatus, res.StatusCode)
			}

			if tt.expectedAllow != "" {
				if got := res.Header.Get("Allow"); got != tt.expectedAllow {
					t.Errorf("unexpected Allow header: want %q, got %q", tt.expectedAllow, got)
				}
			}

			// Routing errors use the same envelope as handler errors, so a
			// client can parse every failure the same way.
			if tt.expectedCode != "" {
				if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
					t.Errorf("unexpected Content-Type: want application/json, got %q", ct)
				}

				var got errorEnvelope
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("body is not valid JSON: %v (body: %q)", err, body)
				}
				if got.Error.Code != tt.expectedCode {
					t.Errorf("unexpected error code: want %q, got %q", tt.expectedCode, got.Error.Code)
				}
				if got.Error.Message == "" {
					t.Errorf("expected a non-empty message, got body %q", body)
				}

				// Routing errors carry no field details, so the key must be
				// absent rather than null or an empty list.
				if got.Error.Details != nil {
					t.Errorf("expected %q to be omitted, got body %s", "details", body)
				}

				// The payload must be nested under "error" and nothing else.
				var top map[string]json.RawMessage
				if err := json.Unmarshal(body, &top); err != nil {
					t.Fatalf("body is not a JSON object: %v (body: %q)", err, body)
				}
				if _, ok := top["error"]; !ok || len(top) != 1 {
					t.Errorf("expected %q to be the only top-level key, got body %s", "error", body)
				}
			}

			if tt.expectEmpty && len(body) != 0 {
				t.Errorf("expected empty body, got %q", body)
			}

			if tt.expectJSON {
				if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
					t.Errorf("unexpected Content-Type: want application/json, got %q", ct)
				}

				var got healthResponse
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("body is not valid JSON: %v (body: %q)", err, body)
				}

				want := healthResponse{Status: "ok", Version: testVersion}
				if got != want {
					t.Errorf("unexpected body: want %+v, got %+v", want, got)
				}
			}
		})
	}
}
