package station

import (
	"context"
	"sync"
)

// MemoryRepository is an in-memory Repository, safe for concurrent use.
type MemoryRepository struct {
	mu     sync.RWMutex
	byID   map[string]Station
	byCode map[string]string
}

// NewMemoryRepository returns an empty MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{byID: make(map[string]Station), byCode: make(map[string]string)}
}

// Create stores s, or returns ErrCodeTaken if its code is in use. The check
// and the insert happen under one lock so concurrent creates cannot both win.
func (m *MemoryRepository) Create(_ context.Context, s Station) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byCode[s.Code]; ok {
		return ErrCodeTaken
	}
	m.byID[s.ID] = s
	m.byCode[s.Code] = s.ID
	return nil
}

// Get returns the station with the given ID or ErrNotFound.
func (m *MemoryRepository) Get(_ context.Context, id string) (Station, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.byID[id]
	if !ok {
		return Station{}, ErrNotFound
	}
	return s, nil
}
