package peersync_test

import (
	"context"
	"testing"

	"motor-autonomo/internal/network/sync"
	"motor-autonomo/internal/storage/memory"
)

func TestInboxCanonicalizer_Reconcile(t *testing.T) {
	store := memory.New()
	c := peersync.NewBoundedInboxCanonicalizer(store)

	// Test empty peerID
	_, err := c.Reconcile(context.Background(), "")
	if err == nil || err.Error() != "peer sync origin does not match authenticated caller" {
		t.Errorf("expected ErrInvalidPeerIdentity, got %v", err)
	}

	// Test valid skeleton call
	count, err := c.Reconcile(context.Background(), "peer1")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %v", count)
	}
}
