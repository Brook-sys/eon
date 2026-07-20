package peersync

import (
	"context"
)

// InboxCanonicalizer attempts to take events stored safely in the PeerSyncInboxRecord
// and applies them to the local node canonical state, provided they pass strict
// domain validity and authority checks.
type InboxCanonicalizer interface {
	Reconcile(ctx context.Context, peerID string) (int, error)
}
