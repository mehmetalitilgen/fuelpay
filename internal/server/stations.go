package server

import (
	"errors"
	"net/http"

	"github.com/mehmetalitilgen/fuelpay/internal/httpjson"
	"github.com/mehmetalitilgen/fuelpay/internal/station"
)

type stationHandlers struct {
	svc *station.Service
}

type createStationRequest struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	City    string `json:"city"`
	Address string `json:"address"`
}

// create handles POST /v1/stations.
func (h stationHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req createStationRequest
	if err := httpjson.DecodeJSON(w, r, &req); err != nil {
		httpjson.WriteError(w, r, err)
		return
	}

	var fe httpjson.FieldErrors
	p := station.CreateParams{
		Code:    code(&fe, "code", req.Code, 3, 32),
		Name:    text(&fe, "name", req.Name, true, 100),
		City:    text(&fe, "city", req.City, true, 64),
		Address: text(&fe, "address", req.Address, false, 200),
	}
	if err := fe.Err(); err != nil {
		httpjson.WriteError(w, r, err)
		return
	}

	st, err := h.svc.Create(r.Context(), p)
	switch {
	case errors.Is(err, station.ErrCodeTaken):
		httpjson.WriteError(w, r, conflict("code", "A station with this code already exists."))
		return
	case err != nil:
		httpjson.WriteError(w, r, err)
		return
	}

	w.Header().Set("Location", "/v1/stations/"+st.ID)
	httpjson.WriteJSON(w, http.StatusCreated, st)
}

// get handles GET /v1/stations/{id}.
func (h stationHandlers) get(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Get(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, station.ErrNotFound):
		httpjson.WriteError(w, r, errNotFound)
		return
	case err != nil:
		httpjson.WriteError(w, r, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, st)
}
