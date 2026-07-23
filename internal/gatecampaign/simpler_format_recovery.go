package gatecampaign

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/bootstrap"
	"motor-autonomo/internal/runtime/source"
)

const SimplerFormatRecoveryCampaignSchemaVersion = 1

// SimplerFormatRecoveryCampaignManifest drives one persisted ModelExecutor flow.
// The first Complete invocation is a deterministic malformed non-effect;
// the second is the only external call and remains in the same Execute.
type SimplerFormatRecoveryCampaignManifest struct {
	SchemaVersion    int                  `json:"schema_version"`
	Name             string               `json:"name"`
	TimeoutSeconds   int                  `json:"timeout_seconds"`
	MaxCalls         int                  `json:"max_calls"`
	InjectedFailures int                  `json:"injected_failures"`
	MaxOutputTokens  int                  `json:"max_output_tokens"`
	ProbePrompt      string               `json:"probe_prompt"`
	Bindings         []RuntimeGateBinding `json:"bindings"`
}

func (m SimplerFormatRecoveryCampaignManifest) Validate() error {
	if m.SchemaVersion != SimplerFormatRecoveryCampaignSchemaVersion || strings.TrimSpace(m.Name) == "" {
		return errors.New("simpler format recovery campaign identity and supported schema version are required")
	}
	if m.TimeoutSeconds <= 0 || m.TimeoutSeconds > 300 {
		return errors.New("simpler format recovery campaign timeout must be between 1 and 300 seconds")
	}
	if m.MaxCalls != 3 || m.InjectedFailures != 2 {
		return errors.New("simpler format recovery campaign requires three model calls with exactly two injected failures")
	}
	if m.MaxOutputTokens < 192 || m.MaxOutputTokens > 512 {
		return errors.New("simpler format recovery campaign max_output_tokens must be between 192 and 512")
	}
	if prompt := strings.TrimSpace(m.ProbePrompt); prompt == "" || len(prompt) > 2048 {
		return errors.New("simpler format recovery campaign probe_prompt is required and bounded to 2048 bytes")
	}
	if len(m.Bindings) != 2 {
		return errors.New("simpler format recovery campaign requires exactly two bindings")
	}
	seenBindings, seenProviders := map[string]bool{}, map[string]bool{}
	for i, binding := range m.Bindings {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("simpler format recovery binding %d: %w", i, err)
		}
		if seenBindings[binding.BindingID] || seenProviders[binding.Provider] {
			return errors.New("simpler format recovery campaign requires distinct provider and binding IDs")
		}
		seenBindings[binding.BindingID], seenProviders[binding.Provider] = true, true
	}
	if m.Bindings[0].Priority == m.Bindings[1].Priority {
		return errors.New("simpler format recovery bindings require distinct priorities")
	}
	return nil
}

type SimplerFormatRecoveryCall struct {
	Call          int                         `json:"call"`
	BindingID     string                      `json:"binding_id"`
	Injected      bool                        `json:"injected"`
	Succeeded     bool                        `json:"succeeded"`
	Latency       time.Duration               `json:"latency,omitempty"`
	InputTokens   int                         `json:"input_tokens,omitempty"`
	OutputTokens  int                         `json:"output_tokens,omitempty"`
	FinishReason  port.CompletionFinishReason `json:"finish_reason,omitempty"`
	ResponseBytes int                         `json:"response_bytes,omitempty"`
	ResponseHash  string                      `json:"response_sha256,omitempty"`
	ErrorClass    string                      `json:"error_class,omitempty"`
}

type SimplerFormatRecoveryCampaignReport struct {
	SchemaVersion   int                         `json:"schema_version"`
	Name            string                      `json:"name"`
	StartedAt       time.Time                   `json:"started_at"`
	CompletedAt     time.Time                   `json:"completed_at"`
	ModelCalls      int                         `json:"model_calls"`
	ExternalCalls   int                         `json:"external_calls"`
	InjectedCalls   int                         `json:"injected_calls"`
	RecoveryStages  []domain.ModelRecoveryStage `json:"recovery_stages"`
	CommitID        domain.CommitID             `json:"commit_id"`
	OperationState  domain.OperationalState     `json:"operation_state"`
	ReceiptCount    int                         `json:"receipt_count"`
	CanonicalStored bool                        `json:"canonical_entity_stored"`
	DurableReopen   bool                        `json:"durable_reopen"`
	Calls           []SimplerFormatRecoveryCall `json:"calls"`
}

