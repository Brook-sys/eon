package kernel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestCommandProcessorPauseResumeShutdownAndReplay(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "learn", Purpose: "learn", Status: domain.MissionActive, Provenance: "fixture", AcceptedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		return tx.ActivateMissionRevision(mission.MissionID, mission.ID)
	}); err != nil {
		t.Fatal(err)
	}
	inbox, err := control.NewCommandInbox(store, control.FixedReceiptFactory("receipt_seed", now))
	if err != nil {
		t.Fatal(err)
	}
	revision := uint64(1)
	pause := domain.OperatorCommand{
		SchemaVersion: domain.SchemaVersionV1, ID: "cmd_pause", IdempotencyKey: "idem_pause",
		ActorType: domain.ActorOperator, ActorID: "operator_1", Kind: domain.CommandPauseMission,
		Target: domain.CommandTarget{MissionID: mission.MissionID}, ExpectedRevision: &revision,
		Reason: "maintenance", SubmittedAt: now,
	}
	if _, err := inbox.SubmitCommand(pause); err != nil {
		t.Fatal(err)
	}
	// Replay identical command returns existing receipt without duplicate create.
	if receipt, err := inbox.SubmitCommand(pause); err != nil || receipt.State != domain.CommandReceived {
		t.Fatalf("replay submit = %#v err=%v", receipt, err)
	}
	processor, err := kernel.NewCommandProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	applied, ok, err := processor.ProcessNext(context.Background())
	if err != nil || !ok || applied.State != domain.CommandApplied || applied.ResultRef != "mission_1@1:PAUSED" {
		t.Fatalf("pause process = %#v ok=%v err=%v", applied, ok, err)
	}
	// Replay after apply is a no-op.
	again, err := processor.Process(context.Background(), pause.ID)
	if err != nil || again.State != domain.CommandApplied || again.ResultRef != applied.ResultRef {
		t.Fatalf("replay process = %#v err=%v", again, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		state, err := r.ControlState()
		if err != nil {
			return err
		}
		if state.AllowsDispatch(mission.MissionID) {
			t.Fatalf("paused control still allows dispatch: %#v", state)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	resume := pause
	resume.ID, resume.IdempotencyKey, resume.Kind, resume.Reason = "cmd_resume", "idem_resume", domain.CommandResumeMission, "done"
	if _, err := inbox.SubmitCommand(resume); err != nil {
		t.Fatal(err)
	}
	// Divergent reuse of the same idempotency key is a conflict.
	divergent := resume
	divergent.Reason = "other"
	if _, err := inbox.SubmitCommand(divergent); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("divergent idempotency error = %v", err)
	}
	applied, ok, err = processor.ProcessNext(context.Background())
	if err != nil || !ok || applied.State != domain.CommandApplied || applied.ResultRef != "mission_1@1:ENABLED" {
		t.Fatalf("resume process = %#v ok=%v err=%v", applied, ok, err)
	}

	shutdown := domain.OperatorCommand{
		SchemaVersion: domain.SchemaVersionV1, ID: "cmd_stop", IdempotencyKey: "idem_stop",
		ActorType: domain.ActorOperator, ActorID: "operator_1", Kind: domain.CommandGracefulShutdown,
		Reason: "deploy", SubmittedAt: now,
	}
	if _, err := inbox.SubmitCommand(shutdown); err != nil {
		t.Fatal(err)
	}
	applied, ok, err = processor.ProcessNext(context.Background())
	if err != nil || !ok || applied.State != domain.CommandApplied || applied.ResultRef != "process:stopping" {
		t.Fatalf("shutdown process = %#v ok=%v err=%v", applied, ok, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		state, err := r.ControlState()
		if err != nil {
			return err
		}
		if state.ProcessMode != domain.ProcessStopping || state.AllowsDispatch(mission.MissionID) {
			t.Fatalf("stopping control = %#v", state)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCommandProcessorRejectsStaleMissionRevision(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 2, 10, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_2", MissionID: "mission_1", Revision: 2,
		OriginalText: "learn", Purpose: "learn", Status: domain.MissionActive, Provenance: "fixture", AcceptedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		return tx.ActivateMissionRevision(mission.MissionID, mission.ID)
	}); err != nil {
		t.Fatal(err)
	}
	inbox, err := control.NewCommandInbox(store, control.FixedReceiptFactory("receipt_seed", now))
	if err != nil {
		t.Fatal(err)
	}
	stale := uint64(1)
	command := domain.OperatorCommand{
		SchemaVersion: domain.SchemaVersionV1, ID: "cmd_pause", IdempotencyKey: "idem_pause",
		ActorType: domain.ActorOperator, ActorID: "operator_1", Kind: domain.CommandPauseMission,
		Target: domain.CommandTarget{MissionID: mission.MissionID}, ExpectedRevision: &stale,
		Reason: "stale", SubmittedAt: now,
	}
	if _, err := inbox.SubmitCommand(command); err != nil {
		t.Fatal(err)
	}
	processor, err := kernel.NewCommandProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok, err := processor.ProcessNext(context.Background())
	if err != nil || !ok || receipt.State != domain.CommandRejected || receipt.FailureCode != "CONTROL_CONFLICT" {
		t.Fatalf("stale process = %#v ok=%v err=%v", receipt, ok, err)
	}
}
