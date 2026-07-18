// Package telegram implements a non-authoritative Telegram Bot API channel.
// It renders canonical operator questions from the durable outbox and converts
// authenticated, correlated updates into untrusted ExternalEvent records.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

const (
	ChannelName           = "telegram"
	defaultMaxResponse    = int64(1 << 20)
	defaultMaxUpdateBytes = int64(1 << 20)
	defaultClientTimeout  = 60 * time.Second
)

type Config struct {
	Token         string
	BaseURL       string
	Client        *http.Client
	MaxResponse   int64
	Destinations  map[string]int64 // durable destination_ref -> Telegram chat_id
	AllowedActors map[int64]string // Telegram user_id -> canonical actor_id
	AllowedChats  map[int64]struct{}
}

type Adapter struct {
	apiRoot        string // https://api.telegram.org/bot<token>
	client         *http.Client
	maxResponse    int64
	destinations   map[string]int64
	destinationRef map[int64]string
	actors         map[int64]string
	chats          map[int64]struct{}
}

type ErrorKind string

const (
	ErrorInvalidConfig ErrorKind = "INVALID_CONFIG"
	ErrorUnauthorized  ErrorKind = "UNAUTHORIZED"
	ErrorUncorrelated  ErrorKind = "UNCORRELATED"
	ErrorTransport     ErrorKind = "TRANSPORT"
	ErrorHTTP          ErrorKind = "HTTP"
	ErrorInvalidReply  ErrorKind = "INVALID_REPLY"
	ErrorTooLarge      ErrorKind = "TOO_LARGE"
)

type Error struct {
	Kind       ErrorKind
	StatusCode int
	Retryable  bool
}

func (e *Error) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("telegram adapter: %s (status %d)", e.Kind, e.StatusCode)
	}
	return "telegram adapter: " + string(e.Kind)
}

func New(config Config) (*Adapter, error) {
	if strings.TrimSpace(config.Token) == "" || len(config.Destinations) == 0 || len(config.AllowedActors) == 0 || len(config.AllowedChats) == 0 {
		return nil, errors.New("telegram token, destinations, actor allowlist, and chat allowlist are required")
	}
	base := strings.TrimSpace(config.BaseURL)
	if base == "" {
		base = "https://api.telegram.org"
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("telegram base URL must be absolute HTTP(S)")
	}
	limit := config.MaxResponse
	if limit == 0 {
		limit = defaultMaxResponse
	}
	if limit < 1 {
		return nil, errors.New("telegram max response must be positive")
	}
	client := config.Client
	if client == nil {
		// The default exceeds Telegram's maximum 50-second long-poll while still
		// bounding stalled connects, headers, and bodies. Deployments needing a
		// different transport policy can inject their own client explicitly.
		client = &http.Client{Timeout: defaultClientTimeout}
	}
	destinations := make(map[string]int64, len(config.Destinations))
	reverse := make(map[int64]string, len(config.Destinations))
	for ref, chatID := range config.Destinations {
		if strings.TrimSpace(ref) == "" || chatID == 0 {
			return nil, errors.New("telegram destination contains empty reference or chat ID")
		}
		if _, duplicate := reverse[chatID]; duplicate {
			return nil, errors.New("telegram chat ID maps to multiple destination references")
		}
		destinations[ref], reverse[chatID] = chatID, ref
	}
	actors := make(map[int64]string, len(config.AllowedActors))
	for userID, actorID := range config.AllowedActors {
		if userID == 0 || strings.TrimSpace(actorID) == "" {
			return nil, errors.New("telegram actor allowlist contains empty identity")
		}
		actors[userID] = actorID
	}
	chats := make(map[int64]struct{}, len(config.AllowedChats))
	for chatID := range config.AllowedChats {
		if chatID == 0 {
			return nil, errors.New("telegram chat allowlist contains zero ID")
		}
		chats[chatID] = struct{}{}
	}
	// Keep token only in the process-local API root; method paths are appended later.
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/bot" + config.Token
	parsed.RawQuery, parsed.Fragment = "", ""
	return &Adapter{
		apiRoot: strings.TrimRight(parsed.String(), "/"), client: client, maxResponse: limit,
		destinations: destinations, destinationRef: reverse, actors: actors, chats: chats,
	}, nil
}

