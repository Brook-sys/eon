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

const DeferExhaustionRecoveryCampaignSchemaVersion = 1

// DeferExhaustionRecoveryCampaignManifest drives one persisted ModelExecutor flow.
// The first three Complete invocations are deterministic validation non-effects;
// the fourth is the only external call and runs on the alternate binding.
type DeferExhaustionRecoveryCampaignManifest struct {
	SchemaVersion    int                  `json:"schema_version"`
	Name             string               `json:"name"`
	TimeoutSeconds   int                  `json:"timeout_seconds"`
	MaxCalls         int                  `json:"max_calls"`
	InjectedFailures int                  `json:"injected_failures"`
	MaxOutputTokens  int                  `json:"max_output_tokens"`
	ProbePrompt      string               `json:"probe_prompt"`
	Bindings         []RuntimeGateBinding `json:"bindings"`
}

func (m DeferExhaustionRecoveryCampaignManifest) Validate() error {
	if m.SchemaVersion != DeferExhaustionRecoveryCampaignSchemaVersion || strings.TrimSpace(m.Name) == "" {
		return errors.New("defer exhaustion recovery campaign identity and supported schema version are required")
	}
	if m.TimeoutSeconds <= 0 || m.TimeoutSeconds > 300 {
		return errors.New("defer exhaustion recovery campaign timeout must be between 1 and 300 seconds")
	}
	if m.MaxCalls != 4 || m.InjectedFailures != 3 {
		return errors.New("defer exhaustion recovery campaign requires four model calls with exactly three injected failures")
	}
	if m.MaxOutputTokens < 192 || m.MaxOutputTokens > 512 {
		return errors.New("defer exhaustion recovery campaign max_output_tokens must be between 192 and 512")
	}
	if prompt := strings.TrimSpace(m.ProbePrompt); prompt == "" || len(prompt) > 2048 {
		return errors.New("defer exhaustion recovery campaign probe_prompt is required and bounded to 2048 bytes")
	}
	if len(m.Bindings) != 2 {
		return errors.New("defer exhaustion recovery campaign requires exactly two bindings")
	}
	seenBindings, seenProviders := map[string]bool{}, map[string]bool{}
	for i, binding := range m.Bindings {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("defer exhaustion recovery binding %d: %w", i, err)
		}
		if seenBindings[binding.BindingID] || seenProviders[binding.Provider] {
			return errors.New("defer exhaustion recovery campaign requires distinct provider and binding IDs")
		}
		seenBindings[binding.BindingID], seenProviders[binding.Provider] = true, true
	}
	if m.Bindings[0].Priority == m.Bindings[1].Priority {
		return errors.New("defer exhaustion recovery bindings require distinct priorities")
	}
	return nil
}

type DeferExhaustionRecoveryCall struct {
	Call             int                         `json:"call"`
	BindingID        string                      `json:"binding_id"`
	Injected         bool                        `json:"injected"`
	Succeeded        bool                        `json:"succeeded"`
	Latency          time.Duration               `json:"latency,omitempty"`
	InputTokens      int                         `json:"input_tokens,omitempty"`
	OutputTokens     int                         `json:"output_tokens,omitempty"`
	FinishReason     port.CompletionFinishReason `json:"finish_reason,omitempty"`
	ResponseBytes    int                         `json:"response_bytes,omitempty"`
	ResponseHash     string                      `json:"response_sha256,omitempty"`
	PresentedInvalid bool                        `json:"presented_invalid"`
	PresentedBytes   int                         `json:"presented_bytes,omitempty"`
	PresentedHash    string                      `json:"presented_sha256,omitempty"`
	ErrorClass       string                      `json:"error_class,omitempty"`
}

