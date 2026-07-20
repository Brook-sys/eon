package memory

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

func TestSubagentStatusIngressCheckpoint(t *testing.T) {
	store := New()
	now := time.Unix(100, 0).UTC()
	record := domain.SubagentRecord{SchemaVersion: 1, ID: "session-1", TaskID: "task-1", MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "work", ContextMode: "isolated", TransportPeerID: "peer-a", MaxAttempts: 2, Deadline: now.Add(time.Minute)}
	receipt := domain.SubagentStatusIngressReceipt{SchemaVersion: 1, CallerPeerID: "peer-a", DeliveryID: "delivery-1", SessionID: record.ID, State: "COMPLETE", Result: "done", Status: domain.SubagentStatusIngressPending, RecordedAt: now}
	rejected, err := domain.RejectSubagentStatusIngressAttemptMismatch(receipt, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentStatusIngressReceipt(rejected)
	}); err != nil {
		t.Fatal(err)
	}
	blob, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewFromBinary(blob)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.View(context.Background(), func(r port.Reader) error {
		got, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-1")
		if err != nil {
			return err
		}
		if !got.Matches(receipt) || got.Status != domain.SubagentStatusIngressRejected || got.RejectionCode != domain.SubagentStatusIngressRejectionAttemptMismatch || !got.RejectedAt.Equal(now.Add(time.Second)) {
			t.Fatalf("got=%+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAppliedSubagentStatusIngressWinnerIsDeterministicAndAttemptScoped(t *testing.T) {
	store := New()
	now := time.Unix(200, 0).UTC()
	record := domain.SubagentRecord{SchemaVersion: 1, ID: "session-1", TaskID: "task-1", MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "work", ContextMode: "isolated", TransportPeerID: "peer-a", MaxAttempts: 2, Deadline: now.Add(time.Minute)}
	makeApplied := func(peer, delivery string, attempt int, recordedAt, appliedAt time.Time, result string) domain.SubagentStatusIngressReceipt {
		receipt := domain.SubagentStatusIngressReceipt{SchemaVersion: 1, CallerPeerID: peer, DeliveryID: delivery, SessionID: record.ID, Attempt: attempt, State: "COMPLETE", Result: result, Status: domain.SubagentStatusIngressPending, RecordedAt: recordedAt}
		applied, err := domain.MarkSubagentStatusIngressApplied(receipt, appliedAt)
		if err != nil {
			t.Fatal(err)
		}
		return applied
	}
	receipts := []domain.SubagentStatusIngressReceipt{
		makeApplied("peer-z", "delivery-later", 0, now, now.Add(2*time.Second), "later"),
		makeApplied("peer-b", "delivery-tie", 0, now, now.Add(time.Second), "tie"),
		makeApplied("peer-a", "delivery-winner", 0, now, now.Add(time.Second), "winner"),
		makeApplied("peer-a", "delivery-old-attempt", 1, now.Add(-time.Second), now, "other-attempt"),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		for _, receipt := range receipts {
			if err := tx.CreateSubagentStatusIngressReceipt(receipt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		winner, err := r.AppliedSubagentStatusIngressWinner(record.ID, 0)
		if err != nil {
			return err
		}
		if winner.DeliveryID != "delivery-winner" || winner.Result != "winner" {
			t.Fatalf("winner=%+v", winner)
		}
		other, err := r.AppliedSubagentStatusIngressWinner(record.ID, 1)
		if err != nil {
			return err
		}
		if other.DeliveryID != "delivery-old-attempt" {
			t.Fatalf("other attempt=%+v", other)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
