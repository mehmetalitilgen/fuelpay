package httpjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mehmetalitilgen/fuelpay/internal/requestid"
)

// decodeTarget is the struct the DecodeJSON tests decode into.
type decodeTarget struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

var errRead = errors.New("read failed")

// errReader stands in for a request body that fails mid-read.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errRead }

// newJSONRequest builds a POST request with the given Content-Type. An empty
// contentType omits the header entirely.
func newJSONRequest(contentType, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if contentType == "" {
		req.Header.Del("Content-Type")
	} else {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

// assertAPIError fails the test unless err is an *Error with the wanted
// status and code, and returns it for further assertions.
func assertAPIError(t *testing.T, err error, wantStatus int, wantCode string) *Error {
	t.Helper()

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if apiErr.Status != wantStatus {
		t.Errorf("unexpected status: want %d, got %d", wantStatus, apiErr.Status)
	}
	if apiErr.Code != wantCode {
		t.Errorf("unexpected code: want %q, got %q", wantCode, apiErr.Code)
	}
	if apiErr.Message == "" {
		t.Error("expected a non-empty message")
	}
	return apiErr
}

// assertDetails compares an error's field details against want.
func assertDetails(t *testing.T, want, got []FieldError) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("unexpected details: want %+v, got %+v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unexpected details[%d]: want %+v, got %+v", i, want[i], got[i])
		}
	}
}

// errorKeys checks that raw is an object whose only top-level key is "error",
// and returns the keys of that inner object.
func errorKeys(t *testing.T, raw []byte) []string {
	t.Helper()

	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("body is not a JSON object: %v (body: %q)", err, raw)
	}

	inner, ok := top["error"]
	if !ok {
		t.Fatalf("body has no %q key: %s", "error", raw)
	}
	if len(top) != 1 {
		t.Errorf("expected %q to be the only top-level key, got body %s", "error", raw)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(inner, &fields); err != nil {
		t.Fatalf("the %q value is not a JSON object: %v (body: %q)", "error", err, raw)
	}

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	return keys
}

func TestErrorError(t *testing.T) {
	err := &Error{Code: "not_found", Message: "no such station"}

	if want, got := "not_found: no such station", err.Error(); got != want {
		t.Errorf("unexpected Error(): want %q, got %q", want, got)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteJSON(rec, http.StatusCreated, map[string]string{"status": "ok"})

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Errorf("unexpected status code: want %d, got %d", http.StatusCreated, res.StatusCode)
	}
	if want, got := "application/json; charset=utf-8", res.Header.Get("Content-Type"); got != want {
		t.Errorf("unexpected Content-Type: want %q, got %q", want, got)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if want, got := `{"status":"ok"}`, strings.TrimSpace(string(body)); got != want {
		t.Errorf("unexpected body: want %s, got %s", want, got)
	}
}

// A value encoding/json cannot marshal must not panic. The status line is
// already on the wire at that point, so it stays as written.
func TestWriteJSONUnencodableValue(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteJSON(rec, http.StatusOK, make(chan int))

	if rec.Code != http.StatusOK {
		t.Errorf("unexpected status code: want %d, got %d", http.StatusOK, rec.Code)
	}
	if body := rec.Body.String(); body != "" {
		t.Errorf("expected empty body, got %q", body)
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
		wantDetails []FieldError
		wantOmitted string // JSON key that must be absent from the body
	}{
		{
			name: "api error",
			err: &Error{
				Status:  http.StatusNotFound,
				Code:    "station_not_found",
				Message: "station does not exist",
			},
			wantStatus:  http.StatusNotFound,
			wantCode:    "station_not_found",
			wantMessage: "station does not exist",
			wantOmitted: "details",
		},
		{
			name: "api error with details",
			err: &Error{
				Status:  http.StatusUnprocessableEntity,
				Code:    "validation_failed",
				Message: "one or more fields are invalid",
				Details: []FieldError{{Field: "count", Issue: "must be positive"}},
			},
			wantStatus:  http.StatusUnprocessableEntity,
			wantCode:    "validation_failed",
			wantMessage: "one or more fields are invalid",
			wantDetails: []FieldError{{Field: "count", Issue: "must be positive"}},
		},
		{
			name: "wrapped api error",
			err: fmt.Errorf("create station: %w", &Error{
				Status:  http.StatusConflict,
				Code:    "station_exists",
				Message: "station already exists",
			}),
			wantStatus:  http.StatusConflict,
			wantCode:    "station_exists",
			wantMessage: "station already exists",
		},
		{
			name:        "unhandled error is masked",
			err:         errors.New("boom"),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "internal_error",
			wantMessage: "internal server error",
		},
		{
			name:        "api error without status defaults to 500",
			err:         &Error{Code: "unset_status", Message: "no status given"},
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "unset_status",
			wantMessage: "no status given",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			WriteError(rec, req, tt.err)

			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Errorf("unexpected status code: want %d, got %d", tt.wantStatus, res.StatusCode)
			}
			if want, got := "application/json; charset=utf-8", res.Header.Get("Content-Type"); got != want {
				t.Errorf("unexpected Content-Type: want %q, got %q", want, got)
			}

			raw := rec.Body.Bytes()

			var got errorEnvelope
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("body is not valid JSON: %v (body: %q)", err, raw)
			}
			if got.Error.Code != tt.wantCode {
				t.Errorf("unexpected code: want %q, got %q", tt.wantCode, got.Error.Code)
			}
			if got.Error.Message != tt.wantMessage {
				t.Errorf("unexpected message: want %q, got %q", tt.wantMessage, got.Error.Message)
			}
			assertDetails(t, tt.wantDetails, got.Error.Details)

			if tt.wantOmitted != "" {
				for _, key := range errorKeys(t, raw) {
					if key == tt.wantOmitted {
						t.Errorf("expected %q to be omitted, got body %s", tt.wantOmitted, raw)
					}
				}
			}
		})
	}
}

