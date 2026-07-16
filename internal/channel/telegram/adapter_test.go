package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func testQuestion(now time.Time) domain.OperatorQuestion {
	return domain.OperatorQuestion{
		SchemaVersion: domain.SchemaVersionV1, ID: "ask_1", MissionID: "mission_1", MissionRevision: "revision_1", Revision: 1,
		Kind: domain.QuestionSingleChoice, Prompt: "Choose", Context: "Needed for artifact", Options: []domain.QuestionOption{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta"}},
		AllowContext: true, AllowSkip: true, FallbackPolicy: domain.QuestionContinueOtherWork, DedupSignature: "choose", Priority: 10,
		Status: domain.OperatorQuestionPending, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
}

func testAdapter(t *testing.T, server *httptest.Server) *Adapter {
	t.Helper()
	adapter, err := New(Config{Token: "secret", BaseURL: server.URL, Client: server.Client(), Destinations: map[string]int64{"operator_primary": 100}, AllowedActors: map[int64]string{7: "operator_1"}, AllowedChats: map[int64]struct{}{100: {}}})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestSendQuestionUsesOpaqueInlineCallbacksAndDoesNotLeakToken(t *testing.T) {
	var path string
	var request struct {
		ChatID      int64          `json:"chat_id"`
		Text        string         `json:"text"`
		ReplyMarkup inlineKeyboard `json:"reply_markup"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	}))
	defer server.Close()
	adapter := testAdapter(t, server)
	messageID, err := adapter.SendQuestion(context.Background(), "operator_primary", testQuestion(time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if messageID != "42" || path != "/botsecret/sendMessage" {
		t.Fatalf("message=%q path=%q", messageID, path)
	}
	keyboard := request.ReplyMarkup
	if len(keyboard.InlineKeyboard) < 2 {
		t.Fatalf("markup = %#v", request.ReplyMarkup)
	}
	if keyboard.InlineKeyboard[0][0].CallbackData != "o:0" || keyboard.InlineKeyboard[1][0].CallbackData != "o:1" {
		t.Fatalf("callbacks = %#v", keyboard)
	}
	encoded, _ := json.Marshal(request)
	if string(encoded) == "" || contains(string(encoded), "ask_1") || contains(string(encoded), "secret") {
		t.Fatalf("request leaked identity or token: %s", encoded)
	}
}

func TestSendQuestionResolvesReminderDestinationMarker(t *testing.T) {
	var gotChat int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req sendMessageRequest
		_ = json.Unmarshal(body, &req)
		gotChat = req.ChatID
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":99}}`))
	}))
	defer server.Close()
	adapter := testAdapter(t, server)
	now := time.Date(2026, 7, 16, 7, 0, 0, 0, time.UTC)
	id, err := adapter.SendQuestion(context.Background(), domain.ReminderDestinationRef("operator_primary", 1), testQuestion(now))
	if err != nil {
		t.Fatal(err)
	}
	if id != "99" || gotChat != 100 {
		t.Fatalf("id=%s chat=%d", id, gotChat)
	}
}

