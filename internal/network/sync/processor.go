package peersync

import (
	"context"

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

	// Real conflict resolution and state convergence will follow here.
	// Returning 0 for now as skeleton implementation.
	return 0, nil
}
