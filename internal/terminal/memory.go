package terminal

import (
	"cmp"
	"context"
	"slices"
	"sync"
)

// MemoryRepository is an in-memory Repository, safe for concurrent use. A
// single lock covers terminals and events, so each write is atomic.
type MemoryRepository struct {
	mu       sync.RWMutex
	byID     map[string]Terminal
	bySerial map[string]string
	events   map[string][]Event
}

// NewMemoryRepository returns an empty MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		byID:     make(map[string]Terminal),
		bySerial: make(map[string]string),
		events:   make(map[string][]Event),
	}
}

// Create stores t and its event, or returns ErrSerialTaken.
func (m *MemoryRepository) Create(_ context.Context, t Terminal, ev Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.bySerial[t.SerialNumber]; ok {
		return ErrSerialTaken
	}
	m.byID[t.ID] = t
	m.bySerial[t.SerialNumber] = t.ID
	m.events[t.ID] = append(m.events[t.ID], ev)
	return nil
}

// Get returns the terminal with the given ID or ErrNotFound.
func (m *MemoryRepository) Get(_ context.Context, id string) (Terminal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.byID[id]
	if !ok {
		return Terminal{}, ErrNotFound
	}
	return t, nil
}

// ListByStation returns the station's terminals ordered by creation time.
func (m *MemoryRepository) ListByStation(_ context.Context, stationID string) ([]Terminal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []Terminal{}
	for _, t := range m.byID {
		if t.StationID == stationID {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b Terminal) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out, nil
}

// UpdateStatus is a compare-and-set on the status under the write lock; the
// event is appended under the same lock.
func (m *MemoryRepository) UpdateStatus(_ context.Context, id string, from Status, ev Event) (Terminal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.byID[id]
	if !ok {
		return Terminal{}, ErrNotFound
	}
	if t.Status != from {
		return Terminal{}, ErrStatusChanged
	}
	t.Status = ev.ToStatus
	t.UpdatedAt = ev.OccurredAt
	if ev.ToStatus == StatusActive {
		at := ev.OccurredAt
		t.ActivatedAt = &at
	}
	m.byID[id] = t
	m.events[id] = append(m.events[id], ev)
	return t, nil
}

// ListEvents returns a copy of the terminal's events, oldest first.
func (m *MemoryRepository) ListEvents(_ context.Context, terminalID string) ([]Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Event{}, m.events[terminalID]...), nil
}
