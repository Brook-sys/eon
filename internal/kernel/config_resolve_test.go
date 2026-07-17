package kernel

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestActivePoliciesFallBackToDefaults(t *testing.T) {
	store := memory.New()
	horizon, err := ActiveHorizonPolicy(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	defH := domain.DefaultHorizonPolicy()
	if horizon.Version != defH.Version || horizon.TargetReady != defH.TargetReady {
		t.Fatalf("horizon default = %#v", horizon)
	}
	gate, version, err := ActiveQuestionGatePolicy(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	def := domain.DefaultInterruptionRuntimePolicy()
	if version != def.Version || gate.MaxPending != def.MaxPending || gate.MinPriority != def.MinPriority {
		t.Fatalf("gate default = %#v version=%s", gate, version)
	}
}

func TestActivePoliciesPreferAppliedRevisions(t *testing.T) {
	store := memory.New()
	clock := source.NewManualClock(time.Date(2026, 7, 16, 17, 0, 0, 0, time.UTC))
	ids := source.NewSequenceIDGenerator(1)
	applier, err := NewConfigApplier(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}

	horizonPolicy := domain.DefaultHorizonPolicy()
	horizonPolicy.TargetReady = 2
	horizonPolicy.LowWatermark = 1
	horizonPolicy.MaxReady = 4
	horizonDraft := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_hz_1", Scope: domain.ConfigScopeHorizon,
		Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen,
		ActorType: domain.ActorOperator, ActorID: "op", Reason: "tighten horizon",
		Horizon: &horizonPolicy, CreatedAt: clock.Now(),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(horizonDraft)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := applier.ValidateDraft(context.Background(), horizonDraft.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if _, _, err := applier.ApplyDraft(context.Background(), horizonDraft.ID); err != nil {
		t.Fatal(err)
	}

	interruption := domain.DefaultInterruptionRuntimePolicy()
	interruption.MaxPending = 1
	interruption.Version = "interruption.tight.v1"
	intDraft := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_int_1", Scope: domain.ConfigScopeInterruption,
		Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen,
		ActorType: domain.ActorOperator, ActorID: "op", Reason: "tighten gate",
		Interruption: &interruption, CreatedAt: clock.Now(),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(intDraft)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := applier.ValidateDraft(context.Background(), intDraft.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if _, _, err := applier.ApplyDraft(context.Background(), intDraft.ID); err != nil {
		t.Fatal(err)
	}

	gotHorizon, err := ActiveHorizonPolicy(context.Background(), store)
	if err != nil || gotHorizon.TargetReady != 2 || gotHorizon.LowWatermark != 1 {
		t.Fatalf("active horizon = %#v err=%v", gotHorizon, err)
	}
	gate, version, err := ActiveQuestionGatePolicy(context.Background(), store)
	if err != nil || gate.MaxPending != 1 || version != "interruption.tight.v1" {
		t.Fatalf("active gate = %#v version=%s err=%v", gate, version, err)
	}

	// Explicit non-zero policy still wins for ResolveHorizonPolicy.
	explicit := domain.DefaultHorizonPolicy()
	explicit.TargetReady = 9
	explicit.MaxReady = 12
	resolved, err := ResolveHorizonPolicy(context.Background(), store, explicit)
	if err != nil || resolved.TargetReady != 9 {
		t.Fatalf("explicit resolve = %#v err=%v", resolved, err)
	}
}

func TestSchedulerConsumesActiveHorizonRevision(t *testing.T) {
	store := memory.New()
	clock := source.NewManualClock(time.Date(2026, 7, 16, 17, 30, 0, 0, time.UTC))
	ids := source.NewSequenceIDGenerator(1)
	seedMission(t, store)

	policy := domain.DefaultHorizonPolicy()
	policy.TargetReady = 1
	policy.LowWatermark = 0
	policy.MaxReady = 2
	policy.Version = "horizon.scheduler.v1"
	applier, err := NewConfigApplier(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	draft := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_hz_sched", Scope: domain.ConfigScopeHorizon,
		Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen,
		ActorType: domain.ActorOperator, ActorID: "op", Reason: "scheduler horizon",
		Horizon: &policy, CreatedAt: clock.Now(),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(draft)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := applier.ValidateDraft(context.Background(), draft.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if _, _, err := applier.ApplyDraft(context.Background(), draft.ID); err != nil {
		t.Fatal(err)
	}

	scheduler := Scheduler{Store: store, Clock: clock, IDs: ids, Strategies: nil}
	decision, err := scheduler.Step(context.Background(), "revision_1")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Horizon.TargetReady != 1 || decision.Horizon.LowWatermark != 0 {
		t.Fatalf("scheduler horizon projection = %#v", decision.Horizon)
	}
}

func TestActiveSchedulerCadenceFallbackAndRevision(t *testing.T) {
	store := memory.New()
	got, err := ActiveSchedulerCadence(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	def := domain.DefaultSchedulerCadenceConfig()
	if got.Version != def.Version || got.MaxDispatches != def.MaxDispatches {
		t.Fatalf("cadence default = %#v", got)
	}

	clock := source.NewManualClock(time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC))
	ids := source.NewSequenceIDGenerator(1)
	applier, err := NewConfigApplier(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	tight := domain.DefaultSchedulerCadenceConfig()
	tight.Version = "scheduler.tight.v1"
	tight.MaxDispatches = 2
	tight.MaxCycleDuration = 5 * time.Second
	draft := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_sched_1", Scope: domain.ConfigScopeScheduler,
		Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen,
		ActorType: domain.ActorOperator, ActorID: "op", Reason: "tighten cadence",
		Scheduler: &tight, CreatedAt: clock.Now(),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(draft)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := applier.ValidateDraft(context.Background(), draft.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if _, _, err := applier.ApplyDraft(context.Background(), draft.ID); err != nil {
		t.Fatal(err)
	}
	active, err := ActiveSchedulerCadence(context.Background(), store)
	if err != nil || active.Version != "scheduler.tight.v1" || active.MaxDispatches != 2 {
		t.Fatalf("active cadence = %#v err=%v", active, err)
	}
	explicit := domain.DefaultSchedulerCadenceConfig()
	explicit.Version = "scheduler.explicit.v1"
	explicit.MaxDispatches = 3
	resolved, err := ResolveSchedulerCadence(context.Background(), store, explicit)
	if err != nil || resolved.MaxDispatches != 3 || resolved.Version != "scheduler.explicit.v1" {
		t.Fatalf("explicit resolve = %#v err=%v", resolved, err)
	}
}

func TestQuestionGateProcessorUsesActiveInterruptionPolicy(t *testing.T) {
	store := memory.New()
	clock := source.NewManualClock(time.Date(2026, 7, 16, 18, 0, 0, 0, time.UTC))
	ids := source.NewSequenceIDGenerator(1)
	now := clock.Now()
	installGateMission(t, store, now)

	policy := domain.DefaultInterruptionRuntimePolicy()
	policy.MinPriority = 80
	policy.UrgentPriority = 90
	policy.Version = "interruption.highmin.v1"
	applier, err := NewConfigApplier(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	draft := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_gate", Scope: domain.ConfigScopeInterruption,
		Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen,
		ActorType: domain.ActorOperator, ActorID: "op", Reason: "raise min",
		Interruption: &policy, CreatedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(draft)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := applier.ValidateDraft(context.Background(), draft.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if _, _, err := applier.ApplyDraft(context.Background(), draft.ID); err != nil {
		t.Fatal(err)
	}

	gate, err := NewActiveQuestionGateProcessor(store, clock, ids, nil)
	if err != nil {
		t.Fatal(err)
	}
	low := gateProposal(clock.Now())
	low.Question.Priority = 50
	decision, err := gate.Process(context.Background(), low)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != domain.PersistedQuestionSuppress || decision.Reason != domain.PersistedQuestionGatePriorityLow {
		t.Fatalf("low priority decision = %#v", decision)
	}
	if decision.PolicyVersion != "interruption.highmin.v1" {
		t.Fatalf("policy version = %s", decision.PolicyVersion)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		if _, err := r.OperatorQuestion(low.Question.ID); !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("suppressed question should not exist: err=%v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
