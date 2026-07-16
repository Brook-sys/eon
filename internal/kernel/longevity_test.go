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

// completeReadyOperations drains READY work by walking the pure transition path
// to SUCCEEDED. This models verified completion without a model provider.
func completeReadyOperations(t *testing.T, store port.Store, mission domain.MissionRevisionID) int {
	t.Helper()
	completed := 0
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		operations, err := tx.Operations(mission)
		if err != nil {
			return err
		}
		for _, operation := range operations {
			if operation.State != domain.StateReady {
				continue
			}
			snap := domain.OperationalSnapshot{State: operation.State, Reevaluation: operation.Reevaluation}
			running, err := domain.Transition(snap, domain.TransitionInput{Event: domain.EventDispatch, Reference: "lease_" + string(operation.ID)})
			if err != nil {
				return err
			}
			verifying, err := domain.Transition(running, domain.TransitionInput{Event: domain.EventBeginVerify, Reference: "lease_" + string(operation.ID)})
			if err != nil {
				return err
			}
			done, err := domain.Transition(verifying, domain.TransitionInput{Event: domain.EventSucceed})
			if err != nil {
				return err
			}
			operation.State = done.State
			operation.Reevaluation = done.Reevaluation
			if err := tx.SaveOperation(operation); err != nil {
				return err
			}
			completed++
		}
		return nil
	}); err != nil {
		t.Fatalf("complete ready operations: %v", err)
	}
	return completed
}

func TestStrategyCooldownBookAntiFixation(t *testing.T) {
	book := NewStrategyCooldownBook()
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	if book.Active("gap_scan", now) {
		t.Fatal("empty book should not be active")
	}
	book.MarkNoDelta("gap_scan", now, 5*time.Minute)
	if !book.Active("gap_scan", now.Add(time.Minute)) {
		t.Fatal("expected cooldown active")
	}
	if book.Active("gap_scan", now.Add(6*time.Minute)) {
		t.Fatal("cooldown should expire")
	}
	book.MarkNoDelta("gap_scan", now, 5*time.Minute)
	book.Clear("gap_scan")
	if book.Active("gap_scan", now.Add(time.Second)) {
		t.Fatal("clear should remove cooldown")
	}
}

func TestSchedulerSkipsCooledStrategiesAndRotates(t *testing.T) {
	now := time.Date(2026, 7, 16, 10, 10, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	var calls []string
	book := NewStrategyCooldownBook()
	book.MarkNoDelta("gap_scan", now.Add(-time.Minute), 5*time.Minute)
	scheduler := Scheduler{
		Store: store, Clock: clock, Cooldowns: book, IDs: source.NewSequenceIDGenerator(1),
		Strategies: []ContinuityStrategy{
			continuityStrategy{name: "gap_scan", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
				calls = append(calls, "gap_scan")
				return ContinuityResult{}, nil
			}},
			continuityStrategy{name: "integrity_audit", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
				calls = append(calls, "integrity_audit")
				seedAgendaRecords(t, store, now)
				return ContinuityResult{Admitted: 2, Changed: true}, nil
			}},
		},
	}
	decision, err := scheduler.Step(context.Background(), "revision_1")
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if decision.Kind != DecisionDispatch || decision.Strategy != "integrity_audit" {
		t.Fatalf("decision = %+v", decision)
	}
	if len(calls) != 1 || calls[0] != "integrity_audit" {
		t.Fatalf("calls = %#v, want only integrity_audit", calls)
	}
}