type simplerRecoveryRecorder struct {
	mu       sync.Mutex
	inject   int
	calls    int
	external int
	records  []SimplerFormatRecoveryCall
}

type simplerTwiceThenLiveProvider struct {
	bindingID string
	provider  port.ModelProvider
	recorder  *simplerRecoveryRecorder
}

func (p simplerTwiceThenLiveProvider) Complete(ctx context.Context, request port.CompletionRequest) (port.CompletionResult, error) {
	p.recorder.mu.Lock()
	p.recorder.calls++
	call := p.recorder.calls
	if call == 1 {
		result := port.CompletionResult{Text: `{"changes":[{"kind":"ADD","entity_type":"observation","entity_id":"observation_simpler_recovery","payload_ref":"artifact_simpler_recovery"}],"expected_delta":"one observation"`, Model: "injected-malformed", FinishReason: port.CompletionFinishStop}
		digest := sha256.Sum256([]byte(result.Text))
		p.recorder.records = append(p.recorder.records, SimplerFormatRecoveryCall{Call: call, BindingID: p.bindingID, Injected: true, Succeeded: true, FinishReason: result.FinishReason, ResponseBytes: len(result.Text), ResponseHash: fmt.Sprintf("%x", digest[:])})
		p.recorder.mu.Unlock()
		// Sleep so the lease doesn't get instantly reaped as concurrent/conflict, simulating real clock advancement
		time.Sleep(10 * time.Millisecond)
		return result, nil
	}
	if call == 2 {
		result := port.CompletionResult{Text: `{"schema_version": 1, "id": "changeset_simpler_recovery", "mission_revision_id": "revision_simpler_recovery", "operation_id": "operation_simpler_recovery", "base_commit_id": "commit_genesis", "read_set": ["manifest"], "preconditions": [], "changes": [{"kind": "ADD", "entity_type": "observation", "entity_id": "observation_simpler_recovery", "payload_ref": "artifact_simpler_recovery"}], "expected_delta": "one observation", "validator_ids": ["schema"], "provenance": "operator", "idempotency_key": "simpler-format-recovery-campaign", "unexpected": true}`, Model: "injected-unknown-field", FinishReason: port.CompletionFinishStop}
		digest := sha256.Sum256([]byte(result.Text))
		p.recorder.records = append(p.recorder.records, SimplerFormatRecoveryCall{Call: call, BindingID: p.bindingID, Injected: true, Succeeded: true, FinishReason: result.FinishReason, ResponseBytes: len(result.Text), ResponseHash: fmt.Sprintf("%x", digest[:])})
		p.recorder.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		return result, nil
	}
	p.recorder.external++
	p.recorder.mu.Unlock()

	started := time.Now()
	result, err := p.provider.Complete(ctx, request)
	record := SimplerFormatRecoveryCall{Call: call, BindingID: p.bindingID, Latency: time.Since(started), Succeeded: err == nil}
	if err == nil {
		record.InputTokens, record.OutputTokens, record.FinishReason = result.InputTokens, result.OutputTokens, result.FinishReason
		record.ResponseBytes = len(result.Text)
		digest := sha256.Sum256([]byte(result.Text))
		record.ResponseHash = fmt.Sprintf("%x", digest[:])
	} else {
		record.ErrorClass = "transport"
		var providerErr port.ProviderError
		if errors.As(err, &providerErr) {
			record.ErrorClass = "provider"
		}
		var httpErr port.ProviderHTTPError
		if errors.As(err, &httpErr) {
			record.ErrorClass = fmt.Sprintf("http_%d", httpErr.HTTPStatusCode())
		}
	}
	p.recorder.mu.Lock()
	p.recorder.records = append(p.recorder.records, record)
	p.recorder.mu.Unlock()
	return result, err
}

type SimplerFormatRecoveryCampaignRunner struct {
	Store     port.Store
	Clock     source.Clock
	Providers map[string]port.ModelProvider
}

