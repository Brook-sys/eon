package peersync

import (
	"context"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// DeleteStaleEvents attempts to delete unreferenced stale events from the canonical store
// if the active retention policy authorizes it (AllowEventLogPrune = true). In MVP
// this is not permitted, but the structure is prepared for future phases.
func DeleteStaleEvents(ctx context.Context, policy domain.StoreRetentionPolicy, deleter port.CanonicalEventDeleter) error {
	if !policy.AllowEventLogPrune {
		return domain.ErrUnauthorizedRetention
	}
	// Stub loop structure for deletion (to be implemented with cursor/batch limits)
	return nil
}
