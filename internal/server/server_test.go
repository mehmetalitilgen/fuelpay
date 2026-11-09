package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/mehmetalitilgen/fuelpay/internal/requestid"
)

var discard = slog.New(slog.DiscardHandler)

// testVersion intentionally differs from the "dev" default so the test
// fails if the handler ignores the injected version.
const testVersion = "v9.9.9-test"

// errorEnvelope mirrors the JSON error body httpjson writes, which is
// unexported there. Details is decoded as a raw value so the test can tell an
// absent key from an empty list.
type errorEnvelope struct {
	Error struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

func TestRoutes(t *testing.T) {
	srv := httptest.NewServer(Routes(testVersion, discard))
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

				// The top level holds only "error" and the request ID, which
				// must match the X-Request-ID response header.
				var top map[string]json.RawMessage
				if err := json.Unmarshal(body, &top); err != nil {
					t.Fatalf("body is not a JSON object: %v (body: %q)", err, body)
				}
				if _, ok := top["error"]; !ok || len(top) != 2 {
					t.Errorf("expected top-level keys error and request_id, got body %s", body)
				}
				if got.RequestID == "" || got.RequestID != res.Header.Get(requestid.Header) {
					t.Errorf("request_id %q does not match header %q", got.RequestID, res.Header.Get(requestid.Header))
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

var generatedID = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestRoutesRequestID(t *testing.T) {
	srv := httptest.NewServer(Routes(testVersion, discard))
	defer srv.Close()

	t.Run("INT1 generated on healthz", func(t *testing.T) {
		res, err := srv.Client().Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if id := res.Header.Get(requestid.Header); !generatedID.MatchString(id) {
			t.Errorf("want a generated request ID, got %q", id)
		}
	})

	t.Run("INT2 client ID echoed in header and body", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/does-not-exist", nil)
		req.Header.Set(requestid.Header, "test-1")
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		var got errorEnvelope
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusNotFound || res.Header.Get(requestid.Header) != "test-1" || got.RequestID != "test-1" {
			t.Errorf("want 404 with request ID test-1, got %d, header %q, body %q",
				res.StatusCode, res.Header.Get(requestid.Header), got.RequestID)
		}
	})
}

// syncBuffer lets the server goroutines write log lines while the test reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestMiddlewarePanicRecovery(t *testing.T) { // INT3
	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) { panic("boom") })
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := httptest.NewServer(withMiddleware(mux, logger))
	defer srv.Close()

	res, err := srv.Client().Get(srv.URL + "/panic")
	if err != nil {
		t.Fatalf("panicking request failed at the connection level: %v", err)
	}
	var got errorEnvelope
	_ = json.NewDecoder(res.Body).Decode(&got)
	res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError || got.Error.Code != "internal_error" {
		t.Errorf("want 500 internal_error, got %d %q", res.StatusCode, got.Error.Code)
	}
	if got.RequestID != res.Header.Get(requestid.Header) {
		t.Errorf("request_id %q does not match header %q", got.RequestID, res.Header.Get(requestid.Header))
	}

	res, err = srv.Client().Get(srv.URL + "/ok")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("server did not recover: second request got %d", res.StatusCode)
	}

	out := logs.String()
	if !strings.Contains(out, `"msg":"panic recovered"`) || !strings.Contains(out, `"path":"/panic","status":500`) {
		t.Errorf("want panic and access log lines for the 500, got:\n%s", out)
	}
}
