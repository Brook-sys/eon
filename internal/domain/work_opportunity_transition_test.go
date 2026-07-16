package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func baseWorkOpportunity(status WorkOpportunityStatus) WorkOpportunity {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	return WorkOpportunity{
		SchemaVersion:   SchemaVersionV1,
		ID:              "opp_1",
		MissionRevision: "revision_1",
		Family:          FamilyGapScan,
		Status:          status,
		Title:           "cover residual gaps",
		Origin:          "test",
		ExpectedGain:    "new inquiries",
		Novelty:         "uncovered scopes",
		StopCondition:   "coverage target",
		DedupSignature:  "gap:root",
		Depth:           0,
		EstimatedCost:   Budget{Tokens: 10, Attempts: 1},
		Risk:            RiskLow,
		Priority:        10,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func TestTransitionWorkOpportunityLifecycle(t *testing.T) {
	base := baseWorkOpportunity(OpportunityOpen)
	later := base.CreatedAt.Add(time.Minute)
	tests := []struct {
		name       string
		current    WorkOpportunity
		transition WorkOpportunityTransition
		wantStatus WorkOpportunityStatus
		wantReason string
		wantError  bool
	}{
		{
			name:       "defer open",
			current:    base,
			transition: WorkOpportunityTransition{Event: OppEventDefer, OccurredAt: later, Reason: "park for later"},
			wantStatus: OpportunityDeferred,
			wantReason: "park for later",
		},
		{
			name: "reopen deferred",
			current: func() WorkOpportunity {
				o := base
				o.Status = OpportunityDeferred
				o.AbandonReason = "parked"
				return o
			}(),
			transition: WorkOpportunityTransition{Event: OppEventReopen, OccurredAt: later},
			wantStatus: OpportunityOpen,
			wantReason: "",
		},
		{
			name:       "abandon open",
			current:    base,
			transition: WorkOpportunityTransition{Event: OppEventAbandon, OccurredAt: later, Reason: "no longer relevant"},
			wantStatus: OpportunityAbandoned,
			wantReason: "no longer relevant",
		},
		{
			name: "abandon deferred",
			current: func() WorkOpportunity {
				o := base
				o.Status = OpportunityDeferred
				return o
			}(),
			transition: WorkOpportunityTransition{Event: OppEventAbandon, OccurredAt: later, Reason: "stale"},
			wantStatus: OpportunityAbandoned,
			wantReason: "stale",
		},
		{
			name:       "supersede open",
			current:    base,
			transition: WorkOpportunityTransition{Event: OppEventSupersede, OccurredAt: later, SupersededBy: "opp_2", Reason: "better unit"},
			wantStatus: OpportunitySuperseded,
			wantReason: "superseded_by:opp_2 better unit",
		},
		{
			name:       "defer non-open",
			current:    baseWorkOpportunity(OpportunityDeferred),
			transition: WorkOpportunityTransition{Event: OppEventDefer, OccurredAt: later},
			wantError:  true,
		},
		{
			name:       "reopen non-deferred",
			current:    base,
			transition: WorkOpportunityTransition{Event: OppEventReopen, OccurredAt: later},
			wantError:  true,
		},
		{
			name:       "abandon without reason",
			current:    base,
			transition: WorkOpportunityTransition{Event: OppEventAbandon, OccurredAt: later},
			wantError:  true,
		},
		{
			name:       "supersede without successor",
			current:    base,
			transition: WorkOpportunityTransition{Event: OppEventSupersede, OccurredAt: later},
			wantError:  true,
		},
		{
			name:       "supersede self",
			current:    base,
			transition: WorkOpportunityTransition{Event: OppEventSupersede, OccurredAt: later, SupersededBy: base.ID},
			wantError:  true,
		},
		{
			name: "terminal admitted",
			current: func() WorkOpportunity {
				o := base
				o.Status = OpportunityAdmitted
				o.AdmittedInquiryID = "inq_1"
				return o
			}(),
			transition: WorkOpportunityTransition{Event: OppEventAbandon, OccurredAt: later, Reason: "nope"},
			wantError:  true,
		},
		{
			name:       "terminal abandoned",
			current:    func() WorkOpportunity { o := base; o.Status = OpportunityAbandoned; o.AbandonReason = "done"; return o }(),
			transition: WorkOpportunityTransition{Event: OppEventReopen, OccurredAt: later},
			wantError:  true,
		},
		{
			name:       "zero time",
			current:    base,
			transition: WorkOpportunityTransition{Event: OppEventDefer},
			wantError:  true,
		},
		{
			name:       "before creation",
			current:    base,
			transition: WorkOpportunityTransition{Event: OppEventDefer, OccurredAt: base.CreatedAt.Add(-time.Second)},
			wantError:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := test.current
			before.Dependencies = append([]string(nil), test.current.Dependencies...)
			next, err := TransitionWorkOpportunity(test.current, test.transition)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %v", err, test.wantError)
			}
			if !reflect.DeepEqual(test.current, before) {
				t.Fatal("transition mutated input")
			}
			if test.wantError {
				return
			}
			if next.Status != test.wantStatus {
				t.Fatalf("status = %s, want %s", next.Status, test.wantStatus)
			}
			if next.AbandonReason != test.wantReason {
				t.Fatalf("reason = %q, want %q", next.AbandonReason, test.wantReason)
			}
			if !next.UpdatedAt.Equal(test.transition.OccurredAt.UTC()) {
				t.Fatalf("updated_at = %s, want %s", next.UpdatedAt, test.transition.OccurredAt.UTC())
			}
		})
	}
}

