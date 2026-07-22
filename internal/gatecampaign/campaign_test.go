package gatecampaign

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

type recordingProvider struct {
	calls  int
	result port.CompletionResult
	err    error
}

func (p *recordingProvider) Complete(context.Context, port.CompletionRequest) (port.CompletionResult, error) {
	p.calls++
	return p.result, p.err
}

func TestBoundedCallRecorderIsSharedAndFailClosed(t *testing.T) {
	recorder := &boundedCallRecorder{max: 1}
	primary := &recordingProvider{}
	fallback := &recordingProvider{}
	first := recordedProvider{bindingID: "primary", provider: primary, recorder: recorder}
	second := recordedProvider{bindingID: "fallback", provider: fallback, recorder: recorder}
	if _, err := first.Complete(context.Background(), port.CompletionRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Complete(context.Background(), port.CompletionRequest{}); err == nil {
		t.Fatal("shared recorder must reject calls beyond the campaign maximum")
	}
	if primary.calls != 1 || fallback.calls != 0 || recorder.calls != 1 {
		t.Fatalf("underlying calls primary=%d fallback=%d recorded=%d", primary.calls, fallback.calls, recorder.calls)
	}
}

func runtimeGateTestManifest() RuntimeGateCampaignManifest {
	return RuntimeGateCampaignManifest{
		SchemaVersion: 1, Name: "bounded-gate", TimeoutSeconds: 10, MaxCalls: 1,
		MaxOutputTokens: 16, ProbePrompt: "Reply with OK only.", ExpectedResponse: "OK", SeedPrimaryCircuitSeconds: 60,
		Bindings: []RuntimeGateBinding{
			{Provider: "groq", ProviderKind: domain.ProviderKindGroq, BindingID: "groq-primary", BaseURL: "https://example.invalid/v1", Model: "primary", APIKeyEnvironment: "GROQ_API_KEY", MaxOutputField: "max_tokens", ContextTokens: 2048, Priority: 0},
			{Provider: "nim", ProviderKind: domain.ProviderKindNVIDIANIM, BindingID: "nim-fallback", BaseURL: "https://example.invalid/v1", Model: "fallback", APIKeyEnvironment: "NVIDIA_NIM_API_KEY", MaxOutputField: "max_tokens", ContextTokens: 2048, Priority: 1},
		},
	}
}

func TestManifestStrictAndBounded(t *testing.T) {
	manifest := runtimeGateTestManifest()
	manifest.MaxCalls = 2
	if err := manifest.Validate(); err == nil {
		t.Fatal("max_calls above one must fail closed")
	}
	_, err := DecodeRuntimeGateCampaignManifest(strings.NewReader(`{"schema_version":1,"name":"x","unknown":true}`), 1024)
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("strict decode error=%v", err)
	}
}

