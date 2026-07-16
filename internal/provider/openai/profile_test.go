package openai_test

import (
	"context"
	"strings"
	"testing"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/provider/openai/fakeserver"
)

func TestDeclaredProfileIsConservativeWithoutIO(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{ResponseText: "should-not-be-called"})
	defer server.Close()
	provider, err := openai.New(openai.Config{
		BaseURL:        server.URL(),
		Model:          "tiny",
		MaxOutputField: openai.MaxOutputTokensCompletion,
		Client:         server.Client(),
	}, openai.WithProfileName("ollama-chat"), openai.WithContextTokens(4096), openai.WithProbeBudget(2))
	if err != nil {
		t.Fatal(err)
	}
	profile := provider.DeclaredProfile()
	if err := profile.Validate(); err != nil {
		t.Fatal(err)
	}
	if profile.Source != domain.CapabilityDeclared {
		t.Fatalf("source = %s", profile.Source)
	}
	if profile.TextToTextConfirmed || profile.SupportsTools || profile.SupportsJSONSchema || profile.SupportsStreaming {
		t.Fatalf("declared profile must not presume rich capabilities: %+v", profile)
	}
	if profile.APIStyle != domain.APIStyleChatCompletions || profile.MaxOutputDialect != domain.MaxOutputDialectCompletion {
		t.Fatalf("unexpected dialect/style: %+v", profile)
	}
	if profile.Name != "ollama-chat" || profile.Model != "tiny" || profile.MaxContextTokens != 4096 {
		t.Fatalf("unexpected metadata: %+v", profile)
	}
	if profile.ProbeBudgetRemaining != 2 {
		t.Fatalf("probe budget = %d", profile.ProbeBudgetRemaining)
	}
	if len(server.Requests()) != 0 {
		t.Fatalf("DeclaredProfile must not perform network I/O: %+v", server.Requests())
	}
}

func TestProbeConfirmsTextToTextAndRespectsBudget(t *testing.T) {
	server := fakeserver.New(
		fakeserver.Exchange{ExpectedPrompt: "ping", ExpectedModel: "tiny", ExpectedMaxOutputField: "max_tokens", ResponseText: "ok", ResponseModel: "tiny-v2", InputTokens: 1, OutputTokens: 1},
		fakeserver.Exchange{ExpectedPrompt: "ping", ExpectedModel: "tiny", ExpectedMaxOutputField: "max_tokens", ResponseText: "again", ResponseModel: "tiny-v3", InputTokens: 1, OutputTokens: 1},
	)
	defer server.Close()
	provider, err := openai.New(openai.Config{
		BaseURL: server.URL(),
		Model:   "tiny",
		Client:  server.Client(),
	}, openai.WithProbeBudget(1))
	if err != nil {
		t.Fatal(err)
	}

	first, err := provider.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Validate(); err != nil {
		t.Fatal(err)
	}
	if !first.TextToTextConfirmed || first.Source != domain.CapabilityProbed {
		t.Fatalf("first probe = %+v", first)
	}
	if first.Model != "tiny-v2" || first.ProbeBudgetRemaining != 0 {
		t.Fatalf("first probe metadata = %+v", first)
	}
	if first.SupportsTools || first.SupportsJSONSchema {
		t.Fatalf("probe must not invent richer capabilities: %+v", first)
	}
	if len(server.Requests()) != 1 {
		t.Fatalf("expected one network call after first probe, got %d", len(server.Requests()))
	}

	// Budget exhausted: return cached/last probe without a second network call.
	second, err := provider.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !second.TextToTextConfirmed || second.ProbeBudgetRemaining != 0 || second.Model != "tiny-v2" {
		t.Fatalf("cached probe = %+v", second)
	}
	if len(server.Requests()) != 1 {
		t.Fatalf("probe budget must prevent loops: requests=%d", len(server.Requests()))
	}
}

func TestProbeFailureDoesNotConfirmTextToText(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{StatusCode: 500, RawBody: `{"error":"secret-body"}`})
	defer server.Close()
	provider, err := openai.New(openai.Config{
		BaseURL: server.URL(),
		Model:   "tiny",
		Client:  server.Client(),
	}, openai.WithProbeBudget(1))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := provider.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if profile.TextToTextConfirmed || profile.Source != domain.CapabilityProbed {
		t.Fatalf("failed probe = %+v", profile)
	}
	if profile.ProbeBudgetRemaining != 0 {
		t.Fatalf("budget after failed probe = %d", profile.ProbeBudgetRemaining)
	}
	if strings.Contains(profile.SafeDetail, "secret-body") {
		t.Fatalf("probe detail leaked body: %q", profile.SafeDetail)
	}
	// Exhausted budget returns last snapshot without I/O.
	again, err := provider.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again.TextToTextConfirmed || again.ProbeBudgetRemaining != 0 {
		t.Fatalf("cached failed probe = %+v", again)
	}
	if len(server.Requests()) != 1 {
		t.Fatalf("expected single request, got %d", len(server.Requests()))
	}
}

func TestProviderImplementsCapabilityReporter(t *testing.T) {
	provider, err := openai.New(openai.Config{BaseURL: "http://example.test", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var _ port.ModelCapabilityReporter = provider
	var _ port.ModelProvider = provider
}
