package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// allowed lists the statuses each status may move to. Decommissioned is
// final: a decommissioned device is never brought back.
var allowed = map[Status][]Status{
	StatusPending:        {StatusActive, StatusDecommissioned},
	StatusActive:         {StatusSuspended, StatusDecommissioned},
	StatusSuspended:      {StatusActive, StatusDecommissioned},
	StatusDecommissioned: nil,
}

// CanTransition reports whether a terminal may move from one status to another.
func CanTransition(from, to Status) bool {
	return slices.Contains(allowed[from], to)
}

var (
	// ErrInvalidTransition is matched by every *TransitionError.
	ErrInvalidTransition = errors.New("invalid terminal status transition")
	// ErrStatusChanged is returned by Repository.UpdateStatus when the stored
	// status no longer equals the expected one (a concurrent update won).
	ErrStatusChanged = errors.New("terminal status changed concurrently")
)

// TransitionError reports a status change the lifecycle does not allow.
type TransitionError struct {
	From, To Status
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("terminal cannot move from %s to %s", e.From, e.To)
}

// Is makes errors.Is(err, ErrInvalidTransition) true for a *TransitionError.
func (e *TransitionError) Is(target error) bool { return target == ErrInvalidTransition }

// maxCASAttempts bounds how often a transition is re-evaluated when another
// request changes the status between our read and our write.
const maxCASAttempts = 3

// Activate moves the terminal to active and records the activation time.
func (s *Service) Activate(ctx context.Context, id string) (Terminal, error) {
	return s.transition(ctx, id, StatusActive)
}

// Suspend moves an active terminal to suspended.
func (s *Service) Suspend(ctx context.Context, id string) (Terminal, error) {
	return s.transition(ctx, id, StatusSuspended)
}

// Decommission retires the terminal permanently.
func (s *Service) Decommission(ctx context.Context, id string) (Terminal, error) {
	return s.transition(ctx, id, StatusDecommissioned)
}

// transition applies a status change with optimistic concurrency: it reads
// the terminal, checks the lifecycle, and writes only if the status is still
// the one it read. Requesting the current status is a no-op, so retried
// requests are safe.
func (s *Service) transition(ctx context.Context, id string, to Status) (Terminal, error) {
	for range maxCASAttempts {
		t, err := s.repo.Get(ctx, id)
		if err != nil {
			return Terminal{}, fmt.Errorf("get terminal: %w", err)
		}
		if t.Status == to {
			return t, nil
		}
		if !CanTransition(t.Status, to) {
			return Terminal{}, &TransitionError{From: t.Status, To: to}
		}

		ev, err := newEvent(ctx, id, EventStatusChanged, t.Status, to, s.timestamp())
		if err != nil {
			return Terminal{}, err
		}
		updated, err := s.repo.UpdateStatus(ctx, id, t.Status, ev)
		if errors.Is(err, ErrStatusChanged) {
			continue
		}
		if err != nil {
			return Terminal{}, fmt.Errorf("update terminal status: %w", err)
		}
		return updated, nil
	}
	return Terminal{}, fmt.Errorf("update terminal status: %w", ErrStatusChanged)
}
