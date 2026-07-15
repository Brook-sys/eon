package openai_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

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
	if result.Text != "B" || result.InputTokens != 5 || result.OutputTokens != 1 || result.Model != "fixture-model-v1" {
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
		retryable bool
	}{
		{name: "rate limit", exchange: fakeserver.Exchange{StatusCode: http.StatusTooManyRequests, RawBody: `{"error":"prompt secret"}`}, kind: openai.ErrorHTTP, retryable: true},
		{name: "invalid response", exchange: fakeserver.Exchange{RawBody: `{"choices":[]}`}, kind: openai.ErrorInvalidResponse},
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
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("error leaked data: %v", err)
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
