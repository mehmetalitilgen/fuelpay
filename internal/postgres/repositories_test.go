package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mehmetalitilgen/fuelpay/internal/station"
	"github.com/mehmetalitilgen/fuelpay/internal/terminal"
)

// cleanPool returns a migrated, empty database of its own.
func cleanPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE terminal_events, terminals, stations"); err != nil {
		t.Fatal(err)
	}
	return pool
}

func newID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

// ts is a timestamp with microsecond precision, as PostgreSQL stores it.
func ts(minute int) time.Time {
	return time.Date(2026, 11, 9, 10, minute, 0, 123456000, time.UTC)
}

func TestStationRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewStationRepository(cleanPool(t))

	s := station.Station{ID: newID(t), Code: "IST-1", Name: "Kadıköy", City: "İstanbul", Address: "Rıhtım", CreatedAt: ts(0)}
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != s {
		t.Errorf("round trip: want %+v, got %+v", s, got)
	}

	dup := s
	dup.ID = newID(t)
	if err := repo.Create(ctx, dup); !errors.Is(err, station.ErrCodeTaken) {
		t.Errorf("duplicate code: want ErrCodeTaken, got %v", err)
	}
	for _, id := range []string{newID(t), "not-a-uuid"} {
		if _, err := repo.Get(ctx, id); !errors.Is(err, station.ErrNotFound) {
			t.Errorf("get %q: want ErrNotFound, got %v", id, err)
		}
	}
}

func TestTerminalRepository(t *testing.T) {
	ctx := context.Background()
	pool := cleanPool(t)
	stations := NewStationRepository(pool)
	repo := NewTerminalRepository(pool)

	stationID := newID(t)
	if err := stations.Create(ctx, station.Station{ID: stationID, Code: "IST-1", Name: "n", City: "c", CreatedAt: ts(0)}); err != nil {
		t.Fatal(err)
	}

	var created []terminal.Terminal
	for i := range 3 {
		term := terminal.Terminal{
			ID: newID(t), StationID: stationID, SerialNumber: fmt.Sprintf("S-00000%d", i), Model: "m",
			Status: terminal.StatusPending, CreatedAt: ts(i), UpdatedAt: ts(i),
		}
		if err := repo.Create(ctx, term, regEvent(t, term)); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		created = append(created, term)
	}

	got, err := repo.Get(ctx, created[0].ID)
	if err != nil || got.ActivatedAt != nil || got.CreatedAt != created[0].CreatedAt {
		t.Errorf("get: want %+v, got %+v (%v)", created[0], got, err)
	}

	list, err := repo.ListByStation(ctx, stationID)
	if err != nil || len(list) != 3 {
		t.Fatalf("list: want 3, got %d (%v)", len(list), err)
	}
	for i := range list {
		if list[i].ID != created[i].ID {
			t.Errorf("list order at %d: want %s, got %s", i, created[i].SerialNumber, list[i].SerialNumber)
		}
	}
	if empty, err := repo.ListByStation(ctx, newID(t)); err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("list unknown station: want empty non-nil slice, got %v (%v)", empty, err)
	}

	dup := created[0]
	dup.ID = newID(t)
	if err := repo.Create(ctx, dup, regEvent(t, dup)); !errors.Is(err, terminal.ErrSerialTaken) {
		t.Errorf("duplicate serial: want ErrSerialTaken, got %v", err)
	}
	orphan := created[0]
	orphan.ID, orphan.SerialNumber, orphan.StationID = newID(t), "S-999999", newID(t)
	if err := repo.Create(ctx, orphan, regEvent(t, orphan)); !errors.Is(err, terminal.ErrStationNotFound) {
		t.Errorf("unknown station: want ErrStationNotFound, got %v", err)
	}
}

func TestTerminalRepositoryUpdateStatus(t *testing.T) {
	ctx := context.Background()
	pool := cleanPool(t)
	stationID := newID(t)
	if err := NewStationRepository(pool).Create(ctx, station.Station{ID: stationID, Code: "IST-1", Name: "n", City: "c", CreatedAt: ts(0)}); err != nil {
		t.Fatal(err)
	}
	repo := NewTerminalRepository(pool)
	term := terminal.Terminal{ID: newID(t), StationID: stationID, SerialNumber: "S-000001", Model: "m", Status: terminal.StatusPending, CreatedAt: ts(0), UpdatedAt: ts(0)}
	if err := repo.Create(ctx, term, regEvent(t, term)); err != nil {
		t.Fatal(err)
	}

	active, err := repo.UpdateStatus(ctx, term.ID, terminal.StatusPending, changeEvent(t, term.ID, terminal.StatusPending, terminal.StatusActive, ts(1)))
	if err != nil || active.Status != terminal.StatusActive || active.ActivatedAt == nil || !active.ActivatedAt.Equal(ts(1)) || !active.UpdatedAt.Equal(ts(1)) {
		t.Fatalf("activate: got %+v (%v)", active, err)
	}

	// The stored status is now active, so a write expecting pending loses.
	if _, err := repo.UpdateStatus(ctx, term.ID, terminal.StatusPending, changeEvent(t, term.ID, terminal.StatusPending, terminal.StatusDecommissioned, ts(2))); !errors.Is(err, terminal.ErrStatusChanged) {
		t.Errorf("stale from: want ErrStatusChanged, got %v", err)
	}
	missingID := newID(t)
	if _, err := repo.UpdateStatus(ctx, missingID, terminal.StatusPending, changeEvent(t, missingID, terminal.StatusPending, terminal.StatusActive, ts(2))); !errors.Is(err, terminal.ErrNotFound) {
		t.Errorf("missing terminal: want ErrNotFound, got %v", err)
	}

	suspended, err := repo.UpdateStatus(ctx, term.ID, terminal.StatusActive, changeEvent(t, term.ID, terminal.StatusActive, terminal.StatusSuspended, ts(3)))
	if err != nil || suspended.ActivatedAt == nil || !suspended.ActivatedAt.Equal(ts(1)) {
		t.Errorf("suspend must keep activated_at: got %+v (%v)", suspended, err)
	}
}

