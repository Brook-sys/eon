package domain

import (
	"errors"
	"fmt"
	"time"
)

type OperatorQuestionTransitionEvent string

const (
	QuestionEventRequestClarification OperatorQuestionTransitionEvent = "REQUEST_CLARIFICATION"
	QuestionEventAnswer               OperatorQuestionTransitionEvent = "ANSWER"
	QuestionEventExpire               OperatorQuestionTransitionEvent = "EXPIRE"
	QuestionEventSupersede            OperatorQuestionTransitionEvent = "SUPERSEDE"
	QuestionEventCancel               OperatorQuestionTransitionEvent = "CANCEL"
)

type OperatorQuestionTransition struct {
	Event        OperatorQuestionTransitionEvent
	OccurredAt   time.Time
	AnswerID     OperatorAnswerID
	SupersededBy OperatorQuestionID
}

// TransitionOperatorQuestion is a pure optimistic state transition. Callers
// persist its return value with the answer/event and dependent operation
// changes in one transaction.
func TransitionOperatorQuestion(current OperatorQuestion, transition OperatorQuestionTransition) (OperatorQuestion, error) {
	if err := current.Validate(); err != nil {
		return OperatorQuestion{}, fmt.Errorf("validate current question: %w", err)
	}
	if current.Status.Terminal() {
		return OperatorQuestion{}, errors.New("terminal operator question cannot transition")
	}
	if transition.OccurredAt.IsZero() || transition.OccurredAt.Before(current.CreatedAt) {
		return OperatorQuestion{}, errors.New("question transition has invalid occurrence time")
	}
	if !current.ExpiresAt.IsZero() && transition.Event != QuestionEventExpire && transition.OccurredAt.After(current.ExpiresAt) {
		return OperatorQuestion{}, errors.New("non-expiration transition occurred after question expiration")
	}
	next := current
	next.Revision++
	switch transition.Event {
	case QuestionEventRequestClarification:
		if current.Status != OperatorQuestionPending || !current.AllowContext || transition.AnswerID != "" || transition.SupersededBy != "" {
			return OperatorQuestion{}, errors.New("clarification request is not allowed")
		}
		next.Status = OperatorQuestionClarificationRequested
	case QuestionEventAnswer:
		if transition.AnswerID == "" || transition.SupersededBy != "" {
			return OperatorQuestion{}, errors.New("answer transition requires only answer ID")
		}
		next.Status = OperatorQuestionAnswered
		next.AnswerID = transition.AnswerID
		next.AnsweredAt = transition.OccurredAt
	case QuestionEventExpire:
		if current.ExpiresAt.IsZero() || transition.OccurredAt.Before(current.ExpiresAt) || transition.AnswerID != "" || transition.SupersededBy != "" {
			return OperatorQuestion{}, errors.New("expiration transition is premature or contains terminal payload")
		}
		next.Status = OperatorQuestionExpired
	case QuestionEventSupersede:
		if transition.SupersededBy == "" || transition.SupersededBy == current.ID || transition.AnswerID != "" {
			return OperatorQuestion{}, errors.New("supersede transition requires a distinct successor")
		}
		next.Status = OperatorQuestionSuperseded
		next.SupersededBy = transition.SupersededBy
	case QuestionEventCancel:
		if transition.AnswerID != "" || transition.SupersededBy != "" {
			return OperatorQuestion{}, errors.New("cancel transition must not contain answer or successor")
		}
		next.Status = OperatorQuestionCancelled
	default:
		return OperatorQuestion{}, fmt.Errorf("unknown operator question transition %q", transition.Event)
	}
	if err := next.Validate(); err != nil {
		return OperatorQuestion{}, fmt.Errorf("validate transitioned question: %w", err)
	}
	return next, nil
}
