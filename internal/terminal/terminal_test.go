package terminal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mehmetalitilgen/fuelpay/internal/station"
)

// newTestService returns a terminal service and the ID of an existing station.
func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	stations := station.NewService(station.NewMemoryRepository())
	st, err := stations.Create(context.Background(), station.CreateParams{Code: "IST-1", Name: "a", City: "b"})
	if err != nil {
		t.Fatal(err)
	}
	return NewService(NewMemoryRepository(), stations), st.ID
}

func TestRegister(t *testing.T) {
	ctx := context.Background()
	svc, stationID := newTestService(t)
	fixed := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	term, err := svc.Register(ctx, RegisterParams{StationID: stationID, SerialNumber: "PAX-A920-0001", Model: "PAX A920"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if term.Status != StatusPending || term.ActivatedAt != nil {
		t.Errorf("new terminal: want pending and not activated, got %s %v", term.Status, term.ActivatedAt)
	}
	if !term.CreatedAt.Equal(fixed) || !term.UpdatedAt.Equal(fixed) {
		t.Errorf("timestamps: want %v, got %v / %v", fixed, term.CreatedAt, term.UpdatedAt)
	}

	got, err := svc.Get(ctx, term.ID)
	if err != nil || got.ID != term.ID || got.SerialNumber != "PAX-A920-0001" {
		t.Errorf("get: want the registered terminal, got %+v (%v)", got, err)
	}
}

func TestRegisterErrors(t *testing.T) {
	ctx := context.Background()
	svc, stationID := newTestService(t)

	if _, err := svc.Register(ctx, RegisterParams{StationID: "missing", SerialNumber: "S-000001", Model: "m"}); !errors.Is(err, ErrStationNotFound) {
		t.Errorf("unknown station: want ErrStationNotFound, got %v", err)
	}
	if _, err := svc.Register(ctx, RegisterParams{StationID: stationID, SerialNumber: "S-000001", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, RegisterParams{StationID: stationID, SerialNumber: "S-000001", Model: "m"}); !errors.Is(err, ErrSerialTaken) {
		t.Errorf("duplicate serial: want ErrSerialTaken, got %v", err)
	}
	if _, err := svc.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("get missing: want ErrNotFound, got %v", err)
	}
	if _, err := svc.ListByStation(ctx, "missing"); !errors.Is(err, ErrStationNotFound) {
		t.Errorf("list for unknown station: want ErrStationNotFound, got %v", err)
	}
}

func TestListByStationOrder(t *testing.T) {
	ctx := context.Background()
	svc, stationID := newTestService(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 3 {
		svc.now = func() time.Time { return base.Add(time.Duration(i) * time.Minute) }
		if _, err := svc.Register(ctx, RegisterParams{StationID: stationID, SerialNumber: fmt.Sprintf("S-00000%d", i), Model: "m"}); err != nil {
			t.Fatal(err)
		}
	}

	list, err := svc.ListByStation(ctx, stationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("want 3 terminals, got %d", len(list))
	}
	for i, term := range list {
		if want := fmt.Sprintf("S-00000%d", i); term.SerialNumber != want {
			t.Errorf("position %d: want %s, got %s", i, want, term.SerialNumber)
		}
	}
}

func TestRegisterConcurrentSameSerial(t *testing.T) {
	ctx := context.Background()
	svc, stationID := newTestService(t)

	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Register(ctx, RegisterParams{StationID: stationID, SerialNumber: "S-000001", Model: "m"})
			switch {
			case err == nil:
				wins.Add(1)
			case !errors.Is(err, ErrSerialTaken):
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Errorf("want exactly 1 successful register, got %d", wins.Load())
	}
}
