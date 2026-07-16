package kernel

import (
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func gateProposal(now time.Time) domain.OperatorQuestionProposal {
	return domain.OperatorQuestionProposal{
		SchemaVersion: domain.SchemaVersionV1,
		Question: domain.OperatorQuestion{
			SchemaVersion: domain.SchemaVersionV1, ID: "ask_1", MissionID: "mission_1", MissionRevision: "revision_1", Revision: 1,
			Kind: domain.QuestionSingleChoice, Prompt: "Choose", Context: "Affects artifact only",
			Options:      []domain.QuestionOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
			AllowContext: true, AllowSkip: true, BlockingScope: []domain.QuestionBlockingTarget{{Kind: domain.QuestionBlockingArtifact, Reference: "artifact_1"}},
			FallbackPolicy: domain.QuestionContinueOtherWork, DedupSignature: "choice:artifact_1", Priority: 50,
			Status: domain.OperatorQuestionPending, CreatedAt: now.Add(time.Minute), ExpiresAt: now.Add(time.Hour),
		},
		Justification: domain.QuestionJustification{MissingInformation: "operator preference", DecisionImpact: "presentation", AlternativesTried: []string{"mission policies"}, ExpectedGain: "avoid rework", CostOfSilence: "use neutral layout"},
		ProposedBy:    "model:small", ProposedAt: now,
	}
}

func gatePolicy() QuestionGatePolicy {
	return QuestionGatePolicy{MinPriority: 20, MaxPending: 3, MaxDeliveredPerWindow: 2, Window: time.Hour, Cooldown: 6 * time.Hour, QuietStartHour: 23, QuietEndHour: 7, UrgentPriority: 90, MinAlternativesTried: 1, SuppressSafeReversibleDefault: true}
}

func TestEvaluateQuestionAdmitsUsefulProposal(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	result, err := EvaluateQuestion(gatePolicy(), now, gateProposal(now), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != QuestionAdmit || result.Reason != QuestionGateAllowed {
		t.Fatalf("result = %+v", result)
	}
}

func TestEvaluateQuestionSuppressesDuplicateAndCheapDefaults(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	proposal := gateProposal(now)
	history := []QuestionGateRecord{{QuestionID: "ask_old", MissionID: proposal.Question.MissionID, DedupSignature: proposal.Question.DedupSignature, Status: domain.OperatorQuestionPending, DeliveredAt: now.Add(-time.Minute)}}
	result, err := EvaluateQuestion(gatePolicy(), now, proposal, history)
	if err != nil || result.Reason != QuestionGateDuplicatePending || result.Decision != QuestionSuppress {
		t.Fatalf("duplicate result = %+v, err = %v", result, err)
	}
	proposal.Justification.HasSafeDefault = true
	proposal.Justification.DefaultReversible = true
	proposal.Question.DedupSignature = "new-signature"
	result, err = EvaluateQuestion(gatePolicy(), now, proposal, nil)
	if err != nil || result.Reason != QuestionGateSafeDefault || result.Decision != QuestionSuppress {
		t.Fatalf("safe default result = %+v, err = %v", result, err)
	}
}

func TestEvaluateQuestionDefersQuietHoursRateAndCooldown(t *testing.T) {
	quiet := time.Date(2026, 7, 16, 23, 30, 0, 0, time.UTC)
	result, err := EvaluateQuestion(gatePolicy(), quiet, gateProposal(quiet), nil)
	if err != nil || result.Reason != QuestionGateQuietHours || result.RetryAfter.Hour() != 7 {
		t.Fatalf("quiet result = %+v, err = %v", result, err)
	}
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	proposal := gateProposal(now)
	rateHistory := []QuestionGateRecord{
		{QuestionID: "ask_a", MissionID: "mission_1", DedupSignature: "other:a", Status: domain.OperatorQuestionAnswered, DeliveredAt: now.Add(-50 * time.Minute), ClosedAt: now.Add(-45 * time.Minute)},
		{QuestionID: "ask_b", MissionID: "mission_1", DedupSignature: "other:b", Status: domain.OperatorQuestionAnswered, DeliveredAt: now.Add(-10 * time.Minute), ClosedAt: now.Add(-5 * time.Minute)},
	}
	result, err = EvaluateQuestion(gatePolicy(), now, proposal, rateHistory)
	if err != nil || result.Reason != QuestionGateRateLimit || !result.RetryAfter.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("rate result = %+v, err = %v", result, err)
	}
	cooldownHistory := []QuestionGateRecord{{QuestionID: "ask_old", MissionID: "mission_1", DedupSignature: proposal.Question.DedupSignature, Status: domain.OperatorQuestionAnswered, DeliveredAt: now.Add(-2 * time.Hour), ClosedAt: now.Add(-time.Hour)}}
	result, err = EvaluateQuestion(gatePolicy(), now, proposal, cooldownHistory)
	if err != nil || result.Reason != QuestionGateCooldown || !result.RetryAfter.Equal(now.Add(5*time.Hour)) {
		t.Fatalf("cooldown result = %+v, err = %v", result, err)
	}
}

func TestEvaluateQuestionUrgentBypassesQuietAndAlternativeSuppression(t *testing.T) {
	now := time.Date(2026, 7, 16, 23, 30, 0, 0, time.UTC)
	proposal := gateProposal(now)
	proposal.Question.Priority = 90
	proposal.Justification.AlternativesTried = nil
	proposal.Justification.HasSafeDefault = true
	proposal.Justification.DefaultReversible = true
	result, err := EvaluateQuestion(gatePolicy(), now, proposal, nil)
	if err != nil || result.Decision != QuestionAdmit {
		t.Fatalf("urgent result = %+v, err = %v", result, err)
	}
}

func TestEvaluateQuestionNormalizesDuplicateSignatures(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	proposal := gateProposal(now)
	proposal.Question.DedupSignature = "Choice: Artifact_1"
	history := []QuestionGateRecord{{
		QuestionID: "ask_old", MissionID: proposal.Question.MissionID,
		DedupSignature: "choice:artifact_1", Status: domain.OperatorQuestionPending, DeliveredAt: now.Add(-time.Minute),
	}}
	result, err := EvaluateQuestion(gatePolicy(), now, proposal, history)
	if err != nil || result.Decision != QuestionSuppress || result.Reason != QuestionGateDuplicatePending {
		t.Fatalf("normalized duplicate = %+v, err = %v", result, err)
	}
}

func TestEvaluateQuestionTopicCooldownAndBudget(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	policy := gatePolicy()
	policy.TopicCooldown = 4 * time.Hour
	policy.MaxAdmittedPerWindow = 1
	policy.MaxDeliveredPerWindow = 0
	proposal := gateProposal(now)
	proposal.Question.DedupSignature = "choice:other_artifact"
	topicHistory := []QuestionGateRecord{{
		QuestionID: "ask_old", MissionID: "mission_1", DedupSignature: "choice:previous",
		Status: domain.OperatorQuestionAnswered, DeliveredAt: now.Add(-2 * time.Hour), ClosedAt: now.Add(-time.Hour),
	}}
	result, err := EvaluateQuestion(policy, now, proposal, topicHistory)
	if err != nil || result.Decision != QuestionDefer || result.Reason != QuestionGateDuplicateTopic {
		t.Fatalf("topic cooldown = %+v, err = %v", result, err)
	}
	pendingTopic := []QuestionGateRecord{{
		QuestionID: "ask_open", MissionID: "mission_1", DedupSignature: "choice:still_open",
		Status: domain.OperatorQuestionPending, DeliveredAt: now.Add(-time.Minute),
	}}
	result, err = EvaluateQuestion(policy, now, proposal, pendingTopic)
	if err != nil || result.Decision != QuestionSuppress || result.Reason != QuestionGateDuplicateTopic {
		t.Fatalf("pending topic = %+v, err = %v", result, err)
	}
	budgetHistory := []QuestionGateRecord{{
		QuestionID: "ask_budget", MissionID: "mission_1", DedupSignature: "other:topic",
		Status: domain.OperatorQuestionPending, AdmittedAt: now.Add(-10 * time.Minute),
	}}
	result, err = EvaluateQuestion(policy, now, proposal, budgetHistory)
	if err != nil || result.Decision != QuestionDefer || result.Reason != QuestionGateBudgetExhausted {
		t.Fatalf("budget = %+v, err = %v", result, err)
	}
}

func TestEvaluateQuestionDigestHoldAndCapacity(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	policy := gatePolicy()
	policy.Digest = domain.DigestPolicy{Hold: time.Hour, MaxItems: 1, MinPriorityImmediate: 80, AlignToHoldBoundaries: false}
	proposal := gateProposal(now)
	result, err := EvaluateQuestion(policy, now, proposal, nil)
	if err != nil || result.Decision != QuestionAdmit || !result.DigestHeld || !result.DeliveryAvailable.Equal(now.Add(time.Hour)) {
		t.Fatalf("digest hold = %+v, err = %v", result, err)
	}
	full := []QuestionGateRecord{{
		QuestionID: "ask_held", MissionID: "mission_1", DedupSignature: "other:held",
		Status: domain.OperatorQuestionPending,
	}}
	result, err = EvaluateQuestion(policy, now, proposal, full)
	if err != nil || result.Decision != QuestionDefer || result.Reason != QuestionGateDigestFull {
		t.Fatalf("digest full = %+v, err = %v", result, err)
	}
	urgent := gateProposal(now)
	urgent.Question.Priority = 85
	result, err = EvaluateQuestion(policy, now, urgent, nil)
	if err != nil || result.Decision != QuestionAdmit || result.DigestHeld || !result.DeliveryAvailable.Equal(now) {
		t.Fatalf("immediate priority = %+v, err = %v", result, err)
	}
}
