package server

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/mehmetalitilgen/fuelpay/internal/httpjson"
)

// Issue codes used in validation_failed details. Like error codes they are
// part of the API contract (ADR 0003): add new ones, never rename.
const (
	issueRequired      = "required"
	issueTooLong       = "too_long"
	issueTooShort      = "too_short"
	issueInvalidFormat = "invalid_format"
	issueAlreadyExists = "already_exists"
)

// text trims s and checks it is present (when required) and at most max
// runes long. It returns the trimmed value.
func text(fe *httpjson.FieldErrors, field, s string, required bool, max int) string {
	s = strings.TrimSpace(s)
	switch {
	case s == "" && required:
		fe.Add(field, issueRequired)
	case utf8.RuneCountInString(s) > max:
		fe.Add(field, issueTooLong)
	}
	return s
}

// code checks an identifier made of upper-case ASCII letters, digits and '-'
// with a length between min and max, e.g. a station code or a serial number.
func code(fe *httpjson.FieldErrors, field, s string, min, max int) string {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		fe.Add(field, issueRequired)
		return s
	case len(s) < min:
		fe.Add(field, issueTooShort)
		return s
	case len(s) > max:
		fe.Add(field, issueTooLong)
		return s
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-') {
			fe.Add(field, issueInvalidFormat)
			break
		}
	}
	return s
}

var (
	errNotFound = &httpjson.Error{
		Status:  http.StatusNotFound,
		Code:    "not_found",
		Message: "The requested resource was not found.",
	}
)

// conflict returns a 409 for a unique field that is already taken.
func conflict(field, message string) *httpjson.Error {
	return &httpjson.Error{
		Status:  http.StatusConflict,
		Code:    "conflict",
		Message: message,
		Details: []httpjson.FieldError{{Field: field, Issue: issueAlreadyExists}},
	}
}
