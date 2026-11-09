// Package station manages fuel stations: the sites that own POS terminals.
package station

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Station is a fuel station. Code is the operator-assigned business key and
// is unique across all stations.
type Station struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	City      string    `json:"city"`
	Address   string    `json:"address,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	// ErrNotFound is returned when no station has the requested ID.
	ErrNotFound = errors.New("station not found")
	// ErrCodeTaken is returned when another station already uses the code.
	ErrCodeTaken = errors.New("station code already exists")
)

// Repository stores stations. Implementations must enforce code uniqueness
// atomically and return ErrCodeTaken on a duplicate.
type Repository interface {
	Create(ctx context.Context, s Station) error
	Get(ctx context.Context, id string) (Station, error)
}

// CreateParams holds the client-supplied fields of a new station. They are
// expected to be validated by the caller.
type CreateParams struct {
	Code    string
	Name    string
	City    string
	Address string
}

// Service implements the station use cases.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService returns a Service backed by repo.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Create assigns an ID and creation time and stores the station.
func (s *Service) Create(ctx context.Context, p CreateParams) (Station, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Station{}, fmt.Errorf("generate station id: %w", err)
	}
	st := Station{
		ID:        id.String(),
		Code:      p.Code,
		Name:      p.Name,
		City:      p.City,
		Address:   p.Address,
		CreatedAt: s.now().UTC().Truncate(time.Microsecond),
	}
	if err := s.repo.Create(ctx, st); err != nil {
		return Station{}, fmt.Errorf("create station: %w", err)
	}
	return st, nil
}

// Get returns the station with the given ID.
func (s *Service) Get(ctx context.Context, id string) (Station, error) {
	st, err := s.repo.Get(ctx, id)
	if err != nil {
		return Station{}, fmt.Errorf("get station: %w", err)
	}
	return st, nil
}
