package telegram

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/runtime/source"
)

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type User struct {
	ID int64 `json:"id"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type Message struct {
	MessageID      int64    `json:"message_id"`
	From           *User    `json:"from,omitempty"`
	Chat           Chat     `json:"chat"`
	Text           string   `json:"text,omitempty"`
	ReplyToMessage *Message `json:"reply_to_message,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

func DecodeUpdate(data []byte) (Update, error) {
	if len(data) == 0 || int64(len(data)) > defaultMaxUpdateBytes {
		return Update{}, &Error{Kind: ErrorTooLarge}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var update Update
	if err := decoder.Decode(&update); err != nil {
		return Update{}, fmt.Errorf("decode Telegram update: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Update{}, errors.New("Telegram update contains trailing JSON")
	}
	if update.UpdateID == 0 || (update.Message == nil) == (update.CallbackQuery == nil) {
		return Update{}, errors.New("Telegram update requires exactly one supported payload")
	}
	return update, nil
}

// ExternalAnswer converts one allowlisted, explicitly correlated Telegram
// update into an untrusted USER_ANSWER event. The supplied delivery is the
// durable server-side message binding; callback_data contains only an action.
func (a *Adapter) ExternalAnswer(update Update, question domain.OperatorQuestion, delivery domain.QuestionDelivery, ids source.IDGenerator, receivedAt time.Time) (domain.ExternalEvent, error) {
	if ids == nil || receivedAt.IsZero() {
		return domain.ExternalEvent{}, errors.New("Telegram answer conversion requires IDs and receipt time")
	}
	if err := question.Validate(); err != nil {
		return domain.ExternalEvent{}, err
	}
	if err := delivery.Validate(); err != nil {
		return domain.ExternalEvent{}, err
	}
	if delivery.Channel != ChannelName || delivery.Status != domain.QuestionDeliveryDelivered || delivery.QuestionID != question.ID || delivery.QuestionRevision != question.Revision {
		return domain.ExternalEvent{}, &Error{Kind: ErrorUncorrelated}
	}
	expectedChat, ok := a.destinations[delivery.DestinationRef]
	if !ok {
		return domain.ExternalEvent{}, &Error{Kind: ErrorInvalidConfig}
	}
	expectedMessageID, err := strconv.ParseInt(delivery.TransportMessageID, 10, 64)
	if err != nil || expectedMessageID == 0 {
		return domain.ExternalEvent{}, &Error{Kind: ErrorUncorrelated}
	}

	var userID, chatID, incomingMessageID int64
	var dedupKey string
	answer := domain.UserAnswer{SchemaVersion: domain.SchemaVersionV1, QuestionID: question.ID, ExpectedQuestionRevision: question.Revision, Channel: ChannelName, ReceivedAt: receivedAt.UTC()}
	if update.UpdateID == 0 || (update.Message == nil) == (update.CallbackQuery == nil) {
		return domain.ExternalEvent{}, errors.New("Telegram update requires exactly one supported payload")
	}
	if update.CallbackQuery != nil {
		callback := update.CallbackQuery
		if callback.ID == "" || callback.Message == nil {
			return domain.ExternalEvent{}, &Error{Kind: ErrorUncorrelated}
		}
		userID, chatID, incomingMessageID = callback.From.ID, callback.Message.Chat.ID, callback.Message.MessageID
		if incomingMessageID != expectedMessageID {
			return domain.ExternalEvent{}, &Error{Kind: ErrorUncorrelated}
		}
		dedupKey = "telegram:callback:" + callback.ID
		if err := bindCallback(&answer, question, callback.Data); err != nil {
			return domain.ExternalEvent{}, err
		}
	} else {
		message := update.Message
		if message.From == nil || message.ReplyToMessage == nil || message.ReplyToMessage.MessageID != expectedMessageID {
			return domain.ExternalEvent{}, &Error{Kind: ErrorUncorrelated}
		}
		userID, chatID, incomingMessageID = message.From.ID, message.Chat.ID, message.MessageID
		dedupKey = "telegram:update:" + strconv.FormatInt(update.UpdateID, 10)
		if question.Kind != domain.QuestionFreeText && question.Kind != domain.QuestionClarification && !question.AllowOther {
			return domain.ExternalEvent{}, &Error{Kind: ErrorUncorrelated}
		}
		if strings.TrimSpace(message.Text) == "" {
			return domain.ExternalEvent{}, errors.New("Telegram reply text is empty")
		}
		if question.Kind == domain.QuestionFreeText || question.Kind == domain.QuestionClarification {
			answer.Kind = domain.AnswerFreeText
		} else {
			answer.Kind = domain.AnswerOther
		}
		answer.Text = message.Text
	}
	actorID, allowedActor := a.actors[userID]
	_, allowedChat := a.chats[chatID]
	if !allowedActor || !allowedChat || chatID != expectedChat {
		return domain.ExternalEvent{}, &Error{Kind: ErrorUnauthorized}
	}
	answerID, err := ids.NewID("answer")
	if err != nil {
		return domain.ExternalEvent{}, err
	}
	eventID, err := ids.NewID("external_event")
	if err != nil {
		return domain.ExternalEvent{}, err
	}
	answer.ID = domain.OperatorAnswerID(answerID)
	answer.ActorID = actorID
	answer.TransportEventID = dedupKey
	answer.TransportMessageID = strconv.FormatInt(incomingMessageID, 10)
	if err := answer.ValidateForQuestion(question); err != nil {
		return domain.ExternalEvent{}, fmt.Errorf("validate Telegram answer: %w", err)
	}
	structured, err := json.Marshal(answer)
	if err != nil {
		return domain.ExternalEvent{}, err
	}
	event := domain.ExternalEvent{
		SchemaVersion: domain.SchemaVersionV1, ID: domain.ExternalEventID(eventID), DeduplicationKey: dedupKey,
		Source: ChannelName, SourceActorID: actorID, Kind: domain.ExternalUserAnswer, MissionID: question.MissionID,
		CorrelationID: string(question.ID), TransportMessageID: answer.TransportMessageID,
		Content: domain.ExternalContent{MediaType: "application/json", Structured: structured}, ReceivedAt: receivedAt.UTC(),
	}
	return event, event.Validate()
}

func bindCallback(answer *domain.UserAnswer, question domain.OperatorQuestion, data string) error {
	switch data {
	case "confirm":
		answer.Kind = domain.AnswerConfirm
	case "decline":
		answer.Kind = domain.AnswerDecline
	case "skip":
		answer.Kind = domain.AnswerSkip
	case "context":
		answer.Kind, answer.Text = domain.AnswerNeedContext, "Please provide more context."
	default:
		if !strings.HasPrefix(data, "o:") {
			return &Error{Kind: ErrorUncorrelated}
		}
		index, err := strconv.Atoi(strings.TrimPrefix(data, "o:"))
		if err != nil || index < 0 || index >= len(question.Options) {
			return &Error{Kind: ErrorUncorrelated}
		}
		answer.Kind, answer.OptionIDs = domain.AnswerOptions, []string{question.Options[index].ID}
	}
	return nil
}
