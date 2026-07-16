package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func seedDeliveredQuestion(t *testing.T, store port.Store, now time.Time) domain.OperatorQuestion {
	t.Helper()
	question := testQuestion(now)
	delivery := domain.QuestionDelivery{
		SchemaVersion: 1, ID: "delivery_ingress_1", QuestionID: question.ID, QuestionRevision: 1,
		Channel: ChannelName, DestinationRef: "operator_primary", Status: domain.QuestionDeliveryPending,
		MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		mission := domain.MissionRevision{
			SchemaVersion: 1, ID: question.MissionRevision, MissionID: question.MissionID, Revision: 1,
			OriginalText: "test mission", Purpose: "test", Status: domain.MissionActive, Provenance: "test", AcceptedAt: now,
		}
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.CreateOperatorQuestion(question); err != nil {
			return err
		}
		if err := tx.CreateQuestionDelivery(delivery); err != nil {
			return err
		}
		leased, err := domain.LeaseQuestionDelivery(delivery, "worker", now, now.Add(time.Minute))
		if err != nil {
			return err
		}
		if err := tx.SaveQuestionDelivery(leased, delivery.Status, delivery.Attempt); err != nil {
			return err
		}
		completed, err := domain.CompleteQuestionDelivery(leased, "worker", "42", now.Add(time.Second))
		if err != nil {
			return err
		}
		return tx.SaveQuestionDelivery(completed, leased.Status, leased.Attempt)
	}); err != nil {
		t.Fatal(err)
	}
	return question
}

func TestIngressConfigValidate(t *testing.T) {
	t.Parallel()
	cfg := IngressConfig{}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != IngressNone || cfg.PollLimit != 20 {
		t.Fatalf("defaults = %#v", cfg)
	}
	cfg = IngressConfig{Mode: "weird"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unknown mode error")
	}
	cfg = IngressConfig{Mode: IngressWebhook, WebhookPath: "relative"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected path error")
	}
	cfg = IngressConfig{Mode: IngressWebhook}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.WebhookPath != "/telegram/webhook" {
		t.Fatalf("path = %q", cfg.WebhookPath)
	}
}

func TestIngressPollAcceptsCorrelatedCallbackAndAdvancesOffset(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	var gotPath string
	var gotBody getUpdatesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		if strings.HasSuffix(r.URL.Path, "/getUpdates") {
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":55,"callback_query":{"id":"cb_poll","from":{"id":7},"message":{"message_id":42,"chat":{"id":100}},"data":"o:1"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()

	adapter := testAdapter(t, server)
	store := memory.New()
	_ = seedDeliveredQuestion(t, store, now)
	inbox, err := control.NewExternalEventInbox(store, control.DispositionFactoryFrom(source.NewManualClock(now.Add(time.Minute))))
	if err != nil {
		t.Fatal(err)
	}
	ingress, err := NewIngress(adapter, store, inbox, source.NewSequenceIDGenerator(1), source.NewManualClock(now.Add(time.Minute)), IngressConfig{
		Mode: IngressPoll, PollLimit: 10, PollTimeout: 0, RejectUX: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ingress.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 1 || result.Accepted != 1 || result.Rejected != 0 || result.NextOffset != 56 {
		t.Fatalf("result = %#v", result)
	}
	if !strings.HasSuffix(gotPath, "/getUpdates") || gotBody.Limit != 10 {
		t.Fatalf("path=%q body=%#v", gotPath, gotBody)
	}
	if ingress.Offset() != 56 {
		t.Fatalf("offset = %d", ingress.Offset())
	}
	// Replay of the same getUpdates payload should count as duplicate once offset is not advanced by Telegram,
	// but our process advances offset so a second poll with empty result is idle.
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer server2.Close()
	adapter2 := testAdapter(t, server2)
	ingress.Adapter = adapter2
	result, err = ingress.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 0 || result.Accepted != 0 || ingress.Offset() != 56 {
		t.Fatalf("idle poll = %#v offset=%d", result, ingress.Offset())
	}
}

func TestIngressPollRejectsUncorrelatedAndNotifiesCallback(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	var answered atomic.Bool
	var answerText string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":9,"callback_query":{"id":"cb_bad","from":{"id":7},"message":{"message_id":99,"chat":{"id":100}},"data":"o:0"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/answerCallbackQuery"):
			answered.Store(true)
			var req answerCallbackRequest
			_ = json.Unmarshal(body, &req)
			answerText = req.Text
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		}
	}))
	defer server.Close()

	adapter := testAdapter(t, server)
	store := memory.New()
	_ = seedDeliveredQuestion(t, store, now)
	inbox, err := control.NewExternalEventInbox(store, control.DispositionFactoryFrom(source.NewManualClock(now)))
	if err != nil {
		t.Fatal(err)
	}
	ingress, err := NewIngress(adapter, store, inbox, source.NewSequenceIDGenerator(1), source.NewManualClock(now), IngressConfig{
		Mode: IngressPoll, RejectUX: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ingress.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Rejected != 1 || result.Accepted != 0 || result.NextOffset != 10 {
		t.Fatalf("result = %#v", result)
	}
	if !answered.Load() || answerText == "" {
		t.Fatalf("expected rejection callback UX, text=%q", answerText)
	}
}

func TestIngressWebhookValidatesSecretAndAcceptsUpdate(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 30, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()
	adapter := testAdapter(t, server)
	store := memory.New()
	_ = seedDeliveredQuestion(t, store, now)
	inbox, err := control.NewExternalEventInbox(store, control.DispositionFactoryFrom(source.NewManualClock(now.Add(time.Minute))))
	if err != nil {
		t.Fatal(err)
	}
	ingress, err := NewIngress(adapter, store, inbox, source.NewSequenceIDGenerator(1), source.NewManualClock(now.Add(time.Minute)), IngressConfig{
		Mode: IngressWebhook, WebhookPath: "/telegram/webhook", WebhookSecret: "hook-secret", RejectUX: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := ingress.Handler()
	if handler == nil {
		t.Fatal("expected webhook handler")
	}

	payload := []byte(`{"update_id":70,"callback_query":{"id":"cb_hook","from":{"id":7},"message":{"message_id":42,"chat":{"id":100}},"data":"o:0"}}`)
	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(string(payload)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing secret status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(string(payload)))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "hook-secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("accept status = %d body=%s", rec.Code, rec.Body.String())
	}
	var pending []domain.ExternalEvent
	if err := store.View(context.Background(), func(r port.Reader) error {
		var err error
		pending, err = r.PendingExternalEvents(10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].CorrelationID != "ask_1" {
		t.Fatalf("pending = %#v", pending)
	}
}

func TestIngressWebhookRejectsWithoutBindingQuietly(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 45, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()
	adapter := testAdapter(t, server)
	store := memory.New()
	inbox, err := control.NewExternalEventInbox(store, control.DispositionFactoryFrom(source.NewManualClock(now)))
	if err != nil {
		t.Fatal(err)
	}
	ingress, err := NewIngress(adapter, store, inbox, source.NewSequenceIDGenerator(1), source.NewManualClock(now), IngressConfig{
		Mode: IngressWebhook, WebhookSecret: "s",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Uncorrelated update: still 200 so Telegram does not spin, and no event stored.
	payload := []byte(`{"update_id":1,"callback_query":{"id":"x","from":{"id":7},"message":{"message_id":1,"chat":{"id":100}},"data":"o:0"}}`)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(payload)))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "s")
	rec := httptest.NewRecorder()
	ingress.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
