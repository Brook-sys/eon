package fakeserver_test

import (
	"context"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/provider/openai/fakeserver"
)

func TestScriptRecordsMismatchForContractTests(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{ExpectedPrompt: "expected", ResponseText: "ok"})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Complete(context.Background(), port.CompletionRequest{Prompt: "actual"}); err != nil {
		t.Fatal(err)
	}
	if len(server.Failures()) != 1 {
		t.Fatalf("expected recorded mismatch, got %v", server.Failures())
	}
}