// Every error response is nested under an "error" key, so a client can tell a
// failure from a success without reading the status code.
func TestWriteErrorEnvelopeShape(t *testing.T) {
	t.Run("without details", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		WriteError(rec, req, &Error{
			Status:  http.StatusNotFound,
			Code:    "not_found",
			Message: "The requested resource was not found.",
		})

		want := `{"error":{"code":"not_found","message":"The requested resource was not found."}}`
		if got := strings.TrimSpace(rec.Body.String()); got != want {
			t.Errorf("unexpected body:\nwant %s\ngot  %s", want, got)
		}
	})

	t.Run("with details", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		WriteError(rec, req, &Error{
			Status:  http.StatusBadRequest,
			Code:    "invalid_json",
			Message: "Request body contains an unknown field.",
			Details: []FieldError{{Field: "colour", Issue: "unknown_field"}},
		})

		want := `{"error":{"code":"invalid_json","message":"Request body contains an unknown field.","details":[{"field":"colour","issue":"unknown_field"}]}}`
		if got := strings.TrimSpace(rec.Body.String()); got != want {
			t.Errorf("unexpected body:\nwant %s\ngot  %s", want, got)
		}
	})

	t.Run("with request id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(requestid.NewContext(req.Context(), "req-1"))

		WriteError(rec, req, &Error{
			Status:  http.StatusNotFound,
			Code:    "not_found",
			Message: "The requested resource was not found.",
		})

		want := `{"error":{"code":"not_found","message":"The requested resource was not found."},"request_id":"req-1"}`
		if got := strings.TrimSpace(rec.Body.String()); got != want {
			t.Errorf("unexpected body:\nwant %s\ngot  %s", want, got)
		}
	})
}

// An unhandled error must never reach the client, since its text can carry
// internal detail such as host names or SQL.
func TestWriteErrorDoesNotLeakInternalDetail(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	WriteError(rec, req, errors.New("dial tcp 10.0.0.3:5432: connection refused"))

	if body := rec.Body.String(); strings.Contains(body, "10.0.0.3") {
		t.Errorf("response leaked internal detail: %s", body)
	}
}

func TestUnknownField(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantField string
		wantOK    bool
	}{
		{
			name:      "unknown field error",
			err:       errors.New(`json: unknown field "x"`),
			wantField: "x",
			wantOK:    true,
		},
		{
			name:      "field name keeps its inner characters",
			err:       errors.New(`json: unknown field "station-id"`),
			wantField: "station-id",
			wantOK:    true,
		},
		{
			name: "unrelated error",
			err:  errors.New("boom"),
		},
		{
			name: "other json error",
			err:  errors.New("json: cannot unmarshal string into Go value of type int"),
		},
		{
			name: "nil error",
			err:  nil,
		},
		{
			name: "prefix only in the middle",
			err:  errors.New(`decode: json: unknown field "x"`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, ok := unknownField(tt.err)

			if ok != tt.wantOK {
				t.Errorf("unexpected ok: want %v, got %v", tt.wantOK, ok)
			}
			if field != tt.wantField {
				t.Errorf("unexpected field: want %q, got %q", tt.wantField, field)
			}
		})
	}
}

