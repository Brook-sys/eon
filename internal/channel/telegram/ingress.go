package telegram

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// IngressMode selects how Telegram updates enter the process.
type IngressMode string

const (
	// IngressNone disables inbound update collection. Delivery still works.
	IngressNone IngressMode = "none"
	// IngressPoll uses Bot API getUpdates (long poll) from the control cycle.
	IngressPoll IngressMode = "poll"
	// IngressWebhook accepts validated POSTs on a local HTTP path.
	IngressWebhook IngressMode = "webhook"
)

// EventSubmitter is the narrow inbox surface used by ingress. Implemented by
// control.ExternalEventInbox. Ingress never advances dispositions itself.
type EventSubmitter interface {
	SubmitExternalEvent(event domain.ExternalEvent) (domain.ExternalEventDisposition, error)
}

// IngressConfig configures non-authoritative inbound Telegram collection.
type IngressConfig struct {
	Mode IngressMode
	// PollLimit caps updates returned by one getUpdates call.
	PollLimit int
	// PollTimeout is the long-poll seconds requested from Telegram (0..50).
	PollTimeout int
	// WebhookPath is the HTTP path mounted on the process server (e.g. /telegram/webhook).
	WebhookPath string
	// WebhookSecret is compared to X-Telegram-Bot-Api-Secret-Token when non-empty.
	// Prefer loading from an env var at bootstrap; never store the raw token in durable config.
	WebhookSecret string
	// RejectUX enables answerCallbackQuery / short chat notices on allowlisted
	// failures. Disabled by default so tests and headless runs stay quiet.
	RejectUX bool
}

// Validate fills defaults and rejects unsafe combinations.
func (c *IngressConfig) Validate() error {
	if c == nil {
		return errors.New("telegram ingress config is required")
	}
	mode := IngressMode(strings.TrimSpace(string(c.Mode)))
	if mode == "" {
		mode = IngressNone
	}
	switch mode {
	case IngressNone, IngressPoll, IngressWebhook:
		c.Mode = mode
	default:
		return fmt.Errorf("unknown telegram ingress mode %q", c.Mode)
	}
	if c.PollLimit <= 0 {
		c.PollLimit = 20
	}
	if c.PollLimit > 100 {
		return errors.New("telegram poll limit is capped at 100")
	}
	if c.PollTimeout < 0 {
		return errors.New("telegram poll timeout must be non-negative")
	}
	if c.PollTimeout > 50 {
		return errors.New("telegram poll timeout is capped at 50 seconds")
	}
	if c.Mode == IngressWebhook {
		path := strings.TrimSpace(c.WebhookPath)
		if path == "" {
			path = "/telegram/webhook"
		}
		if !strings.HasPrefix(path, "/") {
			return errors.New("telegram webhook path must start with /")
		}
		if strings.Contains(path, "..") {
			return errors.New("telegram webhook path must not contain ..")
		}
		c.WebhookPath = path
	}
	return nil
}

// IngressResult summarizes one poll/webhook handling pass for diagnostics.
type IngressResult struct {
	Fetched   int
	Accepted  int
	Rejected  int
	Duplicate int
	// NextOffset is the Bot API offset to use after a successful poll batch.
	// Zero when no updates were observed.
	NextOffset int64
}

// Ingress owns process-local offset state and converts updates into inbox events.
// It never mutates domain authority beyond durable inbox submission.
type Ingress struct {
	Adapter  *Adapter
	Store    port.Store
	Events   EventSubmitter
	IDs      source.IDGenerator
	Clock    source.Clock
	Config   IngressConfig
	RejectUX bool

	mu     sync.Mutex
	offset int64 // next getUpdates offset (last seen update_id + 1)
}

// NewIngress validates config and returns a ready ingress. Mode none is valid
// and yields a no-op Poll/Handler when called.
func NewIngress(adapter *Adapter, store port.Store, events EventSubmitter, ids source.IDGenerator, clock source.Clock, config IngressConfig) (*Ingress, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Mode == IngressNone {
		return &Ingress{Config: config}, nil
	}
	if adapter == nil || store == nil || events == nil || ids == nil || clock == nil {
		return nil, errors.New("telegram ingress requires adapter, store, events, IDs, and clock")
	}
	return &Ingress{
		Adapter:  adapter,
		Store:    store,
		Events:   events,
		IDs:      ids,
		Clock:    clock,
		Config:   config,
		RejectUX: config.RejectUX,
	}, nil
}

// Poll performs one non-authoritative getUpdates batch and submits accepted
// answers. Safe no-op when mode is not poll.
func (in *Ingress) Poll(ctx context.Context) (IngressResult, error) {
	var result IngressResult
	if in == nil || in.Config.Mode != IngressPoll {
		return result, nil
	}
	if in.Adapter == nil || in.Store == nil || in.Events == nil || in.IDs == nil || in.Clock == nil {
		return result, errors.New("telegram poll ingress is incompletely configured")
	}
	if ctx == nil {
		return result, errors.New("context is required")
	}
	in.mu.Lock()
	offset := in.offset
	in.mu.Unlock()

	updates, err := in.Adapter.GetUpdates(ctx, offset, in.Config.PollLimit, in.Config.PollTimeout)
	if err != nil {
		return result, err
	}
	result.Fetched = len(updates)
	var maxID int64
	for _, update := range updates {
		if update.UpdateID > maxID {
			maxID = update.UpdateID
		}
		outcome, handleErr := in.handleUpdate(ctx, update)
		switch outcome {
		case ingressAccepted:
			result.Accepted++
		case ingressDuplicate:
			result.Duplicate++
		case ingressRejected:
			result.Rejected++
		}
		if handleErr != nil {
			// Transport/store failures abort the batch; offset is not advanced so
			// Telegram redelivers. Per-update rejections are not handleErr.
			return result, handleErr
		}
	}
	if maxID > 0 {
		next := maxID + 1
		in.mu.Lock()
		if next > in.offset {
			in.offset = next
		}
		result.NextOffset = in.offset
		in.mu.Unlock()
	} else {
		in.mu.Lock()
		result.NextOffset = in.offset
		in.mu.Unlock()
	}
	return result, nil
}

