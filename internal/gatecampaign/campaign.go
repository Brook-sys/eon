package gatecampaign

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/bootstrap"
	"motor-autonomo/internal/runtime/source"
)

const RuntimeGateCampaignSchemaVersion = 1

// RuntimeGateCampaignManifest declares a narrow live probe of the real routing
// and ResourceGate path. It intentionally permits exactly one external call:
// a seeded primary circuit routes to the alternate binding, then the successful
// call exhausts a one-call local minute quota so a second reservation parks
// without contacting either provider.
type RuntimeGateCampaignManifest struct {
	SchemaVersion             int                  `json:"schema_version"`
	Name                      string               `json:"name"`
	TimeoutSeconds            int                  `json:"timeout_seconds"`
	MaxCalls                  int                  `json:"max_calls"`
	MaxOutputTokens           int                  `json:"max_output_tokens"`
	ProbePrompt               string               `json:"probe_prompt"`
	ExpectedResponse          string               `json:"expected_response,omitempty"`
	SeedPrimaryCircuitSeconds int                  `json:"seed_primary_circuit_seconds"`
	Bindings                  []RuntimeGateBinding `json:"bindings"`
}

type RuntimeGateBinding struct {
	Provider          string              `json:"provider"`
	ProviderKind      domain.ProviderKind `json:"provider_kind"`
	BindingID         string              `json:"binding_id"`
	BaseURL           string              `json:"base_url"`
	Model             string              `json:"model"`
	APIKeyEnvironment string              `json:"api_key_env"`
	MaxOutputField    string              `json:"max_output_field"`
	ContextTokens     int                 `json:"context_tokens"`
	Priority          int                 `json:"priority"`
}