// The service on top of PostgreSQL: concurrent transitions must not lose an
// update, exactly as with the in-memory repository.
func TestTerminalServiceConcurrentTransitionsPostgres(t *testing.T) {
	ctx := context.Background()
	pool := cleanPool(t)
	stations := station.NewService(NewStationRepository(pool))
	terminals := terminal.NewService(NewTerminalRepository(pool), stations)

	st, err := stations.Create(ctx, station.CreateParams{Code: "IST-1", Name: "n", City: "c"})
	if err != nil {
		t.Fatal(err)
	}
	term, err := terminals.Register(ctx, terminal.RegisterParams{StationID: st.ID, SerialNumber: "S-000001", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	errs := make(chan error, n)
	for range n {
		go func() { _, err := terminals.Activate(ctx, term.ID); errs <- err }()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Errorf("concurrent activate: %v", err)
		}
	}
	got, _ := terminals.Get(ctx, term.ID)
	if got.Status != terminal.StatusActive {
		t.Errorf("want active, got %s", got.Status)
	}
}

func regEvent(t *testing.T, term terminal.Terminal) terminal.Event {
	t.Helper()
	return terminal.Event{ID: newID(t), TerminalID: term.ID, Type: terminal.EventRegistered, ToStatus: terminal.StatusPending, OccurredAt: term.CreatedAt}
}

func changeEvent(t *testing.T, terminalID string, from, to terminal.Status, at time.Time) terminal.Event {
	t.Helper()
	return terminal.Event{ID: newID(t), TerminalID: terminalID, Type: terminal.EventStatusChanged, FromStatus: from, ToStatus: to, RequestID: "req-1", OccurredAt: at}
}

func TestTerminalEventsRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := cleanPool(t)
	stationID := newID(t)
	if err := NewStationRepository(pool).Create(ctx, station.Station{ID: stationID, Code: "IST-1", Name: "n", City: "c", CreatedAt: ts(0)}); err != nil {
		t.Fatal(err)
	}
	repo := NewTerminalRepository(pool)
	term := terminal.Terminal{ID: newID(t), StationID: stationID, SerialNumber: "S-000001", Model: "m", Status: terminal.StatusPending, CreatedAt: ts(0), UpdatedAt: ts(0)}
	reg := regEvent(t, term)
	if err := repo.Create(ctx, term, reg); err != nil {
		t.Fatal(err)
	}
	change := changeEvent(t, term.ID, terminal.StatusPending, terminal.StatusActive, ts(1))
	if _, err := repo.UpdateStatus(ctx, term.ID, terminal.StatusPending, change); err != nil {
		t.Fatal(err)
	}
	// A losing compare-and-set must not leave an event behind.
	stale := changeEvent(t, term.ID, terminal.StatusPending, terminal.StatusDecommissioned, ts(2))
	if _, err := repo.UpdateStatus(ctx, term.ID, terminal.StatusPending, stale); !errors.Is(err, terminal.ErrStatusChanged) {
		t.Fatalf("want ErrStatusChanged, got %v", err)
	}

	evs, err := repo.ListEvents(ctx, term.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[0] != reg || evs[1] != change {
		t.Errorf("events: want [%+v %+v], got %+v", reg, change, evs)
	}
}

// If the event cannot be written, the status change must roll back with it.
func TestTerminalUpdateStatusIsAtomic(t *testing.T) {
	ctx := context.Background()
	pool := cleanPool(t)
	stationID := newID(t)
	if err := NewStationRepository(pool).Create(ctx, station.Station{ID: stationID, Code: "IST-1", Name: "n", City: "c", CreatedAt: ts(0)}); err != nil {
		t.Fatal(err)
	}
	repo := NewTerminalRepository(pool)
	term := terminal.Terminal{ID: newID(t), StationID: stationID, SerialNumber: "S-000001", Model: "m", Status: terminal.StatusPending, CreatedAt: ts(0), UpdatedAt: ts(0)}
	if err := repo.Create(ctx, term, regEvent(t, term)); err != nil {
		t.Fatal(err)
	}

	// NOT VALID keeps existing rows but makes every new event insert fail.
	if _, err := pool.Exec(ctx, "ALTER TABLE terminal_events ADD CONSTRAINT test_reject_inserts CHECK (false) NOT VALID"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "ALTER TABLE terminal_events DROP CONSTRAINT IF EXISTS test_reject_inserts")
	})

	if _, err := repo.UpdateStatus(ctx, term.ID, terminal.StatusPending, changeEvent(t, term.ID, terminal.StatusPending, terminal.StatusActive, ts(1))); err == nil {
		t.Fatal("want the update to fail when the event cannot be written")
	}
	got, err := repo.Get(ctx, term.ID)
	if err != nil || got.Status != terminal.StatusPending || got.ActivatedAt != nil {
		t.Errorf("status change must be rolled back: got %+v (%v)", got, err)
	}

	other := terminal.Terminal{ID: newID(t), StationID: stationID, SerialNumber: "S-000002", Model: "m", Status: terminal.StatusPending, CreatedAt: ts(0), UpdatedAt: ts(0)}
	if err := repo.Create(ctx, other, regEvent(t, other)); err == nil {
		t.Fatal("want create to fail when the event cannot be written")
	}
	if _, err := repo.Get(ctx, other.ID); !errors.Is(err, terminal.ErrNotFound) {
		t.Errorf("terminal must not exist without its event: %v", err)
	}
}
