package station

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestServiceCreateAndGet(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryRepository())
	fixed := time.Date(2026, 11, 9, 10, 0, 0, 123456789, time.FixedZone("TRT", 3*3600))
	svc.now = func() time.Time { return fixed }

	st, err := svc.Create(ctx, CreateParams{Code: "IST-0001", Name: "Kadıköy", City: "İstanbul"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := uuid.Parse(st.ID); err != nil {
		t.Errorf("ID is not a UUID: %q", st.ID)
	}
	if want := fixed.UTC().Truncate(time.Microsecond); !st.CreatedAt.Equal(want) || st.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt: want %v in UTC, got %v", want, st.CreatedAt)
	}

	got, err := svc.Get(ctx, st.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != st {
		t.Errorf("get returned %+v, want %+v", got, st)
	}
}

func TestServiceErrors(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryRepository())

	if _, err := svc.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("get missing: want ErrNotFound, got %v", err)
	}
	if _, err := svc.Create(ctx, CreateParams{Code: "IST-0001", Name: "a", City: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, CreateParams{Code: "IST-0001", Name: "c", City: "d"}); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("duplicate code: want ErrCodeTaken, got %v", err)
	}
}

// Many goroutines racing to create the same code: exactly one must win.
func TestMemoryRepositoryConcurrentCreate(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryRepository())

	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Create(ctx, CreateParams{Code: "IST-0001", Name: fmt.Sprint(i), City: "x"})
			switch {
			case err == nil:
				wins.Add(1)
			case !errors.Is(err, ErrCodeTaken):
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Errorf("want exactly 1 successful create, got %d", wins.Load())
	}
}
