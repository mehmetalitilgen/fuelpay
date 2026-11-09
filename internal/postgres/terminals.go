package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mehmetalitilgen/fuelpay/internal/postgres/db"
	"github.com/mehmetalitilgen/fuelpay/internal/terminal"
)

// TerminalRepository implements terminal.Repository on PostgreSQL.
type TerminalRepository struct {
	q *db.Queries
}

// NewTerminalRepository returns a TerminalRepository using pool.
func NewTerminalRepository(pool *pgxpool.Pool) *TerminalRepository {
	return &TerminalRepository{q: db.New(pool)}
}

// Create inserts t. Unique and foreign key violations become
// terminal.ErrSerialTaken and terminal.ErrStationNotFound.
func (r *TerminalRepository) Create(ctx context.Context, t terminal.Terminal) error {
	id, err := uuid.Parse(t.ID)
	if err != nil {
		return err
	}
	stationID, err := uuid.Parse(t.StationID)
	if err != nil {
		return terminal.ErrStationNotFound
	}
	err = r.q.CreateTerminal(ctx, db.CreateTerminalParams{
		ID:           id,
		StationID:    stationID,
		SerialNumber: t.SerialNumber,
		Model:        t.Model,
		Status:       string(t.Status),
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	})
	switch {
	case constraintViolated(err, constraintTerminalSerial):
		return terminal.ErrSerialTaken
	case constraintViolated(err, constraintTerminalStationFK):
		return terminal.ErrStationNotFound
	}
	return err
}

// Get returns the terminal or terminal.ErrNotFound.
func (r *TerminalRepository) Get(ctx context.Context, id string) (terminal.Terminal, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return terminal.Terminal{}, terminal.ErrNotFound
	}
	row, err := r.q.GetTerminal(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return terminal.Terminal{}, terminal.ErrNotFound
	}
	if err != nil {
		return terminal.Terminal{}, err
	}
	return toTerminal(row), nil
}

// ListByStation returns the station's terminals ordered by creation time.
func (r *TerminalRepository) ListByStation(ctx context.Context, stationID string) ([]terminal.Terminal, error) {
	uid, err := uuid.Parse(stationID)
	if err != nil {
		return []terminal.Terminal{}, nil
	}
	rows, err := r.q.ListTerminalsByStation(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]terminal.Terminal, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTerminal(row))
	}
	return out, nil
}

// UpdateStatus is a single conditional UPDATE: it succeeds only if the status
// is still "from". When no row matches, a second read tells a missing
// terminal (ErrNotFound) from a concurrent change (ErrStatusChanged).
func (r *TerminalRepository) UpdateStatus(ctx context.Context, id string, from, to terminal.Status, at time.Time) (terminal.Terminal, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return terminal.Terminal{}, terminal.ErrNotFound
	}
	row, err := r.q.UpdateTerminalStatus(ctx, db.UpdateTerminalStatusParams{
		ID: uid, OldStatus: string(from), NewStatus: string(to), ChangedAt: at,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := r.Get(ctx, id); getErr != nil {
			return terminal.Terminal{}, getErr
		}
		return terminal.Terminal{}, terminal.ErrStatusChanged
	}
	if err != nil {
		return terminal.Terminal{}, err
	}
	return toTerminal(row), nil
}

func toTerminal(row db.Terminal) terminal.Terminal {
	t := terminal.Terminal{
		ID:           row.ID.String(),
		StationID:    row.StationID.String(),
		SerialNumber: row.SerialNumber,
		Model:        row.Model,
		Status:       terminal.Status(row.Status),
		CreatedAt:    row.CreatedAt.UTC(),
		UpdatedAt:    row.UpdatedAt.UTC(),
	}
	if row.ActivatedAt != nil {
		at := row.ActivatedAt.UTC()
		t.ActivatedAt = &at
	}
	return t
}
