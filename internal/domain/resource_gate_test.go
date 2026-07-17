package domain

import (
	"testing"
	"time"
)

func TestAcquireConcurrencyAndQuota(t *testing.T) {
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	limit := ResourceLimit{
		Resource:            "model:local",
		MaxConcurrent:       2,
		MaxPerMinute:        3,
		MaxPerDay:           10,
		MaxTokensPerMinute:  1000,
		FailureThreshold:    3,
		CooldownBase:        time.Second,
		CooldownMax:         time.Minute,
		ReservedForCritical: 1,
	}
	usage := ResourceUsage{Resource: "model:local"}
	cost := ResourceCost{Slots: 1, Calls: 1, Tokens: 100}

	// Normal priority cannot use reserved slot when one in flight.
	usage.InFlight = 1
	res, err := Acquire(limit, usage, cost, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed {
		t.Fatalf("normal priority should not use reserved slot: %+v", res)
	}
	if res.FailureCode != "RESOURCE_CONCURRENCY" {
		t.Fatalf("code = %s", res.FailureCode)
	}

	// Critical may use reserved.
	res, err = Acquire(limit, usage, cost, PriorityCritical, now)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Allowed || res.Permit == nil || res.Usage.InFlight != 2 {
		t.Fatalf("critical grant = %+v", res)
	}

	// Minute quota
	usage = ResourceUsage{Resource: "model:local", MinuteWindowStart: now.Truncate(time.Minute), MinuteCount: 3}
	res, err = Acquire(limit, usage, cost, PriorityCritical, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed || res.WaitUntil == nil || res.FailureCode != "RESOURCE_RATE_LIMIT" {
		t.Fatalf("minute quota = %+v", res)
	}

	// Day quota
	usage = ResourceUsage{
		Resource:          "model:local",
		MinuteWindowStart: now.Truncate(time.Minute),
		DayWindowStart:    time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC),
		DayCount:          10,
	}
	res, err = Acquire(limit, usage, cost, PriorityCritical, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed || res.FailureCode != "RESOURCE_DAILY_QUOTA" {
		t.Fatalf("day quota = %+v", res)
	}
}

func TestAcquireCircuitAndReport(t *testing.T) {
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	limit := ResourceLimit{
		Resource:         "web:searxng",
		MaxConcurrent:    4,
		MaxPerMinute:     60,
		FailureThreshold: 2,
		CooldownBase:     2 * time.Second,
		CooldownMax:      30 * time.Second,
	}
	usage := ResourceUsage{Resource: "web:searxng"}
	cost := ResourceCost{Slots: 1, Calls: 1}

	// First failure does not open circuit (threshold 2).
	usage.InFlight = 1
	next, err := ReportFailure(usage, limit, cost, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if next.InFlight != 0 || next.ConsecutiveFailures != 1 || next.CircuitOpenUntil != nil {
		t.Fatalf("first failure = %+v", next)
	}

	// Second failure opens circuit.
	next.InFlight = 1
	next, err = ReportFailure(next, limit, cost, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if next.CircuitOpenUntil == nil {
		t.Fatal("expected circuit open")
	}
	openUntil := *next.CircuitOpenUntil

	res, err := Acquire(limit, next, cost, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed || res.FailureCode != "DEPENDENCY_CIRCUIT_OPEN" {
		t.Fatalf("circuit deny = %+v", res)
	}

	// Retry-After wins if later.
	later := now.Add(time.Minute)
	usage = ResourceUsage{Resource: "web:searxng", InFlight: 1, ConsecutiveFailures: 1}
	next, err = ReportFailure(usage, limit, cost, &later, now)
	if err != nil {
		t.Fatal(err)
	}
	if next.CircuitOpenUntil == nil || !next.CircuitOpenUntil.Equal(later.UTC()) {
		t.Fatalf("retry-after = %v want %v", next.CircuitOpenUntil, later.UTC())
	}
	_ = openUntil

	// Success clears streak.
	usage = ResourceUsage{Resource: "web:searxng", InFlight: 1, ConsecutiveFailures: 5}
	ok, err := ReportSuccess(usage, cost, now)
	if err != nil {
		t.Fatal(err)
	}
	if ok.InFlight != 0 || ok.ConsecutiveFailures != 0 {
		t.Fatalf("success = %+v", ok)
	}
}

func TestThrottleTransitionInput(t *testing.T) {
	until := time.Date(2026, 7, 16, 15, 1, 0, 0, time.UTC)
	withWait := ResourceAcquireResult{
		Allowed:     false,
		WaitUntil:   &until,
		FailureCode: "RESOURCE_RATE_LIMIT",
		Reason:      "per-minute quota exhausted",
	}
	in, err := ThrottleTransitionInput(withWait, "model:local")
	if err != nil {
		t.Fatal(err)
	}
	if in.Event != EventWaitUntil || in.NotBefore == nil || !in.NotBefore.Equal(until) {
		t.Fatalf("wait_until input = %+v", in)
	}

	noWait := ResourceAcquireResult{
		Allowed:     false,
		FailureCode: "RESOURCE_CONCURRENCY",
		Reason:      "concurrency saturated",
	}
	in, err = ThrottleTransitionInput(noWait, "model:local")
	if err != nil {
		t.Fatal(err)
	}
	if in.Event != EventThrottle || in.Reference != "model:local:RESOURCE_CONCURRENCY" {
		t.Fatalf("throttle input = %+v", in)
	}

	// Apply through pure Transition.
	from := snapshotFor(StateRunning, "lease_1")
	snap, err := Transition(from, in)
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != StateThrottled || snap.Reevaluation.Kind != ReevaluateCapacity {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestNewResourceBudgetFailure(t *testing.T) {
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	op := Operation{
		SchemaVersion:   SchemaVersionV1,
		ID:              "op_1",
		InquiryID:       "inq_1",
		MissionRevision: "mr_1",
		SpecID:          "spec_1",
		ExpectedOutput:  "x",
		IdempotencyKey:  "idem_1",
		State:           StateRunning,
		Reevaluation:    ReevaluationCondition{Kind: ReevaluateLease, Reference: "lease_1"},
	}
	retryAt := now.Add(time.Minute)
	rec, err := NewResourceBudgetFailure(
		"fail_1", op, 1, "RESOURCE_RATE_LIMIT", FailureResource, RetryAfter, &retryAt,
		"per-minute quota exhausted", "resource-gate@1", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Class != FailureResource || rec.RetryAt == nil {
		t.Fatalf("rec = %+v", rec)
	}
}

func TestWindowRoll(t *testing.T) {
	now := time.Date(2026, 7, 16, 15, 1, 0, 0, time.UTC)
	limit := ResourceLimit{Resource: "r", MaxPerMinute: 2}
	// Old minute window should reset.
	usage := ResourceUsage{
		Resource:          "r",
		MinuteWindowStart: now.Add(-2 * time.Minute),
		MinuteCount:       2,
	}
	res, err := Acquire(limit, usage, ResourceCost{Calls: 1}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Allowed || res.Usage.MinuteCount != 1 {
		t.Fatalf("rolled window = %+v", res)
	}
}
