package kernel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func answerExternalEvent(t *testing.T) domain.ExternalEvent {
	t.Helper()
	answer := domain.UserAnswer{
		SchemaVersion: domain.SchemaVersionV1, ID: "answer_1", QuestionID: "ask_1", ExpectedQuestionRevision: 1,
		Kind: domain.AnswerOptions, OptionIDs: []string{"a"}, ActorID: "operator_1", Channel: "telegram",
		TransportEventID: "telegram:update:42", TransportMessageID: "message_7", ReceivedAt: time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC),
	}
	payload, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	return domain.ExternalEvent{
		SchemaVersion: domain.SchemaVersionV1, ID: "external_1", DeduplicationKey: answer.TransportEventID, Source: answer.Channel,
		SourceActorID: answer.ActorID, Kind: domain.ExternalUserAnswer, MissionID: "mission_1", CorrelationID: string(answer.QuestionID),
		TransportMessageID: answer.TransportMessageID, Content: domain.ExternalContent{MediaType: "application/json", Structured: payload}, ReceivedAt: answer.ReceivedAt,
	}
}

func TestDecodeUserAnswerExternalEventBindsEnvelope(t *testing.T) {
	event := answerExternalEvent(t)
	answer, err := DecodeUserAnswerExternalEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if answer.ID != "answer_1" || answer.QuestionID != "ask_1" {
		t.Fatalf("answer = %#v", answer)
	}
}

func TestDecodeUserAnswerExternalEventFailsClosedOnEnvelopeMismatch(t *testing.T) {
	tests := []struct {
		name string
		edit func(*domain.ExternalEvent)
	}{
		{name: "actor", edit: func(e *domain.ExternalEvent) { e.SourceActorID = "other" }},
		{name: "channel", edit: func(e *domain.ExternalEvent) { e.Source = "dashboard" }},
		{name: "correlation", edit: func(e *domain.ExternalEvent) { e.CorrelationID = "ask_other" }},
		{name: "dedup", edit: func(e *domain.ExternalEvent) { e.DeduplicationKey = "update_other" }},
		{name: "message", edit: func(e *domain.ExternalEvent) { e.TransportMessageID = "message_other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := answerExternalEvent(t)
			test.edit(&event)
			if _, err := DecodeUserAnswerExternalEvent(event); err == nil {
				t.Fatal("mismatched envelope accepted")
			}
		})
	}
}

func TestDecodeUserAnswerExternalEventRejectsUnknownFieldsAndWrongKind(t *testing.T) {
	event := answerExternalEvent(t)
	event.Content.Structured = []byte(strings.TrimSuffix(string(event.Content.Structured), "}") + `,"authority":"APPLY"}`)
	if _, err := DecodeUserAnswerExternalEvent(event); err == nil {
		t.Fatal("unknown authority field accepted")
	}
	event = answerExternalEvent(t)
	event.Kind = domain.ExternalUserMessage
	if _, err := DecodeUserAnswerExternalEvent(event); err == nil {
		t.Fatal("wrong event kind accepted")
	}
}
