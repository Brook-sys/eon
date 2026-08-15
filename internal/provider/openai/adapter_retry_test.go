package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

// TestDoWithRetryRecoversFrom429 verifies that the adapter retries a 429
// response and succeeds on the second attempt when retry is configured.
func TestDoWithRetryRecoversFrom429(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		body := map[string]any{
			"id":      "test-1",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "DATE: 2024-01-01 | CONFLICT: NO"}}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	provider, err := openai.New(openai.Config{
		APIKey:           "test-key",
		BaseURL:          server.URL,
		Model:            "test-model",
		MaxResponseBytes: 1 << 20,
	
		Client:           server.Client(),
		Retry: &openai.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   1 * time.Millisecond,
			MaxDelay:    10 * time.Millisecond,
			MaxJitter:   0,
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := provider.Complete(context.Background(), port.CompletionRequest{
		Prompt:          "Extract the date from: January 1, 2024",
		MaxOutputTokens: 100,
		Temperature:     0.0,
	})
	if err != nil {
		t.Fatalf("Complete after retry: %v", err)
	}
	if result.Text == "" {
		t.Fatal("empty text after retry")
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

// TestDoWithRetryExhaustedOn429 verifies that after exhausting all attempts,
// the adapter returns a budget-exhausted error wrapping the last 429.
func TestDoWithRetryExhaustedOn429(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider, err := openai.New(openai.Config{
		APIKey:           "test-key",
		BaseURL:          server.URL,
		Model:            "test-model",
		MaxResponseBytes: 1 << 20,
	
		Client:           server.Client(),
		Retry: &openai.RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   1 * time.Millisecond,
			MaxDelay:    5 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = provider.Complete(context.Background(), port.CompletionRequest{
		Prompt:          "test",
		MaxOutputTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

// TestDoWithRetryNoRetryConfigBackwardCompatible verifies that without Retry
// config, the adapter makes a single attempt (backward compatible).
func TestDoWithRetryNoRetryConfigBackwardCompatible(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider, err := openai.New(openai.Config{
		APIKey:           "test-key",
		BaseURL:          server.URL,
		Model:            "test-model",
		MaxResponseBytes: 1 << 20,
	
		Client:           server.Client(),
		// No Retry config — backward compatible
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = provider.Complete(context.Background(), port.CompletionRequest{
		Prompt:          "test",
		MaxOutputTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error on 429 without retry")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", calls)
	}
}

// TestDoWithRetrySkipsNonRetryable verifies that a 400 error is not retried.
func TestDoWithRetrySkipsNonRetryable(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	provider, err := openai.New(openai.Config{
		APIKey:           "test-key",
		BaseURL:          server.URL,
		Model:            "test-model",
		MaxResponseBytes: 1 << 20,
	
		Client:           server.Client(),
		Retry: &openai.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   1 * time.Millisecond,
			MaxDelay:    5 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = provider.Complete(context.Background(), port.CompletionRequest{
		Prompt:          "test",
		MaxOutputTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call (non-retryable), got %d", calls)
	}
}

// TestDoWithRetryRecoversFrom5xx verifies that the adapter retries a 500
// response and succeeds on the second attempt.
func TestDoWithRetryRecoversFrom5xx(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body := map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "OK"}}},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	provider, err := openai.New(openai.Config{
		APIKey:           "test-key",
		BaseURL:          server.URL,
		Model:            "test-model",
		MaxResponseBytes: 1 << 20,
	
		Client:           server.Client(),
		Retry: &openai.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   1 * time.Millisecond,
			MaxDelay:    5 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := provider.Complete(context.Background(), port.CompletionRequest{
		Prompt:          "test",
		MaxOutputTokens: 100,
	})
	if err != nil {
		t.Fatalf("Complete after 5xx retry: %v", err)
	}
	if result.Text != "OK" {
		t.Fatalf("expected 'OK', got %q", result.Text)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

// TestDoWithRetrySucceedsFirstAttempt verifies that when the first attempt
// succeeds, no retries are attempted and the call count is 1.
func TestDoWithRetrySucceedsFirstAttempt(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body := map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "READY"}}},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	provider, err := openai.New(openai.Config{
		APIKey:           "test-key",
		BaseURL:          server.URL,
		Model:            "test-model",
		MaxResponseBytes: 1 << 20,
	
		Client:           server.Client(),
		Retry: &openai.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   1 * time.Millisecond,
			MaxDelay:    5 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := provider.Complete(context.Background(), port.CompletionRequest{
		Prompt:          "test",
		MaxOutputTokens: 100,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result.Text != "READY" {
		t.Fatalf("expected 'READY', got %q", result.Text)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}
