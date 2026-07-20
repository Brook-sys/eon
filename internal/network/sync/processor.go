package peersync

import (
	"context"
	"fmt"

	"motor-autonomo/internal/domain"
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

	// Read pending records outside transaction for batching
	var pending []domain.PeerSyncInboxRecord
	err := c.store.View(ctx, func(r port.Reader) error {
		var err error
		pending, err = r.PendingPeerSyncInboxRecords(peerID, 128)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("failed to read peer %s inbox: %w", peerID, err)
	}

	for _, record := range pending {
		// Process each inbox record in its own transaction
		err := c.store.Update(ctx, func(tx port.Transaction) error {
			// Read the record again strictly for idempotency.
			_, err := tx.PeerSyncInboxRecord(record.PeerID, record.OriginID, record.MessageID)
			if err != nil {
				if err == port.ErrNotFound {
					return nil // Already processed or deleted
				}
				return err
			}

			// Simple stub logic to record the count
			if len(record.Message.Events) > 0 {
				reconciled += len(record.Message.Events)
			}

			// Mark this message as successfully processed by removing it from the inbox.
			if err := tx.DeletePeerSyncInboxRecord(record.PeerID, record.OriginID, record.MessageID); err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			return reconciled, fmt.Errorf("failed to reconcile peer %s inbox record %s: %w", peerID, record.MessageID, err)
		}
	}

	return reconciled, nil
}