func TestExternalAnswerRequiresAllowlistAndDurableMessageBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	adapter := testAdapter(t, server)
	now := time.Date(2026, 7, 16, 7, 0, 0, 0, time.UTC)
	question := testQuestion(now)
	delivery := domain.QuestionDelivery{SchemaVersion: 1, ID: "delivery_1", QuestionID: question.ID, QuestionRevision: 1, Channel: ChannelName, DestinationRef: "operator_primary", Status: domain.QuestionDeliveryDelivered, Attempt: 1, MaxAttempts: 3, AvailableAt: now, TransportMessageID: "42", CreatedAt: now, UpdatedAt: now.Add(time.Second)}
	update := Update{UpdateID: 9, CallbackQuery: &CallbackQuery{ID: "cb_1", From: User{ID: 7}, Message: &Message{MessageID: 42, Chat: Chat{ID: 100}}, Data: "o:1"}}
	event, err := adapter.ExternalAnswer(update, question, delivery, source.NewSequenceIDGenerator(1), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	answer, err := kernel.DecodeUserAnswerExternalEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if answer.ActorID != "operator_1" || len(answer.OptionIDs) != 1 || answer.OptionIDs[0] != "b" || event.DeduplicationKey != "telegram:callback:cb_1" {
		t.Fatalf("answer/event = %#v %#v", answer, event)
	}

	update.CallbackQuery.Message.MessageID = 41
	if _, err := adapter.ExternalAnswer(update, question, delivery, source.NewSequenceIDGenerator(20), now.Add(time.Minute)); !isKind(err, ErrorUncorrelated) {
		t.Fatalf("wrong message error = %v", err)
	}
	update.CallbackQuery.Message.MessageID, update.CallbackQuery.From.ID = 42, 8
	if _, err := adapter.ExternalAnswer(update, question, delivery, source.NewSequenceIDGenerator(30), now.Add(time.Minute)); !isKind(err, ErrorUnauthorized) {
		t.Fatalf("unauthorized error = %v", err)
	}
}

func TestIngestUpdateResolvesDeliveryByTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	adapter := testAdapter(t, server)
	now := time.Date(2026, 7, 16, 7, 0, 0, 0, time.UTC)
	store := memory.New()
	question := testQuestion(now)
	delivery := domain.QuestionDelivery{
		SchemaVersion: 1, ID: "delivery_ingest_1", QuestionID: question.ID, QuestionRevision: 1,
		Channel: ChannelName, DestinationRef: domain.ReminderDestinationRef("operator_primary", 2),
		Status: domain.QuestionDeliveryPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		mission := domain.MissionRevision{SchemaVersion: 1, ID: question.MissionRevision, MissionID: question.MissionID, Revision: 1, OriginalText: "test mission", Purpose: "test", Status: domain.MissionActive, Provenance: "test", AcceptedAt: now}
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
	update := Update{UpdateID: 11, CallbackQuery: &CallbackQuery{ID: "cb_ingest", From: User{ID: 7}, Message: &Message{MessageID: 42, Chat: Chat{ID: 100}}, Data: "o:0"}}
	event, err := adapter.IngestUpdate(context.Background(), store, update, source.NewSequenceIDGenerator(1), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if event.CorrelationID != string(question.ID) {
		t.Fatalf("event = %#v", event)
	}
}

func TestReplyAndCallbackDeduplicateThroughExistingExternalInbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	adapter := testAdapter(t, server)
	now := time.Date(2026, 7, 16, 7, 0, 0, 0, time.UTC)
	question := testQuestion(now)
	delivery := domain.QuestionDelivery{SchemaVersion: 1, ID: "delivery_1", QuestionID: question.ID, QuestionRevision: 1, Channel: ChannelName, DestinationRef: "operator_primary", Status: domain.QuestionDeliveryDelivered, Attempt: 1, MaxAttempts: 3, AvailableAt: now, TransportMessageID: "42", CreatedAt: now, UpdatedAt: now.Add(time.Second)}
	update := Update{UpdateID: 9, CallbackQuery: &CallbackQuery{ID: "cb_1", From: User{ID: 7}, Message: &Message{MessageID: 42, Chat: Chat{ID: 100}}, Data: "o:0"}}
	event, err := adapter.ExternalAnswer(update, question, delivery, source.NewSequenceIDGenerator(1), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	store := memory.New()
	inbox, err := control.NewExternalEventInbox(store, control.DispositionFactoryFrom(source.NewManualClock(now.Add(time.Minute))))
	if err != nil {
		t.Fatal(err)
	}
	first, err := inbox.SubmitExternalEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := inbox.SubmitExternalEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if first.EventID != second.EventID || first.State != second.State {
		t.Fatalf("dispositions differ: %#v %#v", first, second)
	}
	event.ID = "different"
	if _, err := inbox.SubmitExternalEvent(event); !errors.Is(err, nil) {
		// A different event identity with the same exact dedup envelope is rejected
		// by design because the durable identity is part of replay equality.
		return
	}
	t.Fatal("different event ID unexpectedly replayed")
}

func TestDeliveryWorkerLeasesAndCompletesTelegramOutbox(t *testing.T) {
	now := time.Date(2026, 7, 16, 7, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	}))
	defer server.Close()
	adapter := testAdapter(t, server)
	store := memory.New()
	question := testQuestion(now)
	delivery := domain.QuestionDelivery{SchemaVersion: 1, ID: "delivery_1", QuestionID: question.ID, QuestionRevision: 1, Channel: ChannelName, DestinationRef: "operator_primary", Status: domain.QuestionDeliveryPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		mission := domain.MissionRevision{SchemaVersion: 1, ID: question.MissionRevision, MissionID: question.MissionID, Revision: 1, OriginalText: "test mission", Purpose: "test", Status: domain.MissionActive, Provenance: "test", AcceptedAt: now}
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.CreateOperatorQuestion(question); err != nil {
			return err
		}
		return tx.CreateQuestionDelivery(delivery)
	}); err != nil {
		t.Fatal(err)
	}
	worker := DeliveryWorker{Store: store, Adapter: adapter, Clock: source.NewManualClock(now), Owner: "worker_1", LeaseDuration: time.Minute, RetryDelay: time.Minute}
	processed, err := worker.ProcessDue(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d", processed)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.QuestionDelivery(delivery.ID)
		if err != nil {
			return err
		}
		if got.Status != domain.QuestionDeliveryDelivered || got.TransportMessageID != "42" || got.Attempt != 1 {
			t.Fatalf("delivery = %#v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeliveryWorkerParksExpiredLeaseAsEffectUnknownWithoutResend(t *testing.T) {
	now := time.Date(2026, 7, 16, 7, 0, 0, 0, time.UTC)
	sends := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends++
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":99}}`))
	}))
	defer server.Close()
	adapter := testAdapter(t, server)
	store := memory.New()
	question := testQuestion(now)
	leaseUntil := now.Add(time.Minute)
	pending := domain.QuestionDelivery{SchemaVersion: 1, ID: "delivery_1", QuestionID: question.ID, QuestionRevision: 1, Channel: ChannelName, DestinationRef: "operator_primary", Status: domain.QuestionDeliveryPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	leased, err := domain.LeaseQuestionDelivery(pending, "crashed_worker", now, leaseUntil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		mission := domain.MissionRevision{SchemaVersion: 1, ID: question.MissionRevision, MissionID: question.MissionID, Revision: 1, OriginalText: "test mission", Purpose: "test", Status: domain.MissionActive, Provenance: "test", AcceptedAt: now}
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.CreateOperatorQuestion(question); err != nil {
			return err
		}
		if err := tx.CreateQuestionDelivery(pending); err != nil {
			return err
		}
		return tx.SaveQuestionDelivery(leased, pending.Status, pending.Attempt)
	}); err != nil {
		t.Fatal(err)
	}
	clock := source.NewManualClock(leaseUntil)
	worker := DeliveryWorker{Store: store, Adapter: adapter, Clock: clock, Owner: "worker_2", LeaseDuration: time.Minute, RetryDelay: time.Minute}
	processed, err := worker.ProcessDue(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d", processed)
	}
	if sends != 0 {
		t.Fatalf("unexpected resend after lease expiry: %d", sends)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.QuestionDelivery(pending.ID)
		if err != nil {
			return err
		}
		if got.Status != domain.QuestionDeliveryEffectUnknown || got.LastFailureCode != domain.DeliveryFailureLeaseExpired {
			t.Fatalf("delivery = %#v", got)
		}
		due, err := r.DueQuestionDeliveries(leaseUntil.Add(time.Hour), 10)
		if err != nil {
			return err
		}
		if len(due) != 0 {
			t.Fatalf("EFFECT_UNKNOWN must not be due: %#v", due)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A second pass must not re-send either.
	processed, err = worker.ProcessDue(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 || sends != 0 {
		t.Fatalf("second pass processed=%d sends=%d", processed, sends)
	}
}

func TestDeliveryWorkerParksAmbiguousTransportAsEffectUnknown(t *testing.T) {
	now := time.Date(2026, 7, 16, 7, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate truncated / invalid reply after request accepted.
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer server.Close()
	adapter := testAdapter(t, server)
	store := memory.New()
	question := testQuestion(now)
	delivery := domain.QuestionDelivery{SchemaVersion: 1, ID: "delivery_1", QuestionID: question.ID, QuestionRevision: 1, Channel: ChannelName, DestinationRef: "operator_primary", Status: domain.QuestionDeliveryPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		mission := domain.MissionRevision{SchemaVersion: 1, ID: question.MissionRevision, MissionID: question.MissionID, Revision: 1, OriginalText: "test mission", Purpose: "test", Status: domain.MissionActive, Provenance: "test", AcceptedAt: now}
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.CreateOperatorQuestion(question); err != nil {
			return err
		}
		return tx.CreateQuestionDelivery(delivery)
	}); err != nil {
		t.Fatal(err)
	}
	worker := DeliveryWorker{Store: store, Adapter: adapter, Clock: source.NewManualClock(now), Owner: "worker_1", LeaseDuration: time.Minute, RetryDelay: time.Minute}
	processed, err := worker.ProcessDue(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d", processed)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.QuestionDelivery(delivery.ID)
		if err != nil {
			return err
		}
		if got.Status != domain.QuestionDeliveryEffectUnknown || got.LastFailureCode != domain.DeliveryFailureAmbiguousTransport {
			t.Fatalf("delivery = %#v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSetWebhookAndDeleteWebhookCallBotAPI(t *testing.T) {
	var paths []string
	var setBody setWebhookRequest
	var deleteBody deleteWebhookRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/setWebhook"):
			_ = json.Unmarshal(body, &setBody)
		case strings.HasSuffix(r.URL.Path, "/deleteWebhook"):
			_ = json.Unmarshal(body, &deleteBody)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()
	adapter := testAdapter(t, server)
	if err := adapter.SetWebhook(context.Background(), WebhookConfig{
		URL: "https://example.test/telegram/webhook", SecretToken: "hook-secret",
		MaxConnections: 20, DropPendingUpdates: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.DeleteWebhook(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/botsecret/setWebhook" || paths[1] != "/botsecret/deleteWebhook" {
		t.Fatalf("paths = %#v", paths)
	}
	if setBody.URL != "https://example.test/telegram/webhook" || setBody.SecretToken != "hook-secret" || setBody.MaxConnections != 20 || !setBody.DropPendingUpdates {
		t.Fatalf("setWebhook body = %#v", setBody)
	}
	if len(setBody.AllowedUpdates) != 2 || setBody.AllowedUpdates[0] != "message" || setBody.AllowedUpdates[1] != "callback_query" {
		t.Fatalf("allowed_updates = %#v", setBody.AllowedUpdates)
	}
	if !deleteBody.DropPendingUpdates {
		t.Fatalf("deleteWebhook body = %#v", deleteBody)
	}
	if err := adapter.SetWebhook(context.Background(), WebhookConfig{URL: "http://insecure.example/hook"}); !isKind(err, ErrorInvalidConfig) {
		t.Fatalf("insecure URL error = %v", err)
	}
}

func TestDecodeUpdateRejectsAmbiguousAndUnknownPayloads(t *testing.T) {
	if _, err := DecodeUpdate([]byte(`{"update_id":1,"message":{"message_id":2,"chat":{"id":100}},"extra":true}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := DecodeUpdate([]byte(`{"update_id":1}`)); err == nil {
		t.Fatal("empty payload accepted")
	}
}

func contains(value, substring string) bool {
	return len(substring) > 0 && len(value) >= len(substring) && (func() bool {
		for i := 0; i+len(substring) <= len(value); i++ {
			if value[i:i+len(substring)] == substring {
				return true
			}
		}
		return false
	})()
}
func isKind(err error, kind ErrorKind) bool {
	var got *Error
	return errors.As(err, &got) && got.Kind == kind
}
