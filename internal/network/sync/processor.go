package peersync

import (
	"context"
	"fmt"

	"motor-autonomo/internal/port"
)

// InboxCanonicalizer attempts to take events stored safely in the PeerSyncInboxRecord
// and applies them to the local node canonical state, provided they pass strict
// domain validity and authority checks.
type InboxCanonicalizer interface {
	Reconcile(ctx context.Context, peerID string) (int, error)
}

// BoundedInboxCanonicalizer processes events from PeerSyncInboxRecord.
type BoundedInboxCanonicalizer struct {
	store port.Store
}

func NewBoundedInboxCanonicalizer(store port.Store) *BoundedInboxCanonicalizer {
	return &BoundedInboxCanonicalizer{store: store}
}

func (c *BoundedInboxCanonicalizer) Reconcile(ctx context.Context, peerID string) (int, error) {
	if peerID == "" {
		return 0, ErrInvalidPeerIdentity
	}

	var reconciled int

	err := c.store.Update(ctx, func(tx port.Transaction) error {
		records, err := tx.PendingPeerSyncInboxRecords(peerID, 128)
		if err != nil {
			return err
		}

		for _, record := range records {
			if len(record.Message.Events) > 0 {
				reconciled += len(record.Message.Events)
			}
		}

		// TODO: proper state convergence applying to canonical log and conflict resolution.
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("failed to reconcile peer %s inbox: %w", peerID, err)
	}

	return reconciled, nil
}
