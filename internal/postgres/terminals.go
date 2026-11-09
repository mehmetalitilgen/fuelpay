package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mehmetalitilgen/fuelpay/internal/postgres/db"
	"github.com/mehmetalitilgen/fuelpay/internal/terminal"
)

// TerminalRepository implements terminal.Repository on PostgreSQL.
type TerminalRepository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewTerminalRepository returns a TerminalRepository using pool.
func NewTerminalRepository(pool *pgxpool.Pool) *TerminalRepository {
	return &TerminalRepository{pool: pool, q: db.New(pool)}
}

// Create inserts t and its event in one transaction. Unique and foreign key
// violations become terminal.ErrSerialTaken and terminal.ErrStationNotFound.
func (r *TerminalRepository) Create(ctx context.Context, t terminal.Terminal, ev terminal.Event) error {
	id, err := uuid.Parse(t.ID)
	if err != nil {
		return err
	}
	stationID, err := uuid.Parse(t.StationID)
	if err != nil {
		return terminal.ErrStationNotFound
	}
	evParams, err := eventParams(ev)
	if err != nil {
		return err
	}

	err = withTx(ctx, r.pool, func(q *db.Queries) error {
		err := q.CreateTerminal(ctx, db.CreateTerminalParams{
			ID:           id,
			StationID:    stationID,
			SerialNumber: t.SerialNumber,
			Model:        t.Model,
			Status:       string(t.Status),
			CreatedAt:    t.CreatedAt,
			UpdatedAt:    t.UpdatedAt,
		})
		if err != nil {
			return err
		}
		return q.CreateTerminalEvent(ctx, evParams)
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

// errNoRowUpdated signals inside the transaction that the conditional UPDATE
// matched nothing, so the transaction is rolled back before investigating.
var errNoRowUpdated = errors.New("no row updated")

// UpdateStatus runs the conditional UPDATE and the event INSERT in one
// transaction. The UPDATE succeeds only if the status is still from; if it
// matches no row, a read after the rollback tells a missing terminal
// (ErrNotFound) from a concurrent change (ErrStatusChanged).
func (r *TerminalRepository) UpdateStatus(ctx context.Context, id string, from terminal.Status, ev terminal.Event) (terminal.Terminal, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return terminal.Terminal{}, terminal.ErrNotFound
	}
	evParams, err := eventParams(ev)
	if err != nil {
		return terminal.Terminal{}, err
	}

	var updated db.Terminal
	err = withTx(ctx, r.pool, func(q *db.Queries) error {
		row, err := q.UpdateTerminalStatus(ctx, db.UpdateTerminalStatusParams{
			ID: uid, OldStatus: string(from), NewStatus: string(ev.ToStatus), ChangedAt: ev.OccurredAt,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoRowUpdated
		}
		if err != nil {
			return err
		}
		updated = row
		return q.CreateTerminalEvent(ctx, evParams)
	})
	if errors.Is(err, errNoRowUpdated) {
		if _, getErr := r.Get(ctx, id); getErr != nil {
			return terminal.Terminal{}, getErr
		}
		return terminal.Terminal{}, terminal.ErrStatusChanged
	}
	if err != nil {
		return terminal.Terminal{}, err
	}
	return toTerminal(updated), nil
}

// ListEvents returns the terminal's events, oldest first.
func (r *TerminalRepository) ListEvents(ctx context.Context, terminalID string) ([]terminal.Event, error) {
	uid, err := uuid.Parse(terminalID)
	if err != nil {
		return []terminal.Event{}, nil
	}
	rows, err := r.q.ListTerminalEvents(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]terminal.Event, 0, len(rows))
	for _, row := range rows {
		e := terminal.Event{
			ID:         row.ID.String(),
			TerminalID: row.TerminalID.String(),
			Type:       terminal.EventType(row.Type),
			ToStatus:   terminal.Status(row.ToStatus),
			RequestID:  row.RequestID,
			OccurredAt: row.OccurredAt.UTC(),
		}
		if row.FromStatus != nil {
			e.FromStatus = terminal.Status(*row.FromStatus)
		}
		out = append(out, e)
	}
	return out, nil
}

func eventParams(ev terminal.Event) (db.CreateTerminalEventParams, error) {
	id, err := uuid.Parse(ev.ID)
	if err != nil {
		return db.CreateTerminalEventParams{}, fmt.Errorf("event id: %w", err)
	}
	terminalID, err := uuid.Parse(ev.TerminalID)
	if err != nil {
		return db.CreateTerminalEventParams{}, fmt.Errorf("event terminal id: %w", err)
	}
	p := db.CreateTerminalEventParams{
		ID:         id,
		TerminalID: terminalID,
		Type:       string(ev.Type),
		ToStatus:   string(ev.ToStatus),
		RequestID:  ev.RequestID,
		OccurredAt: ev.OccurredAt,
	}
	if ev.FromStatus != "" {
		from := string(ev.FromStatus)
		p.FromStatus = &from
	}
	return p, nil
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
