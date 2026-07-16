package kernel

import (
	"errors"
	"fmt"

	"motor-autonomo/internal/domain"
)

const OperatorQuestionAnsweredEventType = "operator_question_answered"

// ApplyQuestionWait moves only explicitly targeted operations into a local
// event wait. Non-operation targets are intentionally left for their owning
// services; no mission-wide state exists here.
func ApplyQuestionWait(question domain.OperatorQuestion, operations []domain.Operation) ([]domain.Operation, error) {
	if err := question.Validate(); err != nil {
		return nil, fmt.Errorf("validate operator question: %w", err)
	}
	if question.Status != domain.OperatorQuestionPending {
		return nil, errors.New("only a pending operator question may block dependencies")
	}
	targets := operationBlockingTargets(question.BlockingScope)
	result := cloneOperations(operations)
	seen := make(map[domain.OperationID]struct{}, len(result))
	for i := range result {
		operation := &result[i]
		if _, duplicate := seen[operation.ID]; duplicate {
			return nil, fmt.Errorf("duplicate operation %s", operation.ID)
		}
		seen[operation.ID] = struct{}{}
		if _, targeted := targets[operation.ID]; !targeted {
			continue
		}
		if operation.MissionRevision != question.MissionRevision {
			return nil, fmt.Errorf("target operation %s belongs to another mission revision", operation.ID)
		}
		next, err := domain.Transition(domain.OperationalSnapshot{State: operation.State, Reevaluation: operation.Reevaluation}, domain.TransitionInput{
			Event: domain.EventWaitEvent, EventType: OperatorQuestionAnsweredEventType, Reference: string(question.ID),
		})
		if err != nil {
			return nil, fmt.Errorf("block operation %s: %w", operation.ID, err)
		}
		operation.State, operation.Reevaluation = next.State, next.Reevaluation
	}
	for target := range targets {
		if _, ok := seen[target]; !ok {
			return nil, fmt.Errorf("blocking target operation %s was not supplied", target)
		}
	}
	return result, nil
}

// ResumeQuestionWait resumes only operations that still wait on this exact
// question. Already completed, independently waiting or unrelated operations
// are preserved verbatim, making replay safe after the question is terminal.
func ResumeQuestionWait(question domain.OperatorQuestion, operations []domain.Operation) ([]domain.Operation, error) {
	if err := question.Validate(); err != nil {
		return nil, fmt.Errorf("validate operator question: %w", err)
	}
	if !question.Status.Terminal() {
		return nil, errors.New("only a terminal operator question may resume dependencies")
	}
	result := cloneOperations(operations)
	for i := range result {
		operation := &result[i]
		if operation.State != domain.StateWaitingEvent || operation.Reevaluation.EventType != OperatorQuestionAnsweredEventType || operation.Reevaluation.Reference != string(question.ID) {
			continue
		}
		next, err := domain.Transition(domain.OperationalSnapshot{State: operation.State, Reevaluation: operation.Reevaluation}, domain.TransitionInput{Event: domain.EventResume})
		if err != nil {
			return nil, fmt.Errorf("resume operation %s: %w", operation.ID, err)
		}
		operation.State, operation.Reevaluation = next.State, next.Reevaluation
	}
	return result, nil
}

func operationBlockingTargets(scope []domain.QuestionBlockingTarget) map[domain.OperationID]struct{} {
	targets := make(map[domain.OperationID]struct{})
	for _, target := range scope {
		if target.Kind == domain.QuestionBlockingOperation {
			targets[domain.OperationID(target.Reference)] = struct{}{}
		}
	}
	return targets
}

func cloneOperations(operations []domain.Operation) []domain.Operation {
	result := append([]domain.Operation(nil), operations...)
	for i := range result {
		result[i].ReadSet = append([]string(nil), result[i].ReadSet...)
		result[i].InputRefs = append([]string(nil), result[i].InputRefs...)
	}
	return result
}
