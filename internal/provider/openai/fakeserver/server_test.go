package fakeserver_test

import (
	"bytes"
	"context"
	"net/http"
	"strings"
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

func TestServerRejectsTrailingAndOversizedRequests(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "trailing JSON",
			body: []byte(`{"model":"fixture","messages":[{"role":"user","content":"prompt"}]} {}`),
		},
		{
			name: "oversized body",
			body: []byte(`{"model":"fixture","messages":[{"role":"user","content":"` + strings.Repeat("x", 1<<20) + `"}]}`),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := fakeserver.New(fakeserver.Exchange{ResponseText: "unused"})
			defer server.Close()
			request, err := http.NewRequest(http.MethodPost, server.URL()+"/v1/chat/completions", bytes.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
			}
			if got := server.Failures(); len(got) != 1 || got[0] != "invalid Chat Completions request" {
				t.Fatalf("failures = %v", got)
			}
		})
	}
}
