package domain

import (
	"errors"
	"testing"
	"time"
)

func TestApplyOperatorCommandPauseResumeCancelAndShutdown(t *testing.T) {
	now := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	revision := uint64(3)
	active := MissionRevision{
		SchemaVersion: SchemaVersionV1, ID: "revision_3", MissionID: "mission_1", Revision: revision,
		OriginalText: "learn", Purpose: "learn", Status: MissionActive, Provenance: "fixture", AcceptedAt: now,
	}
	state := DefaultControlState(now)
	pause := OperatorCommand{
		SchemaVersion: SchemaVersionV1, ID: "cmd_pause", IdempotencyKey: "idem_pause",
		ActorType: ActorOperator, ActorID: "operator_1", Kind: CommandPauseMission,
		Target: CommandTarget{MissionID: "mission_1"}, ExpectedRevision: &revision,
		Reason: "maintenance", SubmittedAt: now,
	}
	paused, ref, err := ApplyOperatorCommand(state, pause, active, now)
	if err != nil || ref != "mission_1@3:PAUSED" || paused.Missions["mission_1"].Mode != MissionDispatchPaused {
		t.Fatalf("pause = %#v ref=%s err=%v", paused, ref, err)
	}
	if paused.AllowsDispatch("mission_1") {
		t.Fatal("paused mission still allows dispatch")
	}
	resume := pause
	resume.ID, resume.IdempotencyKey, resume.Kind, resume.Reason = "cmd_resume", "idem_resume", CommandResumeMission, "done"
	resumed, ref, err := ApplyOperatorCommand(paused, resume, active, now.Add(time.Minute))
	if err != nil || ref != "mission_1@3:ENABLED" || !resumed.AllowsDispatch("mission_1") {
		t.Fatalf("resume = %#v ref=%s err=%v", resumed, ref, err)
	}
	cancel := pause
	cancel.ID, cancel.IdempotencyKey, cancel.Kind = "cmd_cancel", "idem_cancel", CommandCancelMission
	cancelled, ref, err := ApplyOperatorCommand(resumed, cancel, active, now.Add(2*time.Minute))
	if err != nil || ref != "mission_1@3:CANCELLED" || cancelled.AllowsDispatch("mission_1") {
		t.Fatalf("cancel = %#v ref=%s err=%v", cancelled, ref, err)
	}
	shutdown := OperatorCommand{
		SchemaVersion: SchemaVersionV1, ID: "cmd_stop", IdempotencyKey: "idem_stop",
		ActorType: ActorOperator, ActorID: "operator_1", Kind: CommandGracefulShutdown,
		Reason: "deploy", SubmittedAt: now,
	}
	stopping, ref, err := ApplyOperatorCommand(state, shutdown, MissionRevision{}, now)
	if err != nil || ref != "process:stopping" || stopping.ProcessMode != ProcessStopping || stopping.AllowsDispatch("mission_1") {
		t.Fatalf("shutdown = %#v ref=%s err=%v", stopping, ref, err)
	}
}

func TestApplyOperatorCommandRejectsStaleRevisionAndIllegalResume(t *testing.T) {
	now := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	revision := uint64(2)
	active := MissionRevision{
		SchemaVersion: SchemaVersionV1, ID: "revision_2", MissionID: "mission_1", Revision: 3,
		OriginalText: "learn", Purpose: "learn", Status: MissionActive, Provenance: "fixture", AcceptedAt: now,
	}
	pause := OperatorCommand{
		SchemaVersion: SchemaVersionV1, ID: "cmd_pause", IdempotencyKey: "idem_pause",
		ActorType: ActorOperator, ActorID: "operator_1", Kind: CommandPauseMission,
		Target: CommandTarget{MissionID: "mission_1"}, ExpectedRevision: &revision,
		Reason: "maintenance", SubmittedAt: now,
	}
	if _, _, err := ApplyOperatorCommand(DefaultControlState(now), pause, active, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision error = %v", err)
	}
	active.Revision = 2
	paused, _, err := ApplyOperatorCommand(DefaultControlState(now), pause, active, now)
	if err != nil {
		t.Fatal(err)
	}
	resume := pause
	resume.ID, resume.IdempotencyKey, resume.Kind = "cmd_resume", "idem_resume", CommandResumeMission
	stale := active
	stale.Revision = 9
	expected := uint64(9)
	resume.ExpectedRevision = &expected
	if _, _, err := ApplyOperatorCommand(paused, resume, stale, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("resume without pause match error = %v", err)
	}
}

func TestAdvanceCommandReceiptIsMonotonic(t *testing.T) {
	now := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	received := CommandReceipt{SchemaVersion: SchemaVersionV1, ID: "receipt_1", CommandID: "cmd_1", State: CommandReceived, RecordedAt: now}
	validating := received
	validating.State, validating.RecordedAt = CommandValidating, now.Add(time.Second)
	if err := AdvanceCommandReceipt(received, validating); err != nil {
		t.Fatal(err)
	}
	applied := validating
	applied.State, applied.ResultRef = CommandApplied, "mission_1@1:PAUSED"
	if err := AdvanceCommandReceipt(validating, applied); err == nil {
		t.Fatal("skipped acceptance path")
	}
	accepted := validating
	accepted.State = CommandAccepted
	if err := AdvanceCommandReceipt(validating, accepted); err != nil {
		t.Fatal(err)
	}
	applying := accepted
	applying.State, applying.RecordedAt = CommandApplying, now.Add(2*time.Second)
	if err := AdvanceCommandReceipt(accepted, applying); err != nil {
		t.Fatal(err)
	}
	done := applying
	done.State, done.ResultRef, done.RecordedAt = CommandApplied, "mission_1@1:PAUSED", now.Add(3*time.Second)
	if err := AdvanceCommandReceipt(applying, done); err != nil {
		t.Fatal(err)
	}
	regress := done
	regress.State, regress.ResultRef, regress.RecordedAt = CommandApplying, "", now.Add(4*time.Second)
	if err := AdvanceCommandReceipt(done, regress); !errors.Is(err, ErrConflict) {
		t.Fatalf("terminal advance error = %v", err)
	}
}
