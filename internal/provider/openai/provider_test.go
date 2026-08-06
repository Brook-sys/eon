package openai_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/provider/openai/fakeserver"
)

func TestProviderCompletesPlainTextAgainstFakeServer(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{ExpectedPrompt: "choose A or B", ExpectedModel: "fixture-model", ExpectedMaxOutputField: "max_tokens", ResponseText: "B", ResponseModel: "fixture-model-v1", InputTokens: 5, OutputTokens: 1})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), APIKey: "secret", Model: "fixture-model", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Complete(context.Background(), port.CompletionRequest{Prompt: "choose A or B", MaxOutputTokens: 4, Temperature: 0})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "B" || result.InputTokens != 5 || result.OutputTokens != 1 || result.Model != "fixture-model-v1" || result.FinishReason != port.CompletionFinishStop {
		t.Fatalf("unexpected result: %+v", result)
	}
	requests := server.Requests()
	if len(requests) != 1 || requests[0].Authorization != "Bearer secret" || requests[0].MaxOutputTokens != 4 {
		t.Fatalf("unexpected request: %+v", requests)
	}
	if failures := server.Failures(); len(failures) != 0 {
		t.Fatalf("fake server failures: %v", failures)
	}
}

func TestProviderHandlesTrailingDataDoneSuffix(t *testing.T) {
	ts := fakeserver.New(fakeserver.Exchange{
		RawBody: `{"id":"chatcmpl-1","choices":[{"index":0,"message":{"role":"assistant","content":"READY"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}data: [DONE]`,
	})
	defer ts.Close()

	provider, err := openai.New(openai.Config{BaseURL: ts.URL(), APIKey: "test", Model: "test-model", Client: ts.Client()})
	if err != nil {
		t.Fatal(err)
	}
	res, err := provider.Complete(context.Background(), port.CompletionRequest{Prompt: "test", MaxOutputTokens: 10})
	if err != nil {
		t.Fatalf("expected success with trailing data: [DONE], got err: %v", err)
	}
	if res.Text != "READY" {
		t.Fatalf("expected text READY, got %q", res.Text)
	}
}

func TestProviderClassifiesFinishReasonWithoutRetainingUnknownWireValue(t *testing.T) {
	server := fakeserver.New(
		fakeserver.Exchange{ResponseText: "partial", FinishReason: "length"},
		fakeserver.Exchange{ResponseText: "future", FinishReason: "vendor_future_reason"},
	)
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	length, err := provider.Complete(context.Background(), port.CompletionRequest{Prompt: "first", MaxOutputTokens: 4})
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := provider.Complete(context.Background(), port.CompletionRequest{Prompt: "second", MaxOutputTokens: 4})
	if err != nil {
		t.Fatal(err)
	}
	if length.FinishReason != port.CompletionFinishLength || unknown.FinishReason != port.CompletionFinishOther {
		t.Fatalf("finish reasons length=%q unknown=%q", length.FinishReason, unknown.FinishReason)
	}
}

func TestProviderSupportsConfiguredMaxCompletionTokensDialect(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{ExpectedMaxOutputField: "max_completion_tokens", ResponseText: "ok"})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", MaxOutputField: openai.MaxOutputTokensCompletion, Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Complete(context.Background(), port.CompletionRequest{Prompt: "test", MaxOutputTokens: 7}); err != nil {
		t.Fatal(err)
	}
	requests := server.Requests()
	if len(requests) != 1 || requests[0].MaxOutputField != "max_completion_tokens" || requests[0].MaxOutputTokens != 7 {
		t.Fatalf("unexpected request: %+v", requests)
	}
	if failures := server.Failures(); len(failures) != 0 {
		t.Fatalf("fake server failures: %v", failures)
	}
}