type DeferExhaustionRecoveryCampaignReport struct {
	SchemaVersion    int                             `json:"schema_version"`
	Name             string                          `json:"name"`
	StartedAt        time.Time                       `json:"started_at"`
	CompletedAt      time.Time                       `json:"completed_at"`
	ModelCalls       int                             `json:"model_calls"`
	ExternalCalls    int                             `json:"external_calls"`
	InjectedCalls    int                             `json:"injected_calls"`
	RecoveryStages   []domain.ModelRecoveryStage     `json:"recovery_stages"`
	OperationState   domain.OperationalState         `json:"operation_state"`
	ReceiptCount     int                             `json:"receipt_count"`
	CanonicalAbsent  bool                            `json:"canonical_entity_absent"`
	FinalDisposition domain.ModelRecoveryDisposition `json:"final_disposition"`
	ExhaustionReason string                          `json:"exhaustion_reason"`
	ExhaustionEvents int                             `json:"exhaustion_events"`
	DurableReopen    bool                            `json:"durable_reopen"`
	PrimaryBinding   string                          `json:"primary_binding"`
	FallbackBinding  string                          `json:"fallback_binding"`
	BindingSwitched  bool                            `json:"binding_switched"`
	Calls            []DeferExhaustionRecoveryCall   `json:"calls"`
}

type deferExhaustionRecorder struct {
	mu       sync.Mutex
	inject   int
	calls    int
	external int
	records  []DeferExhaustionRecoveryCall
}

type deferThriceThenLiveProvider struct {
	bindingID string
	provider  port.ModelProvider
	recorder  *deferExhaustionRecorder
}

func (p deferThriceThenLiveProvider) Complete(ctx context.Context, request port.CompletionRequest) (port.CompletionResult, error) {
	p.recorder.mu.Lock()
	p.recorder.calls++
	call := p.recorder.calls
	if call == 1 {
		result := port.CompletionResult{Text: `{"changes":[{"kind":"ADD","entity_type":"observation","entity_id":"observation_defer_exhaustion","payload_ref":"artifact_defer_exhaustion"}],"expected_delta":"one observation"`, Model: "injected-malformed", FinishReason: port.CompletionFinishStop}
		digest := sha256.Sum256([]byte(result.Text))
		p.recorder.records = append(p.recorder.records, DeferExhaustionRecoveryCall{Call: call, BindingID: p.bindingID, Injected: true, Succeeded: true, FinishReason: result.FinishReason, ResponseBytes: len(result.Text), ResponseHash: fmt.Sprintf("%x", digest[:])})
		p.recorder.mu.Unlock()
		// Sleep so the lease doesn't get instantly reaped as concurrent/conflict, simulating real clock advancement
		time.Sleep(10 * time.Millisecond)
		return result, nil
	}
	if call == 2 {
		result := port.CompletionResult{Text: `{"schema_version": 1, "id": "changeset_defer_exhaustion", "mission_revision_id": "revision_defer_exhaustion", "operation_id": "operation_defer_exhaustion", "base_commit_id": "commit_genesis", "read_set": ["manifest"], "preconditions": [], "changes": [{"kind": "ADD", "entity_type": "observation", "entity_id": "observation_defer_exhaustion", "payload_ref": "artifact_defer_exhaustion"}], "expected_delta": "one observation", "validator_ids": ["schema"], "provenance": "operator", "idempotency_key": "defer-exhaustion-recovery-campaign", "unexpected": true}`, Model: "injected-unknown-field", FinishReason: port.CompletionFinishStop}
		digest := sha256.Sum256([]byte(result.Text))
		p.recorder.records = append(p.recorder.records, DeferExhaustionRecoveryCall{Call: call, BindingID: p.bindingID, Injected: true, Succeeded: true, FinishReason: result.FinishReason, ResponseBytes: len(result.Text), ResponseHash: fmt.Sprintf("%x", digest[:])})
		p.recorder.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		return result, nil
	}
	if call == 3 {
		result := port.CompletionResult{Text: `CHANGESET/1
id: changeset_defer_exhaustion
operation_id: operation_defer_exhaustion
change: ADD|observation|observation_defer_exhaustion|artifact_defer_exhaustion`, Model: "injected-incomplete-simpler-format", FinishReason: port.CompletionFinishStop}
		digest := sha256.Sum256([]byte(result.Text))
		p.recorder.records = append(p.recorder.records, DeferExhaustionRecoveryCall{Call: call, BindingID: p.bindingID, Injected: true, Succeeded: true, FinishReason: result.FinishReason, ResponseBytes: len(result.Text), ResponseHash: fmt.Sprintf("%x", digest[:])})
		p.recorder.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		return result, nil
	}
	p.recorder.external++
	p.recorder.mu.Unlock()

	started := time.Now()
	result, err := p.provider.Complete(ctx, request)
	record := DeferExhaustionRecoveryCall{Call: call, BindingID: p.bindingID, Latency: time.Since(started), Succeeded: err == nil}
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
	if err != nil {
		return result, err
	}
	// The live contact supplies transport, latency, usage, and adherence evidence,
	// but terminal exhaustion must not depend on a provider choosing to fail.
	// Present a separate, known-invalid non-effect to the executor and report both
	// hashes so the durable receipt is never confused with the raw live response.
	presented := `CHANGESET/1
id: changeset_defer_exhaustion
operation_id: operation_defer_exhaustion
change: ADD|observation|observation_defer_exhaustion|artifact_defer_exhaustion`
	presentedDigest := sha256.Sum256([]byte(presented))
	p.recorder.mu.Lock()
	p.recorder.records[len(p.recorder.records)-1].PresentedInvalid = true
	p.recorder.records[len(p.recorder.records)-1].PresentedBytes = len(presented)
	p.recorder.records[len(p.recorder.records)-1].PresentedHash = fmt.Sprintf("%x", presentedDigest[:])
	p.recorder.mu.Unlock()
	result.Text = presented
	return result, nil
}

