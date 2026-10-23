package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutes(t *testing.T) {
	srv := httptest.NewServer(routes())
	defer srv.Close()

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		expectedAllow  string
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
		},
		{
			name:           "PUT healthz",
			method:         http.MethodPut,
			path:           "/healthz",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedAllow:  "GET, HEAD",
		},
		{
			name:           "PATCH healthz",
			method:         http.MethodPatch,
			path:           "/healthz",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedAllow:  "GET, HEAD",
		},
		{
			name:           "DELETE healthz",
			method:         http.MethodDelete,
			path:           "/healthz",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedAllow:  "GET, HEAD",
		},
		{
			name:           "unknown path",
			method:         http.MethodGet,
			path:           "/does-not-exist",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "root path",
			method:         http.MethodGet,
			path:           "/",
			expectedStatus: http.StatusNotFound,
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

				want := healthResponse{Status: "ok", Version: version}
				if got != want {
					t.Errorf("unexpected body: want %+v, got %+v", want, got)
				}
			}
		})
	}
}
