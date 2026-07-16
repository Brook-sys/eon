package domain

import (
	"testing"
	"time"
)

func validGateDecision(now time.Time) QuestionGateDecisionRecord {
	return QuestionGateDecisionRecord{
		SchemaVersion:  SchemaVersionV1,
		ID:             "gate_1",
		QuestionID:     "ask_1",
		MissionID:      "mission_1",
		DedupSignature: "presentation:artifact_1",
		Decision:       PersistedQuestionAdmit,
		Reason:         PersistedQuestionGateAllowed,
		PolicyVersion:  "default@1",
		EvaluatedAt:    now,
	}
}

func TestQuestionGateDecisionRecordValidate(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	if err := validGateDecision(now).Validate(); err != nil {
		t.Fatal(err)
	}
	deferred := validGateDecision(now)
	deferred.Decision = PersistedQuestionDefer
	deferred.Reason = PersistedQuestionGateQuietHours
	deferred.RetryAfter = now.Add(time.Hour)
	if err := deferred.Validate(); err != nil {
		t.Fatal(err)
	}
	badRetry := deferred
	badRetry.RetryAfter = now
	if err := badRetry.Validate(); err == nil {
		t.Fatal("accepted non-future retry")
	}
	suppressWithRetry := validGateDecision(now)
	suppressWithRetry.Decision = PersistedQuestionSuppress
	suppressWithRetry.Reason = PersistedQuestionGateSafeDefault
	suppressWithRetry.RetryAfter = now.Add(time.Minute)
	if err := suppressWithRetry.Validate(); err == nil {
		t.Fatal("accepted suppress with retry")
	}
	admitBadReason := validGateDecision(now)
	admitBadReason.Reason = PersistedQuestionGateCooldown
	if err := admitBadReason.Validate(); err == nil {
		t.Fatal("accepted admit without ALLOWED")
	}
}
