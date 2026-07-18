package inspect_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/provider/openai/fakeserver"
	"motor-autonomo/internal/storage/memory"
)

type stubProvider struct {
	profile domain.ProviderProfile
	calls   int
}

func (s *stubProvider) Complete(context.Context, port.CompletionRequest) (port.CompletionResult, error) {
	return port.CompletionResult{}, nil
}

func (s *stubProvider) DeclaredProfile() domain.ProviderProfile {
	return s.profile
}

func (s *stubProvider) Probe(context.Context) (domain.ProviderProfile, error) {
	s.calls++
	p := s.profile
	p.Source = domain.CapabilityProbed
	p.TextToTextConfirmed = true
	p.ProbeBudgetRemaining = 0
	p.SafeDetail = "stub probe"
	return p, nil
}

func TestProviderProfileInspectWithoutProvider(t *testing.T) {
	store := memory.New()
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := projector.ProviderProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if view.Configured || view.Profile != nil || view.Note == "" {
		t.Fatalf("unconfigured view = %#v", view)
	}
	probe, err := projector.ProviderProfileProbe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if probe.Configured || probe.Live {
		t.Fatalf("unconfigured probe = %#v", probe)
	}
}

func TestProviderProfileDeclaredAndProbe(t *testing.T) {
	now := time.Date(2026, 7, 16, 15, 30, 0, 0, time.UTC)
	base := domain.BaselineDeclaredProfile("fixture", "tiny", domain.MaxOutputDialectLegacy, 2048, now)
	stub := &stubProvider{profile: base}

	store := memory.New()
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.SetModelProvider(stub)

	declared, err := projector.ProviderProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !declared.Configured || declared.Live || declared.Profile == nil {
		t.Fatalf("declared = %#v", declared)
	}
	if declared.Profile.Source != domain.CapabilityDeclared || declared.Profile.TextToTextConfirmed {
		t.Fatalf("declared profile = %+v", declared.Profile)
	}
	if stub.calls != 0 {
		t.Fatalf("DeclaredProfile must not call Probe, calls=%d", stub.calls)
	}

	live, err := projector.ProviderProfileProbe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !live.Configured || !live.Live || live.Profile == nil || !live.Profile.TextToTextConfirmed {
		t.Fatalf("live probe = %#v", live)
	}
	if stub.calls != 1 {
		t.Fatalf("probe calls = %d", stub.calls)
	}

	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	resp := mustGET(t, server.URL+"/provider/profile")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("profile status = %d", resp.StatusCode)
	}
	var httpView inspect.ProviderProfileView
	if err := json.NewDecoder(resp.Body).Decode(&httpView); err != nil {
		t.Fatal(err)
	}
	if !httpView.Configured || httpView.Live || httpView.Profile == nil {
		t.Fatalf("http declared = %#v", httpView)
	}

	probeResp := mustGET(t, server.URL+"/provider/profile/probe")
	defer probeResp.Body.Close()
	if probeResp.StatusCode != 200 {
		t.Fatalf("probe status = %d", probeResp.StatusCode)
	}
	var httpProbe inspect.ProviderProfileView
	if err := json.NewDecoder(probeResp.Body).Decode(&httpProbe); err != nil {
		t.Fatal(err)
	}
	if !httpProbe.Live || httpProbe.Profile == nil || !httpProbe.Profile.TextToTextConfirmed {
		t.Fatalf("http probe = %#v", httpProbe)
	}
}

func TestProviderProfileAgainstOpenAIAdapter(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{
		ExpectedPrompt: "ping", ExpectedModel: "tiny", ExpectedMaxOutputField: "max_tokens",
		ResponseText: "pong", ResponseModel: "tiny-live", InputTokens: 1, OutputTokens: 1,
	})
	t.Cleanup(server.Close)
	provider, err := openai.New(openai.Config{
		BaseURL: server.URL(),
		Model:   "tiny",
		Client:  server.Client(),
	}, openai.WithProfileName("local-chat"), openai.WithContextTokens(2048), openai.WithProbeBudget(1))
	if err != nil {
		t.Fatal(err)
	}
	store := memory.New()
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.SetModelProvider(provider)

	declared, err := projector.ProviderProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if declared.Profile == nil || declared.Profile.Name != "local-chat" || declared.Profile.TextToTextConfirmed {
		t.Fatalf("declared openai profile = %#v", declared)
	}
	if len(server.Requests()) != 0 {
		t.Fatal("declared must not hit network")
	}

	live, err := projector.ProviderProfileProbe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if live.Profile == nil || !live.Profile.TextToTextConfirmed || live.Profile.Model != "tiny-live" {
		t.Fatalf("live openai profile = %#v", live)
	}
	// Second probe is budget-cached.
	again, err := projector.ProviderProfileProbe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !again.Profile.TextToTextConfirmed || len(server.Requests()) != 1 {
		t.Fatalf("budgeted probe loop: profile=%#v requests=%d", again, len(server.Requests()))
	}
}

func TestProviderModelsAgainstOpenAIAdapterIsInformationalOnly(t *testing.T) {
	server := fakeserver.New(fakeserver.Exchange{ModelsResponse: []string{"tiny", "unconfigured"}})
	t.Cleanup(server.Close)
	provider, err := openai.New(
		openai.Config{BaseURL: server.URL(), Model: "tiny", Client: server.Client()},
		openai.WithAllowedModels("tiny"),
	)
	if err != nil {
		t.Fatal(err)
	}
	store := memory.New()
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.SetModelProvider(provider)

	view, err := projector.ProviderModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !view.Configured || len(view.Models) != 1 || view.Models[0] != "tiny" || view.Note == "" {
		t.Fatalf("models view = %#v", view)
	}

	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(api.Handler())
	t.Cleanup(httpServer.Close)
	resp := mustGET(t, httpServer.URL+"/provider/models")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("models status = %d", resp.StatusCode)
	}
	var got inspect.ProviderModelsView
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 1 || got.Models[0] != "tiny" {
		t.Fatalf("http models = %#v", got)
	}
}
