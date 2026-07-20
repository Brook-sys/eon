package peersync_test

import (
	"context"
	"testing"
	"motor-autonomo/internal/network/sync"
)

func TestInboxCanonicalizer_Reconcile(t *testing.T) {
	var c peersync.InboxCanonicalizer
	if c != nil {
		c.Reconcile(context.Background())
	}
}

