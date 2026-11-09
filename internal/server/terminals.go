package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/mehmetalitilgen/fuelpay/internal/httpjson"
	"github.com/mehmetalitilgen/fuelpay/internal/terminal"
)

type terminalHandlers struct {
	svc *terminal.Service
}

type registerTerminalRequest struct {
	SerialNumber string `json:"serial_number"`
	Model        string `json:"model"`
}

type terminalList struct {
	Items []terminal.Terminal `json:"items"`
}

// register handles POST /v1/stations/{id}/terminals.
func (h terminalHandlers) register(w http.ResponseWriter, r *http.Request) {
	var req registerTerminalRequest
	if err := httpjson.DecodeJSON(w, r, &req); err != nil {
		httpjson.WriteError(w, r, err)
		return
	}

	var fe httpjson.FieldErrors
	p := terminal.RegisterParams{
		StationID:    r.PathValue("id"),
		SerialNumber: code(&fe, "serial_number", req.SerialNumber, 6, 32),
		Model:        text(&fe, "model", req.Model, true, 64),
	}
	if err := fe.Err(); err != nil {
		httpjson.WriteError(w, r, err)
		return
	}

	t, err := h.svc.Register(r.Context(), p)
	if err != nil {
		httpjson.WriteError(w, r, terminalError(err))
		return
	}
	w.Header().Set("Location", "/v1/terminals/"+t.ID)
	httpjson.WriteJSON(w, http.StatusCreated, t)
}

// get handles GET /v1/terminals/{id}.
func (h terminalHandlers) get(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, r, terminalError(err))
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, t)
}

// listByStation handles GET /v1/stations/{id}/terminals.
func (h terminalHandlers) listByStation(w http.ResponseWriter, r *http.Request) {
	ts, err := h.svc.ListByStation(r.Context(), r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, r, terminalError(err))
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, terminalList{Items: ts})
}

// transition returns a handler for POST /v1/terminals/{id}/<action>. The
// request has no body; the response is the terminal after the change.
func (h terminalHandlers) transition(apply func(*terminal.Service, context.Context, string) (terminal.Terminal, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := apply(h.svc, r.Context(), r.PathValue("id"))
		if err != nil {
			httpjson.WriteError(w, r, terminalError(err))
			return
		}
		httpjson.WriteJSON(w, http.StatusOK, t)
	}
}

// terminalError maps terminal domain errors to API errors. Unknown errors are
// returned unchanged so WriteError logs them and answers 500.
func terminalError(err error) error {
	var te *terminal.TransitionError
	switch {
	case errors.Is(err, terminal.ErrNotFound), errors.Is(err, terminal.ErrStationNotFound):
		return errNotFound
	case errors.Is(err, terminal.ErrSerialTaken):
		return conflict("serial_number", "A terminal with this serial number is already registered.")
	case errors.As(err, &te):
		return &httpjson.Error{
			Status:  http.StatusConflict,
			Code:    "conflict",
			Message: fmt.Sprintf("Terminal status cannot change from %s to %s.", te.From, te.To),
			Details: []httpjson.FieldError{{Field: "status", Issue: issueInvalidTransition}},
		}
	case errors.Is(err, terminal.ErrStatusChanged):
		return &httpjson.Error{
			Status:  http.StatusConflict,
			Code:    "conflict",
			Message: "Terminal status was changed by another request; retry.",
			Details: []httpjson.FieldError{{Field: "status", Issue: issueConcurrentUpdate}},
		}
	}
	return err
}
