package inspect_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestProjectorFilteredEventPaginationMatchesRequestID(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		for i := 1; i <= 5; i++ {
			reqID := ""
			if i == 3 {
				reqID = "my_request_id"
			}
			_, err := tx.AppendEvent(domain.Event{
				SchemaVersion: domain.SchemaVersionV1,
				ID:            domain.EventID(fmt.Sprintf("event_%03d", i)),
				Kind:          "test",
				RequestID:     reqID,
				OccurredAt:    now.Add(time.Duration(i) * time.Millisecond),
			})
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}

	page, err := projector.ListEvents(context.Background(), inspect.EventFilter{RequestID: "my_request_id", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	if len(page.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(page.Events))
	}
	if page.Events[0].RequestID != "my_request_id" {
		t.Fatalf("expected request_id 'my_request_id', got %q", page.Events[0].RequestID)
	}
	if page.Events[0].ID != "event_003" {
		t.Fatalf("expected event_003, got %s", page.Events[0].ID)
	}
}
