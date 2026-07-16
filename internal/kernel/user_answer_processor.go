package kernel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"motor-autonomo/internal/domain"
)

// DecodeUserAnswerExternalEvent decodes the bounded structured payload only
// after transport ingestion has classified it as USER_ANSWER. Authentication
// and actor allowlists remain adapter/control-service responsibilities.
func DecodeUserAnswerExternalEvent(event domain.ExternalEvent) (domain.UserAnswer, error) {
	if err := event.Validate(); err != nil {
		return domain.UserAnswer{}, fmt.Errorf("validate external event: %w", err)
	}
	if event.Kind != domain.ExternalUserAnswer {
		return domain.UserAnswer{}, errors.New("external event is not a user answer")
	}
	if len(event.Content.Structured) == 0 {
		return domain.UserAnswer{}, errors.New("user answer event requires structured content")
	}
	decoder := json.NewDecoder(bytes.NewReader(event.Content.Structured))
	decoder.DisallowUnknownFields()
	var answer domain.UserAnswer
	if err := decoder.Decode(&answer); err != nil {
		return domain.UserAnswer{}, fmt.Errorf("decode user answer: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.UserAnswer{}, errors.New("user answer payload contains trailing JSON")
	}
	if err := answer.Validate(); err != nil {
		return domain.UserAnswer{}, fmt.Errorf("validate user answer: %w", err)
	}
	if string(answer.QuestionID) != event.CorrelationID || answer.ActorID != event.SourceActorID || answer.Channel != event.Source || answer.TransportEventID != event.DeduplicationKey {
		return domain.UserAnswer{}, errors.New("user answer payload disagrees with authenticated transport envelope")
	}
	if answer.TransportMessageID != event.TransportMessageID {
		return domain.UserAnswer{}, errors.New("user answer transport message ID disagrees with envelope")
	}
	return answer, nil
}