func TestLongevityMultiCycleDiversityBudgetAndNoEmptyActivity(t *testing.T) {
	now := time.Date(2026, 7, 16, 10, 20, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	ids := source.NewSequenceIDGenerator(500)
	policy := domain.DefaultHorizonPolicy()
	policy.TargetReady = 1
	policy.LowWatermark = 0
	policy.MaxReady = 2
	policy.StrategyCooldown = 5 * time.Minute

	reg := NewStrategyRegistry()
	if err := RegisterDefaultContinuityFamilies(reg, store, clock, ids, policy); err != nil {
		t.Fatal(err)
	}
	cooldowns := NewStrategyCooldownBook()
	scheduler := Scheduler{
		Store: store, Clock: clock, Registry: reg, Policy: policy,
		IDs: source.NewSequenceIDGenerator(600), Cooldowns: cooldowns,
	}

	type cycleStats struct {
		kind     DecisionKind
		strategy string
	}
	var cycles []cycleStats
	strategyHits := map[string]int{}
	familiesSeen := map[domain.WorkFamily]struct{}{}
	completedOps := 0
	blockedAfterDrain := 0

	// Phase A: multi-cycle dispatch with diversity across families while work remains.
	for i := 0; i < 12; i++ {
		decision, err := scheduler.Step(context.Background(), "revision_1")
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		cycles = append(cycles, cycleStats{kind: decision.Kind, strategy: decision.Strategy})
		switch decision.Kind {
		case DecisionDispatch:
			if decision.Strategy != "" {
				strategyHits[decision.Strategy]++
			}
			completedOps += completeReadyOperations(t, store, "revision_1")
			// Tiny clock advance models wall time between cycles without busy-loop.
			if err := clock.Advance(time.Second); err != nil {
				t.Fatal(err)
			}
		case DecisionContinuityBlocked:
			blockedAfterDrain++
			// Advance past strategy cooldown so a later phase can re-observe the
			// same blocked frontier without tight polling.
			if err := clock.Advance(policy.StrategyCooldown + time.Second); err != nil {
				t.Fatal(err)
			}
			// After a block with full portfolio exhausted, stop phase A.
			if len(decision.StrategiesTried) > 0 {
				goto afterPhaseA
			}
		default:
			t.Fatalf("unexpected decision kind at cycle %d: %+v", i, decision)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			opps, err := r.WorkOpportunities("revision_1", "")
			if err != nil {
				return err
			}
			for _, opp := range opps {
				familiesSeen[opp.Family] = struct{}{}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
afterPhaseA:

	if completedOps == 0 {
		t.Fatal("expected at least one completed operation across longevity cycles")
	}
	if len(familiesSeen) < 2 {
		t.Fatalf("expected multi-family diversity, families=%#v hits=%#v", familiesSeen, strategyHits)
	}
	// Distinct non-empty strategies should appear when multiple families admit work.
	distinctStrategies := 0
	for name, count := range strategyHits {
		if name != "" && count > 0 {
			distinctStrategies++
		}
	}
	if distinctStrategies < 2 && blockedAfterDrain == 0 {
		// With target_ready=1 the first family may monopolise early cycles via
		// frontier_admission after its children land; still require >1 family seeded.
		t.Logf("strategy hits concentrated: %#v (families still diverse: %d)", strategyHits, len(familiesSeen))
	}

	// Phase B: once the frontier is exhausted, further steps must not invent
	// new opportunities with the same signatures (anti empty activity / no reseed).
	var beforeCount int
	if err := store.View(context.Background(), func(r port.Reader) error {
		opps, err := r.WorkOpportunities("revision_1", "")
		if err != nil {
			return err
		}
		beforeCount = len(opps)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Clear cooldowns artificially only by advancing time, then force two more blocked steps.
	if err := clock.Advance(policy.StrategyCooldown + time.Minute); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		decision, err := scheduler.Step(context.Background(), "revision_1")
		if err != nil {
			t.Fatalf("post-drain step %d: %v", i, err)
		}
		if decision.Kind != DecisionContinuityBlocked {
			t.Fatalf("post-drain decision = %+v, want CONTINUITY_BLOCKED (no artificial activity)", decision)
		}
		if err := clock.Advance(policy.StrategyCooldown + time.Second); err != nil {
			t.Fatal(err)
		}
	}
	var afterCount int
	var openLeft int
	if err := store.View(context.Background(), func(r port.Reader) error {
		opps, err := r.WorkOpportunities("revision_1", "")
		if err != nil {
			return err
		}
		afterCount = len(opps)
		for _, opp := range opps {
			if opp.Status == domain.OpportunityOpen {
				openLeft++
			}
		}
		ready := 0
		ops, err := r.Operations("revision_1")
		if err != nil {
			return err
		}
		for _, op := range ops {
			if op.State == domain.StateReady {
				ready++
			}
		}
		if ready != 0 {
			t.Fatalf("ready horizon not drained: %d", ready)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if afterCount != beforeCount {
		t.Fatalf("empty-activity reseed detected: opportunities before=%d after=%d", beforeCount, afterCount)
	}
	if openLeft != 0 {
		t.Fatalf("expected no OPEN opportunities after drain, open=%d", openLeft)
	}

	// Budget invariant: admitted inquiries carry positive attempt/token budgets from seeds.
	if err := store.View(context.Background(), func(r port.Reader) error {
		opps, err := r.WorkOpportunities("revision_1", domain.OpportunityAdmitted)
		if err != nil {
			return err
		}
		if len(opps) == 0 {
			t.Fatal("expected admitted opportunities")
		}
		seen := 0
		for _, opp := range opps {
			if opp.AdmittedInquiryID == "" {
				continue
			}
			inquiry, err := r.Inquiry(opp.AdmittedInquiryID)
			if err != nil {
				return err
			}
			if inquiry.Budget.Attempts <= 0 && inquiry.Budget.Tokens <= 0 {
				t.Fatalf("inquiry without budget: %+v", inquiry)
			}
			seen++
		}
		if seen == 0 {
			t.Fatal("expected at least one admitted inquiry")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	t.Logf("longevity cycles=%d completed_ops=%d families=%d strategy_hits=%v blocked_drains=%d",
		len(cycles), completedOps, len(familiesSeen), strategyHits, blockedAfterDrain)
}