func TestPlanFrontierHygieneDefersExcessAndAbandonsDeep(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 30, 0, 0, time.UTC)
	policy := DefaultHorizonPolicy()
	policy.MaxCandidates = 2
	policy.MaxDepth = 1

	mk := func(id string, priority uint8, depth int, status WorkOpportunityStatus) WorkOpportunity {
		o := baseWorkOpportunity(status)
		o.ID = WorkOpportunityID(id)
		o.Priority = priority
		o.Depth = depth
		if depth > 0 {
			o.ParentID = "opp_parent"
		}
		o.DedupSignature = "sig:" + id
		o.Title = id
		return o
	}
	open := []WorkOpportunity{
		mk("opp_low", 1, 0, OpportunityOpen),
		mk("opp_mid", 5, 0, OpportunityOpen),
		mk("opp_high", 9, 0, OpportunityOpen),
		mk("opp_deep", 8, 2, OpportunityOpen),
	}
	actions, err := PlanFrontierHygiene(open, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	// 1 abandon (depth) + 1 defer (3 remaining open > MaxCandidates 2).
	if len(actions) != 2 {
		t.Fatalf("actions = %+v", actions)
	}
	if actions[0].OpportunityID != "opp_deep" || actions[0].Transition.Event != OppEventAbandon {
		t.Fatalf("first action = %+v", actions[0])
	}
	if !strings.Contains(actions[0].Transition.Reason, "depth_exceeds_policy") {
		t.Fatalf("abandon reason = %q", actions[0].Transition.Reason)
	}
	if actions[1].OpportunityID != "opp_low" || actions[1].Transition.Event != OppEventDefer {
		t.Fatalf("second action = %+v", actions[1])
	}
	if !strings.Contains(actions[1].Transition.Reason, "max_candidates") {
		t.Fatalf("defer reason = %q", actions[1].Transition.Reason)
	}
}

func TestPlanFrontierHygieneNoopWhenWithinLimits(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 30, 0, 0, time.UTC)
	policy := DefaultHorizonPolicy()
	open := []WorkOpportunity{baseWorkOpportunity(OpportunityOpen)}
	actions, err := PlanFrontierHygiene(open, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 0 {
		t.Fatalf("expected no actions, got %+v", actions)
	}
}
