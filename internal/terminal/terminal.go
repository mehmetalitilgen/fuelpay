// Package terminal manages POS terminals installed at fuel stations.
package terminal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mehmetalitilgen/fuelpay/internal/station"
)

// Status is the lifecycle state of a terminal.
type Status string

// Terminal statuses. A terminal is registered as pending and can take
// payments only while active.
const (
	StatusPending        Status = "pending"
	StatusActive         Status = "active"
	StatusSuspended      Status = "suspended"
	StatusDecommissioned Status = "decommissioned"
)

// Terminal is a POS terminal. SerialNumber is the manufacturer serial and is
// unique across all stations.
type Terminal struct {
	ID           string     `json:"id"`
	StationID    string     `json:"station_id"`
	SerialNumber string     `json:"serial_number"`
	Model        string     `json:"model"`
	Status       Status     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ActivatedAt  *time.Time `json:"activated_at,omitempty"`
}

var (
	// ErrNotFound is returned when no terminal has the requested ID.
	ErrNotFound = errors.New("terminal not found")
	// ErrSerialTaken is returned when the serial number is already registered.
	ErrSerialTaken = errors.New("terminal serial number already registered")
	// ErrStationNotFound is returned when the owning station does not exist.
	ErrStationNotFound = errors.New("station not found")
)

// Repository stores terminals. Create must enforce serial number uniqueness
// atomically and return ErrSerialTaken on a duplicate.
type Repository interface {
	Create(ctx context.Context, t Terminal) error
	Get(ctx context.Context, id string) (Terminal, error)
	ListByStation(ctx context.Context, stationID string) ([]Terminal, error)
}

// StationGetter is the part of the station service this package needs.
type StationGetter interface {
	Get(ctx context.Context, id string) (station.Station, error)
}

// RegisterParams holds the validated fields of a new terminal.
type RegisterParams struct {
	StationID    string
	SerialNumber string
	Model        string
}

// Service implements the terminal use cases.
type Service struct {
	repo     Repository
	stations StationGetter
	now      func() time.Time
}

// NewService returns a Service backed by repo.
func NewService(repo Repository, stations StationGetter) *Service {
	return &Service{repo: repo, stations: stations, now: time.Now}
}

func (s *Service) timestamp() time.Time {
	return s.now().UTC().Truncate(time.Microsecond)
}

// Register adds a pending terminal to an existing station.
func (s *Service) Register(ctx context.Context, p RegisterParams) (Terminal, error) {
	if err := s.requireStation(ctx, p.StationID); err != nil {
		return Terminal{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Terminal{}, fmt.Errorf("generate terminal id: %w", err)
	}
	now := s.timestamp()
	t := Terminal{
		ID:           id.String(),
		StationID:    p.StationID,
		SerialNumber: p.SerialNumber,
		Model:        p.Model,
		Status:       StatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return Terminal{}, fmt.Errorf("register terminal: %w", err)
	}
	return t, nil
}

// Get returns the terminal with the given ID.
func (s *Service) Get(ctx context.Context, id string) (Terminal, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return Terminal{}, fmt.Errorf("get terminal: %w", err)
	}
	return t, nil
}

// ListByStation returns the terminals of a station, oldest first.
func (s *Service) ListByStation(ctx context.Context, stationID string) ([]Terminal, error) {
	if err := s.requireStation(ctx, stationID); err != nil {
		return nil, err
	}
	ts, err := s.repo.ListByStation(ctx, stationID)
	if err != nil {
		return nil, fmt.Errorf("list terminals: %w", err)
	}
	return ts, nil
}

func (s *Service) requireStation(ctx context.Context, id string) error {
	_, err := s.stations.Get(ctx, id)
	switch {
	case errors.Is(err, station.ErrNotFound):
		return ErrStationNotFound
	case err != nil:
		return fmt.Errorf("look up station: %w", err)
	}
	return nil
}