func (r SimplerFormatRecoveryCampaignRunner) Run(ctx context.Context, manifest SimplerFormatRecoveryCampaignManifest) (SimplerFormatRecoveryCampaignReport, error) {
	if err := manifest.Validate(); err != nil {
		return SimplerFormatRecoveryCampaignReport{}, err
	}
	if r.Store == nil || r.Clock == nil || len(r.Providers) != 2 {
		return SimplerFormatRecoveryCampaignReport{}, errors.New("simpler format recovery campaign requires store, clock, and two providers")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	started := r.Clock.Now().UTC()
	if _, err := simplerFormatRecoverySeed(r.Store, manifest, started); err != nil {
		return SimplerFormatRecoveryCampaignReport{}, err
	}
	executor, err := bootstrap.BuildModelExecutor(bootstrap.Options{Model: &bootstrap.ModelOptions{Enabled: true, PolicyVersion: "policy@simpler-format-recovery", LeaseTTL: time.Duration(manifest.TimeoutSeconds) * time.Second}}, r.Store, r.Clock, source.NewSequenceIDGenerator(1), nil)
	if err != nil {
		return SimplerFormatRecoveryCampaignReport{}, fmt.Errorf("build simpler format recovery executor: %w", err)
	}
	// This harness isolates recovery semantics. ResourceGate routing remains covered
	// by runtime-gate-campaign; disabling it here prevents injected non-network
	// calls from being misrepresented as provider quota consumption.
	executor.Authorizer = nil
	recorder := &simplerRecoveryRecorder{inject: manifest.InjectedFailures}
	executor.Providers = make(map[string]port.ModelProvider, 2)
	for bindingID, provider := range r.Providers {
		executor.Providers[bindingID] = simplerTwiceThenLiveProvider{bindingID: bindingID, provider: provider, recorder: recorder}
	}
	execution, err := executor.Execute(ctx, "operation_simpler_recovery")
	if err != nil {
		return SimplerFormatRecoveryCampaignReport{}, fmt.Errorf("execute simpler format recovery campaign: %w", err)
	}
	if !execution.Completed || execution.CommitID == "" {
		return SimplerFormatRecoveryCampaignReport{}, fmt.Errorf("simpler format recovery campaign did not commit: %+v", execution)
	}
	recorder.mu.Lock()
	records := append([]SimplerFormatRecoveryCall(nil), recorder.records...)
	external, total := recorder.external, recorder.calls
	recorder.mu.Unlock()
	if total != manifest.MaxCalls || external != 1 {
		return SimplerFormatRecoveryCampaignReport{}, fmt.Errorf("simpler format recovery calls total=%d external=%d, want total=%d external=1", total, external, manifest.MaxCalls)
	}
	report := SimplerFormatRecoveryCampaignReport{SchemaVersion: SimplerFormatRecoveryCampaignSchemaVersion, Name: manifest.Name, StartedAt: started, CompletedAt: r.Clock.Now().UTC(), ModelCalls: execution.ModelCalls, ExternalCalls: external, InjectedCalls: manifest.InjectedFailures, RecoveryStages: execution.RecoveryStages, CommitID: execution.CommitID, Calls: records}
	if err := simplerFormatRecoverySnapshot(ctx, r.Store, &report); err != nil {
		return SimplerFormatRecoveryCampaignReport{}, err
	}
	return report, nil
}

func simplerFormatRecoverySeed(store port.Store, manifest SimplerFormatRecoveryCampaignManifest, now time.Time) (domain.ModelsConfig, error) {
	providers := make([]domain.ModelProviderConfig, 0, 2)
	bindings := make([]domain.ModelBindingConfig, 0, 2)
	for _, item := range manifest.Bindings {
		providerLimit := domain.ResourceLimit{Resource: domain.ModelProviderResource(item.Provider), MaxConcurrent: 1, MaxPerMinute: manifest.MaxCalls, MaxPerDay: manifest.MaxCalls, MaxTokensPerMinute: 16000, FailureThreshold: 2, CooldownBase: time.Second, CooldownMax: time.Minute}
		bindingLimit := providerLimit
		bindingLimit.Resource = domain.ModelBindingResource(item.BindingID)
		providers = append(providers, domain.ModelProviderConfig{ID: item.Provider, Kind: item.ProviderKind, BaseURL: item.BaseURL, APIKeyEnv: item.APIKeyEnvironment, Timeout: time.Duration(manifest.TimeoutSeconds) * time.Second, MaxResponseBytes: 1 << 20, GlobalLimit: providerLimit})
		dialect := domain.MaxOutputDialectLegacy
		if item.MaxOutputField == "max_completion_tokens" {
			dialect = domain.MaxOutputDialectCompletion
		}
		bindings = append(bindings, domain.ModelBindingConfig{ID: item.BindingID, ProviderRef: item.Provider, ModelID: item.Model, Enabled: true, Priority: item.Priority, ContextTokens: item.ContextTokens, MaxOutputTokens: manifest.MaxOutputTokens, MaxOutputDialect: dialect, Limit: bindingLimit})
	}
	config := domain.ModelsConfig{Version: "models@simpler-format-recovery", Providers: providers, Bindings: bindings}
	if err := config.Validate(); err != nil {
		return config, err
	}
	spec := domain.OperationSpec{SchemaVersion: 1, ID: "simpler-format-recovery@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "probe_text", OutputSchema: "proposed_changeset", Budget: domain.Budget{ModelCalls: manifest.MaxCalls, Tokens: 16000, Attempts: 1}, MaxOutputTokens: manifest.MaxOutputTokens, SafetyMargin: 1, Validators: []string{"schema"}, RetryPolicy: "bounded_recovery", FallbackPolicy: "catalog", MaximumAuthority: domain.AuthorityProposeOnly}
	revision := domain.MissionRevision{SchemaVersion: 1, ID: "revision_simpler_recovery", MissionID: "mission_simpler_recovery", Revision: 1, OriginalText: "bounded simpler format recovery probe", Purpose: "validate persisted recovery ladder", Domains: []string{"operations"}, Policies: []string{"no authority"}, Status: domain.MissionActive, Provenance: "operator-manifest", AcceptedAt: now, Budget: domain.Budget{ModelCalls: manifest.MaxCalls, Tokens: 16000, Attempts: 1}}
	question := domain.Question{SchemaVersion: 1, ID: "question_simpler_recovery", MissionRevision: revision.ID, Text: "does malformed output recover through the bounded ladder?", Origin: "campaign", Relevance: "diagnostic", AnswerCondition: "durable commit"}
	candidate := domain.InquiryCandidate{SchemaVersion: 1, ID: "candidate_simpler_recovery", MissionRevision: revision.ID, QuestionID: question.ID, DerivedFrom: []string{"manifest"}, ExpectedProgress: "runtime evidence", Novelty: "dated probe", Risk: domain.RiskLow, SourcePlan: []string{"provider"}, AnswerCondition: "report", StopCondition: "one flow", ReviewAfter: now.Add(time.Hour)}
	inquiry := domain.Inquiry{SchemaVersion: 1, ID: "inquiry_simpler_recovery", CandidateID: candidate.ID, MissionRevision: revision.ID, QuestionID: question.ID, AdmissionReason: "bounded campaign", StopCondition: "one flow", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	operation := domain.Operation{SchemaVersion: 1, ID: "operation_simpler_recovery", InquiryID: inquiry.ID, MissionRevision: revision.ID, SpecID: spec.ID, ReadSet: []string{"manifest"}, InputRefs: []string{"probe_prompt"}, ExpectedOutput: manifest.ProbePrompt, IdempotencyKey: "simpler-format-recovery-campaign", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	return config, store.Update(context.Background(), func(tx port.Transaction) error {
		hash, err := domain.ConfigPayloadHash(domain.ConfigScopeModels, nil, nil, nil, nil, nil, &config)
		if err != nil {
			return err
		}
		draft := domain.ConfigDraft{SchemaVersion: 1, ID: "draft_simpler_recovery_models", Scope: domain.ConfigScopeModels, Applicability: domain.ConfigHot, Status: domain.ConfigDraftOpen, ActorType: domain.ActorOperator, ActorID: "simpler-format-recovery-campaign", Reason: "bounded live simpler format recovery campaign", Models: &config, CreatedAt: now}
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
		rev := domain.ConfigRevision{SchemaVersion: 1, ID: "config_simpler_recovery_models", Scope: domain.ConfigScopeModels, Revision: 1, Applicability: domain.ConfigHot, ContentHash: hash, ActorType: domain.ActorOperator, ActorID: "simpler-format-recovery-campaign", Reason: draft.Reason, DraftID: draft.ID, Models: &config, AcceptedAt: now}
		if err := tx.AppendConfigRevision(rev); err != nil {
			return err
		}
		if err := tx.ActivateConfigRevision(domain.ConfigScopeModels, rev.ID); err != nil {
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
		return tx.CreateOperation(operation)
	})
}

func simplerFormatRecoverySnapshot(ctx context.Context, store port.ReadStore, report *SimplerFormatRecoveryCampaignReport) error {
	return store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation("operation_simpler_recovery")
		if err != nil {
			return err
		}
		report.OperationState = op.State
		for call := uint32(1); call <= uint32(report.ModelCalls); call++ {
			if _, err := r.ModelCompletionReceipt(op.ID, op.Attempt, call); err != nil {
				return fmt.Errorf("read completion receipt %d: %w", call, err)
			}
			report.ReceiptCount++
		}
		entity, err := r.CanonicalEntity("observation", "observation_simpler_recovery")
		if err != nil {
			return err
		}
		report.CanonicalStored = entity.CommitID == report.CommitID && entity.PayloadRef == "artifact_simpler_recovery"
		if op.State != domain.StateSucceeded || !report.CanonicalStored || report.ReceiptCount != report.ModelCalls {
			return errors.New("simpler format recovery durable state is incomplete")
		}
		return nil
	})
}

func VerifySimplerFormatRecoveryDurability(ctx context.Context, store port.ReadStore, expected SimplerFormatRecoveryCampaignReport) error {
	actual := expected
	actual.ReceiptCount = 0
	actual.CanonicalStored = false
	if err := simplerFormatRecoverySnapshot(ctx, store, &actual); err != nil {
		return err
	}
	if actual.OperationState != expected.OperationState || actual.ReceiptCount != expected.ReceiptCount || actual.CanonicalStored != expected.CanonicalStored {
		return errors.New("reopened simpler format recovery state differs from report")
	}
	return nil
}

func WriteSimplerFormatRecoveryCampaignManifest(path string, manifest SimplerFormatRecoveryCampaignManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(body, '\n'))
}

func WriteSimplerFormatRecoveryCampaignArtifacts(directory string, report SimplerFormatRecoveryCampaignReport) error {
	if strings.TrimSpace(directory) == "" || report.SchemaVersion != SimplerFormatRecoveryCampaignSchemaVersion || report.ExternalCalls != 1 || report.ReceiptCount != report.ModelCalls {
		return errors.New("artifact directory and complete simpler format recovery report are required")
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(directory+"/simpler-format-recovery.json", append(body, '\n')); err != nil {
		return err
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# Malformed recovery campaign\n\n- Name: `%s`\n- Model calls: %d\n- Injected malformed calls: %d\n- External live calls: %d\n- Recovery stages: `%s`\n- Commit: `%s`\n- Operation state: `%s`\n- Completion receipts: %d\n- Canonical entity stored: `%t`\n- Durable reopen verified: `%t`\n\n", report.Name, report.ModelCalls, report.InjectedCalls, report.ExternalCalls, strings.Join(simplerRecoveryStageStrings(report.RecoveryStages), " -> "), report.CommitID, report.OperationState, report.ReceiptCount, report.CanonicalStored, report.DurableReopen)
	md.WriteString("| Call | Binding | Source | Success | Latency | Input tokens | Output tokens | Finish | Bytes | SHA-256 | Error |\n| ---: | --- | --- | --- | ---: | ---: | ---: | --- | ---: | --- | --- |\n")
	for _, call := range report.Calls {
		source := "live"
		if call.Injected {
			source = "deterministic malformed"
		}
		fmt.Fprintf(&md, "| %d | %s | %s | %t | %s | %d | %d | %s | %d | %s | %s |\n", call.Call, call.BindingID, source, call.Succeeded, call.Latency, call.InputTokens, call.OutputTokens, call.FinishReason, call.ResponseBytes, call.ResponseHash, call.ErrorClass)
	}
	md.WriteString("\nProvider text is not stored in this report. Injected outputs are known non-effects; the live output remains subject to typed parsing, validators, commit authority, and receipt hashing.\n")
	return atomicWrite(directory+"/simpler-format-recovery.md", []byte(md.String()))
}

func simplerRecoveryStageStrings(stages []domain.ModelRecoveryStage) []string {
	out := make([]string, len(stages))
	for i := range stages {
		out[i] = string(stages[i])
	}
	return out
}