func TestRunRoutesAroundSeededCircuitThenThrottlesWithoutSecondCall(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 30, 0, time.UTC)
	clock := source.NewManualClock(now)
	primary := &recordingProvider{}
	proposal, err := json.Marshal(domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_runtime_gate", MissionRevision: "revision_runtime_gate",
		OperationID: "operation_runtime_gate", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"manifest"}, Preconditions: []string{}, Changes: []domain.Change{{
			Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "runtime_gate_observation", PayloadRef: "runtime_gate_payload",
		}}, ExpectedDelta: "runtime provider gate evidence", ValidatorIDs: []string{"schema"},
		Provenance: "model:fixture", IdempotencyKey: "runtime-gate-campaign",
	})
	if err != nil {
		t.Fatal(err)
	}
	fallback := &recordingProvider{result: port.CompletionResult{Text: string(proposal), InputTokens: 7, OutputTokens: 1, Model: "fallback"}}
	store := memory.New()
	runner := RuntimeGateCampaignRunner{
		Store: store, Clock: clock,
		Providers: map[string]port.ModelProvider{"groq-primary": primary, "nim-fallback": fallback},
	}
	report, err := runner.Run(context.Background(), runtimeGateTestManifest())
	if err != nil {
		t.Fatal(err)
	}
	if primary.calls != 0 || fallback.calls != 1 || report.ExternalCalls != 1 {
		t.Fatalf("calls primary=%d fallback=%d report=%+v", primary.calls, fallback.calls, report)
	}
	if report.SelectedBindingID != "nim-fallback" || !report.ProviderSucceeded {
		t.Fatalf("route/success=%+v", report)
	}
	if !report.ExpectedResponseSet || report.ExpectedResponseMatch || report.ResponseBytes != len(proposal) || len(report.ResponseSHA256) != 64 {
		t.Fatalf("safe response evidence=%+v", report)
	}
	if report.SecondAcquireReason != "resource_resource_rate_limit" || report.SecondAcquireWait == nil || report.OperationState != domain.StateWaitingTime {
		t.Fatalf("second acquire=%+v", report)
	}
	var routed, invoked bool
	if err := store.View(context.Background(), func(reader port.Reader) error {
		events, err := reader.Events(0, 100)
		for _, event := range events {
			if event.OperationID == "operation_runtime_gate" && event.Kind == "operation.model_routed" {
				routed = true
			}
			if event.OperationID == "operation_runtime_gate" && event.Kind == "operation.model_invoked" {
				invoked = true
			}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !routed || !invoked {
		t.Fatalf("executor evidence routed=%t invoked=%t", routed, invoked)
	}
	for _, usage := range report.Usages {
		if usage.InFlight != 0 {
			t.Fatalf("leaked permit: %+v", usage)
		}
	}
	if err := VerifyRuntimeGateDurability(context.Background(), store, report); err != nil {
		t.Fatalf("durability evidence: %v", err)
	}
	var selected RuntimeGateUsage
	for _, usage := range report.Usages {
		if usage.Resource == domain.ModelBindingResource("nim-fallback") {
			selected = usage
		}
	}
	if selected.DayCount != 1 || selected.TokenMinuteCount != 8 || selected.MinuteWindowStart.IsZero() || selected.DayWindowStart.IsZero() || selected.TokenMinuteWindowStart.IsZero() {
		t.Fatalf("complete quota accounting missing from report: %+v", selected)
	}
}

func TestVerifyRuntimeGateDurabilityRejectsIncompleteAccounting(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 30, 0, time.UTC)
	proposal, err := json.Marshal(domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_runtime_gate", MissionRevision: "revision_runtime_gate",
		OperationID: "operation_runtime_gate", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"manifest"}, Changes: []domain.Change{{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "runtime_gate_observation", PayloadRef: "runtime_gate_payload"}},
		ExpectedDelta: "runtime provider gate evidence", ValidatorIDs: []string{"schema"}, Provenance: "model:fixture", IdempotencyKey: "runtime-gate-campaign",
	})
	if err != nil {
		t.Fatal(err)
	}
	store := memory.New()
	report, err := (RuntimeGateCampaignRunner{
		Store: store, Clock: source.NewManualClock(now),
		Providers: map[string]port.ModelProvider{
			"groq-primary": &recordingProvider{},
			"nim-fallback": &recordingProvider{result: port.CompletionResult{Text: string(proposal), InputTokens: 7, OutputTokens: 1}},
		},
	}).Run(context.Background(), runtimeGateTestManifest())
	if err != nil {
		t.Fatal(err)
	}
	for i := range report.Usages {
		if report.Usages[i].Resource == domain.ModelBindingResource("nim-fallback") {
			report.Usages[i].DayCount++
		}
	}
	if err := VerifyRuntimeGateDurability(context.Background(), store, report); err == nil || !strings.Contains(err.Error(), "durable usage mismatch") {
		t.Fatalf("tampered accounting must fail, got %v", err)
	}
}