// The prefix match must keep working against the error encoding/json actually
// produces, not just a hand-written copy of its text.
func TestUnknownFieldMatchesRealDecoderError(t *testing.T) {
	dec := json.NewDecoder(strings.NewReader(`{"colour":"red"}`))
	dec.DisallowUnknownFields()

	err := dec.Decode(&decodeTarget{})
	if err == nil {
		t.Fatal("expected a decode error, got nil")
	}

	field, ok := unknownField(err)
	if !ok {
		t.Fatalf("expected the decoder error to match, got %v", err)
	}
	if field != "colour" {
		t.Errorf("unexpected field: want %q, got %q", "colour", field)
	}
}

func TestDecodeJSONValid(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        decodeTarget
	}{
		{
			name:        "plain content type",
			contentType: "application/json",
			body:        `{"name":"station-1","count":3}`,
			want:        decodeTarget{Name: "station-1", Count: 3},
		},
		{
			name:        "content type with charset",
			contentType: "application/json; charset=utf-8",
			body:        `{"name":"station-1","count":3}`,
			want:        decodeTarget{Name: "station-1", Count: 3},
		},
		{
			name:        "content type is case insensitive",
			contentType: "Application/JSON",
			body:        `{"name":"station-1","count":3}`,
			want:        decodeTarget{Name: "station-1", Count: 3},
		},
		{
			name:        "missing fields keep zero values",
			contentType: "application/json",
			body:        `{}`,
			want:        decodeTarget{},
		},
		{
			name:        "trailing whitespace is allowed",
			contentType: "application/json",
			body:        "{\"name\":\"station-1\"}\n\n",
			want:        decodeTarget{Name: "station-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := newJSONRequest(tt.contentType, tt.body)

			var got decodeTarget
			if err := DecodeJSON(rec, req, &got); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("unexpected decoded value: want %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestDecodeJSONErrors(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
		wantCode    string
		wantDetails []FieldError
	}{
		{
			name:        "missing content type",
			contentType: "",
			body:        `{"name":"a"}`,
			wantStatus:  http.StatusUnsupportedMediaType,
			wantCode:    "unsupported_media_type",
		},
		{
			name:        "wrong content type",
			contentType: "text/plain",
			body:        `{"name":"a"}`,
			wantStatus:  http.StatusUnsupportedMediaType,
			wantCode:    "unsupported_media_type",
		},
		{
			name:        "unparsable content type",
			contentType: "application/json; charset",
			body:        `{"name":"a"}`,
			wantStatus:  http.StatusUnsupportedMediaType,
			wantCode:    "unsupported_media_type",
		},
		{
			name:        "empty body",
			contentType: "application/json",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantCode:    "invalid_json",
		},
		{
			name:        "malformed json",
			contentType: "application/json",
			body:        `{"name": }`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "invalid_json",
		},
		{
			name:        "truncated json",
			contentType: "application/json",
			body:        `{"name": "a"`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "invalid_json",
		},
		{
			name:        "body is not an object",
			contentType: "application/json",
			body:        `[1,2,3]`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "invalid_json",
		},
		{
			name:        "field has wrong type",
			contentType: "application/json",
			body:        `{"count":"three"}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "invalid_json",
			wantDetails: []FieldError{{Field: "count", Issue: "invalid_type"}},
		},
		{
			name:        "unknown field",
			contentType: "application/json",
			body:        `{"name":"a","colour":"red"}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "invalid_json",
			wantDetails: []FieldError{{Field: "colour", Issue: "unknown_field"}},
		},
		{
			name:        "more than one json object",
			contentType: "application/json",
			body:        `{"name":"a"}{"name":"b"}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "invalid_json",
		},
		{
			name:        "trailing garbage after object",
			contentType: "application/json",
			body:        `{"name":"a"} oops`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "invalid_json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := newJSONRequest(tt.contentType, tt.body)

			var dst decodeTarget
			err := DecodeJSON(rec, req, &dst)
			if err == nil {
				t.Fatalf("expected an error, got nil (decoded %+v)", dst)
			}

			apiErr := assertAPIError(t, err, tt.wantStatus, tt.wantCode)
			assertDetails(t, tt.wantDetails, apiErr.Details)
		})
	}
}

// The syntax error message carries the byte offset so a client can locate the
// problem in the body it sent.
func TestDecodeJSONMalformedMessageHasOffset(t *testing.T) {
	rec := httptest.NewRecorder()
	req := newJSONRequest("application/json", `{"name": }`)

	var dst decodeTarget
	apiErr := assertAPIError(t, DecodeJSON(rec, req, &dst), http.StatusBadRequest, "invalid_json")

	if !strings.Contains(apiErr.Message, "position") {
		t.Errorf("expected the message to mention the position, got %q", apiErr.Message)
	}
}

func TestDecodeJSONBodyTooLarge(t *testing.T) {
	body := `{"name":"` + strings.Repeat("a", 1<<20) + `"}`

	rec := httptest.NewRecorder()
	req := newJSONRequest("application/json", body)

	var dst decodeTarget
	assertAPIError(t, DecodeJSON(rec, req, &dst), http.StatusRequestEntityTooLarge, "payload_too_large")
}

// The limit must also be reported when the first object parses and the
// oversized part only shows up in the trailing read.
func TestDecodeJSONTooLargeAfterFirstObject(t *testing.T) {
	body := `{"name":"a"}` + strings.Repeat(" ", 1<<20)

	rec := httptest.NewRecorder()
	req := newJSONRequest("application/json", body)

	var dst decodeTarget
	assertAPIError(t, DecodeJSON(rec, req, &dst), http.StatusRequestEntityTooLarge, "payload_too_large")
}

// A body right at the 1 MiB limit must still decode, so the limit does not
// reject legitimate requests.
func TestDecodeJSONBodyAtLimit(t *testing.T) {
	name := strings.Repeat("a", (1<<20)-len(`{"name":""}`))
	body := `{"name":"` + name + `"}`

	if len(body) != 1<<20 {
		t.Fatalf("test setup: want a %d byte body, got %d", 1<<20, len(body))
	}

	rec := httptest.NewRecorder()
	req := newJSONRequest("application/json", body)

	var dst decodeTarget
	if err := DecodeJSON(rec, req, &dst); err != nil {
		t.Fatalf("unexpected error for a body at the limit: %v", err)
	}
	if dst.Name != name {
		t.Errorf("unexpected name length: want %d, got %d", len(name), len(dst.Name))
	}
}

// An error DecodeJSON does not recognise is returned as is, so WriteError
// logs it and answers 500 rather than guessing a client error.
func TestDecodeJSONUnknownErrorIsPassedThrough(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", errReader{})
	req.Header.Set("Content-Type", "application/json")

	var dst decodeTarget
	err := DecodeJSON(rec, req, &dst)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	var apiErr *Error
	if errors.As(err, &apiErr) {
		t.Fatalf("expected the raw error to pass through, got *Error %+v", apiErr)
	}
	if !errors.Is(err, errRead) {
		t.Errorf("unexpected error: want %v, got %v", errRead, err)
	}
}

func TestFieldErrors(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		var fe FieldErrors

		if err := fe.Err(); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("collects fields in order", func(t *testing.T) {
		var fe FieldErrors
		fe.Add("name", "must not be empty")
		fe.Add("count", "must be positive")

		apiErr := assertAPIError(t, fe.Err(), http.StatusBadRequest, "validation_failed")

		assertDetails(t, []FieldError{
			{Field: "name", Issue: "must not be empty"},
			{Field: "count", Issue: "must be positive"},
		}, apiErr.Details)
	})

	// Err clones the slice, so mutating the collector afterwards must not
	// change an error that was already returned.
	t.Run("details are detached from the collector", func(t *testing.T) {
		var fe FieldErrors
		fe.Add("name", "must not be empty")

		apiErr := assertAPIError(t, fe.Err(), http.StatusBadRequest, "validation_failed")

		fe[0] = FieldError{Field: "mutated", Issue: "mutated"}
		fe.Add("count", "must be positive")

		assertDetails(t, []FieldError{{Field: "name", Issue: "must not be empty"}}, apiErr.Details)
	})
}
