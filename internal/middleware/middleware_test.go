package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mehmetalitilgen/fuelpay/internal/requestid"
)

func TestChainOrder(t *testing.T) { // CH1
	var order []string
	mw := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { order = append(order, "h") })

	Chain(h, mw("A"), mw("B")).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if got := strings.Join(order, ","); got != "A,B,h" {
		t.Errorf("want order A,B,h, got %s", got)
	}
}

func TestChainNoMiddleware(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	Chain(h).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Error("handler was not called")
	}
}

func TestResponseWriter(t *testing.T) {
	tests := []struct {
		name        string
		handler     func(w http.ResponseWriter)
		wantStatus  int
		wantBytes   int64
		wantStarted bool
	}{
		{
			name: "RW1 explicit status and body",
			handler: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte("hello"))
			},
			wantStatus: http.StatusCreated, wantBytes: 5, wantStarted: true,
		},
		{
			name:       "RW2 body only implies 200",
			handler:    func(w http.ResponseWriter) { _, _ = w.Write([]byte("hi")) },
			wantStatus: http.StatusOK, wantBytes: 2, wantStarted: true,
		},
		{
			name:       "RW3 nothing written",
			handler:    func(http.ResponseWriter) {},
			wantStatus: http.StatusOK, wantBytes: 0, wantStarted: false,
		},
		{
			name: "second WriteHeader is not recorded",
			handler: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusAccepted)
				w.WriteHeader(http.StatusTeapot)
			},
			wantStatus: http.StatusAccepted, wantStarted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rw := &responseWriter{w: rec}

			tt.handler(rw)

			if rw.Status() != tt.wantStatus {
				t.Errorf("status: want %d, got %d", tt.wantStatus, rw.Status())
			}
			if rw.BytesWritten() != tt.wantBytes {
				t.Errorf("bytes: want %d, got %d", tt.wantBytes, rw.BytesWritten())
			}
			if rw.started != tt.wantStarted {
				t.Errorf("started: want %v, got %v", tt.wantStarted, rw.started)
			}
			if tt.wantStarted && rec.Code != tt.wantStatus {
				t.Errorf("underlying writer got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestResponseWriterUnwrapFlush(t *testing.T) { // RW4
	rec := httptest.NewRecorder()
	rw := &responseWriter{w: rec}

	if err := http.NewResponseController(rw).Flush(); err != nil {
		t.Fatalf("Flush through wrapper failed: %v", err)
	}
	if !rec.Flushed {
		t.Error("underlying recorder was not flushed")
	}
}

// logLines decodes every JSON log line written to buf.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	dec := json.NewDecoder(buf)
	for {
		var m map[string]any
		if err := dec.Decode(&m); err == io.EOF {
			return lines
		} else if err != nil {
			t.Fatalf("log line is not JSON: %v", err)
		}
		lines = append(lines, m)
	}
}

func newLogger(buf *bytes.Buffer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level}))
}

