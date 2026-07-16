package kernel

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func missionWithRecurring(t *testing.T, store port.Store, now time.Time, obligations []domain.RecurringObligation) domain.MissionRevision {
	t.Helper()
	revision := domain.MissionRevision{
		SchemaVersion:        domain.SchemaVersionV1,
		ID:                   "revision_1",
		MissionID:            "mission_1",
		Revision:             1,
		OriginalText:         "Investigate epistemic runtimes.",
		Purpose:              "Build cited knowledge",
		Domains:              []string{"runtime", "knowledge"},
		Policies:             []string{"cite", "no_model_authority"},
		Budget:               domain.Budget{ModelCalls: 10, Tokens: 8000, Bytes: 65536, Attempts: 3, Duration: time.Hour},
		Status:               domain.MissionActive,
		RecurringObligations: append([]domain.RecurringObligation(nil), obligations...),
		Provenance:           "test",
		AcceptedAt:           now,
	}
	if err := revision.Validate(); err != nil {
		t.Fatalf("mission validate: %v", err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		return tx.ActivateMissionRevision(revision.MissionID, revision.ID)
	}); err != nil {
		t.Fatalf("install mission: %v", err)
	}
	return revision
}

func TestRecurringSeederCadenceAntiRepetitionAndDelta(t *testing.T) {
	// Virtual clock at a fixed cadence boundary (FR-DUR-011 acceptance).
	start := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(start)
	store := memory.New()
	ids := source.NewSequenceIDGenerator(1)
	policy := domain.DefaultHorizonPolicy()
	policy.TargetReady = 2
	policy.LowWatermark = 0
	policy.MaxReady = 8
	policy.MaxCandidates = 32

	ob := domain.RecurringObligation{
		SchemaVersion:  domain.SchemaVersionV1,
		ID:             "harness_hourly",
		Kind:           domain.RecurringKindHarness,
		Title:          "offline harness evaluation",
		Cadence:        time.Hour,
		Budget:         domain.Budget{Tokens: 32, Attempts: 1},
		DeltaCriterion: "new offline compile report or fixture change",
		AntiRepetition: domain.AntiRepRequireStateChange,
		MaxPerWindow:   1,
		Priority:       18,
		Enabled:        true,
		Objective:      "keep cognitive-v1 compile green",
	}
	revision := missionWithRecurring(t, store, start, []domain.RecurringObligation{ob})

	seeder := RecurringSeeder{Store: store, Clock: clock, IDs: ids, Policy: policy}
	result, err := seeder.SeedDue(context.Background(), revision.ID)
	if err != nil {
		t.Fatalf("seed due: %v", err)
	}
	if !result.Changed || result.Admitted < 1 {
		t.Fatalf("expected seed+admit, got %+v", result)
	}

	var firstSig string
	var firstOrigin string
	if err := store.View(context.Background(), func(r port.Reader) error {
		items, err := r.WorkOpportunities(revision.ID, "")
		if err != nil {
			return err
		}
		if len(items) != 1 {
			t.Fatalf("expected one opportunity, got %#v", items)
		}
		firstSig = items[0].DedupSignature
		firstOrigin = items[0].Origin
		if items[0].Family != domain.FamilyHarnessEvaluation {
			t.Fatalf("family = %s", items[0].Family)
		}
		if items[0].Origin != "recurring:harness_hourly" {
			t.Fatalf("origin = %s", items[0].Origin)
		}
		ops, err := r.Operations(revision.ID)
		if err != nil {
			return err
		}
		if len(ops) < 1 || ops[0].State != domain.StateReady {
			t.Fatalf("operations after admit = %#v", ops)
		}
		events, err := r.Events(0, 50)
		if err != nil {
			return err
		}
		found := false
		for _, event := range events {
			if event.Kind == domain.EventContinuityRecurringSeeded {
				found = true
			}
		}
		if !found {
			t.Fatal("missing continuity.recurring_seeded event")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Same cadence window, no state fingerprint change: no duplicate (anti-repetition).
	if err := clock.Advance(15 * time.Minute); err != nil {
		t.Fatal(err)
	}
	result, err = seeder.SeedDue(context.Background(), revision.ID)
	if err != nil {
		t.Fatalf("reseed mid-window: %v", err)
	}
	if result.Changed || result.Admitted != 0 {
		t.Fatalf("expected no mid-window seed without delta, got %+v", result)
	}

	// Mid-window state fingerprint (head commit) enables one delta seed.
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return seedHeadCommit(tx, revision.ID, "commit_head_2", clock.Now().UTC())
	}); err != nil {
		t.Fatalf("install head: %v", err)
	}

	result, err = seeder.SeedDue(context.Background(), revision.ID)
	if err != nil {
		t.Fatalf("delta seed: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected state_delta seed, got %+v", result)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		items, err := r.WorkOpportunities(revision.ID, "")
		if err != nil {
			return err
		}
		if len(items) < 2 {
			t.Fatalf("expected cadence + delta opportunities, got %#v", items)
		}
		foundDelta := false
		for _, item := range items {
			if item.DedupSignature != firstSig && item.Origin == firstOrigin {
				foundDelta = true
				if item.Family != domain.FamilyHarnessEvaluation {
					t.Fatalf("delta family = %s", item.Family)
				}
			}
		}
		if !foundDelta {
			t.Fatalf("missing delta seed among %#v", items)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Same fingerprint again: no empty reseed.
	if err := clock.Advance(5 * time.Minute); err != nil {
		t.Fatal(err)
	}
	result, err = seeder.SeedDue(context.Background(), revision.ID)
	if err != nil {
		t.Fatalf("same fp reseed: %v", err)
	}
	if result.Changed {
		t.Fatalf("expected no reseed for same fingerprint, got %+v", result)
	}

	// Next cadence bucket: limited recurrence even without further fingerprint.
	if err := clock.Advance(time.Hour); err != nil {
		t.Fatal(err)
	}
	result, err = seeder.SeedDue(context.Background(), revision.ID)
	if err != nil {
		t.Fatalf("next period: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected next cadence seed, got %+v", result)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		items, err := r.WorkOpportunities(revision.ID, "")
		if err != nil {
			return err
		}
		if len(items) < 3 {
			t.Fatalf("expected at least 3 opportunities across periods, got %#v", items)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRecurringStrategyIdempotentAndRegisteredInDefaults(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	ids := source.NewSequenceIDGenerator(50)
	policy := domain.DefaultHorizonPolicy()

	reg := NewStrategyRegistry()
	if err := EnsureRecurringStrategy(reg, store, clock, ids, policy); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRecurringStrategy(reg, store, clock, ids, policy); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if reg.Len() != 1 {
		t.Fatalf("len = %d", reg.Len())
	}
	d, ok := reg.Descriptor("recurring_obligations")
	if !ok || d.Priority != 40 {
		t.Fatalf("descriptor = %+v ok=%v", d, ok)
	}

	reg2 := NewStrategyRegistry()
	if err := RegisterDefaultContinuityFamilies(reg2, store, clock, ids, policy); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg2.Descriptor("recurring_obligations"); !ok {
		t.Fatal("defaults must include recurring_obligations")
	}
	// Recurring must sort before opportunistic families by priority.
	names := reg2.Strategies()
	if len(names) == 0 || names[0].Name() != "recurring_obligations" {
		got := ""
		if len(names) > 0 {
			got = names[0].Name()
		}
		t.Fatalf("first strategy = %q, want recurring_obligations", got)
	}
}

func TestRecurringSeederNoWorkWithoutObligations(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	seeder := RecurringSeeder{
		Store: store, Clock: clock, IDs: source.NewSequenceIDGenerator(1), Policy: domain.DefaultHorizonPolicy(),
	}
	result, err := seeder.SeedDue(context.Background(), "revision_1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || result.Admitted != 0 {
		t.Fatalf("empty obligations must not invent work: %+v", result)
	}
}
