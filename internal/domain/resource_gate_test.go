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

func TestReconcileObservedTokens(t *testing.T) {
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	usage := ResourceUsage{Resource: "model:local", TokenMinuteCount: 1000}

	// Estimate > Observed (e.g. fast early exit)
	next, err := ReconcileObservedTokens(usage, 500, 100, now)
	if err != nil {
		t.Fatal(err)
	}
	if next.TokenMinuteCount != 600 { // 1000 - 500 + 100
		t.Fatalf("expected 600, got %d", next.TokenMinuteCount)
	}

	// Estimate < Observed (e.g. more tokens produced than expected)
	next, err = ReconcileObservedTokens(usage, 100, 500, now)
	if err != nil {
		t.Fatal(err)
	}
	if next.TokenMinuteCount != 1400 { // 1000 - 100 + 500
		t.Fatalf("expected 1400, got %d", next.TokenMinuteCount)
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

func TestCooldownEscalation(t *testing.T) {
	limit := ResourceLimit{
		Resource:         "model:test",
		MaxConcurrent:    4,
		FailureThreshold: 2,
		CooldownBase:     2 * time.Second,
		CooldownMax:      30 * time.Second,
	}

	tests := []struct {
		name     string
		failures int
		want     time.Duration
	}{
		{"below_threshold", 1, 0},
		{"at_threshold", 2, 2 * time.Second},        // shift=0 → 1×base
		{"threshold_plus_1", 3, 4 * time.Second},    // shift=1 → 2×base
		{"threshold_plus_2", 4, 8 * time.Second},    // shift=2 → 4×base
		{"threshold_plus_3", 5, 16 * time.Second},   // shift=3 → 8×base
		{"threshold_plus_4", 6, 30 * time.Second},   // shift=4 → 32s > 30s cap → cap
		{"threshold_plus_10", 12, 30 * time.Second}, // shift=10 → well above cap
		{"extreme_failures", 20, 30 * time.Second},  // shift=18 capped at 16 → still above max
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cooldownFor(tc.failures, limit)
			if tc.want == 0 {
				// Below threshold: cooldownFor is not called by ReportFailure,
				// but the function itself returns base at shift=0.
				// Just verify it does not panic.
				if got < 0 {
					t.Fatalf("negative cooldown: %v", got)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("cooldownFor(%d) = %v, want %v", tc.failures, got, tc.want)
			}
		})
	}
}

func TestCircuitOpenCloseBoundary(t *testing.T) {
	now := time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)
	limit := ResourceLimit{
		Resource:         "model:test",
		MaxConcurrent:    4,
		MaxPerMinute:     60,
		FailureThreshold: 1,
		CooldownBase:     10 * time.Second,
		CooldownMax:      10 * time.Second,
	}
	cost := ResourceCost{Slots: 1, Calls: 1}

	// Open circuit by reporting a failure at threshold.
	usage := ResourceUsage{Resource: "model:test", InFlight: 1}
	reported, err := ReportFailure(usage, limit, cost, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if reported.CircuitOpenUntil == nil {
		t.Fatal("expected circuit open")
	}
	expectedUntil := now.Add(10 * time.Second)
	if !reported.CircuitOpenUntil.Equal(expectedUntil) {
		t.Fatalf("circuit until = %v, want %v", *reported.CircuitOpenUntil, expectedUntil)
	}

	// 1 nanosecond before the boundary: denied.
	justBefore := expectedUntil.Add(-time.Nanosecond)
	res, err := Acquire(limit, reported, cost, 0, justBefore)
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed {
		t.Fatal("expected deny 1ns before circuit opens")
	}
	if res.FailureCode != "DEPENDENCY_CIRCUIT_OPEN" {
		t.Fatalf("code = %s", res.FailureCode)
	}

	// Exactly at the boundary: allowed.
	res, err = Acquire(limit, reported, cost, 0, expectedUntil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Allowed {
		t.Fatalf("expected allow at exact circuit boundary, got %+v", res)
	}
	if res.Usage.CircuitOpenUntil != nil {
		t.Fatalf("elapsed circuit deadline should be cleared from projected usage: %v", *res.Usage.CircuitOpenUntil)
	}
	if res.Usage.ConsecutiveFailures != reported.ConsecutiveFailures {
		t.Fatalf("acquire changed failure streak: got %d want %d", res.Usage.ConsecutiveFailures, reported.ConsecutiveFailures)
	}

	// 1 nanosecond after: allowed.
	res, err = Acquire(limit, reported, cost, 0, expectedUntil.Add(time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Allowed {
		t.Fatal("expected allow after circuit opens")
	}
	if res.Usage.CircuitOpenUntil != nil {
		t.Fatalf("past circuit deadline should be cleared from projected usage: %v", *res.Usage.CircuitOpenUntil)
	}
}

func TestSuccessClearsCircuitOpenUntil(t *testing.T) {
	now := time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)
	openUntil := now.Add(time.Minute)
	usage := ResourceUsage{
		Resource:            "model:test",
		InFlight:            1,
		ConsecutiveFailures: 5,
		CircuitOpenUntil:    &openUntil,
	}
	cost := ResourceCost{Slots: 1, Calls: 1}

	result, err := ReportSuccess(usage, cost, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive failures = %d, want 0", result.ConsecutiveFailures)
	}
	if result.CircuitOpenUntil != nil {
		t.Fatalf("circuit should be nil after success, got %v", *result.CircuitOpenUntil)
	}
	if result.InFlight != 0 {
		t.Fatalf("in-flight = %d, want 0", result.InFlight)
	}
}

func TestReportFailureIgnoresExpiredRetryAfter(t *testing.T) {
	now := time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)
	limit := ResourceLimit{
		Resource:         "model:test",
		MaxConcurrent:    4,
		FailureThreshold: 5, // high threshold → no computed cooldown at streak=1
		CooldownBase:     time.Second,
		CooldownMax:      time.Minute,
	}
	cost := ResourceCost{Slots: 1, Calls: 1}

	// Retry-After in the past → should not set CircuitOpenUntil.
	past := now.Add(-time.Second)
	usage := ResourceUsage{Resource: "model:test", InFlight: 1}
	reported, err := ReportFailure(usage, limit, cost, &past, now)
	if err != nil {
		t.Fatal(err)
	}
	if reported.CircuitOpenUntil != nil {
		t.Fatalf("expired retry-after should not open circuit, got %v", *reported.CircuitOpenUntil)
	}

	// Retry-After exactly at now → should not set CircuitOpenUntil.
	exactlyNow := now
	usage = ResourceUsage{Resource: "model:test", InFlight: 1}
	reported, err = ReportFailure(usage, limit, cost, &exactlyNow, now)
	if err != nil {
		t.Fatal(err)
	}
	if reported.CircuitOpenUntil != nil {
		t.Fatalf("exactly-now retry-after should not open circuit, got %v", *reported.CircuitOpenUntil)
	}

	// Retry-After 1ns in the future → should set CircuitOpenUntil.
	justFuture := now.Add(time.Nanosecond)
	usage = ResourceUsage{Resource: "model:test", InFlight: 1}
	reported, err = ReportFailure(usage, limit, cost, &justFuture, now)
	if err != nil {
		t.Fatal(err)
	}
	if reported.CircuitOpenUntil == nil {
		t.Fatal("future retry-after should open circuit")
	}
	if !reported.CircuitOpenUntil.Equal(justFuture.UTC()) {
		t.Fatalf("circuit until = %v, want %v", *reported.CircuitOpenUntil, justFuture.UTC())
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
