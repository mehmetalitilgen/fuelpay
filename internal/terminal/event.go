package terminal

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mehmetalitilgen/fuelpay/internal/requestid"
)

// EventType classifies an audit event.
type EventType string

// Audit event types.
const (
	EventRegistered    EventType = "registered"
	EventStatusChanged EventType = "status_changed"
)

// Event is an entry in a terminal's audit trail. It records who changed the
// terminal (RequestID, matching the X-Request-ID of the API call) and how.
type Event struct {
	ID         string    `json:"id"`
	TerminalID string    `json:"terminal_id"`
	Type       EventType `json:"type"`
	FromStatus Status    `json:"from_status,omitempty"`
	ToStatus   Status    `json:"to_status"`
	RequestID  string    `json:"request_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

func newEvent(ctx context.Context, terminalID string, typ EventType, from, to Status, at time.Time) (Event, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Event{}, fmt.Errorf("generate event id: %w", err)
	}
	return Event{
		ID:         id.String(),
		TerminalID: terminalID,
		Type:       typ,
		FromStatus: from,
		ToStatus:   to,
		RequestID:  requestid.FromContext(ctx),
		OccurredAt: at,
	}, nil
}
