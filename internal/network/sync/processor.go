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
	store    port.Store
	resolver EventConflictResolver
}

func NewBoundedInboxCanonicalizer(store port.Store, resolver EventConflictResolver) (*BoundedInboxCanonicalizer, error) {
	if resolver == nil {
		return nil, ErrConflictResolverRequired
	}
	return &BoundedInboxCanonicalizer{
		store:    store,
		resolver: resolver,
	}, nil
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

			// Delegate resolution of each event
			for _, event := range record.Message.Events {
				// Filter events directly inside canonicalizer for valid Phase 25 test resolution
				// and check against a real resolver implementation.
				// (Validation of structural integrity is delegated via schema version checking)
				if event.SchemaVersion != domain.SchemaVersionV1 || event.ID == "" {
					// Discard inherently malformed remotes
					continue
				}

				disp, err := c.resolver.ResolveConflict(ctx, tx, event)
				if err != nil {
					return fmt.Errorf("failed to resolve conflict for event %s: %w", event.ID, err)
				}

				switch disp {
				case DispositionApply:
					reconciled++
					eventToApply := event
					eventToApply.Sequence = 0 // Strip sequence since it belongs to the local log
					if _, err := tx.AppendEvent(eventToApply); err != nil {
						return fmt.Errorf("failed to apply event %s to canonical store: %w", event.ID, err)
					}
				case DispositionDiscard:
					// Treat as safely processed without mutating canonical state.
				case DispositionEscalate:
					return fmt.Errorf("%w: event %s", ErrConflictEscalated, event.ID)
				default:
					return fmt.Errorf("%w: %s", ErrInvalidDisposition, disp)
				}
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
