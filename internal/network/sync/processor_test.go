package peersync_test

import (
	"context"
	"motor-autonomo/internal/network/sync"
	"testing"
)

func TestInboxCanonicalizer_Reconcile(t *testing.T) {
	var c peersync.InboxCanonicalizer
	if c != nil {
		c.Reconcile(context.Background(), "peer1")
	}
}