// Handler returns an HTTP handler for webhook mode. Nil when not webhook.
// Secret comparison is constant-time when a secret is configured.
func (in *Ingress) Handler() http.Handler {
	if in == nil || in.Config.Mode != IngressWebhook {
		return nil
	}
	return http.HandlerFunc(in.serveWebhook)
}

func (in *Ingress) serveWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if secret := strings.TrimSpace(in.Config.WebhookSecret); secret != "" {
		got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, defaultMaxUpdateBytes+1))
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}
	if int64(len(body)) > defaultMaxUpdateBytes {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	update, err := DecodeUpdate(body)
	if err != nil {
		// Acknowledge malformed payloads so Telegram does not retry forever;
		// they cannot become authority without correlation.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}
	if _, err := in.handleUpdate(r.Context(), update); err != nil {
		// Durable store errors should be retried by Telegram.
		http.Error(w, "temporary failure", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

type ingressOutcome int

const (
	ingressRejected ingressOutcome = iota
	ingressAccepted
	ingressDuplicate
)

func (in *Ingress) handleUpdate(ctx context.Context, update Update) (ingressOutcome, error) {
	receivedAt := in.Clock.Now().UTC()
	event, err := in.Adapter.IngestUpdate(ctx, in.Store, update, in.IDs, receivedAt)
	if err != nil {
		in.notifyRejection(ctx, update, err)
		return ingressRejected, nil
	}
	// Capture whether the event already existed so diagnostics distinguish
	// first accept from pure dedupe replay.
	var existed bool
	if err := in.Store.View(ctx, func(r port.Reader) error {
		if _, lookupErr := r.ExternalEventByDeduplicationKey(event.DeduplicationKey); lookupErr == nil {
			existed = true
			return nil
		} else if errors.Is(lookupErr, port.ErrNotFound) {
			return nil
		} else {
			return lookupErr
		}
	}); err != nil {
		return ingressRejected, err
	}
	if _, err := in.Events.SubmitExternalEvent(event); err != nil {
		// Conflict on divergent payload is a rejection; other store errors bubble.
		if errors.Is(err, port.ErrConflict) {
			in.notifyRejection(ctx, update, err)
			return ingressRejected, nil
		}
		return ingressRejected, err
	}
	if in.RejectUX && update.CallbackQuery != nil && update.CallbackQuery.ID != "" {
		// Best-effort UX; failure must not fail the durable accept path.
		_ = in.Adapter.AnswerCallbackQuery(ctx, update.CallbackQuery.ID, "Received", false)
	}
	if existed {
		return ingressDuplicate, nil
	}
	return ingressAccepted, nil
}

func (in *Ingress) notifyRejection(ctx context.Context, update Update, cause error) {
	if in == nil || !in.RejectUX || in.Adapter == nil {
		return
	}
	msg := rejectionMessage(cause)
	if update.CallbackQuery != nil && update.CallbackQuery.ID != "" {
		_ = in.Adapter.AnswerCallbackQuery(ctx, update.CallbackQuery.ID, msg, true)
		return
	}
	// Only notify chats that are already allowlisted; never probe unknown chats.
	chatID := updateChatID(update)
	if chatID == 0 {
		return
	}
	if _, ok := in.Adapter.chats[chatID]; !ok {
		return
	}
	// Bound notices: free-text replies without correlation get a short hint.
	if update.Message != nil {
		_ = in.Adapter.NotifyChat(ctx, chatID, msg)
	}
}

func updateChatID(update Update) int64 {
	if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
		return update.CallbackQuery.Message.Chat.ID
	}
	if update.Message != nil {
		return update.Message.Chat.ID
	}
	return 0
}

func rejectionMessage(err error) string {
	var adapterErr *Error
	if errors.As(err, &adapterErr) {
		switch adapterErr.Kind {
		case ErrorUnauthorized:
			return "Not authorized for this bot."
		case ErrorUncorrelated:
			return "Reply must use the question buttons or a direct reply to the bot message."
		case ErrorInvalidConfig:
			return "Channel configuration rejected this update."
		}
	}
	if errors.Is(err, port.ErrConflict) {
		return "Conflicting answer payload was rejected."
	}
	// Avoid leaking internal error strings to the operator chat.
	return "Update was not accepted."
}

// Offset returns the next getUpdates offset (for tests/diagnostics).
func (in *Ingress) Offset() int64 {
	if in == nil {
		return 0
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.offset
}

// SetOffset seeds the poll offset (tests or restore). Non-positive values are ignored.
func (in *Ingress) SetOffset(offset int64) {
	if in == nil || offset <= 0 {
		return
	}
	in.mu.Lock()
	in.offset = offset
	in.mu.Unlock()
}
