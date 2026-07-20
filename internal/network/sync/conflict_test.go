package peersync_test

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	peersync "motor-autonomo/internal/network/sync"
	"motor-autonomo/internal/port"
)

type dummyResolver struct {
	disposition peersync.ConflictDisposition
}

func (d *dummyResolver) ResolveConflict(ctx context.Context, local port.Reader, remote domain.Event) (peersync.ConflictDisposition, error) {
	return d.disposition, nil
}

func TestEventConflictResolver_Skeleton(t *testing.T) {
	r := &dummyResolver{disposition: peersync.DispositionApply}
	disp, err := r.ResolveConflict(context.Background(), nil, domain.Event{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "evt1",
		Kind:          "test",
		OccurredAt:    time.Now(),
		Sequence:      1,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if disp != peersync.DispositionApply {
		t.Fatalf("expected APPLY, got %v", disp)
	}
}
