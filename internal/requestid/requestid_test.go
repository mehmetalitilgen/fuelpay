package requestid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var generated = regexp.MustCompile(`^[0-9a-f]{32}$`)

// serve runs Middleware for one request and returns the response header value
// and the ID the next handler saw in its context.
func serve(t *testing.T, incoming string, set bool) (header, fromCtx string) {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fromCtx = FromContext(r.Context())
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if set {
		req.Header.Set(Header, incoming)
	}
	rec := httptest.NewRecorder()
	Middleware(next).ServeHTTP(rec, req)
	return rec.Header().Get(Header), fromCtx
}

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		set      bool
		want     string // empty means a new ID must be generated
	}{
		{name: "RID1 no header", set: false},
		{name: "RID2 simple id is reused", incoming: "abc-123", set: true, want: "abc-123"},
		{name: "RID3 dots and underscores are reused", incoming: "terminal-42.req_7", set: true, want: "terminal-42.req_7"},
		{name: "128 characters is reused", incoming: strings.Repeat("a", 128), set: true, want: strings.Repeat("a", 128)},
		{name: "RID4 129 characters is replaced", incoming: strings.Repeat("a", 129), set: true},
		{name: "RID5 space is replaced", incoming: "a b", set: true},
		{name: "RID5 newline is replaced", incoming: "a\nb", set: true},
		{name: "non-ASCII letter is replaced", incoming: "istasyon-ş", set: true},
		{name: "empty value is replaced", incoming: "", set: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, fromCtx := serve(t, tt.incoming, tt.set)

			if header != fromCtx {
				t.Errorf("header %q and context %q differ", header, fromCtx)
			}
			if tt.want != "" {
				if header != tt.want {
					t.Errorf("want reused ID %q, got %q", tt.want, header)
				}
				return
			}
			if !generated.MatchString(header) {
				t.Errorf("want a generated 32-char hex ID, got %q", header)
			}
		})
	}
}

func TestMiddlewareGeneratesDistinctIDs(t *testing.T) { // RID6
	a, _ := serve(t, "", false)
	b, _ := serve(t, "", false)
	if a == b {
		t.Errorf("two requests got the same ID %q", a)
	}
}

func TestFromContextEmpty(t *testing.T) { // RID7
	if got := FromContext(context.Background()); got != "" {
		t.Errorf("want empty ID, got %q", got)
	}
}

func TestNewContextRoundTrip(t *testing.T) {
	ctx := NewContext(context.Background(), "abc")
	if got := FromContext(ctx); got != "abc" {
		t.Errorf("want %q, got %q", "abc", got)
	}
}