func DecodeRuntimeGateCampaignManifest(r io.Reader, maxBytes int64) (RuntimeGateCampaignManifest, error) {
	if r == nil || maxBytes <= 0 {
		return RuntimeGateCampaignManifest{}, errors.New("runtime gate campaign reader and positive byte limit are required")
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return RuntimeGateCampaignManifest{}, fmt.Errorf("read runtime gate campaign manifest: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return RuntimeGateCampaignManifest{}, errors.New("runtime gate campaign manifest exceeds byte limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest RuntimeGateCampaignManifest
	if err := decoder.Decode(&manifest); err != nil {
		return RuntimeGateCampaignManifest{}, fmt.Errorf("decode runtime gate campaign manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return RuntimeGateCampaignManifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return RuntimeGateCampaignManifest{}, err
	}
	return manifest, nil
}

func (m RuntimeGateCampaignManifest) Validate() error {
	if m.SchemaVersion != RuntimeGateCampaignSchemaVersion || strings.TrimSpace(m.Name) == "" {
		return errors.New("runtime gate campaign identity and supported schema version are required")
	}
	if m.TimeoutSeconds <= 0 || m.TimeoutSeconds > 300 {
		return errors.New("runtime gate campaign timeout must be between 1 and 300 seconds")
	}
	if m.MaxCalls != 1 {
		return errors.New("runtime gate campaign requires max_calls exactly 1")
	}
	if m.MaxOutputTokens <= 0 || m.MaxOutputTokens > 128 {
		return errors.New("runtime gate campaign max_output_tokens must be between 1 and 128")
	}
	if prompt := strings.TrimSpace(m.ProbePrompt); prompt == "" || len(prompt) > 1024 {
		return errors.New("runtime gate campaign probe_prompt is required and bounded to 1024 bytes")
	}
	if len(m.ExpectedResponse) > 1024 {
		return errors.New("runtime gate campaign expected_response is bounded to 1024 bytes")
	}
	if m.SeedPrimaryCircuitSeconds <= 0 || m.SeedPrimaryCircuitSeconds > 300 {
		return errors.New("seed_primary_circuit_seconds must be between 1 and 300")
	}
	if len(m.Bindings) != 2 {
		return errors.New("runtime gate campaign requires exactly two bindings")
	}
	seenBindings, seenProviders := map[string]bool{}, map[string]bool{}
	for i, binding := range m.Bindings {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("runtime gate binding %d: %w", i, err)
		}
		if seenBindings[binding.BindingID] || seenProviders[binding.Provider] {
			return errors.New("runtime gate campaign requires distinct provider and binding IDs")
		}
		seenBindings[binding.BindingID], seenProviders[binding.Provider] = true, true
	}
	ordered := append([]RuntimeGateBinding(nil), m.Bindings...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Priority < ordered[j].Priority })
	if ordered[0].Priority >= ordered[1].Priority {
		return errors.New("runtime gate campaign bindings require distinct priorities")
	}
	return nil
}

func (b RuntimeGateBinding) Validate() error {
	for label, value := range map[string]string{
		"provider": b.Provider, "binding_id": b.BindingID, "base_url": b.BaseURL,
		"model": b.Model, "api_key_env": b.APIKeyEnvironment, "max_output_field": b.MaxOutputField,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
	}
	switch b.ProviderKind {
	case domain.ProviderKindGroq, domain.ProviderKindNVIDIANIM, domain.ProviderKindOpenAICompatible:
	default:
		return fmt.Errorf("unsupported provider_kind %q", b.ProviderKind)
	}
	if b.MaxOutputField != "max_tokens" && b.MaxOutputField != "max_completion_tokens" {
		return errors.New("unsupported max_output_field")
	}
	if b.ContextTokens <= 0 {
		return errors.New("context_tokens must be positive")
	}
	return nil
}

type RuntimeGateUsage struct {
	Resource               domain.ResourceID `json:"resource"`
	InFlight               int               `json:"in_flight"`
	MinuteWindowStart      time.Time         `json:"minute_window_start,omitempty"`
	MinuteCount            int               `json:"minute_count"`
	DayWindowStart         time.Time         `json:"day_window_start,omitempty"`
	DayCount               int               `json:"day_count"`
	TokenMinuteWindowStart time.Time         `json:"token_minute_window_start,omitempty"`
	TokenMinuteCount       int               `json:"token_minute_count"`
	ConsecutiveFailures    int               `json:"consecutive_failures"`
	CircuitOpenUntil       *time.Time        `json:"circuit_open_until,omitempty"`
	LastFailureAt          *time.Time        `json:"last_failure_at,omitempty"`
}

type RuntimeGateCampaignReport struct {
	SchemaVersion         int                         `json:"schema_version"`
	Name                  string                      `json:"name"`
	StartedAt             time.Time                   `json:"started_at"`
	CompletedAt           time.Time                   `json:"completed_at"`
	MaxCalls              int                         `json:"max_calls"`
	ExternalCalls         int                         `json:"external_calls"`
	SeededCircuit         domain.ResourceID           `json:"seeded_circuit"`
	SelectedProviderID    string                      `json:"selected_provider_id"`
	SelectedBindingID     string                      `json:"selected_binding_id"`
	RouteRejected         map[string]string           `json:"route_rejected,omitempty"`
	ProviderSucceeded     bool                        `json:"provider_succeeded"`
	ProviderLatency       time.Duration               `json:"provider_latency"`
	ProviderErrorClass    string                      `json:"provider_error_class,omitempty"`
	ProviderHTTPStatus    int                         `json:"provider_http_status,omitempty"`
	ProviderRetryAfter    time.Duration               `json:"provider_retry_after,omitempty"`
	ObservedInputTokens   int                         `json:"observed_input_tokens,omitempty"`
	ObservedOutputTokens  int                         `json:"observed_output_tokens,omitempty"`
	FinishReason          port.CompletionFinishReason `json:"finish_reason,omitempty"`
	ResponseBytes         int                         `json:"response_bytes,omitempty"`
	ResponseSHA256        string                      `json:"response_sha256,omitempty"`
	ExpectedResponseSet   bool                        `json:"expected_response_set"`
	ExpectedResponseMatch bool                        `json:"expected_response_match"`
	SecondAcquireReason   string                      `json:"second_acquire_reason"`
	SecondAcquireWait     *time.Time                  `json:"second_acquire_wait_until,omitempty"`
	OperationState        domain.OperationalState     `json:"operation_state"`
	Usages                []RuntimeGateUsage          `json:"usages"`
	DurableReopen         bool                        `json:"durable_reopen"`
}

// RuntimeGateCampaignRunner executes the bounded probe against an already-open
// store. The caller owns durable reopen verification and artifact persistence.
type RuntimeGateCampaignRunner struct {
	Store     port.Store
	Clock     source.Clock
	Providers map[string]port.ModelProvider
}

type boundedCallRecorder struct {
	max       int
	calls     int
	bindingID string
	latency   time.Duration
	result    port.CompletionResult
	err       error
}

type recordedProvider struct {
	bindingID string
	provider  port.ModelProvider
	recorder  *boundedCallRecorder
}

func (p recordedProvider) Complete(ctx context.Context, request port.CompletionRequest) (port.CompletionResult, error) {
	if p.recorder.calls >= p.recorder.max {
		return port.CompletionResult{}, errors.New("runtime gate external call budget exhausted")
	}
	p.recorder.calls++
	p.recorder.bindingID = p.bindingID
	started := time.Now()
	p.recorder.result, p.recorder.err = p.provider.Complete(ctx, request)
	p.recorder.latency = time.Since(started)
	return p.recorder.result, p.recorder.err
}

func (r RuntimeGateCampaignRunner) Run(ctx context.Context, manifest RuntimeGateCampaignManifest) (RuntimeGateCampaignReport, error) {
	if err := manifest.Validate(); err != nil {
		return RuntimeGateCampaignReport{}, err
	}
	if r.Store == nil || r.Clock == nil || len(r.Providers) != 2 {
		return RuntimeGateCampaignReport{}, errors.New("runtime gate campaign requires store, clock, and two providers")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	started := r.Clock.Now().UTC()
	ordered := append([]RuntimeGateBinding(nil), manifest.Bindings...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Priority < ordered[j].Priority })
	primary := ordered[0]

	config, _, _, err := runtimeGateSeed(r.Store, manifest, started)
	if err != nil {
		return RuntimeGateCampaignReport{}, err
	}
	ids := source.NewSequenceIDGenerator(1)
	executor, err := bootstrap.BuildModelExecutor(bootstrap.Options{Model: &bootstrap.ModelOptions{
		Enabled: true, PolicyVersion: "policy@runtime-gate-campaign", LeaseTTL: time.Duration(manifest.TimeoutSeconds) * time.Second,
	}}, r.Store, r.Clock, ids, nil)
	if err != nil {
		return RuntimeGateCampaignReport{}, fmt.Errorf("build runtime gate model executor: %w", err)
	}
	if executor == nil {
		return RuntimeGateCampaignReport{}, errors.New("runtime gate model executor was not enabled")
	}
	recorder := &boundedCallRecorder{max: manifest.MaxCalls}
	executor.Providers = make(map[string]port.ModelProvider, len(r.Providers))
	for bindingID, provider := range r.Providers {
		executor.Providers[bindingID] = recordedProvider{bindingID: bindingID, provider: provider, recorder: recorder}
	}
	_, _ = executor.Execute(ctx, "operation_runtime_gate")
	if recorder.calls != manifest.MaxCalls {
		return RuntimeGateCampaignReport{}, fmt.Errorf("runtime gate executor made %d external calls, want %d", recorder.calls, manifest.MaxCalls)
	}
	secondResult, err := executor.Execute(ctx, "operation_runtime_gate_quota")
	if err != nil {
		return RuntimeGateCampaignReport{}, err
	}
	if !secondResult.Skipped || secondResult.SkipReason == "" {
		return RuntimeGateCampaignReport{}, errors.New("second executor operation unexpectedly passed the one-call quota")
	}
	if recorder.calls != manifest.MaxCalls {
		return RuntimeGateCampaignReport{}, errors.New("second executor operation contacted a provider")
	}
	selected := config.Bindings[0]
	for _, binding := range config.Bindings {
		if binding.ID == recorder.bindingID {
			selected = binding
			break
		}
	}
	if selected.ID == primary.BindingID {
		return RuntimeGateCampaignReport{}, errors.New("seeded primary circuit did not route executor to alternate binding")
	}
	report := RuntimeGateCampaignReport{
		SchemaVersion: RuntimeGateCampaignSchemaVersion, Name: manifest.Name, StartedAt: started,
		MaxCalls: manifest.MaxCalls, ExternalCalls: recorder.calls,
		SeededCircuit:      domain.ModelBindingResource(primary.BindingID),
		SelectedProviderID: selected.ProviderRef, SelectedBindingID: selected.ID,
		RouteRejected:       map[string]string{primary.BindingID: "circuit_open"},
		ProviderLatency:     recorder.latency,
		SecondAcquireReason: secondResult.SkipReason,
	}
	if recorder.err == nil {
		report.ProviderSucceeded = true
		report.ObservedInputTokens = recorder.result.InputTokens
		report.ObservedOutputTokens = recorder.result.OutputTokens
		report.FinishReason = recorder.result.FinishReason
		report.ResponseBytes = len(recorder.result.Text)
		digest := sha256.Sum256([]byte(recorder.result.Text))
		report.ResponseSHA256 = fmt.Sprintf("%x", digest[:])
		if manifest.ExpectedResponse != "" {
			report.ExpectedResponseSet = true
			report.ExpectedResponseMatch = recorder.result.Text == manifest.ExpectedResponse
		}
	} else {
		report.ProviderErrorClass = "transport"
		var providerErr port.ProviderError
		if errors.As(recorder.err, &providerErr) {
			report.ProviderErrorClass = "provider"
			report.ProviderRetryAfter = providerErr.RetryAfterDelay()
		}
		var httpErr port.ProviderHTTPError
		if errors.As(recorder.err, &httpErr) {
			report.ProviderErrorClass = "http"
			report.ProviderHTTPStatus = httpErr.HTTPStatusCode()
		}
	}
	if err := runtimeGateSnapshot(ctx, r.Store, config, &report); err != nil {
		return RuntimeGateCampaignReport{}, err
	}
	report.CompletedAt = r.Clock.Now().UTC()
	return report, nil
}

func runtimeGateSeed(store port.Store, manifest RuntimeGateCampaignManifest, now time.Time) (domain.ModelsConfig, domain.OperationSpec, domain.Operation, error) {
	limit := func(resource domain.ResourceID) domain.ResourceLimit {
		return domain.ResourceLimit{Resource: resource, MaxConcurrent: 1, MaxPerMinute: 1, FailureThreshold: 1, CooldownBase: time.Minute, CooldownMax: time.Minute}
	}
	providers := make([]domain.ModelProviderConfig, 0, 2)
	bindings := make([]domain.ModelBindingConfig, 0, 2)
	for _, item := range manifest.Bindings {
		providers = append(providers, domain.ModelProviderConfig{ID: item.Provider, Kind: item.ProviderKind, BaseURL: item.BaseURL, APIKeyEnv: item.APIKeyEnvironment, Timeout: time.Duration(manifest.TimeoutSeconds) * time.Second, MaxResponseBytes: 1 << 20, GlobalLimit: limit(domain.ModelProviderResource(item.Provider))})
		dialect := domain.MaxOutputDialectLegacy
		if item.MaxOutputField == "max_completion_tokens" {
			dialect = domain.MaxOutputDialectCompletion
		}
		bindings = append(bindings, domain.ModelBindingConfig{ID: item.BindingID, ProviderRef: item.Provider, ModelID: item.Model, Enabled: true, Priority: item.Priority, ContextTokens: item.ContextTokens, MaxOutputTokens: manifest.MaxOutputTokens, MaxOutputDialect: dialect, Limit: limit(domain.ModelBindingResource(item.BindingID))})
	}
	config := domain.ModelsConfig{Version: "models@runtime-gate-campaign", Providers: providers, Bindings: bindings}
	if err := config.Validate(); err != nil {
		return config, domain.OperationSpec{}, domain.Operation{}, err
	}
	spec := domain.OperationSpec{SchemaVersion: 1, ID: "runtime-gate-probe@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "probe_text", OutputSchema: "proposed changeset", Budget: domain.Budget{ModelCalls: 1, Tokens: 4000, Attempts: 1}, MaxOutputTokens: manifest.MaxOutputTokens, SafetyMargin: 1, Validators: []string{"schema"}, RetryPolicy: "none", FallbackPolicy: "catalog", MaximumAuthority: domain.AuthorityProposeOnly}
	revision := domain.MissionRevision{SchemaVersion: 1, ID: "revision_runtime_gate", MissionID: "mission_runtime_gate", Revision: 1, OriginalText: "bounded provider gate probe", Purpose: "validate quota and routing", Domains: []string{"operations"}, Policies: []string{"no authority"}, Status: domain.MissionActive, Provenance: "operator-manifest", AcceptedAt: now, Budget: domain.Budget{ModelCalls: 1, Tokens: 4000, Attempts: 1}}
	question := domain.Question{SchemaVersion: 1, ID: "question_runtime_gate", MissionRevision: revision.ID, Text: "is the provider gate operational?", Origin: "campaign", Relevance: "diagnostic", AnswerCondition: "one bounded call"}
	candidate := domain.InquiryCandidate{SchemaVersion: 1, ID: "candidate_runtime_gate", MissionRevision: revision.ID, QuestionID: question.ID, DerivedFrom: []string{"manifest"}, ExpectedProgress: "runtime evidence", Novelty: "dated probe", Risk: domain.RiskLow, SourcePlan: []string{"provider"}, AnswerCondition: "report", StopCondition: "one call", ReviewAfter: now.Add(time.Hour)}
	inquiry := domain.Inquiry{SchemaVersion: 1, ID: "inquiry_runtime_gate", CandidateID: candidate.ID, MissionRevision: revision.ID, QuestionID: question.ID, AdmissionReason: "bounded campaign", StopCondition: "one call", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	operation := domain.Operation{SchemaVersion: 1, ID: "operation_runtime_gate", InquiryID: inquiry.ID, MissionRevision: revision.ID, SpecID: spec.ID, ReadSet: []string{"manifest"}, InputRefs: []string{"probe_prompt"}, ExpectedOutput: manifest.ProbePrompt, IdempotencyKey: "runtime-gate-campaign", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	quotaOperation := operation
	quotaOperation.ID = "operation_runtime_gate_quota"
	quotaOperation.IdempotencyKey = "runtime-gate-campaign-quota"
	ordered := append([]RuntimeGateBinding(nil), manifest.Bindings...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Priority < ordered[j].Priority })
	primaryCircuit := now.Add(time.Duration(manifest.SeedPrimaryCircuitSeconds) * time.Second)
	err := store.Update(context.Background(), func(tx port.Transaction) error {
		hash, err := domain.ConfigPayloadHash(domain.ConfigScopeModels, nil, nil, nil, nil, nil, &config)
		if err != nil {
			return err
		}
		draft := domain.ConfigDraft{SchemaVersion: 1, ID: "draft_runtime_gate_models", Scope: domain.ConfigScopeModels, Applicability: domain.ConfigHot, Status: domain.ConfigDraftOpen, ActorType: domain.ActorOperator, ActorID: "runtime-gate-campaign", Reason: "bounded live model gate campaign", Models: &config, CreatedAt: now}
		revisionConfig := domain.ConfigRevision{SchemaVersion: 1, ID: "config_runtime_gate_models", Scope: domain.ConfigScopeModels, Revision: 1, Applicability: domain.ConfigHot, ContentHash: hash, ActorType: domain.ActorOperator, ActorID: "runtime-gate-campaign", Reason: draft.Reason, DraftID: draft.ID, Models: &config, AcceptedAt: now}
		if err := tx.CreateConfigDraft(draft); err != nil {
			return err
		}
		draft.Status = domain.ConfigDraftValidated
		draft.ValidatedAt = now
		if err := tx.SaveConfigDraft(draft); err != nil {
			return err
		}
		draft.Status = domain.ConfigDraftApplied
		if err := tx.SaveConfigDraft(draft); err != nil {
			return err
		}
		if err := tx.AppendConfigRevision(revisionConfig); err != nil {
			return err
		}
		if err := tx.ActivateConfigRevision(domain.ConfigScopeModels, revisionConfig.ID); err != nil {
			return err
		}
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		if err := tx.CreateOperation(operation); err != nil {
			return err
		}
		if err := tx.CreateOperation(quotaOperation); err != nil {
			return err
		}
		return tx.SaveResourceUsage(domain.ResourceUsage{Resource: domain.ModelBindingResource(ordered[0].BindingID), ConsecutiveFailures: 1, CircuitOpenUntil: &primaryCircuit, LastFailureAt: &now})
	})
	return config, spec, operation, err
}

func runtimeGateSnapshot(ctx context.Context, store port.Store, config domain.ModelsConfig, report *RuntimeGateCampaignReport) error {
	return store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation("operation_runtime_gate_quota")
		if err != nil {
			return err
		}
		report.OperationState = op.State
		report.SecondAcquireWait = op.Reevaluation.NotBefore
		for _, provider := range config.Providers {
			usage, err := r.ResourceUsage(provider.GlobalLimit.Resource)
			if err != nil && !errors.Is(err, port.ErrNotFound) {
				return err
			}
			if errors.Is(err, port.ErrNotFound) {
				usage.Resource = provider.GlobalLimit.Resource
			}
			report.Usages = append(report.Usages, runtimeGateUsage(usage))
		}
		for _, binding := range config.Bindings {
			usage, err := r.ResourceUsage(binding.Limit.Resource)
			if err != nil && !errors.Is(err, port.ErrNotFound) {
				return err
			}
			if errors.Is(err, port.ErrNotFound) {
				usage.Resource = binding.Limit.Resource
			}
			report.Usages = append(report.Usages, runtimeGateUsage(usage))
		}
		sort.Slice(report.Usages, func(i, j int) bool { return report.Usages[i].Resource < report.Usages[j].Resource })
		return nil
	})
}

func runtimeGateUsage(usage domain.ResourceUsage) RuntimeGateUsage {
	return RuntimeGateUsage{
		Resource: usage.Resource, InFlight: usage.InFlight,
		MinuteWindowStart: usage.MinuteWindowStart, MinuteCount: usage.MinuteCount,
		DayWindowStart: usage.DayWindowStart, DayCount: usage.DayCount,
		TokenMinuteWindowStart: usage.TokenMinuteWindowStart, TokenMinuteCount: usage.TokenMinuteCount,
		ConsecutiveFailures: usage.ConsecutiveFailures, CircuitOpenUntil: usage.CircuitOpenUntil,
		LastFailureAt: usage.LastFailureAt,
	}
}

// VerifyRuntimeGateDurability proves that reopening the campaign store
// preserved the complete ResourceGate accounting, the parked quota operation,
// and the executor audit trail. It deliberately compares window timestamps and
// token/day counters too: matching only in-flight/minute counters can conceal a
// checkpoint regression that grants excess capacity after restart.
func VerifyRuntimeGateDurability(ctx context.Context, store port.ReadStore, report RuntimeGateCampaignReport) error {
	if store == nil {
		return errors.New("runtime gate durability verification requires a store")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return store.View(ctx, func(reader port.Reader) error {
		for _, expected := range report.Usages {
			persisted, err := reader.ResourceUsage(expected.Resource)
			if errors.Is(err, port.ErrNotFound) && runtimeGateUsageIsZero(expected) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read durable usage %s: %w", expected.Resource, err)
			}
			if actual := runtimeGateUsage(persisted); !runtimeGateUsageEqual(actual, expected) {
				return fmt.Errorf("durable usage mismatch for %s: got %+v want %+v", expected.Resource, actual, expected)
			}
			if persisted.InFlight != 0 {
				return fmt.Errorf("durable usage %s retained %d in-flight permits", expected.Resource, persisted.InFlight)
			}
		}
		op, err := reader.Operation("operation_runtime_gate_quota")
		if err != nil {
			return fmt.Errorf("read durable quota operation: %w", err)
		}
		if op.State != report.OperationState || !equalTimePtr(op.Reevaluation.NotBefore, report.SecondAcquireWait) {
			return fmt.Errorf("durable quota operation mismatch: state=%s wait=%v, want state=%s wait=%v", op.State, op.Reevaluation.NotBefore, report.OperationState, report.SecondAcquireWait)
		}
		events, err := reader.Events(0, 100)
		if err != nil {
			return fmt.Errorf("read durable runtime gate events: %w", err)
		}
		required := map[string]bool{
			"operation.model_routed":  false,
			"operation.model_invoked": false,
		}
		// A successful first call consumes the one-call quota and the second
		// operation must persist resource.throttled. A failed first call may open
		// the selected circuit instead, leaving the second operation parked by
		// routing unavailability without acquiring a resource permit.
		if strings.HasPrefix(report.SecondAcquireReason, "resource_") {
			required["resource.throttled"] = false
		}
		for _, event := range events {
			if event.OperationID == "operation_runtime_gate" && (event.Kind == "operation.model_routed" || event.Kind == "operation.model_invoked") {
				required[event.Kind] = true
			}
			if event.OperationID == "operation_runtime_gate_quota" && event.Kind == "resource.throttled" {
				required[event.Kind] = true
			}
		}
		for kind, found := range required {
			if !found {
				return fmt.Errorf("durable runtime gate event %s is missing", kind)
			}
		}
		return nil
	})
}

func runtimeGateUsageIsZero(usage RuntimeGateUsage) bool {
	return usage.InFlight == 0 && usage.MinuteWindowStart.IsZero() && usage.MinuteCount == 0 &&
		usage.DayWindowStart.IsZero() && usage.DayCount == 0 && usage.TokenMinuteWindowStart.IsZero() &&
		usage.TokenMinuteCount == 0 && usage.ConsecutiveFailures == 0 && usage.CircuitOpenUntil == nil && usage.LastFailureAt == nil
}

func runtimeGateUsageEqual(a, b RuntimeGateUsage) bool {
	return a.Resource == b.Resource && a.InFlight == b.InFlight && a.MinuteWindowStart.Equal(b.MinuteWindowStart) &&
		a.MinuteCount == b.MinuteCount && a.DayWindowStart.Equal(b.DayWindowStart) && a.DayCount == b.DayCount &&
		a.TokenMinuteWindowStart.Equal(b.TokenMinuteWindowStart) && a.TokenMinuteCount == b.TokenMinuteCount &&
		a.ConsecutiveFailures == b.ConsecutiveFailures && equalTimePtr(a.CircuitOpenUntil, b.CircuitOpenUntil) &&
		equalTimePtr(a.LastFailureAt, b.LastFailureAt)
}

func equalTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func WriteRuntimeGateCampaignManifest(path string, manifest RuntimeGateCampaignManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(body, '\n'))
}