type providerHTTPError struct{ status int }

func (e providerHTTPError) Error() string                  { return "provider unavailable" }
func (e providerHTTPError) RetryAfterDelay() time.Duration { return 20 * time.Second }
func (e providerHTTPError) HTTPStatusCode() int            { return e.status }
func (e providerHTTPError) RetryableFailure() bool         { return true }

type wrappedProvider struct{ err error }

func (p wrappedProvider) Complete(context.Context, port.CompletionRequest) (port.CompletionResult, error) {
	return port.CompletionResult{}, fmt.Errorf("wrapped provider failure: %w", p.err)
}

func TestRunProjectsWrappedProviderHTTPStatus(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	runner := RuntimeGateCampaignRunner{
		Store: memory.New(), Clock: source.NewManualClock(now),
		Providers: map[string]port.ModelProvider{
			"groq-primary": &recordingProvider{},
			"nim-fallback": wrappedProvider{err: providerHTTPError{status: 401}},
		},
	}
	report, err := runner.Run(context.Background(), runtimeGateTestManifest())
	if err != nil {
		t.Fatal(err)
	}
	if report.ProviderErrorClass != "http" || report.ProviderHTTPStatus != 401 {
		t.Fatalf("wrapped provider evidence=%+v", report)
	}
	if report.ProviderLatency < 0 {
		t.Fatalf("provider latency=%s", report.ProviderLatency)
	}
}

func TestRunRecordsExactExpectedResponseWithoutPersistingRawText(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 20, 0, 0, time.UTC)
	runner := RuntimeGateCampaignRunner{
		Store: memory.New(), Clock: source.NewManualClock(now),
		Providers: map[string]port.ModelProvider{
			"groq-primary": &recordingProvider{},
			"nim-fallback": &recordingProvider{result: port.CompletionResult{Text: "OK", InputTokens: 3, OutputTokens: 1}},
		},
	}
	report, err := runner.Run(context.Background(), runtimeGateTestManifest())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ExpectedResponseSet || !report.ExpectedResponseMatch || report.ResponseBytes != 2 || len(report.ResponseSHA256) != 64 {
		t.Fatalf("exact response evidence=%+v", report)
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"text":"OK"`) || strings.Contains(string(body), `"response":"OK"`) {
		t.Fatalf("raw provider text leaked into report: %s", body)
	}
}

func TestRunRecordsNaturalProviderThrottleAndReleasesPermits(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 30, 0, time.UTC)
	provider := &recordingProvider{err: providerHTTPError{status: 429}}
	runner := RuntimeGateCampaignRunner{
		Store: memory.New(), Clock: source.NewManualClock(now),
		Providers: map[string]port.ModelProvider{"groq-primary": &recordingProvider{}, "nim-fallback": provider},
	}
	report, err := runner.Run(context.Background(), runtimeGateTestManifest())
	if err != nil {
		t.Fatal(err)
	}
	if report.ProviderSucceeded || report.ProviderErrorClass != "http" || report.ProviderHTTPStatus != 429 || report.ProviderRetryAfter != 20*time.Second {
		t.Fatalf("provider evidence=%+v", report)
	}
	for _, usage := range report.Usages {
		if usage.InFlight != 0 {
			t.Fatalf("leaked permit: %+v", usage)
		}
	}
	if err := VerifyRuntimeGateDurability(context.Background(), runner.Store, report); err != nil {
		t.Fatalf("provider failure audit must survive durability verification: %v", err)
	}
}

func TestArtifactsRejectOverBudgetReport(t *testing.T) {
	err := WriteRuntimeGateCampaignArtifacts(t.TempDir(), RuntimeGateCampaignReport{SchemaVersion: 1, MaxCalls: 1, ExternalCalls: 2, Usages: []RuntimeGateUsage{{Resource: "x"}}})
	if err == nil {
		t.Fatal("over-budget report must fail")
	}
}
