package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestTransitionOperatorQuestion(t *testing.T) {
	base := validOperatorQuestion()
	tests := []struct {
		name       string
		transition OperatorQuestionTransition
		wantStatus OperatorQuestionStatus
		wantError  bool
	}{
		{name: "request clarification", transition: OperatorQuestionTransition{Event: QuestionEventRequestClarification, OccurredAt: base.CreatedAt.Add(time.Minute)}, wantStatus: OperatorQuestionClarificationRequested},
		{name: "answer", transition: OperatorQuestionTransition{Event: QuestionEventAnswer, OccurredAt: base.CreatedAt.Add(time.Minute), AnswerID: "answer_1"}, wantStatus: OperatorQuestionAnswered},
		{name: "expire", transition: OperatorQuestionTransition{Event: QuestionEventExpire, OccurredAt: base.ExpiresAt}, wantStatus: OperatorQuestionExpired},
		{name: "supersede", transition: OperatorQuestionTransition{Event: QuestionEventSupersede, OccurredAt: base.CreatedAt.Add(time.Minute), SupersededBy: "ask_2"}, wantStatus: OperatorQuestionSuperseded},
		{name: "cancel", transition: OperatorQuestionTransition{Event: QuestionEventCancel, OccurredAt: base.CreatedAt.Add(time.Minute)}, wantStatus: OperatorQuestionCancelled},
		{name: "answer missing ID", transition: OperatorQuestionTransition{Event: QuestionEventAnswer, OccurredAt: base.CreatedAt.Add(time.Minute)}, wantError: true},
		{name: "premature expiry", transition: OperatorQuestionTransition{Event: QuestionEventExpire, OccurredAt: base.ExpiresAt.Add(-time.Second)}, wantError: true},
		{name: "same superseder", transition: OperatorQuestionTransition{Event: QuestionEventSupersede, OccurredAt: base.CreatedAt.Add(time.Minute), SupersededBy: base.ID}, wantError: true},
		{name: "after expiry", transition: OperatorQuestionTransition{Event: QuestionEventCancel, OccurredAt: base.ExpiresAt.Add(time.Second)}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := base
			before.Options = append([]QuestionOption(nil), base.Options...)
			before.BlockingScope = append([]QuestionBlockingTarget(nil), base.BlockingScope...)
			next, err := TransitionOperatorQuestion(base, test.transition)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %v", err, test.wantError)
			}
			if !reflect.DeepEqual(base, before) {
				t.Fatal("transition mutated input")
			}
			if test.wantError {
				return
			}
			if next.Status != test.wantStatus || next.Revision != base.Revision+1 {
				t.Fatalf("next = status %s revision %d", next.Status, next.Revision)
			}
		})
	}
}

func TestTransitionOperatorQuestionRejectsTerminalState(t *testing.T) {
	base := validOperatorQuestion()
	answered, err := TransitionOperatorQuestion(base, OperatorQuestionTransition{Event: QuestionEventAnswer, OccurredAt: base.CreatedAt.Add(time.Minute), AnswerID: "answer_1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = TransitionOperatorQuestion(answered, OperatorQuestionTransition{Event: QuestionEventCancel, OccurredAt: base.CreatedAt.Add(2 * time.Minute)})
	if err == nil {
		t.Fatal("terminal question transitioned")
	}
}