func TestProviderClassifiesBoundedFailuresWithoutLeakingBody(t *testing.T) {
	tests := []struct {
		name      string
		exchange  fakeserver.Exchange
		limit     int64
		kind      openai.ErrorKind
		reason    string
		retryable bool
		retryWait time.Duration
	}{
		{name: "rate limit", exchange: fakeserver.Exchange{StatusCode: http.StatusTooManyRequests, RawBody: `{"error":"prompt secret"}`, Headers: map[string]string{"Retry-After": "42", "x-ratelimit-limit-requests": "30", "x-ratelimit-remaining-requests": "0", "x-ratelimit-reset-requests": "2m1.5s", "x-ratelimit-limit-tokens": "6000", "x-ratelimit-remaining-tokens": "1234", "x-ratelimit-reset-tokens": "1.25s", "x-secret-quota": "must-not-project"}}, kind: openai.ErrorHTTP, retryable: true, retryWait: 42 * time.Second},
		{name: "invalid response", exchange: fakeserver.Exchange{RawBody: `{"choices":[]}`}, kind: openai.ErrorInvalidResponse, reason: "choices_count"},
		{name: "too large", exchange: fakeserver.Exchange{RawBody: strings.Repeat("x", 33)}, limit: 32, kind: openai.ErrorResponseTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := fakeserver.New(test.exchange)
			defer server.Close()
			provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client(), MaxResponseBytes: test.limit})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.Complete(context.Background(), port.CompletionRequest{Prompt: "sensitive prompt"})
			var providerError *openai.Error
			if !errors.As(err, &providerError) || providerError.Kind != test.kind || providerError.Retryable != test.retryable {
				t.Fatalf("unexpected error: %#v", err)
			}
			if providerError.DiagnosticReason() != test.reason {
				t.Fatalf("diagnostic reason = %q, want %q", providerError.DiagnosticReason(), test.reason)
			}
			if test.retryWait > 0 && providerError.RetryAfter != test.retryWait {
				t.Fatalf("retry wait = %v, want %v", providerError.RetryAfter, test.retryWait)
			}
			if test.name == "rate limit" {
				metadata := providerError.RateLimitMetadata()
				if !metadata.HasRequestLimit || metadata.RequestLimit != 30 || !metadata.HasRequestRemaining || metadata.RequestRemaining != 0 || !metadata.HasRequestReset || metadata.RequestReset != 2*time.Minute+1500*time.Millisecond {
					t.Fatalf("unexpected request quota metadata: %+v", metadata)
				}
				if !metadata.HasTokenLimit || metadata.TokenLimit != 6000 || !metadata.HasTokenRemaining || metadata.TokenRemaining != 1234 || !metadata.HasTokenReset || metadata.TokenReset != 1250*time.Millisecond {
					t.Fatalf("unexpected token quota metadata: %+v", metadata)
				}
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("error leaked data: %v", err)
			}
		})
	}
}

