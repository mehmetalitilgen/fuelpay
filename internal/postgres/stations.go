package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mehmetalitilgen/fuelpay/internal/postgres/db"
	"github.com/mehmetalitilgen/fuelpay/internal/station"
)

// Constraint names from the migrations, used to map unique and foreign key
// violations to domain errors.
const (
	constraintStationCode       = "stations_code_key"
	constraintTerminalSerial    = "terminals_serial_number_key"
	constraintTerminalStationFK = "terminals_station_id_fkey"
)

// StationRepository implements station.Repository on PostgreSQL.
type StationRepository struct {
	q *db.Queries
}

// NewStationRepository returns a StationRepository using pool.
func NewStationRepository(pool *pgxpool.Pool) *StationRepository {
	return &StationRepository{q: db.New(pool)}
}

// Create inserts s. A duplicate code is reported as station.ErrCodeTaken by
// the unique constraint, so concurrent inserts cannot both succeed.
func (r *StationRepository) Create(ctx context.Context, s station.Station) error {
	id, err := uuid.Parse(s.ID)
	if err != nil {
		return err
	}
	err = r.q.CreateStation(ctx, db.CreateStationParams{
		ID: id, Code: s.Code, Name: s.Name, City: s.City, Address: s.Address, CreatedAt: s.CreatedAt,
	})
	if constraintViolated(err, constraintStationCode) {
		return station.ErrCodeTaken
	}
	return err
}

// Get returns the station or station.ErrNotFound. An ID that is not a UUID
// cannot exist, so it is reported as not found without a query.
func (r *StationRepository) Get(ctx context.Context, id string) (station.Station, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return station.Station{}, station.ErrNotFound
	}
	row, err := r.q.GetStation(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return station.Station{}, station.ErrNotFound
	}
	if err != nil {
		return station.Station{}, err
	}
	return station.Station{
		ID:        row.ID.String(),
		Code:      row.Code,
		Name:      row.Name,
		City:      row.City,
		Address:   row.Address,
		CreatedAt: row.CreatedAt.UTC(),
	}, nil
}

// constraintViolated reports whether err is a unique (23505) or foreign key
// (23503) violation of the named constraint.
func constraintViolated(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		(pgErr.Code == "23505" || pgErr.Code == "23503") &&
		pgErr.ConstraintName == constraint
}
