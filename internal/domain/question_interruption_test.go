package domain

import (
	"testing"
	"time"
)

func TestNormalizeDedupSignatureAndTopic(t *testing.T) {
	if got := NormalizeDedupSignature("  Choice: Artifact_1 \n"); got != "choice:artifact_1" {
		t.Fatalf("normalize = %q", got)
	}
	if NormalizeDedupSignature("Choice:Artifact_1") != NormalizeDedupSignature(" choice : artifact_1 ") {
		t.Fatal("equivalent signatures must normalize identically")
	}
	if got := SemanticTopicKey("Choice:Artifact_1"); got != "choice" {
		t.Fatalf("topic = %q", got)
	}
	if got := SemanticTopicKey("single-topic"); got != "single-topic" {
		t.Fatalf("topic without separator = %q", got)
	}
	if SemanticTopicKey("   ") != "" {
		t.Fatal("empty signature should yield empty topic")
	}
}

func TestInterruptionBudgetAndDigestPolicyValidate(t *testing.T) {
	budget := InterruptionBudgetPolicy{PolicyVersion: "default@1", MaxPending: 3, MaxAdmittedPerWindow: 2, Window: time.Hour}
	if err := budget.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (InterruptionBudgetPolicy{PolicyVersion: "v", MaxAdmittedPerWindow: 1}).Validate(); err == nil {
		t.Fatal("accepted window limit without window")
	}
	digest := DigestPolicy{Hold: time.Hour, MaxItems: 5, MinPriorityImmediate: 80, AlignToHoldBoundaries: true}
	if err := digest.Validate(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 12, 10, 0, 0, time.UTC)
	available := digest.NextDigestAvailable(now)
	if !available.After(now) {
		t.Fatalf("aligned digest available = %v", available)
	}
	later := digest.NextDigestAvailable(now.Add(30 * time.Minute))
	if !later.Equal(available) && later.Before(available) {
		t.Fatalf("same bucket expected later=%v available=%v", later, available)
	}
}

func TestPlanQuestionReminderStopsAndSchedules(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	question := OperatorQuestion{
		SchemaVersion: SchemaVersionV1, ID: "ask_1", MissionID: "mission_1", MissionRevision: "revision_1", Revision: 1,
		Kind: QuestionConfirmation, Prompt: "Confirm?", Context: "impact", DedupSignature: "confirm:x", Priority: 40,
		FallbackPolicy: QuestionContinueOtherWork, Status: OperatorQuestionPending, CreatedAt: now.Add(-time.Hour),
		ExpiresAt: now.Add(24 * time.Hour),
	}
	disabled, err := PlanQuestionReminder(question, now.Add(-time.Hour), 0, now, ReminderPolicy{})
	if err != nil || disabled.Due || disabled.StopReason != "REMINDERS_DISABLED" {
		t.Fatalf("disabled = %+v err=%v", disabled, err)
	}
	policy := ReminderPolicy{Enabled: true, MaxCount: 2, FirstAfter: time.Hour, Interval: 2 * time.Hour}
	deliveredAt := now.Add(-30 * time.Minute)
	waiting, err := PlanQuestionReminder(question, deliveredAt, 0, now, policy)
	if err != nil || waiting.Due || waiting.ReminderIndex != 1 || !waiting.AvailableAt.Equal(deliveredAt.Add(time.Hour)) {
		t.Fatalf("waiting = %+v err=%v", waiting, err)
	}
	due, err := PlanQuestionReminder(question, deliveredAt, 0, deliveredAt.Add(time.Hour), policy)
	if err != nil || !due.Due || due.ReminderIndex != 1 {
		t.Fatalf("due = %+v err=%v", due, err)
	}
	maxed, err := PlanQuestionReminder(question, deliveredAt, 2, now.Add(10*time.Hour), policy)
	if err != nil || maxed.Due || maxed.StopReason != "MAX_REMINDERS" {
		t.Fatalf("maxed = %+v err=%v", maxed, err)
	}
	answered := question
	answered.Status = OperatorQuestionAnswered
	answered.AnsweredAt = now
	answered.AnswerID = "answer_1"
	answered.Revision = 2
	stopped, err := PlanQuestionReminder(answered, deliveredAt, 0, now, policy)
	if err != nil || stopped.StopReason != "QUESTION_ANSWERED" {
		t.Fatalf("answered = %+v err=%v", stopped, err)
	}
	if ReminderDestinationRef("operator_primary", 1) != "operator_primary#reminder:1" {
		t.Fatal("reminder destination ref")
	}
}
