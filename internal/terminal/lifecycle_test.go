package terminal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mehmetalitilgen/fuelpay/internal/requestid"
	"github.com/mehmetalitilgen/fuelpay/internal/station"
)

func TestCanTransition(t *testing.T) {
	all := []Status{StatusPending, StatusActive, StatusSuspended, StatusDecommissioned}
	want := map[[2]Status]bool{
		{StatusPending, StatusActive}:           true,
		{StatusPending, StatusDecommissioned}:   true,
		{StatusActive, StatusSuspended}:         true,
		{StatusActive, StatusDecommissioned}:    true,
		{StatusSuspended, StatusActive}:         true,
		{StatusSuspended, StatusDecommissioned}: true,
	}
	for _, from := range all {
		for _, to := range all {
			if got := CanTransition(from, to); got != want[[2]Status{from, to}] {
				t.Errorf("%s -> %s: want %v, got %v", from, to, want[[2]Status{from, to}], got)
			}
		}
	}
}

func register(t *testing.T, svc *Service, stationID string) Terminal {
	t.Helper()
	term, err := svc.Register(context.Background(), RegisterParams{StationID: stationID, SerialNumber: "S-000001", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	return term
}

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, stationID := newTestService(t)
	clock := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return clock }
	term := register(t, svc, stationID)

	clock = clock.Add(time.Minute)
	active, err := svc.Activate(ctx, term.ID)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if active.Status != StatusActive || active.ActivatedAt == nil || !active.ActivatedAt.Equal(clock) || !active.UpdatedAt.Equal(clock) {
		t.Errorf("activate: want active at %v, got %+v", clock, active)
	}

	// Repeating the current status is a no-op and does not touch updated_at.
	clock = clock.Add(time.Minute)
	again, err := svc.Activate(ctx, term.ID)
	if err != nil || !again.UpdatedAt.Equal(active.UpdatedAt) {
		t.Errorf("repeat activate: want unchanged terminal, got %+v (%v)", again, err)
	}

	suspended, err := svc.Suspend(ctx, term.ID)
	if err != nil || suspended.Status != StatusSuspended || !suspended.ActivatedAt.Equal(*active.ActivatedAt) {
		t.Errorf("suspend: want suspended keeping activated_at, got %+v (%v)", suspended, err)
	}

	if _, err := svc.Activate(ctx, term.ID); err != nil {
		t.Errorf("reactivate: %v", err)
	}
	gone, err := svc.Decommission(ctx, term.ID)
	if err != nil || gone.Status != StatusDecommissioned {
		t.Fatalf("decommission: got %+v (%v)", gone, err)
	}

	_, err = svc.Activate(ctx, term.ID)
	var te *TransitionError
	if !errors.As(err, &te) || !errors.Is(err, ErrInvalidTransition) || te.From != StatusDecommissioned || te.To != StatusActive {
		t.Errorf("activate decommissioned: want TransitionError decommissioned->active, got %v", err)
	}
}

func TestLifecycleInvalidFromPending(t *testing.T) {
	svc, stationID := newTestService(t)
	term := register(t, svc, stationID)
	if _, err := svc.Suspend(context.Background(), term.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("suspend pending: want ErrInvalidTransition, got %v", err)
	}
	if _, err := svc.Activate(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("activate missing: want ErrNotFound, got %v", err)
	}
}

// Concurrent suspend and decommission on an active terminal: both may
// succeed only in the order suspend -> decommission; decommission -> suspend
// must be rejected. Whatever happens, the final status is decommissioned and
// no request sees a lost update.
func TestLifecycleConcurrentTransitions(t *testing.T) {
	ctx := context.Background()
	for range 50 {
		svc, stationID := newTestService(t)
		term := register(t, svc, stationID)
		if _, err := svc.Activate(ctx, term.ID); err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		var suspendErr error
		wg.Add(2)
		go func() { defer wg.Done(); _, suspendErr = svc.Suspend(ctx, term.ID) }()
		go func() {
			defer wg.Done()
			if _, err := svc.Decommission(ctx, term.ID); err != nil {
				t.Errorf("decommission: %v", err)
			}
		}()
		wg.Wait()

		if suspendErr != nil && !errors.Is(suspendErr, ErrInvalidTransition) {
			t.Errorf("suspend: want success or ErrInvalidTransition, got %v", suspendErr)
		}
		final, _ := svc.Get(ctx, term.ID)
		if final.Status != StatusDecommissioned {
			t.Errorf("final status: want decommissioned, got %s", final.Status)
		}
	}
}

// casRepo forces UpdateStatus to report a concurrent change a fixed number
// of times, to exercise the retry loop deterministically.
type casRepo struct {
	*MemoryRepository
	conflicts atomic.Int32
}

func (r *casRepo) UpdateStatus(ctx context.Context, id string, from Status, ev Event) (Terminal, error) {
	if r.conflicts.Add(-1) >= 0 {
		return Terminal{}, ErrStatusChanged
	}
	return r.MemoryRepository.UpdateStatus(ctx, id, from, ev)
}

func TestTransitionRetriesOnConcurrentChange(t *testing.T) {
	ctx := context.Background()
	stations := station.NewService(station.NewMemoryRepository())
	st, err := stations.Create(ctx, station.CreateParams{Code: "IST-1", Name: "a", City: "b"})
	if err != nil {
		t.Fatal(err)
	}
	stationID := st.ID

	repo := &casRepo{MemoryRepository: NewMemoryRepository()}
	svc := NewService(repo, stations)
	term := register(t, svc, stationID)

	repo.conflicts.Store(maxCASAttempts - 1)
	if _, err := svc.Activate(ctx, term.ID); err != nil {
		t.Errorf("want success after %d conflicts, got %v", maxCASAttempts-1, err)
	}

	repo.conflicts.Store(maxCASAttempts)
	if _, err := svc.Suspend(ctx, term.ID); !errors.Is(err, ErrStatusChanged) {
		t.Errorf("want ErrStatusChanged after %d conflicts, got %v", maxCASAttempts, err)
	}
}

func TestEventsRecordEveryChange(t *testing.T) {
	ctx := requestid.NewContext(context.Background(), "req-42")
	svc, stationID := newTestService(t)
	term := register(t, svc, stationID)
	for _, step := range []func(context.Context, string) (Terminal, error){svc.Activate, svc.Activate, svc.Suspend, svc.Decommission} {
		if _, err := step(ctx, term.ID); err != nil {
			t.Fatal(err)
		}
	}
	// A rejected transition must not leave an event behind.
	if _, err := svc.Activate(ctx, term.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}

	evs, err := svc.Events(ctx, term.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		typ      EventType
		from, to Status
	}{
		{EventRegistered, "", StatusPending},
		{EventStatusChanged, StatusPending, StatusActive},
		// the second Activate was a no-op: no event
		{EventStatusChanged, StatusActive, StatusSuspended},
		{EventStatusChanged, StatusSuspended, StatusDecommissioned},
	}
	if len(evs) != len(want) {
		t.Fatalf("want %d events, got %d: %+v", len(want), len(evs), evs)
	}
	for i, w := range want {
		e := evs[i]
		if e.Type != w.typ || e.FromStatus != w.from || e.ToStatus != w.to || e.TerminalID != term.ID {
			t.Errorf("event %d: want %s %s->%s, got %+v", i, w.typ, w.from, w.to, e)
		}
		if i > 0 && e.RequestID != "req-42" {
			t.Errorf("event %d: want request_id req-42, got %q", i, e.RequestID)
		}
	}
	if _, err := svc.Events(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("events of missing terminal: want ErrNotFound, got %v", err)
	}
}
