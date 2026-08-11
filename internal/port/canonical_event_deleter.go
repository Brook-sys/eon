package port

import "context"

// CanonicalEventDeleter represents the interface for permanently deleting events
// from the underlying canonical store. It is guarded by domain.StoreRetentionPolicy.
type CanonicalEventDeleter interface {
	DeleteEvent(ctx context.Context, id []byte) error
}
