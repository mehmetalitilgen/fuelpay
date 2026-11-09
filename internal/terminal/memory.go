package terminal

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"
)

// UpdateStatus is a compare-and-set on the status under the write lock.
func (m *MemoryRepository) UpdateStatus(_ context.Context, id string, from, to Status, at time.Time) (Terminal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.byID[id]
	if !ok {
		return Terminal{}, ErrNotFound
	}
	if t.Status != from {
		return Terminal{}, ErrStatusChanged
	}
	t.Status = to
	t.UpdatedAt = at
	if to == StatusActive {
		t.ActivatedAt = &at
	}
	m.byID[id] = t
	return t, nil
}

// MemoryRepository is an in-memory Repository, safe for concurrent use.
type MemoryRepository struct {
	mu       sync.RWMutex
	byID     map[string]Terminal
	bySerial map[string]string
}

// NewMemoryRepository returns an empty MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{byID: make(map[string]Terminal), bySerial: make(map[string]string)}
}

// Create stores t, or returns ErrSerialTaken if the serial is registered.
func (m *MemoryRepository) Create(_ context.Context, t Terminal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.bySerial[t.SerialNumber]; ok {
		return ErrSerialTaken
	}
	m.byID[t.ID] = t
	m.bySerial[t.SerialNumber] = t.ID
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
