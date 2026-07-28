package domain

import (
	"testing"
	"time"
)

// TestReportFailureRetryAfterBoundary exercises the retry-after boundary cases
// identified by Phases 318–319: past dates, zero time, exactly-now, and the
// interaction between the computed cooldown and the provider-supplied
// retry-after deadline. Uses a fixed virtual clock (no real time).
func TestReportFailureRetryAfterBoundary(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	limit := ResourceLimit{
		Resource:         "model:groq",
		MaxConcurrent:    1,
		CooldownBase:     2 * time.Second,
		CooldownMax:      30 * time.Second,
		FailureThreshold: 3,
	}
	cost := ResourceCost{Slots: 1, Calls: 1}

	t.Run("past_retry_after_does_not_open_circuit_below_threshold", func(t *testing.T) {
		past := now.Add(-5 * time.Minute)
		usage := ResourceUsage{Resource: "model:groq", InFlight: 1, ConsecutiveFailures: 0}
		next, err := ReportFailure(usage, limit, cost, &past, now)
		if err != nil {
			t.Fatal(err)
		}
		if next.CircuitOpenUntil != nil {
			t.Fatalf("past retry-after must not open circuit below threshold; got %v", *next.CircuitOpenUntil)
		}
	})

	t.Run("past_retry_after_does_not_extend_circuit_when_open", func(t *testing.T) {
		// Two failures to reach threshold (threshold=3, need 3 consecutive).
		usage := ResourceUsage{
			Resource:            "model:groq",
			InFlight:            1,
			ConsecutiveFailures: 2, // one more will trigger circuit
		}
		past := now.Add(-1 * time.Hour)
		next, err := ReportFailure(usage, limit, cost, &past, now)
		if err != nil {
			t.Fatal(err)
		}
		if next.CircuitOpenUntil == nil {
			t.Fatal("expected circuit open from consecutive failures even with past retry-after")
		}
		// The cooldown from 3 failures is cooldownFor(3, limit). The past
		// retry-after should NOT extend it further.
		expected := now.Add(cooldownFor(3, limit))
		if !next.CircuitOpenUntil.Equal(expected.UTC()) {
			t.Fatalf("circuit open until = %v, want %v (cooldown only, past retry-after ignored)", *next.CircuitOpenUntil, expected.UTC())
		}
	})

	t.Run("zero_retry_after_is_ignored", func(t *testing.T) {
		zero := time.Time{}
		usage := ResourceUsage{Resource: "model:groq", InFlight: 1, ConsecutiveFailures: 0}
		next, err := ReportFailure(usage, limit, cost, &zero, now)
		if err != nil {
			t.Fatal(err)
		}
		if next.CircuitOpenUntil != nil {
			t.Fatalf("zero retry-after must not open circuit below threshold; got %v", *next.CircuitOpenUntil)
		}
	})

	t.Run("future_retry_after_extends_past_cooldown", func(t *testing.T) {
		usage := ResourceUsage{
			Resource:            "model:groq",
			InFlight:            1,
			ConsecutiveFailures: 2, // triggers circuit
		}
		future := now.Add(2 * time.Minute)
		next, err := ReportFailure(usage, limit, cost, &future, now)
		if err != nil {
			t.Fatal(err)
		}
		if next.CircuitOpenUntil == nil {
			t.Fatal("expected circuit open")
		}
		// Cooldown would be cooldownFor(3, limit) from now; retry-after 2m
		// is later, so it should win.
		cooldownUntil := now.Add(cooldownFor(3, limit))
		if !next.CircuitOpenUntil.Equal(future.UTC()) {
			t.Fatalf("circuit open until = %v, want %v (retry-after wins over cooldown %v)", *next.CircuitOpenUntil, future.UTC(), cooldownUntil.UTC())
		}
	})

	t.Run("retry_after_just_after_cooldown_wins", func(t *testing.T) {
		usage := ResourceUsage{
			Resource:            "model:groq",
			InFlight:            1,
			ConsecutiveFailures: 2,
		}
		cooldownDur := cooldownFor(3, limit)
		justAfter := now.Add(cooldownDur + 1*time.Second)
		next, err := ReportFailure(usage, limit, cost, &justAfter, now)
		if err != nil {
			t.Fatal(err)
		}
		if next.CircuitOpenUntil == nil {
			t.Fatal("expected circuit open")
		}
		if !next.CircuitOpenUntil.Equal(justAfter.UTC()) {
			t.Fatalf("circuit open until = %v, want %v (retry-after 1s after cooldown wins)", *next.CircuitOpenUntil, justAfter.UTC())
		}
	})

	t.Run("retry_after_just_before_cooldown_loses", func(t *testing.T) {
		usage := ResourceUsage{
			Resource:            "model:groq",
			InFlight:            1,
			ConsecutiveFailures: 2,
		}
		cooldownDur := cooldownFor(3, limit)
		justBefore := now.Add(cooldownDur - 1*time.Second)
		next, err := ReportFailure(usage, limit, cost, &justBefore, now)
		if err != nil {
			t.Fatal(err)
		}
		if next.CircuitOpenUntil == nil {
			t.Fatal("expected circuit open")
		}
		expected := now.Add(cooldownDur)
		if !next.CircuitOpenUntil.Equal(expected.UTC()) {
			t.Fatalf("circuit open until = %v, want %v (cooldown wins over earlier retry-after)", *next.CircuitOpenUntil, expected.UTC())
		}
	})

	t.Run("retry_after_equals_now_is_ignored", func(t *testing.T) {
		// retry-after == now is not in the future relative to now, and the
		// code checks retryAfter.UTC() which equals now. The cooldown
		// computation should not be affected.
		usage := ResourceUsage{Resource: "model:groq", InFlight: 1, ConsecutiveFailures: 0}
		retryAtNow := now
		next, err := ReportFailure(usage, limit, cost, &retryAtNow, now)
		if err != nil {
			t.Fatal(err)
		}
		if next.CircuitOpenUntil != nil {
			t.Fatalf("retry-after == now must not open circuit below threshold; got %v", *next.CircuitOpenUntil)
		}
	})

	t.Run("nil_retry_after_uses_cooldown_only", func(t *testing.T) {
		usage := ResourceUsage{
			Resource:            "model:groq",
			InFlight:            1,
			ConsecutiveFailures: 2,
		}
		next, err := ReportFailure(usage, limit, cost, nil, now)
		if err != nil {
			t.Fatal(err)
		}
		if next.CircuitOpenUntil == nil {
			t.Fatal("expected circuit open from threshold")
		}
		expected := now.Add(cooldownFor(3, limit))
		if !next.CircuitOpenUntil.Equal(expected.UTC()) {
			t.Fatalf("circuit open until = %v, want %v (cooldown only, nil retry-after)", *next.CircuitOpenUntil, expected.UTC())
		}
	})
}

