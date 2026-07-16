package control_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestCommandInboxIdempotentSubmit(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)
	inbox, err := control.NewCommandInbox(store, control.FixedReceiptFactory("receipt_1", now))
	if err != nil {
		t.Fatal(err)
	}
	revision := uint64(1)
	command := domain.OperatorCommand{
		SchemaVersion: domain.SchemaVersionV1, ID: "cmd_1", IdempotencyKey: "idem_1",
		ActorType: domain.ActorOperator, ActorID: "operator_1", Kind: domain.CommandPauseMission,
		Target: domain.CommandTarget{MissionID: "mission_1"}, ExpectedRevision: &revision,
		Reason: "hold", SubmittedAt: now,
	}
	first, err := inbox.SubmitCommand(command)
	if err != nil || first.State != domain.CommandReceived {
		t.Fatalf("first = %#v err=%v", first, err)
	}
	second, err := inbox.SubmitCommand(command)
	if err != nil || second != first {
		t.Fatalf("second = %#v err=%v", second, err)
	}
	// Ensure durable lookup works through the port.
	got, err := inbox.Command(command.ID)
	if err != nil || got.ID != command.ID {
		t.Fatalf("command = %#v err=%v", got, err)
	}
	receipt, err := inbox.CommandReceipt(command.ID)
	if err != nil || receipt.ID != first.ID {
		t.Fatalf("receipt = %#v err=%v", receipt, err)
	}
	divergent := command
	divergent.Reason = "other"
	if _, err := inbox.SubmitCommand(divergent); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("divergent error = %v", err)
	}
	// Create without inbox path still rolls back on later error.
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return errors.New("abort")
	}); err == nil {
		t.Fatal("expected abort")
	}
}
