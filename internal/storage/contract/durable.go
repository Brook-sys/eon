package contract

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// DurableHarness owns a backend instance whose durable state can be reopened.
// Restart must simulate a process boundary: in-memory caches and connections are
// discarded, while committed durable data remains available.
type DurableHarness interface {
	Store() port.Store
	Restart() (port.Store, error)
	Close() error
}

// DurableFactory creates an isolated backend location for one contract test.
type DurableFactory func(testing.TB) DurableHarness

// TestDurableStore verifies the minimum recovery semantics required from disk
// backends. It is intentionally separate from TestStore so the in-memory fake
// is not mistaken for evidence of process-crash durability.
func TestDurableStore(t *testing.T, factory DurableFactory) {
	t.Helper()
	t.Run("committed state survives restart and rollback does not", func(t *testing.T) {
		h := factory(t)
		t.Cleanup(func() {
			if err := h.Close(); err != nil {
				t.Errorf("close durable harness: %v", err)
			}
		})

		store := h.Store()
		revision := missionRevision()
		committed := event("event_committed", "durability.committed")
		rolledBack := event("event_rolled_back", "durability.rolled_back")
		rollbackSentinel := errors.New("force rollback")

		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(revision); err != nil {
				return err
			}
			if err := tx.ActivateMissionRevision(revision.MissionID, revision.ID); err != nil {
				return err
			}
			_, err := tx.AppendEvent(committed)
			return err
		}); err != nil {
			t.Fatalf("commit fixture: %v", err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if _, err := tx.AppendEvent(rolledBack); err != nil {
				return err
			}
			return rollbackSentinel
		}); !errors.Is(err, rollbackSentinel) {
			t.Fatalf("rollback error = %v, want sentinel", err)
		}

		reopened, err := h.Restart()
		if err != nil {
			t.Fatalf("restart backend: %v", err)
		}
		if err := reopened.View(context.Background(), func(r port.Reader) error {
			active, err := r.ActiveMissionRevision(revision.MissionID)
			if err != nil {
				return err
			}
			if active.ID != revision.ID {
				t.Fatalf("active revision after restart = %q, want %q", active.ID, revision.ID)
			}
			if _, err := r.EventByID(committed.ID); err != nil {
				return err
			}
			if _, err := r.EventByID(rolledBack.ID); !errors.Is(err, port.ErrNotFound) {
				t.Fatalf("rolled-back event after restart error = %v, want ErrNotFound", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("idempotency completion survives repeated restarts", func(t *testing.T) {
		h := factory(t)
		t.Cleanup(func() {
			if err := h.Close(); err != nil {
				t.Errorf("close durable harness: %v", err)
			}
		})
		now := time.Date(2026, 7, 15, 17, 0, 0, 0, time.UTC)
		record := domain.IdempotencyRecord{
			SchemaVersion: 1,
			Key:           "durable_idempotency_1",
			OperationID:   "operation_1",
			Intent:        "durable contract intent",
			Status:        domain.IdempotencyReserved,
			ReservedAt:    now,
		}
		if err := h.Store().Update(context.Background(), func(tx port.Transaction) error {
			if _, err := tx.ReserveIdempotency(record); err != nil {
				return err
			}
			_, err := tx.CompleteIdempotency(record.Key, "receipt_1", "result_1", now.Add(time.Second))
			return err
		}); err != nil {
			t.Fatalf("complete idempotency: %v", err)
		}

		for restart := 1; restart <= 2; restart++ {
			store, err := h.Restart()
			if err != nil {
				t.Fatalf("restart %d: %v", restart, err)
			}
			if err := store.View(context.Background(), func(r port.Reader) error {
				got, err := r.IdempotencyRecord(record.Key)
				if err != nil {
					return err
				}
				if got.Status != domain.IdempotencyCompleted || got.ReceiptID != "receipt_1" || got.ResultRef != "result_1" {
					t.Fatalf("idempotency after restart %d = %#v", restart, got)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
	})
}
