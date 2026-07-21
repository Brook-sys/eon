package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func receiptFixture(now time.Time, peer, request, receiver string) domain.SubagentSpawnReceipt {
	return domain.SubagentSpawnReceipt{SchemaVersion: 1, CallerPeerID: peer, RequestID: request, SourceSessionID: "source", Attempt: 0, Task: "work", ContextMode: "isolated", ReceiverSessionID: receiver, RecordedAt: now, Status: domain.SubagentSpawnReceiptPending, UpdatedAt: now}
}

func receiptRecord(now time.Time, id string) domain.SubagentRecord {
	return domain.SubagentRecord{SchemaVersion: 1, ID: id, TaskID: "task-" + id, MissionID: "mission", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "work", ContextMode: "isolated", Attempt: 0, MaxAttempts: 3}
}

func TestSubagentSpawnReceiptStorageDueSaveAndCheckpoint(t *testing.T) {
	now := time.Date(2026, 7, 20, 13, 0, 0, 0, time.UTC)
	store := memory.New()
	later := receiptFixture(now.Add(time.Second), "peer-a", "request-b", "receiver-b")
	earlierB := receiptFixture(now, "peer-b", "request-a", "receiver-c")
	earlierA := receiptFixture(now, "peer-a", "request-c", "receiver-a")
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		for _, record := range []domain.SubagentRecord{receiptRecord(now, "receiver-a"), receiptRecord(now, "receiver-b"), receiptRecord(now, "receiver-c")} {
			if err := tx.CreateSubagentRecord(record); err != nil {
				return err
			}
		}
		for _, receipt := range []domain.SubagentSpawnReceipt{later, earlierB, earlierA} {
			if err := tx.CreateSubagentSpawnReceipt(receipt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		due, err := r.DueSubagentSpawnReceipts(now.Add(time.Second), 10)
		if err != nil {
			return err
		}
		if len(due) != 3 || due[0].RequestID != "request-c" || due[1].RequestID != "request-a" || due[2].RequestID != "request-b" {
			t.Fatalf("due order = %+v", due)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	leased, err := domain.LeaseSubagentSpawnReceipt(earlierA, "worker", now.Add(time.Second), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentSpawnReceipt(leased, earlierA.Status, earlierA.UpdatedAt)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentSpawnReceipt(leased, earlierA.Status, earlierA.UpdatedAt)
	}); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("stale save error = %v", err)
	}
	complete, err := domain.CompleteSubagentSpawnReceipt(leased, "worker", "done", now.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentSpawnReceipt(complete, leased.Status, leased.UpdatedAt)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		pending, err := r.TerminalUndeliveredSubagentSpawnReceipts(1)
		if err != nil {
			return err
		}
		if len(pending) != 1 || pending[0].RequestID != complete.RequestID {
			t.Fatalf("terminal undelivered = %+v", pending)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	inFlight, err := domain.BeginSubagentSpawnReceiptStatusDelivery(complete, now.Add(31*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	delivered, err := domain.MarkSubagentSpawnReceiptStatusDelivered(inFlight, now.Add(32*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.SaveSubagentSpawnReceipt(inFlight, complete.Status, complete.UpdatedAt); err != nil {
			return err
		}
		return tx.SaveSubagentSpawnReceipt(delivered, inFlight.Status, inFlight.UpdatedAt)
	}); err != nil {
		t.Fatal(err)
	}
	mutatedGeneration := delivered
	mutatedGeneration.ReceiverAttempt++
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentSpawnReceipt(mutatedGeneration, delivered.Status, delivered.UpdatedAt)
	}); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("receiver generation mutation error=%v", err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		pending, err := r.TerminalUndeliveredSubagentSpawnReceipts(10)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			t.Fatalf("delivered receipt remained queryable: %+v", pending)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	checkpoint, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := memory.NewFromBinary(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.View(context.Background(), func(r port.Reader) error {
		got, err := r.SubagentSpawnReceipt("peer-a", "request-c")
		if err != nil {
			return err
		}
		if got.Status != domain.SubagentSpawnReceiptComplete || got.StatusDelivery != domain.SubagentStatusDeliveryDelivered || got.Result != "done" {
			t.Fatalf("checkpoint receipt = %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalSubagentSpawnReceiptForReceiverElectsDeterministically(t *testing.T) {
	now := time.Date(2026, 7, 20, 17, 50, 0, 0, time.UTC)
	store := memory.New()
	record := receiptRecord(now, "receiver-a")
	makeTerminal := func(peer, request string, recordedAt, updatedAt time.Time, status domain.SubagentSpawnReceiptStatus) domain.SubagentSpawnReceipt {
		receipt := receiptFixture(recordedAt, peer, request, record.ID)
		receipt.Status, receipt.UpdatedAt, receipt.StatusDelivery = status, updatedAt, domain.SubagentStatusDeliveryPending
		if status == domain.SubagentSpawnReceiptComplete {
			receipt.Result = request
		} else {
			receipt.Failure = request
		}
		return receipt
	}
	late := makeTerminal("peer-a", "late", now, now.Add(3*time.Second), domain.SubagentSpawnReceiptComplete)
	tieB := makeTerminal("peer-b", "tie-b", now.Add(time.Second), now.Add(2*time.Second), domain.SubagentSpawnReceiptFailed)
	tieA := makeTerminal("peer-a", "tie-a", now.Add(time.Second), now.Add(2*time.Second), domain.SubagentSpawnReceiptComplete)
	newGeneration := makeTerminal("peer-c", "new-generation", now, now.Add(time.Second), domain.SubagentSpawnReceiptComplete)
	newGeneration.ReceiverAttempt = 1
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		for _, receipt := range []domain.SubagentSpawnReceipt{late, tieB, tieA, newGeneration} {
			if err := tx.CreateSubagentSpawnReceipt(receipt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		winner, err := r.TerminalSubagentSpawnReceiptForReceiver(record.ID, 0)
		if err != nil {
			return err
		}
		if winner.RequestID != tieA.RequestID {
			t.Fatalf("winner=%+v", winner)
		}
		winner, err = r.TerminalSubagentSpawnReceiptForReceiver(record.ID, 1)
		if err != nil || winner.RequestID != newGeneration.RequestID {
			t.Fatalf("generation 1 winner=%+v err=%v", winner, err)
		}
		if _, err := r.TerminalSubagentSpawnReceiptForReceiver("missing", 0); !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("missing error=%v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
