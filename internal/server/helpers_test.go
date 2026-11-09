package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// response is a decoded API response used by the resource tests.
type response struct {
	Status int
	Header http.Header
	Body   map[string]any
	Raw    string
}

// call sends a JSON request to srv and decodes the JSON response body.
func call(t *testing.T, srv *httptest.Server, method, path, body string) response {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	out := response{Status: res.StatusCode, Header: res.Header, Raw: string(raw)}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out.Body); err != nil {
			t.Fatalf("response is not a JSON object: %v (%s)", err, raw)
		}
	}
	return out
}

// errorCode returns error.code from an error envelope, or "".
func (r response) errorCode() string {
	e, _ := r.Body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

// details returns error.details as "field:issue" strings.
func (r response) details() []string {
	e, _ := r.Body["error"].(map[string]any)
	list, _ := e["details"].([]any)
	var out []string
	for _, d := range list {
		m, _ := d.(map[string]any)
		out = append(out, m["field"].(string)+":"+m["issue"].(string))
	}
	return out
}

func (r response) str(key string) string {
	s, _ := r.Body[key].(string)
	return s
}
