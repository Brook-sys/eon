package domain

import (
	"strings"
	"testing"
	"time"
)

func validOperatorQuestion() OperatorQuestion {
	created := time.Date(2026, 7, 16, 3, 30, 0, 0, time.UTC)
	return OperatorQuestion{
		SchemaVersion: SchemaVersionV1,
		ID:            "ask_1", MissionID: "mission_1", MissionRevision: "mission_revision_1",
		InquiryID: "inquiry_1", OperationID: "operation_1", Revision: 1,
		Kind: QuestionSingleChoiceWithOther, Prompt: "Que estilo você prefere?",
		Context:    "A resposta afeta apenas a apresentação do artifact.",
		Options:    []QuestionOption{{ID: "minimal", Label: "Minimalista"}, {ID: "modern", Label: "Moderno"}},
		AllowOther: true, AllowContext: true, AllowSkip: true,
		BlockingScope: []QuestionBlockingTarget{{Kind: QuestionBlockingArtifact, Reference: "artifact_1"}}, FallbackPolicy: QuestionContinueOtherWork,
		DedupSignature: "style:artifact_1", Priority: 50, Status: OperatorQuestionPending,
		CreatedAt: created, ExpiresAt: created.Add(24 * time.Hour),
	}
}

func TestOperatorQuestionValidation(t *testing.T) {
	base := validOperatorQuestion()
	tests := []struct {
		name string
		edit func(*OperatorQuestion)
	}{
		{name: "missing identity", edit: func(q *OperatorQuestion) { q.ID = "" }},
		{name: "choice requires two options", edit: func(q *OperatorQuestion) { q.Options = q.Options[:1] }},
		{name: "duplicate option", edit: func(q *OperatorQuestion) { q.Options[1].ID = q.Options[0].ID }},
		{name: "duplicate blocking scope", edit: func(q *OperatorQuestion) {
			q.BlockingScope = []QuestionBlockingTarget{{Kind: QuestionBlockingArtifact, Reference: "1"}, {Kind: QuestionBlockingArtifact, Reference: "1"}}
		}},
		{name: "with other must allow other", edit: func(q *OperatorQuestion) { q.AllowOther = false }},
		{name: "invalid expiration", edit: func(q *OperatorQuestion) { q.ExpiresAt = q.CreatedAt }},
		{name: "default requires policy", edit: func(q *OperatorQuestion) { q.DefaultOptionID = "minimal" }},
		{name: "default must resolve", edit: func(q *OperatorQuestion) { q.FallbackPolicy = QuestionApplyDefault; q.DefaultOptionID = "missing" }},
		{name: "pending cannot claim answer", edit: func(q *OperatorQuestion) { q.AnswerID = "answer_1"; q.AnsweredAt = q.CreatedAt.Add(time.Minute) }},
		{name: "oversized prompt", edit: func(q *OperatorQuestion) { q.Prompt = strings.Repeat("x", MaxOperatorQuestionTextBytes+1) }},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid question rejected: %v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := base
			got.Options = append([]QuestionOption(nil), base.Options...)
			got.BlockingScope = append([]QuestionBlockingTarget(nil), base.BlockingScope...)
			test.edit(&got)
			if err := got.Validate(); err == nil {
				t.Fatal("invalid question accepted")
			}
		})
	}
}

func TestOperatorQuestionTerminalStateValidation(t *testing.T) {
	base := validOperatorQuestion()
	answered := base
	answered.Status = OperatorQuestionAnswered
	answered.AnswerID = "answer_1"
	answered.AnsweredAt = base.CreatedAt.Add(time.Minute)
	if err := answered.Validate(); err != nil {
		t.Fatalf("answered question rejected: %v", err)
	}
	if !answered.Status.Terminal() || base.Status.Terminal() {
		t.Fatal("terminal status classification is wrong")
	}
	superseded := base
	superseded.Status = OperatorQuestionSuperseded
	superseded.SupersededBy = "ask_2"
	if err := superseded.Validate(); err != nil {
		t.Fatalf("superseded question rejected: %v", err)
	}
}