func TestAccessLog(t *testing.T) {
	tests := []struct {
		name       string
		level      slog.Level
		method     string
		target     string
		handler    http.HandlerFunc
		wantLines  int
		wantLevel  string
		wantPath   string
		wantStatus float64
		wantBytes  float64
	}{
		{
			name: "AL1 status and bytes", level: slog.LevelInfo, method: http.MethodPost, target: "/x",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte("hello"))
			},
			wantLines: 1, wantLevel: "INFO", wantPath: "/x", wantStatus: 201, wantBytes: 5,
		},
		{
			name: "AL2 nothing written", level: slog.LevelInfo, method: http.MethodGet, target: "/x",
			handler:   func(http.ResponseWriter, *http.Request) {},
			wantLines: 1, wantLevel: "INFO", wantPath: "/x", wantStatus: 200, wantBytes: 0,
		},
		{
			name: "AL3 query string is dropped", level: slog.LevelInfo, method: http.MethodGet, target: "/x?token=secret",
			handler:   func(http.ResponseWriter, *http.Request) {},
			wantLines: 1, wantLevel: "INFO", wantPath: "/x", wantStatus: 200,
		},
		{
			name: "AL4 health check at DEBUG", level: slog.LevelDebug, method: http.MethodGet, target: "/healthz",
			handler:   func(http.ResponseWriter, *http.Request) {},
			wantLines: 1, wantLevel: "DEBUG", wantPath: "/healthz", wantStatus: 200,
		},
		{
			name: "AL5 server error at ERROR", level: slog.LevelInfo, method: http.MethodGet, target: "/x",
			handler:   func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
			wantLines: 1, wantLevel: "ERROR", wantPath: "/x", wantStatus: 503,
		},
		{
			name: "AL6 health check hidden at INFO", level: slog.LevelInfo, method: http.MethodGet, target: "/healthz",
			handler:   func(http.ResponseWriter, *http.Request) {},
			wantLines: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			req := httptest.NewRequest(tt.method, tt.target, nil)
			req = req.WithContext(requestid.NewContext(req.Context(), "req-1"))

			AccessLog(newLogger(&buf, tt.level))(tt.handler).ServeHTTP(httptest.NewRecorder(), req)

			if strings.Contains(buf.String(), "secret") {
				t.Errorf("query string leaked into the log: %s", buf.String())
			}
			lines := logLines(t, &buf)
			if len(lines) != tt.wantLines {
				t.Fatalf("want %d log lines, got %d", tt.wantLines, len(lines))
			}
			if tt.wantLines == 0 {
				return
			}
			l := lines[0]
			checks := map[string]any{
				"msg": "http request", "level": tt.wantLevel, "method": tt.method, "path": tt.wantPath,
				"status": tt.wantStatus, "request_id": "req-1", "remote_addr": "192.0.2.1",
			}
			for k, want := range checks {
				if l[k] != want {
					t.Errorf("%s: want %v, got %v", k, want, l[k])
				}
			}
			if tt.wantBytes != 0 && l["bytes"] != tt.wantBytes {
				t.Errorf("bytes: want %v, got %v", tt.wantBytes, l["bytes"])
			}
			if d, ok := l["duration_ms"].(float64); !ok || d < 0 {
				t.Errorf("duration_ms: want a non-negative number, got %v", l["duration_ms"])
			}
		})
	}
}

func TestRecover(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
		wantBody   string // substring of the response body
		secret     string // must not appear in the body
	}{
		{
			name:       "RC1 string panic",
			handler:    func(http.ResponseWriter, *http.Request) { panic("boom") },
			wantStatus: http.StatusInternalServerError, wantBody: `"code":"internal_error"`, secret: "boom",
		},
		{
			name:       "RC4 error panic",
			handler:    func(http.ResponseWriter, *http.Request) { panic(errors.New("db down")) },
			wantStatus: http.StatusInternalServerError, wantBody: `"code":"internal_error"`, secret: "db down",
		},
		{
			name: "RC2 panic after the response started",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("partial"))
				panic("late")
			},
			wantStatus: http.StatusOK, wantBody: "partial", secret: "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req = req.WithContext(requestid.NewContext(req.Context(), "req-1"))

			Recover(newLogger(&buf, slog.LevelInfo))(tt.handler).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: want %d, got %d", tt.wantStatus, rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body: want it to contain %q, got %q", tt.wantBody, body)
			}
			if strings.Contains(body, tt.secret) {
				t.Errorf("body must not contain %q, got %q", tt.secret, body)
			}

			lines := logLines(t, &buf)
			var found bool
			for _, l := range lines {
				if l["msg"] != "panic recovered" {
					continue
				}
				found = true
				if l["level"] != "ERROR" || l["request_id"] != "req-1" {
					t.Errorf("unexpected panic log: %v", l)
				}
				if s, _ := l["stack"].(string); s == "" {
					t.Error("panic log has no stack")
				}
			}
			if !found {
				t.Errorf("no panic log line, got %v", lines)
			}
		})
	}
}

func TestRecoverRepanicsAbortHandler(t *testing.T) { // RC3
	h := Recover(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("want http.ErrAbortHandler to be re-panicked, got %v", v)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Error("expected a panic")
}

// AccessLog must record the 500 that Recover produces when it wraps Recover.
func TestAccessLogSeesRecoveredPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, slog.LevelInfo)
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }),
		AccessLog(logger), Recover(logger))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	for _, l := range logLines(t, &buf) {
		if l["msg"] == "http request" {
			if l["status"] != float64(500) || l["level"] != "ERROR" {
				t.Errorf("access log: want status 500 at ERROR, got %v", l)
			}
			return
		}
	}
	t.Error("no access log line for the panicking request")
}
