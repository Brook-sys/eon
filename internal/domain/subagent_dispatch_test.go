package domain

import (
	"testing"
	"time"
)

func testSubagentDispatch(now time.Time) SubagentDispatch {
	return SubagentDispatch{
		SchemaVersion: SchemaVersionV1, RequestID: "request-1", SessionID: "session-1", Attempt: 2,
		PeerID: "peer-1", Status: SubagentDispatchPending, MaxSendAttempts: 3,
		AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func TestSubagentDispatchGenerationAndSendAttemptsAreIndependent(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	original := testSubagentDispatch(now)
	leased, err := LeaseSubagentDispatch(original, "worker-1", now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if leased.Attempt != original.Attempt || leased.RequestID != original.RequestID || leased.SendAttempt != 1 {
		t.Fatalf("lease changed generation identity or wrong send attempt: %+v", leased)
	}
	retry, err := FailSubagentDispatch(leased, "worker-1", "TEMPORARY", now.Add(time.Second), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	leasedAgain, err := LeaseSubagentDispatch(retry, "worker-2", now.Add(time.Minute), now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if leasedAgain.Attempt != 2 || leasedAgain.RequestID != "request-1" || leasedAgain.SendAttempt != 2 {
		t.Fatalf("retry did not preserve generation identity: %+v", leasedAgain)
	}
}

func TestSubagentDispatchAmbiguousAndExpiredRequireReconciliation(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	leased, err := LeaseSubagentDispatch(testSubagentDispatch(now), "worker", now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	ambiguous, err := MarkAmbiguousSubagentDispatch(leased, "worker", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.Status != SubagentDispatchEffectUnknown || ambiguous.Due(now.Add(24*time.Hour)) {
		t.Fatalf("ambiguous dispatch must be parked: %+v", ambiguous)
	}
	if _, err := LeaseSubagentDispatch(ambiguous, "worker", now.Add(time.Hour), now.Add(2*time.Hour)); err == nil {
		t.Fatal("effect-unknown dispatch was leaseable")
	}
	retry, err := ResolveSubagentDispatchEffectUnknown(ambiguous, now.Add(time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if retry.Status != SubagentDispatchRetry || !retry.Due(now.Add(time.Hour)) {
		t.Fatalf("unexpected reconciliation: %+v", retry)
	}

	leased, err = LeaseSubagentDispatch(testSubagentDispatch(now), "worker", now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	expired, err := ReclaimExpiredSubagentDispatch(leased, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if expired.Status != SubagentDispatchEffectUnknown || expired.LastFailureCode != SubagentDispatchFailureLeaseExpired || expired.Due(now.Add(time.Hour)) {
		t.Fatalf("expired lease was not parked: %+v", expired)
	}
}

func TestSubagentDispatchValidationIsStrictAndBounded(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	valid := testSubagentDispatch(now)
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.RequestID = SubagentDispatchRequestID(string(make([]byte, 129)))
	if err := invalid.Validate(); err == nil {
		t.Fatal("oversized request id accepted")
	}
	invalid = valid
	invalid.Status = "UNKNOWN"
	if err := invalid.Validate(); err == nil {
		t.Fatal("unknown status accepted")
	}
	invalid = valid
	invalid.Attempt = -1
	if err := invalid.Validate(); err == nil {
		t.Fatal("negative generation attempt accepted")
	}
	invalid = valid
	invalid.SendAttempt = invalid.MaxSendAttempts + 1
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid send attempt accepted")
	}
}

func TestSubagentDispatchEffectUnknownCannotBeCancelled(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	leased, err := LeaseSubagentDispatch(testSubagentDispatch(now), "worker", now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := MarkAmbiguousSubagentDispatch(leased, "worker", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CancelSubagentDispatch(unknown, now.Add(2*time.Second)); err == nil {
		t.Fatal("effect-unknown dispatch was cancelled without reconciliation")
	}
}