// TestReportFailureRetryAfterClamping verifies that the ReportFailure function
// does not clamp retry-after to any maximum but accepts any future deadline
// the provider declares. Clamping, if needed, must happen in the kernel
// (authorize.go), not in the pure domain function.
func TestReportFailureRetryAfterClamping(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	limit := ResourceLimit{
		Resource:         "model:groq",
		MaxConcurrent:    1,
		CooldownBase:     2 * time.Second,
		CooldownMax:      30 * time.Second,
		FailureThreshold: 3,
	}
	cost := ResourceCost{Slots: 1, Calls: 1}

	usage := ResourceUsage{
		Resource:            "model:groq",
		InFlight:            1,
		ConsecutiveFailures: 2,
	}
	// A very large retry-after (1 hour) should be accepted as-is by the
	// domain function. The kernel is responsible for policy clamping.
	hugeRetryAfter := now.Add(1 * time.Hour)
	next, err := ReportFailure(usage, limit, cost, &hugeRetryAfter, now)
	if err != nil {
		t.Fatal(err)
	}
	if next.CircuitOpenUntil == nil {
		t.Fatal("expected circuit open")
	}
	if !next.CircuitOpenUntil.Equal(hugeRetryAfter.UTC()) {
		t.Fatalf("circuit open until = %v, want %v (domain must not clamp retry-after)", *next.CircuitOpenUntil, hugeRetryAfter.UTC())
	}
}