func TestProviderClassifiesReasoningEatenBudgetSeparatelyFromEmptyContent(t *testing.T) {
	tests := []struct {
		name    string
		rawBody string
		reason  string
	}{
		{
			name:    "reasoning consumed the whole budget",
			rawBody: `{"id":"chatcmpl-r1","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":94,"completion_tokens":32,"completion_tokens_details":{"reasoning_tokens":30}}}`,
			reason:  "reasoning_budget_exhausted",
		},
		{
			name:    "empty content without reasoning or truncation",
			rawBody: `{"id":"chatcmpl-r2","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":94,"completion_tokens":0}}`,
			reason:  "empty_content",
		},
		{
			name:    "length finish without reasoning tokens stays empty_content",
			rawBody: `{"id":"chatcmpl-r3","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":94,"completion_tokens":32}}`,
			reason:  "empty_content",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := fakeserver.New(fakeserver.Exchange{RawBody: test.rawBody})
			defer server.Close()
			provider, err := openai.New(openai.Config{BaseURL: server.URL(), APIKey: "test", Model: "fixture", Client: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.Complete(context.Background(), port.CompletionRequest{Prompt: "bounded", MaxOutputTokens: 32})
			var providerError *openai.Error
			if !errors.As(err, &providerError) || providerError.Kind != openai.ErrorInvalidResponse {
				t.Fatalf("unexpected error: %#v", err)
			}
			if providerError.DiagnosticReason() != test.reason {
				t.Fatalf("diagnostic reason = %q, want %q", providerError.DiagnosticReason(), test.reason)
			}
		})
	}
}

func TestProviderRejectsInvalidConfigurationAndRequest(t *testing.T) {
	if _, err := openai.New(openai.Config{BaseURL: "file:///tmp/model", Model: "fixture"}); err == nil {
		t.Fatal("expected invalid URL error")
	}
	if _, err := openai.New(openai.Config{BaseURL: "http://example.test", Model: "fixture", MaxOutputField: "unknown"}); err == nil {
		t.Fatal("expected invalid max output field error")
	}
	if _, err := openai.New(openai.Config{BaseURL: "http://example.test", Model: "fixture", Timeout: -time.Second}); err == nil {
		t.Fatal("expected invalid timeout error")
	}
	if _, err := openai.New(openai.Config{BaseURL: "http://example.test", Model: "fixture", Timeout: time.Second, Client: http.DefaultClient}); err == nil {
		t.Fatal("expected timeout/custom client conflict")
	}
	provider, err := openai.New(openai.Config{BaseURL: "http://example.test", Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Complete(context.Background(), port.CompletionRequest{Prompt: " ", Temperature: 3})
	var providerError *openai.Error
	if !errors.As(err, &providerError) || providerError.Kind != openai.ErrorInvalidRequest {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProviderEmitsJSONObjectResponseFormatWhenRequested(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{
		ExpectedResponseFormat: "json_object",
		RequireResponseFormat:  true,
		ResponseText:           `{"ok":true}`,
		ResponseModel:          "fixture",
	})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Complete(context.Background(), port.CompletionRequest{
		Prompt: "return json", MaxOutputTokens: 16, Temperature: 0,
		ResponseFormat: "json_object",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != `{"ok":true}` {
		t.Fatalf("text = %q", result.Text)
	}
	if len(server.Requests()) != 1 || server.Requests()[0].ResponseFormat != "json_object" {
		t.Fatalf("requests = %+v", server.Requests())
	}
	if failures := server.Failures(); len(failures) != 0 {
		t.Fatalf("failures: %v", failures)
	}
}

func TestProviderEmitsPrefillAssistantAsTrailingAssistantMessage(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{
		ExpectedPrefillAssistant: "{",
		ResponseText:             `"title":"x"}`,
		ResponseModel:            "fixture",
		FinishReason:             "stop",
	})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Complete(context.Background(), port.CompletionRequest{
		Prompt: "return json", MaxOutputTokens: 64, Temperature: 0,
		PrefillAssistant: "{",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != `"title":"x"}` {
		t.Fatalf("text = %q", result.Text)
	}
	requests := server.Requests()
	if len(requests) != 1 {
		t.Fatalf("requests = %d", len(requests))
	}
	if requests[0].PrefillAssistant != "{" {
		t.Fatalf("prefill = %q", requests[0].PrefillAssistant)
	}
	if failures := server.Failures(); len(failures) != 0 {
		t.Fatalf("failures: %v", failures)
	}
}

func TestProviderOmitsPrefillMessageWhenUnset(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{
		ResponseText:  "ok",
		ResponseModel: "fixture",
	})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Complete(context.Background(), port.CompletionRequest{
		Prompt: "return text", MaxOutputTokens: 16, Temperature: 0,
	}); err != nil {
		t.Fatal(err)
	}
	requests := server.Requests()
	if len(requests) != 1 {
		t.Fatalf("requests = %d", len(requests))
	}
	if requests[0].PrefillAssistant != "" {
		t.Fatalf("unexpected prefill %q", requests[0].PrefillAssistant)
	}
	if failures := server.Failures(); len(failures) != 0 {
		t.Fatalf("failures: %v", failures)
	}
}

func TestProviderRejectsUnknownResponseFormat(t *testing.T) {
	provider, err := openai.New(openai.Config{BaseURL: "http://example.test", Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Complete(context.Background(), port.CompletionRequest{
		Prompt: "x", ResponseFormat: "not-a-real-format",
	})
	var providerError *openai.Error
	if !errors.As(err, &providerError) || providerError.Kind != openai.ErrorInvalidRequest {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProviderDiscoversModelsWithCacheAndAllowlist(t *testing.T) {
	server := fakeserver.New(
		fakeserver.Exchange{ModelsResponse: []string{"model-a", "model-b", "unknown-model", ""}},
		fakeserver.Exchange{ModelsResponse: []string{"model-c"}},
	)
	defer server.Close()

	provider, err := openai.New(
		openai.Config{BaseURL: server.URL(), APIKey: "secret", Model: "model-a", Client: server.Client()},
		openai.WithAllowedModels("model-a", "model-b"),
		openai.WithModelsCacheTTL(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}

	// First call hits the network, filters by allowlist, and caches.
	models, err := provider.DiscoverModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != "model-a" || models[1] != "model-b" {
		t.Fatalf("unexpected models: %v", models)
	}

	// Second call hits cache (no network).
	cached, err := provider.DiscoverModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cached) != 2 {
		t.Fatalf("unexpected cached models: %v", cached)
	}
}

func TestProviderDiscoverModelsRejectsErrorsAndTooLargeBodies(t *testing.T) {
	tests := []struct {
		name     string
		exchange fakeserver.Exchange
		limit    int64
		kind     openai.ErrorKind
	}{
		{"too large", fakeserver.Exchange{RawBody: `{"data":[{"id":"1"}]}`}, 10, openai.ErrorResponseTooLarge},
		{"invalid json", fakeserver.Exchange{RawBody: `{invalid}`}, 1000, openai.ErrorInvalidResponse},
		{"http error", fakeserver.Exchange{StatusCode: 401, RawBody: `{"error":"auth"}`}, 1000, openai.ErrorHTTP},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := fakeserver.New(test.exchange)
			defer server.Close()
			provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client(), MaxResponseBytes: test.limit})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.DiscoverModels(context.Background())
			var providerError *openai.Error
			if !errors.As(err, &providerError) || providerError.Kind != test.kind {
				t.Fatalf("unexpected error: %#v", err)
			}
		})
	}
}

func TestProviderCompletesWithToolsAndReturnsToolCalls(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{
		ExpectedPrompt: "call test tool", ExpectedModel: "fixture-model",
		ResponseModel: "fixture-model-v1", InputTokens: 5, OutputTokens: 1,
		ToolCalls: []struct {
			Name      string
			Arguments string
		}{
			{Name: "test_tool", Arguments: `{"arg":"val"}`},
		},
	})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), APIKey: "secret", Model: "fixture-model", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.CompleteWithTools(context.Background(), port.CompletionRequest{Prompt: "call test tool"}, []port.ToolDefinition{
		{Name: "test_tool", Description: "desc", Parameters: []byte(`{"type":"object"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "test_tool" || result.ToolCalls[0].Arguments != `{"arg":"val"}` {
		t.Fatalf("unexpected tool calls: %+v", result.ToolCalls)
	}
}

func TestProviderEmitsReasoningEffortWhenRequested(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{
		ExpectedReasoningEffort: "none",
		ResponseText:            "READY",
		ResponseModel:           "qwen-fixture",
	})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), APIKey: "secret", Model: "qwen-fixture", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Complete(context.Background(), port.CompletionRequest{
		Prompt:          "choose A or B",
		MaxOutputTokens: 256,
		ReasoningEffort: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "READY" {
		t.Fatalf("unexpected result text: %q", result.Text)
	}
	requests := server.Requests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if requests[0].ReasoningEffort != "none" {
		t.Fatalf("expected reasoning_effort 'none', got %q", requests[0].ReasoningEffort)
	}
	if !strings.HasPrefix(requests[0].UserAgent, "motor-autonomo-openai-adapter/") {
		t.Fatalf("expected User-Agent header, got %q", requests[0].UserAgent)
	}
	if failures := server.Failures(); len(failures) != 0 {
		t.Fatalf("fake server failures: %v", failures)
	}
}

func TestProviderSetsUserAgentOnAllRequests(t *testing.T) {
	server := fakeserver.New(
		fakeserver.Exchange{ResponseText: "ok"},
		fakeserver.Exchange{ModelsResponse: []string{"model-1"}},
	)
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), APIKey: "secret", Model: "model-1", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Completion request
	if _, err := provider.Complete(context.Background(), port.CompletionRequest{Prompt: "hi", MaxOutputTokens: 10}); err != nil {
		t.Fatal(err)
	}
	requests := server.Requests()
	if len(requests) != 1 || !strings.HasPrefix(requests[0].UserAgent, "motor-autonomo-openai-adapter/") {
		t.Fatalf("completion request missing User-Agent: %+v", requests)
	}

	// 2. DiscoverModels request
	models, err := provider.DiscoverModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0] != "model-1" {
		t.Fatalf("unexpected discovered models: %v", models)
	}
	if failures := server.Failures(); len(failures) != 0 {
		t.Fatalf("fake server failures: %v", failures)
	}
}
