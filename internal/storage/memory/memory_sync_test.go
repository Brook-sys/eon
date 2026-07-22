package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

func TestStore_ConcurrentAppendEvent(t *testing.T) {
	store := New()

	var wg sync.WaitGroup
	const concurrency = 1000

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = store.Update(context.Background(), func(tx port.Transaction) error {
				event := domain.Event{
					SchemaVersion: domain.SchemaVersionV1,
					ID:            domain.EventID(fmt.Sprintf("evt-%d", id)),
					Kind:          "test.kind",
					OccurredAt:    time.Now().UTC(),
				}
				_, err := tx.AppendEvent(event)
				return err
			})
		}(i)
	}

	wg.Wait()

	err := store.View(context.Background(), func(reader port.Reader) error {
		events, err := reader.Events(0, concurrency+10)
		if err != nil {
			return err
		}
		if len(events) != concurrency {
			t.Errorf("expected %d events, got %d", concurrency, len(events))
		}
		for i := 0; i < len(events); i++ {
			if events[i].Sequence != uint64(i+1) {
				t.Errorf("gap or disorder at index %d: expected sequence %d, got %d", i, i+1, events[i].Sequence)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("View failed: %v", err)
	}
}