func WriteRuntimeGateCampaignArtifacts(directory string, report RuntimeGateCampaignReport) error {
	if strings.TrimSpace(directory) == "" || report.SchemaVersion != RuntimeGateCampaignSchemaVersion || report.ExternalCalls > report.MaxCalls || len(report.Usages) == 0 {
		return errors.New("artifact directory and complete bounded runtime gate report are required")
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(directory+"/runtime-gate.json", append(body, '\n')); err != nil {
		return err
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# Runtime provider gate campaign\n\n- Name: `%s`\n- External calls: %d/%d\n- Seeded circuit: `%s`\n- Selected route: `%s` / `%s`\n- Provider success: `%t`\n- Provider latency: `%s`\n- Provider error class: `%s`\n- Provider HTTP status: %d\n- Provider Retry-After: `%s`\n- Finish reason: `%s`\n- Response bytes: %d\n- Response SHA-256: `%s`\n- Expected response configured: `%t`\n- Expected response exact match: `%t`\n- Second acquire: `%s`", report.Name, report.ExternalCalls, report.MaxCalls, report.SeededCircuit, report.SelectedProviderID, report.SelectedBindingID, report.ProviderSucceeded, report.ProviderLatency, report.ProviderErrorClass, report.ProviderHTTPStatus, report.ProviderRetryAfter, report.FinishReason, report.ResponseBytes, report.ResponseSHA256, report.ExpectedResponseSet, report.ExpectedResponseMatch, report.SecondAcquireReason)
	if report.SecondAcquireWait != nil {
		fmt.Fprintf(&md, " until `%s`", report.SecondAcquireWait.UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(&md, "\n- Operation state after local throttle: `%s`\n- Durable reopen verified: `%t`\n\n", report.OperationState, report.DurableReopen)
	md.WriteString("| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |\n| --- | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, usage := range report.Usages {
		until := ""
		if usage.CircuitOpenUntil != nil {
			until = usage.CircuitOpenUntil.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(&md, "| %s | %d | %d | %d | %d | %d | %s |\n", usage.Resource, usage.InFlight, usage.MinuteCount, usage.DayCount, usage.TokenMinuteCount, usage.ConsecutiveFailures, until)
	}
	md.WriteString("\nThe primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.\n")
	return atomicWrite(directory+"/runtime-gate.md", []byte(md.String()))
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing runtime gate manifest data: %w", err)
	}
	return errors.New("runtime gate campaign manifest contains trailing data")
}

func atomicWrite(path string, body []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("artifact path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".runtime-gate-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