func TestUserAnswerValidationForQuestion(t *testing.T) {
	question := validOperatorQuestion()
	answer := UserAnswer{
		SchemaVersion: SchemaVersionV1, ID: "answer_1", QuestionID: question.ID,
		ExpectedQuestionRevision: question.Revision, Kind: AnswerOptions, OptionIDs: []string{"minimal"},
		ActorID: "operator_1", Channel: "telegram", TransportEventID: "callback_1",
		TransportMessageID: "message_9", ReceivedAt: question.CreatedAt.Add(time.Minute),
	}
	if err := answer.ValidateForQuestion(question); err != nil {
		t.Fatalf("valid answer rejected: %v", err)
	}
	tests := []struct {
		name         string
		editAnswer   func(*UserAnswer)
		editQuestion func(*OperatorQuestion)
	}{
		{name: "stale revision", editAnswer: func(a *UserAnswer) { a.ExpectedQuestionRevision++ }},
		{name: "unknown option", editAnswer: func(a *UserAnswer) { a.OptionIDs = []string{"unknown"} }},
		{name: "multiple options on single choice", editAnswer: func(a *UserAnswer) { a.OptionIDs = []string{"minimal", "modern"} }},
		{name: "late answer", editAnswer: func(a *UserAnswer) { a.ReceivedAt = question.ExpiresAt.Add(time.Nanosecond) }},
		{name: "closed question", editQuestion: func(q *OperatorQuestion) { q.Status = OperatorQuestionCancelled }},
		{name: "unallowed skip", editAnswer: func(a *UserAnswer) { a.Kind = AnswerSkip; a.OptionIDs = nil }, editQuestion: func(q *OperatorQuestion) { q.AllowSkip = false }},
		{name: "unallowed context request", editAnswer: func(a *UserAnswer) { a.Kind = AnswerNeedContext; a.OptionIDs = nil; a.Text = "Por que isso importa?" }, editQuestion: func(q *OperatorQuestion) { q.AllowContext = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			q := question
			q.Options = append([]QuestionOption(nil), question.Options...)
			a := answer
			a.OptionIDs = append([]string(nil), answer.OptionIDs...)
			if test.editQuestion != nil {
				test.editQuestion(&q)
			}
			if test.editAnswer != nil {
				test.editAnswer(&a)
			}
			if err := a.ValidateForQuestion(q); err == nil {
				t.Fatal("invalid correlated answer accepted")
			}
		})
	}
}

func TestAnswerKindsHaveUnambiguousPayloads(t *testing.T) {
	now := time.Date(2026, 7, 16, 3, 31, 0, 0, time.UTC)
	base := UserAnswer{SchemaVersion: SchemaVersionV1, ID: "answer_1", QuestionID: "ask_1", ExpectedQuestionRevision: 1, ActorID: "operator_1", Channel: "dashboard", TransportEventID: "request_1", ReceivedAt: now}
	valid := []UserAnswer{
		func() UserAnswer { a := base; a.Kind = AnswerOptions; a.OptionIDs = []string{"a"}; return a }(),
		func() UserAnswer { a := base; a.Kind = AnswerFreeText; a.Text = "custom"; return a }(),
		func() UserAnswer { a := base; a.Kind = AnswerOther; a.Text = "other"; return a }(),
		func() UserAnswer { a := base; a.Kind = AnswerNeedContext; a.Text = "explain"; return a }(),
		func() UserAnswer { a := base; a.Kind = AnswerSkip; return a }(),
		func() UserAnswer { a := base; a.Kind = AnswerNoPreference; return a }(),
		func() UserAnswer { a := base; a.Kind = AnswerConfirm; return a }(),
		func() UserAnswer { a := base; a.Kind = AnswerDecline; return a }(),
	}
	for _, answer := range valid {
		if err := answer.Validate(); err != nil {
			t.Fatalf("kind %s rejected: %v", answer.Kind, err)
		}
	}
	ambiguous := valid[0]
	ambiguous.Text = "also text"
	if err := ambiguous.Validate(); err == nil {
		t.Fatal("ambiguous option and text payload accepted")
	}
}