type DeferExhaustionRecoveryCampaignRunner struct {
	Store     port.Store
	Clock     source.Clock
	Providers map[string]port.ModelProvider
}

func (r DeferExhaustionRecoveryCampaignRunner) Run(ctx context.Context, manifest DeferExhaustionRecoveryCampaignManifest) (DeferExhaustionRecoveryCampaignReport, error) {
	if err := manifest.Validate(); err != nil {
		return DeferExhaustionRecoveryCampaignReport{}, err
	}
	if r.Store == nil || r.Clock == nil || len(r.Providers) != 2 {
		return DeferExhaustionRecoveryCampaignReport{}, errors.New("defer exhaustion recovery campaign requires store, clock, and two providers")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	started := r.Clock.Now().UTC()
	if _, err := deferExhaustionRecoverySeed(r.Store, manifest, started); err != nil {
		return DeferExhaustionRecoveryCampaignReport{}, err
	}
	executor, err := bootstrap.BuildModelExecutor(bootstrap.Options{Model: &bootstrap.ModelOptions{Enabled: true, PolicyVersion: "policy@defer-exhaustion-recovery", LeaseTTL: time.Duration(manifest.TimeoutSeconds) * time.Second}}, r.Store, r.Clock, source.NewSequenceIDGenerator(1), nil)
	if err != nil {
		return DeferExhaustionRecoveryCampaignReport{}, fmt.Errorf("build defer exhaustion recovery executor: %w", err)
	}
	// This harness isolates recovery semantics. ResourceGate routing remains covered
	// by runtime-gate-campaign; disabling it here prevents injected non-network
	// calls from being misrepresented as provider quota consumption.
	executor.Authorizer = nil
	recorder := &deferExhaustionRecorder{inject: manifest.InjectedFailures}
	executor.Providers = make(map[string]port.ModelProvider, 2)
	for bindingID, provider := range r.Providers {
		executor.Providers[bindingID] = deferThriceThenLiveProvider{bindingID: bindingID, provider: provider, recorder: recorder}
	}
	execution, executeErr := executor.Execute(ctx, "operation_defer_exhaustion")
	if executeErr == nil || !strings.Contains(executeErr.Error(), "model recovery exhausted") {
		return DeferExhaustionRecoveryCampaignReport{}, fmt.Errorf("defer exhaustion recovery campaign expected terminal exhaustion, result=%+v err=%v", execution, executeErr)
	}
	if execution.Completed || !execution.Exhausted || execution.CommitID != "" {
		return DeferExhaustionRecoveryCampaignReport{}, fmt.Errorf("defer exhaustion recovery campaign terminal result is unsafe: %+v", execution)
	}
	recorder.mu.Lock()
	records := append([]DeferExhaustionRecoveryCall(nil), recorder.records...)
	external, total := recorder.external, recorder.calls
	recorder.mu.Unlock()
	if total != manifest.MaxCalls || external != 1 {
		return DeferExhaustionRecoveryCampaignReport{}, fmt.Errorf("defer exhaustion recovery calls total=%d external=%d, want total=%d external=1", total, external, manifest.MaxCalls)
	}
	wantStages := []domain.ModelRecoveryStage{domain.RecoveryShortCorrection, domain.RecoverySimplerFormat, domain.RecoveryFallbackModel, domain.RecoveryDefer}
	if len(execution.RecoveryStages) != len(wantStages) {
		return DeferExhaustionRecoveryCampaignReport{}, fmt.Errorf("defer exhaustion recovery stages=%v, want %v", execution.RecoveryStages, wantStages)
	}
	for i := range wantStages {
		if execution.RecoveryStages[i] != wantStages[i] {
			return DeferExhaustionRecoveryCampaignReport{}, fmt.Errorf("defer exhaustion recovery stages=%v, want %v", execution.RecoveryStages, wantStages)
		}
	}
	primary, fallback := manifest.Bindings[0], manifest.Bindings[1]
	if fallback.Priority < primary.Priority {
		primary, fallback = fallback, primary
	}
	switched := len(records) == 4
	for i := 0; switched && i < 3; i++ {
		switched = records[i].Injected && records[i].BindingID == primary.BindingID
	}
	switched = switched && !records[3].Injected && records[3].PresentedInvalid && records[3].BindingID == fallback.BindingID
	if !switched {
		return DeferExhaustionRecoveryCampaignReport{}, fmt.Errorf("unsafe or missing binding switch: primary=%s fallback=%s calls=%+v", primary.BindingID, fallback.BindingID, records)
	}
	report := DeferExhaustionRecoveryCampaignReport{SchemaVersion: DeferExhaustionRecoveryCampaignSchemaVersion, Name: manifest.Name, StartedAt: started, CompletedAt: r.Clock.Now().UTC(), ModelCalls: execution.ModelCalls, ExternalCalls: external, InjectedCalls: manifest.InjectedFailures, RecoveryStages: execution.RecoveryStages, PrimaryBinding: primary.BindingID, FallbackBinding: fallback.BindingID, BindingSwitched: true, Calls: records}
	if err := deferExhaustionRecoverySnapshot(ctx, r.Store, &report); err != nil {
		return DeferExhaustionRecoveryCampaignReport{}, err
	}
	return report, nil
}

func deferExhaustionRecoverySeed(store port.Store, manifest DeferExhaustionRecoveryCampaignManifest, now time.Time) (domain.ModelsConfig, error) {
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
	config := domain.ModelsConfig{Version: "models@defer-exhaustion-recovery", Providers: providers, Bindings: bindings}
	if err := config.Validate(); err != nil {
		return config, err
	}
	spec := domain.OperationSpec{SchemaVersion: 1, ID: "defer-exhaustion-recovery@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "probe_text", OutputSchema: "proposed_changeset", Budget: domain.Budget{ModelCalls: manifest.MaxCalls, Tokens: 16000, Attempts: 1}, MaxOutputTokens: manifest.MaxOutputTokens, SafetyMargin: 1, Validators: []string{"schema"}, RetryPolicy: "bounded_recovery", FallbackPolicy: "catalog", MaximumAuthority: domain.AuthorityProposeOnly}
	revision := domain.MissionRevision{SchemaVersion: 1, ID: "revision_defer_exhaustion", MissionID: "mission_defer_exhaustion", Revision: 1, OriginalText: "bounded defer exhaustion recovery probe", Purpose: "validate persisted recovery ladder", Domains: []string{"operations"}, Policies: []string{"no authority"}, Status: domain.MissionActive, Provenance: "operator-manifest", AcceptedAt: now, Budget: domain.Budget{ModelCalls: manifest.MaxCalls, Tokens: 16000, Attempts: 1}}
	question := domain.Question{SchemaVersion: 1, ID: "question_defer_exhaustion", MissionRevision: revision.ID, Text: "does invalid fallback output terminate without another provider call?", Origin: "campaign", Relevance: "diagnostic", AnswerCondition: "durable exhaustion"}
	candidate := domain.InquiryCandidate{SchemaVersion: 1, ID: "candidate_defer_exhaustion", MissionRevision: revision.ID, QuestionID: question.ID, DerivedFrom: []string{"manifest"}, ExpectedProgress: "runtime evidence", Novelty: "dated probe", Risk: domain.RiskLow, SourcePlan: []string{"provider"}, AnswerCondition: "report", StopCondition: "one flow", ReviewAfter: now.Add(time.Hour)}
	inquiry := domain.Inquiry{SchemaVersion: 1, ID: "inquiry_defer_exhaustion", CandidateID: candidate.ID, MissionRevision: revision.ID, QuestionID: question.ID, AdmissionReason: "bounded campaign", StopCondition: "one flow", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	operation := domain.Operation{SchemaVersion: 1, ID: "operation_defer_exhaustion", InquiryID: inquiry.ID, MissionRevision: revision.ID, SpecID: spec.ID, ReadSet: []string{"manifest"}, InputRefs: []string{"probe_prompt"}, ExpectedOutput: manifest.ProbePrompt, IdempotencyKey: "defer-exhaustion-recovery-campaign", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	return config, store.Update(context.Background(), func(tx port.Transaction) error {
		hash, err := domain.ConfigPayloadHash(domain.ConfigScopeModels, nil, nil, nil, nil, nil, &config)
		if err != nil {
			return err
		}
		draft := domain.ConfigDraft{SchemaVersion: 1, ID: "draft_defer_exhaustion_models", Scope: domain.ConfigScopeModels, Applicability: domain.ConfigHot, Status: domain.ConfigDraftOpen, ActorType: domain.ActorOperator, ActorID: "defer-exhaustion-recovery-campaign", Reason: "bounded live defer exhaustion recovery campaign", Models: &config, CreatedAt: now}
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
		rev := domain.ConfigRevision{SchemaVersion: 1, ID: "config_defer_exhaustion_models", Scope: domain.ConfigScopeModels, Revision: 1, Applicability: domain.ConfigHot, ContentHash: hash, ActorType: domain.ActorOperator, ActorID: "defer-exhaustion-recovery-campaign", Reason: draft.Reason, DraftID: draft.ID, Models: &config, AcceptedAt: now}
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

func deferExhaustionRecoverySnapshot(ctx context.Context, store port.ReadStore, report *DeferExhaustionRecoveryCampaignReport) error {
	return store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation("operation_defer_exhaustion")
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
		_, err = r.CanonicalEntity("observation", "observation_defer_exhaustion")
		report.CanonicalAbsent = errors.Is(err, port.ErrNotFound)
		if err != nil && !report.CanonicalAbsent {
			return err
		}
		events, err := r.Events(0, 1000)
		if err != nil {
			return err
		}
		for _, event := range events {
			if event.OperationID != op.ID {
				continue
			}
			if event.Kind == "operation.model_recovery_decision" && strings.Contains(event.PayloadRef, "stage=DEFER") {
				if strings.Contains(event.PayloadRef, "disposition=EXHAUST") && strings.Contains(event.PayloadRef, "reason=model_recovery_budget_exhausted") && strings.Contains(event.PayloadRef, "calls=4") {
					report.FinalDisposition = domain.DispositionExhaust
					report.ExhaustionReason = "model_recovery_budget_exhausted"
				}
			}
			if event.Kind == "operation.model_exhausted" {
				report.ExhaustionEvents++
			}
		}
		if op.State != domain.StateExhausted || !report.CanonicalAbsent || report.ReceiptCount != report.ModelCalls || report.FinalDisposition != domain.DispositionExhaust || report.ExhaustionEvents != 1 {
			return errors.New("defer exhaustion recovery durable state is incomplete")
		}
		return nil
	})
}

func VerifyDeferExhaustionRecoveryDurability(ctx context.Context, store port.ReadStore, expected DeferExhaustionRecoveryCampaignReport) error {
	actual := expected
	actual.ReceiptCount = 0
	actual.CanonicalAbsent = false
	actual.FinalDisposition = ""
	actual.ExhaustionReason = ""
	actual.ExhaustionEvents = 0
	if err := deferExhaustionRecoverySnapshot(ctx, store, &actual); err != nil {
		return err
	}
	if actual.OperationState != expected.OperationState || actual.ReceiptCount != expected.ReceiptCount || actual.CanonicalAbsent != expected.CanonicalAbsent || actual.FinalDisposition != expected.FinalDisposition || actual.ExhaustionReason != expected.ExhaustionReason || actual.ExhaustionEvents != expected.ExhaustionEvents {
		return errors.New("reopened defer exhaustion recovery state differs from report")
	}
	return nil
}

func WriteDeferExhaustionRecoveryCampaignManifest(path string, manifest DeferExhaustionRecoveryCampaignManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(body, '\n'))
}

func WriteDeferExhaustionRecoveryCampaignArtifacts(directory string, report DeferExhaustionRecoveryCampaignReport) error {
	if strings.TrimSpace(directory) == "" || report.SchemaVersion != DeferExhaustionRecoveryCampaignSchemaVersion || report.ExternalCalls != 1 || report.ReceiptCount != report.ModelCalls || !report.BindingSwitched || report.OperationState != domain.StateExhausted || !report.CanonicalAbsent || report.FinalDisposition != domain.DispositionExhaust {
		return errors.New("artifact directory and complete defer exhaustion recovery report are required")
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(directory+"/defer-exhaustion-recovery.json", append(body, '\n')); err != nil {
		return err
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# Defer-exhaustion recovery campaign\n\n- Name: `%s`\n- Model calls: %d\n- Preparatory injected non-effects: %d\n- External live calls: %d\n- Recovery stages: `%s`\n- Binding switch: `%s` -> `%s` (verified: `%t`)\n- Final decision: `%s` (`%s`)\n- Operation state: `%s`\n- Completion receipts: %d\n- Canonical entity absent: `%t`\n- Durable reopen verified: `%t`\n\n", report.Name, report.ModelCalls, report.InjectedCalls, report.ExternalCalls, strings.Join(deferExhaustionStageStrings(report.RecoveryStages), " -> "), report.PrimaryBinding, report.FallbackBinding, report.BindingSwitched, report.FinalDisposition, report.ExhaustionReason, report.OperationState, report.ReceiptCount, report.CanonicalAbsent, report.DurableReopen)
	md.WriteString("| Call | Binding | Source | Success | Latency | Input tokens | Output tokens | Finish | Live bytes | Live SHA-256 | Presented invalid | Receipt bytes | Receipt SHA-256 | Error |\n| ---: | --- | --- | --- | ---: | ---: | ---: | --- | ---: | --- | --- | ---: | --- | --- |\n")
	for _, call := range report.Calls {
		source := "live"
		if call.Injected {
			source = "deterministic malformed"
		}
		fmt.Fprintf(&md, "| %d | %s | %s | %t | %s | %d | %d | %s | %d | %s | %t | %d | %s | %s |\n", call.Call, call.BindingID, source, call.Succeeded, call.Latency, call.InputTokens, call.OutputTokens, call.FinishReason, call.ResponseBytes, call.ResponseHash, call.PresentedInvalid, call.PresentedBytes, call.PresentedHash, call.ErrorClass)
	}
	md.WriteString("\nProvider text is not stored. The fourth call records live metrics/hash separately from the deterministic invalid non-effect presented to the executor and persisted in its completion receipt.\n")
	return atomicWrite(directory+"/defer-exhaustion-recovery.md", []byte(md.String()))
}

func deferExhaustionStageStrings(stages []domain.ModelRecoveryStage) []string {
	out := make([]string, len(stages))
	for i := range stages {
		out[i] = string(stages[i])
	}
	return out
}
