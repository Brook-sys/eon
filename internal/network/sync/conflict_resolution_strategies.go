package peersync

import (
	"context"
	"fmt"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// BasicConflictResolver deterministically applies unseen events and discards
// events whose identity is already present in the bounded canonical history.
type BasicConflictResolver struct{}

func NewBasicConflictResolver() *BasicConflictResolver {
	return &BasicConflictResolver{}
}

func (r *BasicConflictResolver) ResolveConflict(_ context.Context, local port.Reader, remote domain.Event) (ConflictDisposition, error) {
	events, err := local.Events(0, 500)
	if err != nil {
		return DispositionEscalate, fmt.Errorf("read local event log: %w", err)
	}
	for _, localEvent := range events {
		if localEvent.ID == remote.ID {
			return DispositionDiscard, nil
		}
	}
	return DispositionApply, nil
}
