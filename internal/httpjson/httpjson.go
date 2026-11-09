// Package httpjson writes JSON responses, renders the API error envelope and
// decodes JSON request bodies strictly.
package httpjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"strings"

	"github.com/mehmetalitilgen/fuelpay/internal/requestid"
)

// FieldError describes a problem with one request field.
type FieldError struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// Error is an error that is returned to the client as-is. Any other error
// written with WriteError becomes a 500 internal_error.
type Error struct {
	Status  int
	Code    string
	Message string
	Details []FieldError
}

type errorBody struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []FieldError `json:"details,omitempty"`
}

// errorEnvelope wraps every error response. The "error" key lets a client tell
// a failure from a success without looking at the status code; request_id
// sits next to it so support can match a response to its log lines.
type errorEnvelope struct {
	Error     errorBody `json:"error"`
	RequestID string    `json:"request_id,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		slog.Error("encode response", "err", err)
	}
}

// WriteError writes err in the error envelope. An *Error (possibly wrapped)
// is written as-is; anything else is logged and answered with 500.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *Error
	requestID := requestid.FromContext(r.Context())
	if !errors.As(err, &apiErr) {
		slog.ErrorContext(r.Context(), "unhandled error", "err", err, "request_id", requestID)
		apiErr = &Error{
			Status:  http.StatusInternalServerError,
			Code:    "internal_error",
			Message: "internal server error",
		}
	}

	status := apiErr.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}

	WriteJSON(w, status, errorEnvelope{
		Error: errorBody{
			Code:    apiErr.Code,
			Message: apiErr.Message,
			Details: apiErr.Details,
		},
		RequestID: requestID,
	})
}

// unknownFieldPrefix is the text encoding/json uses when DisallowUnknownFields
// rejects a field. That error type is unexported, so matching the prefix is the
// only way to recognise it.
const unknownFieldPrefix = `json: unknown field `

// unknownField reports whether err is encoding/json's unknown-field error and,
// if so, returns the name of the offending field.
func unknownField(err error) (field string, ok bool) {
	if err == nil {
		return "", false
	}

	msg := err.Error()
	if !strings.HasPrefix(msg, unknownFieldPrefix) {
		return "", false
	}

	return strings.Trim(strings.TrimPrefix(msg, unknownFieldPrefix), `"`), true
}

// DecodeJSON decodes a single JSON object from the request body into dst. It
// requires a JSON Content-Type, limits the body to 1 MiB and rejects unknown
// fields. Client mistakes are returned as *Error.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return &Error{
			Status:  http.StatusUnsupportedMediaType,
			Code:    "unsupported_media_type",
			Message: "Content-Type must be application/json.",
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var (
		maxErr    *http.MaxBytesError
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
	)

	if err := dec.Decode(dst); err != nil {
		field, isUnknownField := unknownField(err)

		switch {
		case errors.As(err, &maxErr):
			return &Error{
				Status:  http.StatusRequestEntityTooLarge,
				Code:    "payload_too_large",
				Message: "Request body must not exceed 1 MiB.",
			}

		case errors.Is(err, io.EOF):
			return &Error{
				Status:  http.StatusBadRequest,
				Code:    "invalid_json",
				Message: "Request body must not be empty.",
			}

		case errors.As(err, &syntaxErr):
			return &Error{
				Status:  http.StatusBadRequest,
				Code:    "invalid_json",
				Message: fmt.Sprintf("Request body contains malformed JSON (at position %d).", syntaxErr.Offset),
			}

		case errors.Is(err, io.ErrUnexpectedEOF):
			return &Error{
				Status:  http.StatusBadRequest,
				Code:    "invalid_json",
				Message: "Request body contains malformed JSON.",
			}

		case errors.As(err, &typeErr):
			if typeErr.Field == "" {
				return &Error{
					Status:  http.StatusBadRequest,
					Code:    "invalid_json",
					Message: "Request body must be a JSON object.",
				}
			}

			return &Error{
				Status:  http.StatusBadRequest,
				Code:    "invalid_json",
				Message: fmt.Sprintf("Request body field %q must be of type %s.", typeErr.Field, typeErr.Type),
				Details: []FieldError{{
					Field: typeErr.Field,
					Issue: "invalid_type",
				}},
			}

		case isUnknownField:
			return &Error{
				Status:  http.StatusBadRequest,
				Code:    "invalid_json",
				Message: "Request body contains an unknown field.",
				Details: []FieldError{{Field: field, Issue: "unknown_field"}},
			}

		default:
			return err // WriteError bunu loglayıp 500 döner
		}
	}

	err = dec.Decode(&struct{}{})
	if errors.As(err, &maxErr) {
		return &Error{
			Status:  http.StatusRequestEntityTooLarge,
			Code:    "payload_too_large",
			Message: "Request body must not exceed 1 MiB.",
		}
	}

	if !errors.Is(err, io.EOF) {
		return &Error{
			Status:  http.StatusBadRequest,
			Code:    "invalid_json",
			Message: "Request body must contain a single JSON object.",
		}
	}

	return nil
}

// FieldErrors collects validation problems in the order they are found.
type FieldErrors []FieldError

// Add records a problem with field.
func (fe *FieldErrors) Add(field, issue string) {
	*fe = append(*fe, FieldError{Field: field, Issue: issue})
}

// Err returns nil when nothing was recorded, otherwise a 400
// validation_failed *Error carrying the details.
func (fe FieldErrors) Err() error {
	if len(fe) == 0 {
		return nil
	}
	return &Error{
		Status:  http.StatusBadRequest,
		Code:    "validation_failed",
		Message: "one or more fields are invalid",
		Details: slices.Clone(fe),
	}
}