func (a *Adapter) methodURL(method string) string {
	if a == nil {
		return ""
	}
	return a.apiRoot + "/" + strings.TrimPrefix(method, "/")
}

type inlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

type forceReply struct {
	ForceReply bool `json:"force_reply"`
	Selective  bool `json:"selective"`
}

type sendMessageRequest struct {
	ChatID      int64  `json:"chat_id"`
	Text        string `json:"text"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

type apiResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		MessageID int64 `json:"message_id"`
	} `json:"result"`
	Parameters struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (a *Adapter) SendQuestion(ctx context.Context, destinationRef string, question domain.OperatorQuestion) (string, error) {
	if err := question.Validate(); err != nil {
		return "", fmt.Errorf("validate Telegram question: %w", err)
	}
	// Reminder outbox routes use DestinationRef "primary#reminder:N"; map them
	// back to the operator chat without inventing a second destination registry.
	primary := domain.PrimaryDestinationRef(destinationRef)
	chatID, ok := a.destinations[primary]
	if !ok {
		return "", &Error{Kind: ErrorInvalidConfig}
	}
	text := question.Prompt
	if question.Context != "" {
		text += "\n\nContext: " + question.Context
	}
	request := sendMessageRequest{ChatID: chatID, Text: text}
	switch question.Kind {
	case domain.QuestionSingleChoice, domain.QuestionMultipleChoice, domain.QuestionSingleChoiceWithOther:
		rows := make([][]inlineButton, 0, len(question.Options)+2)
		for index, option := range question.Options {
			rows = append(rows, []inlineButton{{Text: option.Label, CallbackData: "o:" + strconv.Itoa(index)}})
		}
		if question.AllowSkip {
			rows = append(rows, []inlineButton{{Text: "Skip", CallbackData: "skip"}})
		}
		if question.AllowContext {
			rows = append(rows, []inlineButton{{Text: "Need context", CallbackData: "context"}})
		}
		request.ReplyMarkup = inlineKeyboard{InlineKeyboard: rows}
	case domain.QuestionConfirmation:
		rows := [][]inlineButton{{{Text: "Confirm", CallbackData: "confirm"}, {Text: "Decline", CallbackData: "decline"}}}
		if question.AllowSkip {
			rows = append(rows, []inlineButton{{Text: "Skip", CallbackData: "skip"}})
		}
		request.ReplyMarkup = inlineKeyboard{InlineKeyboard: rows}
	case domain.QuestionFreeText, domain.QuestionClarification:
		request.ReplyMarkup = forceReply{ForceReply: true, Selective: true}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.methodURL("sendMessage"), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(httpRequest)
	if err != nil {
		return "", &Error{Kind: ErrorTransport, Retryable: true}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, a.maxResponse+1))
	if err != nil {
		return "", &Error{Kind: ErrorTransport, Retryable: true}
	}
	if int64(len(body)) > a.maxResponse {
		return "", &Error{Kind: ErrorTooLarge}
	}
	var decoded apiResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", &Error{Kind: ErrorInvalidReply}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !decoded.OK {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return "", &Error{Kind: ErrorHTTP, StatusCode: response.StatusCode, Retryable: retryable}
	}
	if decoded.Result.MessageID == 0 {
		return "", &Error{Kind: ErrorInvalidReply}
	}
	return strconv.FormatInt(decoded.Result.MessageID, 10), nil
}

type getUpdatesRequest struct {
	Offset         int64    `json:"offset,omitempty"`
	Limit          int      `json:"limit,omitempty"`
	Timeout        int      `json:"timeout,omitempty"`
	AllowedUpdates []string `json:"allowed_updates,omitempty"`
}

type getUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

// WebhookConfig describes a remote Bot API setWebhook registration. Secrets and
// URL material stay process-local: the kernel never owns webhook lifecycle.
type WebhookConfig struct {
	URL            string
	SecretToken    string
	MaxConnections int
	// DropPendingUpdates asks Telegram to discard queued updates when registering.
	DropPendingUpdates bool
	AllowedUpdates     []string
}

type setWebhookRequest struct {
	URL                string   `json:"url"`
	SecretToken        string   `json:"secret_token,omitempty"`
	MaxConnections     int      `json:"max_connections,omitempty"`
	DropPendingUpdates bool     `json:"drop_pending_updates,omitempty"`
	AllowedUpdates     []string `json:"allowed_updates,omitempty"`
}

type deleteWebhookRequest struct {
	DropPendingUpdates bool `json:"drop_pending_updates,omitempty"`
}

type boolAPIResponse struct {
	OK          bool   `json:"ok"`
	Result      bool   `json:"result"`
	Description string `json:"description,omitempty"`
}

// SetWebhook registers the process webhook URL with Telegram. Non-authoritative:
// it only configures transport delivery; inbox correlation still applies.
func (a *Adapter) SetWebhook(ctx context.Context, config WebhookConfig) error {
	if a == nil {
		return errors.New("telegram adapter is nil")
	}
	urlValue := strings.TrimSpace(config.URL)
	if urlValue == "" {
		return &Error{Kind: ErrorInvalidConfig}
	}
	if !strings.HasPrefix(urlValue, "https://") {
		return &Error{Kind: ErrorInvalidConfig}
	}
	maxConn := config.MaxConnections
	if maxConn < 0 {
		return &Error{Kind: ErrorInvalidConfig}
	}
	if maxConn > 100 {
		maxConn = 100
	}
	allowed := config.AllowedUpdates
	if len(allowed) == 0 {
		allowed = []string{"message", "callback_query"}
	}
	payload, err := json.Marshal(setWebhookRequest{
		URL:                urlValue,
		SecretToken:        strings.TrimSpace(config.SecretToken),
		MaxConnections:     maxConn,
		DropPendingUpdates: config.DropPendingUpdates,
		AllowedUpdates:     allowed,
	})
	if err != nil {
		return err
	}
	return a.postBoolAPI(ctx, "setWebhook", payload)
}

// DeleteWebhook removes the remote webhook so getUpdates can be used again.
func (a *Adapter) DeleteWebhook(ctx context.Context, dropPending bool) error {
	if a == nil {
		return errors.New("telegram adapter is nil")
	}
	payload, err := json.Marshal(deleteWebhookRequest{DropPendingUpdates: dropPending})
	if err != nil {
		return err
	}
	return a.postBoolAPI(ctx, "deleteWebhook", payload)
}

func (a *Adapter) postBoolAPI(ctx context.Context, method string, payload []byte) error {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.methodURL(method), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(httpRequest)
	if err != nil {
		return &Error{Kind: ErrorTransport, Retryable: true}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, a.maxResponse+1))
	if err != nil {
		return &Error{Kind: ErrorTransport, Retryable: true}
	}
	if int64(len(body)) > a.maxResponse {
		return &Error{Kind: ErrorTooLarge}
	}
	var decoded boolAPIResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return &Error{Kind: ErrorInvalidReply}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !decoded.OK || !decoded.Result {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return &Error{Kind: ErrorHTTP, StatusCode: response.StatusCode, Retryable: retryable}
	}
	return nil
}

// GetUpdates performs one Bot API getUpdates call. Timeout is the long-poll
// duration requested from Telegram (seconds); the HTTP client must tolerate it.
func (a *Adapter) GetUpdates(ctx context.Context, offset int64, limit int, timeoutSeconds int) ([]Update, error) {
	if a == nil {
		return nil, errors.New("telegram adapter is nil")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if timeoutSeconds < 0 {
		timeoutSeconds = 0
	}
	if timeoutSeconds > 50 {
		timeoutSeconds = 50
	}
	payload, err := json.Marshal(getUpdatesRequest{
		Offset:         offset,
		Limit:          limit,
		Timeout:        timeoutSeconds,
		AllowedUpdates: []string{"message", "callback_query"},
	})
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.methodURL("getUpdates"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(httpRequest)
	if err != nil {
		return nil, &Error{Kind: ErrorTransport, Retryable: true}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, a.maxResponse+1))
	if err != nil {
		return nil, &Error{Kind: ErrorTransport, Retryable: true}
	}
	if int64(len(body)) > a.maxResponse {
		return nil, &Error{Kind: ErrorTooLarge}
	}
	var decoded getUpdatesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, &Error{Kind: ErrorInvalidReply}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !decoded.OK {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return nil, &Error{Kind: ErrorHTTP, StatusCode: response.StatusCode, Retryable: retryable}
	}
	return decoded.Result, nil
}

type answerCallbackRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

// AnswerCallbackQuery acknowledges a callback so the client spinner stops and
// optionally surfaces a short non-authoritative UX string.
func (a *Adapter) AnswerCallbackQuery(ctx context.Context, callbackQueryID, text string, showAlert bool) error {
	if a == nil {
		return errors.New("telegram adapter is nil")
	}
	if strings.TrimSpace(callbackQueryID) == "" {
		return &Error{Kind: ErrorInvalidConfig}
	}
	payload, err := json.Marshal(answerCallbackRequest{
		CallbackQueryID: callbackQueryID,
		Text:            text,
		ShowAlert:       showAlert,
	})
	if err != nil {
		return err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.methodURL("answerCallbackQuery"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(httpRequest)
	if err != nil {
		return &Error{Kind: ErrorTransport, Retryable: true}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, a.maxResponse+1))
	if err != nil {
		return &Error{Kind: ErrorTransport, Retryable: true}
	}
	if int64(len(body)) > a.maxResponse {
		return &Error{Kind: ErrorTooLarge}
	}
	var decoded struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return &Error{Kind: ErrorInvalidReply}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !decoded.OK {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return &Error{Kind: ErrorHTTP, StatusCode: response.StatusCode, Retryable: retryable}
	}
	return nil
}

// NotifyChat sends a bounded, non-authoritative operator notice. Used only for
// rejection UX on allowlisted chats; never carries domain authority.
func (a *Adapter) NotifyChat(ctx context.Context, chatID int64, text string) error {
	if a == nil {
		return errors.New("telegram adapter is nil")
	}
	if chatID == 0 || strings.TrimSpace(text) == "" {
		return &Error{Kind: ErrorInvalidConfig}
	}
	if len(text) > 500 {
		text = text[:500]
	}
	payload, err := json.Marshal(sendMessageRequest{ChatID: chatID, Text: text})
	if err != nil {
		return err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.methodURL("sendMessage"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(httpRequest)
	if err != nil {
		return &Error{Kind: ErrorTransport, Retryable: true}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, a.maxResponse+1))
	if err != nil {
		return &Error{Kind: ErrorTransport, Retryable: true}
	}
	if int64(len(body)) > a.maxResponse {
		return &Error{Kind: ErrorTooLarge}
	}
	var decoded apiResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return &Error{Kind: ErrorInvalidReply}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !decoded.OK {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return &Error{Kind: ErrorHTTP, StatusCode: response.StatusCode, Retryable: retryable}
	}
	return nil
}

// DeliveryWorker leases durable outbox records before performing the external
// effect and records only bounded transport evidence afterward.
type DeliveryWorker struct {
	Store         port.Store
	Adapter       *Adapter
	Clock         source.Clock
	Owner         string
	LeaseDuration time.Duration
	RetryDelay    time.Duration
}

// IngestUpdate binds one decoded update to a durable delivered outbox row via
// transport message id, then converts it to an untrusted ExternalEvent.
// The caller owns inbox submission; this method never mutates domain authority.
func (a *Adapter) IngestUpdate(ctx context.Context, store port.Store, update Update, ids source.IDGenerator, receivedAt time.Time) (domain.ExternalEvent, error) {
	if a == nil || store == nil || ids == nil {
		return domain.ExternalEvent{}, errors.New("telegram ingest requires adapter, store, and IDs")
	}
	boundID := BoundTransportMessageID(update)
	if boundID == 0 {
		return domain.ExternalEvent{}, &Error{Kind: ErrorUncorrelated}
	}
	transportKey := strconv.FormatInt(boundID, 10)
	var delivery domain.QuestionDelivery
	var question domain.OperatorQuestion
	err := store.View(ctx, func(r port.Reader) error {
		var err error
		delivery, err = r.QuestionDeliveryByTransport(ChannelName, transportKey)
		if err != nil {
			return err
		}
		question, err = r.OperatorQuestion(delivery.QuestionID)
		return err
	})
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			return domain.ExternalEvent{}, &Error{Kind: ErrorUncorrelated}
		}
		return domain.ExternalEvent{}, err
	}
	return a.ExternalAnswer(update, question, delivery, ids, receivedAt)
}

func (w *DeliveryWorker) ProcessDue(ctx context.Context, limit int) (int, error) {
	if w.Store == nil || w.Adapter == nil || w.Clock == nil || strings.TrimSpace(w.Owner) == "" || w.LeaseDuration <= 0 || w.RetryDelay <= 0 || limit <= 0 {
		return 0, errors.New("telegram delivery worker is incompletely configured")
	}
	now := w.Clock.Now().UTC()
	var due []domain.QuestionDelivery
	if err := w.Store.View(ctx, func(r port.Reader) error { var err error; due, err = r.DueQuestionDeliveries(now, limit); return err }); err != nil {
		return 0, err
	}
	processed := 0
	for _, candidate := range due {
		if candidate.Channel != ChannelName {
			continue
		}
		// Expired leases surface as due LEASED items. Park them as EFFECT_UNKNOWN
		// and never re-lease in the same pass — re-send requires explicit reconcile.
		if candidate.Status == domain.QuestionDeliveryLeased {
			parked, err := domain.ReclaimExpiredQuestionDelivery(candidate, now)
			if err != nil {
				return processed, err
			}
			if err := w.Store.Update(ctx, func(tx port.Transaction) error {
				return tx.SaveQuestionDelivery(parked, candidate.Status, candidate.Attempt)
			}); err != nil {
				if errors.Is(err, port.ErrConflict) {
					continue
				}
				return processed, err
			}
			processed++
			continue
		}
		if candidate.Status.RequiresReconciliation() {
			// Defensive: due queries must not return EFFECT_UNKNOWN, but skip if they do.
			continue
		}
		leased, err := domain.LeaseQuestionDelivery(candidate, w.Owner, now, now.Add(w.LeaseDuration))
		if err != nil {
			return processed, err
		}
		var question domain.OperatorQuestion
		if err := w.Store.Update(ctx, func(tx port.Transaction) error {
			var err error
			question, err = tx.OperatorQuestion(candidate.QuestionID)
			if err != nil {
				return err
			}
			return tx.SaveQuestionDelivery(leased, candidate.Status, candidate.Attempt)
		}); err != nil {
			if errors.Is(err, port.ErrConflict) {
				continue
			}
			return processed, err
		}
		messageID, sendErr := w.Adapter.SendQuestion(ctx, candidate.DestinationRef, question)
		finishedAt := w.Clock.Now().UTC()
		var final domain.QuestionDelivery
		if sendErr == nil {
			final, err = domain.CompleteQuestionDelivery(leased, w.Owner, messageID, finishedAt)
		} else {
			code, retryable, ambiguous := classifyFailure(sendErr)
			switch {
			case ambiguous:
				// Timeout/truncated reply after the request may already have produced a message.
				final, err = domain.MarkAmbiguousTransportAfterSend(leased, w.Owner, finishedAt)
			case retryable:
				final, err = domain.FailQuestionDelivery(leased, w.Owner, code, finishedAt, finishedAt.Add(w.RetryDelay))
			default:
				final, err = domain.PermanentlyFailQuestionDelivery(leased, w.Owner, code, finishedAt)
			}
		}
		if err != nil {
			return processed, err
		}
		if err := w.Store.Update(ctx, func(tx port.Transaction) error { return tx.SaveQuestionDelivery(final, leased.Status, leased.Attempt) }); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func classifyFailure(err error) (code string, retryable bool, ambiguous bool) {
	var adapterError *Error
	if errors.As(err, &adapterError) {
		switch adapterError.Kind {
		case ErrorTransport:
			// Network/timeout after the HTTP request may already have delivered the message.
			return string(adapterError.Kind), false, true
		case ErrorInvalidReply:
			// Truncated/unparseable success body is also ambiguous.
			return string(adapterError.Kind), false, true
		default:
			return string(adapterError.Kind), adapterError.Retryable, false
		}
	}
	// Unknown errors after send started are treated as ambiguous to avoid silent duplicates.
	return "UNKNOWN", false, true
}
