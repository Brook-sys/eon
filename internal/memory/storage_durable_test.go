package memory

import (
	"context"
	"motor-autonomo/internal/port"
	"testing"
	"time"
)

type mockStore struct {
	port.Store
}

func TestDurableMemoryStore(t *testing.T) {
	t.Parallel()
	clock := func() time.Time { return time.Now() }
	ds := NewDurableMemoryStore(&mockStore{}, clock)
	ctx := context.Background()
	
	err := ds.StoreMemory(ctx, "k1", "v1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	
	_, err = ds.RetrieveMemory(ctx, "k1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for now, got %v", err)
	}
	
	err = ds.CompactIrrelevant(ctx)
	if err != nil {
		t.Fatal(err)
	}
}
