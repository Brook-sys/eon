package contract

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
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
	t.Run("semantic memory and audit events survive atomically", func(t *testing.T) {
		h := factory(t)
		t.Cleanup(func() {
			if err := h.Close(); err != nil {
				t.Errorf("close durable harness: %v", err)
			}
		})

		now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
		memory := domain.LongTermMemory{ID: "durable_memory_1", Key: "durable-memory", Scope: domain.MemoryScopeAgent, Value: "durable value"}
		service, err := control.NewSemanticMemory(h.Store(), source.NewManualClock(now), source.NewSequenceIDGenerator(1))
		if err != nil {
			t.Fatal(err)
		}
		if err := service.SaveMemory(context.Background(), memory, "operator_local"); err != nil {
			t.Fatalf("save semantic memory: %v", err)
		}

		reopened, err := h.Restart()
		if err != nil {
			t.Fatalf("restart after memory save: %v", err)
		}
		stored, err := reopened.LongTermMemory(memory.Key)
		if err != nil || stored.ID != memory.ID || stored.Value != memory.Value || !stored.StoredAt.Equal(now) {
			t.Fatalf("memory after restart = %+v, err=%v", stored, err)
		}
		if err := reopened.View(context.Background(), func(r port.Reader) error {
			event, err := r.EventByID("event_0000000000000001")
			if err != nil {
				return err
			}
			if event.Kind != domain.EventMemoryStored {
				t.Fatalf("stored event kind = %q", event.Kind)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}

		service, err = control.NewSemanticMemory(reopened, source.NewManualClock(now.Add(time.Minute)), source.NewSequenceIDGenerator(2))
		if err != nil {
			t.Fatal(err)
		}
		deleted, err := service.DeleteMemory(context.Background(), memory.ID, "contract_cleanup", "operator_local")
		if err != nil || !deleted {
			t.Fatalf("delete semantic memory = %v, err=%v", deleted, err)
		}
		reopened, err = h.Restart()
		if err != nil {
			t.Fatalf("restart after memory delete: %v", err)
		}
		if _, err := reopened.LongTermMemory(memory.Key); !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("deleted memory after restart error = %v", err)
		}
		if err := reopened.View(context.Background(), func(r port.Reader) error {
			event, err := r.EventByID("event_0000000000000002")
			if err != nil {
				return err
			}
			if event.Kind != domain.EventMemoryCompacted {
				t.Fatalf("delete event kind = %q", event.Kind)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

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

	t.Run("question delivery lease survives restart and remains recoverable", func(t *testing.T) {
		h := factory(t)
		t.Cleanup(func() {
			if err := h.Close(); err != nil {
				t.Errorf("close durable harness: %v", err)
			}
		})
		mission := missionRevision()
		question := operatorQuestionRecord()
		delivery := questionDeliveryRecord(question)
		leaseUntil := delivery.AvailableAt.Add(time.Minute)
		leased, err := domain.LeaseQuestionDelivery(delivery, "crashed_worker", delivery.AvailableAt, leaseUntil)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.Store().Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(mission); err != nil {
				return err
			}
			if err := tx.CreateOperatorQuestion(question); err != nil {
				return err
			}
			if err := tx.CreateQuestionDelivery(delivery); err != nil {
				return err
			}
			return tx.SaveQuestionDelivery(leased, delivery.Status, delivery.Attempt)
		}); err != nil {
			t.Fatalf("persist leased delivery: %v", err)
		}

		reopened, err := h.Restart()
		if err != nil {
			t.Fatalf("restart backend: %v", err)
		}
		if err := reopened.View(context.Background(), func(r port.Reader) error {
			due, err := r.DueQuestionDeliveries(leaseUntil, 10)
			if err != nil {
				return err
			}
			if len(due) != 1 || due[0].ID != delivery.ID || due[0].LeaseOwner != "crashed_worker" {
				t.Fatalf("recoverable delivery after restart = %#v", due)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}
