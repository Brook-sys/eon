package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/changeset"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/modeltext"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/runtime/source"
)

// Event kinds for the PROPOSE_ONLY model path. Model text never mutates
// canonical state directly; only changeset.Processor commit does.
const (
	EventOperationModelInvoked  = "operation.model_invoked"
	EventOperationModelVerified = "operation.model_verified"
)

// ModelExecutor completes non-local PROPOSE_ONLY operations under a lease:
// dispatch → compile prompt → Complete → process ProposedChangeSet → SUCCEED.
// Local continuity work remains on LocalExecutor.
type ModelExecutor struct {
	Store    port.Store
	Clock    source.Clock
	IDs      source.IDGenerator
	Provider port.ModelProvider
	// FallbackProvider is the optional FR-MODEL-004 step-7 alternate model.
	// When nil, DecideNextRecovery never selects FALLBACK_MODEL.
	FallbackProvider port.ModelProvider
	Changes          *changeset.Processor
	Compiler         prompt.Compiler
	PolicyVersion    string
	LeaseTTL         time.Duration
	MaxOutputBytes   int64
	// Profile is the FR-MODEL-005 capability snapshot used for progressive
	// adaptation (FR-MODEL-006) and conservative context (FR-MODEL-007).
	// Zero-value falls back to a declared baseline using Compiler context.
	Profile domain.ProviderProfile
	// PreferExpandedContext allows spending more of the safe window on optional facts.
	// Default false keeps FR-MODEL-007 conservative.
	PreferExpandedContext bool
}

// ModelExecuteResult summarizes one model-backed Execute call.
type ModelExecuteResult struct {
	OperationID domain.OperationID
	Completed   bool
	Skipped     bool
	SkipReason  string
	// Exhausted is true when the operation reached terminal EXHAUSTED after
	// FR-MODEL-004 recovery budget ran out (no further Complete allowed).
	Exhausted bool
	CommitID  domain.CommitID
	LeaseRef  string
	// ModelCalls counts Complete invocations performed in this Execute.
	ModelCalls int
	// RecoveryStages lists ladder stages attempted (for audit assertions).
	RecoveryStages []domain.ModelRecoveryStage
	RawArtifact    domain.ArtifactID
}

func (e ModelExecutor) validateDeps() error {
	if e.Store == nil || e.Clock == nil || e.IDs == nil {
		return errors.New("model executor dependencies are incomplete")
	}
	if e.Provider == nil {
		return errors.New("model executor requires a ModelProvider")
	}
	if e.Changes == nil {
		return errors.New("model executor requires a changeset processor")
	}
	if e.Compiler.Estimator == nil || e.Compiler.ProviderContextTokens <= 0 {
		return errors.New("model executor requires a configured prompt compiler")
	}
	if strings.TrimSpace(e.PolicyVersion) == "" {
		return errors.New("model executor requires a policy version")
	}
	return nil
}

func (e ModelExecutor) leaseTTL() time.Duration {
	if e.LeaseTTL <= 0 {
		return 15 * time.Minute
	}
	return e.LeaseTTL
}

// ModelEligible reports whether an OperationSpec should run on the model path.
// Continuity/local specs stay on LocalExecutor even if PROPOSE_ONLY.
func ModelEligible(spec domain.OperationSpec) bool {
	if err := spec.Validate(); err != nil {
		return false
	}
	if LocalEligible(spec) {
		return false
	}
	return spec.MaximumAuthority == domain.AuthorityProposeOnly
}

// Execute runs one READY, model-eligible operation. Non-eligible ops are skipped
// (not errors) so the control loop can try LocalExecutor or wait.
func (e ModelExecutor) Execute(ctx context.Context, operationID domain.OperationID) (ModelExecuteResult, error) {
	if err := e.validateDeps(); err != nil {
		return ModelExecuteResult{}, err
	}
	if operationID == "" {
		return ModelExecuteResult{}, errors.New("operation id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var result ModelExecuteResult
	result.OperationID = operationID

	// Phase 1: claim lease under READY → RUNNING (short transaction).
	var (
		operation domain.Operation
		spec      domain.OperationSpec
		leaseRef  string
		now       time.Time
	)
	err := e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operationID)
		if err != nil {
			return err
		}
		if op.State.Terminal() {
			result.Skipped = true
			result.SkipReason = "terminal"
			return nil
		}
		if op.State != domain.StateReady {
			result.Skipped = true
			result.SkipReason = "not_ready"
			return nil
		}
		loadedSpec, err := tx.OperationSpec(op.SpecID)
		if err != nil {
			return fmt.Errorf("load operation spec %s: %w", op.SpecID, err)
		}
		if !ModelEligible(loadedSpec) {
			result.Skipped = true
			if LocalEligible(loadedSpec) {
				result.SkipReason = "local_path"
			} else {
				result.SkipReason = "not_model_eligible"
			}
			return nil
		}

		leaseID, err := e.IDs.NewID("lease")
		if err != nil {
			return fmt.Errorf("generate lease id: %w", err)
		}
		if strings.TrimSpace(leaseID) == "" {
			return errors.New("generated lease id must not be empty")
		}
		now = e.Clock.Now().UTC()
		until := now.Add(e.leaseTTL())
		leaseRef = FormatLeaseRef(leaseID, op.ID, op.Attempt+1, until)
		result.LeaseRef = leaseRef

		snap := domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation}
		running, err := domain.Transition(snap, domain.TransitionInput{Event: domain.EventDispatch, Reference: leaseRef})
		if err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
		op.State = running.State
		op.Reevaluation = running.Reevaluation
		op.Attempt++
		if err := tx.SaveOperation(op); err != nil {
			return err
		}
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:dispatched:%d", op.ID, op.Attempt)),
			Kind:            EventOperationDispatched,
			OccurredAt:      now,
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      leaseRef,
		}); err != nil {
			return err
		}
		operation = op
		spec = loadedSpec
		return nil
	})
	if err != nil {
		return result, err
	}
	if result.Skipped {
		return result, nil
	}

	// Phase 2: compile + model call(s) outside the write transaction.
	// FR-MODEL-004: at most Budget.ModelCalls Complete invocations; prefer short
	// correction / simpler format over full replan loops; exhaust when spent.
	baseCommit := domain.GenesisCommitID
	_ = e.Store.View(ctx, func(r port.Reader) error {
		if head, headErr := r.HeadCommit(operation.MissionRevision); headErr == nil {
			baseCommit = head.ID
		}
		return nil
	})

	compileInput, err := e.buildPromptInput(operation, spec, baseCommit)
	if err != nil {
		return result, e.failRunning(ctx, operation, leaseRef, err)
	}
	// FR-MODEL-006/007: select reversible enrichment + conservative context.
	profile := e.resolveProfile()
	plan := domain.SelectAdaptationPlan(domain.AdaptationSelectionInput{
		Profile:               profile,
		PreferJSON:            true, // PROPOSE_ONLY path expects JSON ChangeSet.
		PreferExpandedContext: e.PreferExpandedContext,
		AllowNativeTools:      false, // tools not wired in MVP executor path
	})
	compiler := e.Compiler
	if plan.ContextTokens > 0 && (compiler.ProviderContextTokens <= 0 || plan.ContextTokens < compiler.ProviderContextTokens) {
		compiler.ProviderContextTokens = plan.ContextTokens
	}
	compiled, err := compiler.Compile(spec, compileInput)
	if err != nil {
		return result, e.failRunning(ctx, operation, leaseRef, fmt.Errorf("compile prompt: %w", err))
	}
	_ = e.appendAdaptationEvent(ctx, operation, leaseRef, plan, 0)

	maxCalls := spec.Budget.ModelCalls
	if maxCalls <= 0 {
		// Budget zero means no Complete authorized (domain.Budget semantics).
		return result, e.failRunning(ctx, operation, leaseRef, errors.New("operation model_calls budget is zero"))
	}
	budget := domain.NewModelRecoveryBudget(spec, operation.Attempt, 0)
	budget.FallbackAvailable = e.FallbackProvider != nil
	request := compiled.Request
	request.ResponseFormat = plan.ResponseFormat
	activeProvider := e.Provider
	usingFallback := false
	var lastCompletion port.CompletionResult
	var lastErr error
	var lastRaw string

	for {
		if budget.ModelCallsUsed >= maxCalls {
			break
		}
		completion, callErr := activeProvider.Complete(ctx, request)
		budget.ModelCallsUsed++
		result.ModelCalls = budget.ModelCallsUsed
		if callErr != nil {
			lastErr = fmt.Errorf("model complete: %w", callErr)
			// FR-MODEL-006: enrichment-related transport failures demote and retry
			// on baseline when budget remains; other provider errors exit the loop.
			safeDetail := safeErrorDetail(callErr)
			failClass := domain.ClassifyAdaptationFailure(safeDetail)
			if domain.ShouldDemote(plan.Level, failClass) && budget.ModelCallsUsed < maxCalls {
				plan = domain.PlanAfterDemotion(plan, profile)
				request.ResponseFormat = plan.ResponseFormat
				_ = e.appendAdaptationEvent(ctx, operation, leaseRef, plan, budget.ModelCallsUsed)
				continue
			}
			// Transport/provider errors without recoverable enrichment: disposition after loop.
			break
		}
		if strings.TrimSpace(completion.Model) == "" {
			completion.Model = "unknown"
		}
		lastRaw = completion.Text
		// Preserve exact provider text for Process raw artifact; work on a copy for lineage.
		working := completion
		working.Text, err = ensureProposalLineage(completion.Text, operation, baseCommit, e.IDs, completion.Model)
		if err != nil {
			lastErr = fmt.Errorf("prepare proposal lineage: %w", err)
			break
		}
		// Raw preservation: Process must see original bytes when lineage was not rewritten.
		// ensureProposalLineage returns original text when it does not rewrite.
		lastCompletion = working

		// Transition RUNNING → VERIFYING once before first Process; stay VERIFYING on retries.
		if operation.State == domain.StateRunning {
			err = e.Store.Update(ctx, func(tx port.Transaction) error {
				op, err := tx.Operation(operationID)
				if err != nil {
					return err
				}
				if op.State != domain.StateRunning || op.Reevaluation.Reference != leaseRef {
					return fmt.Errorf("%w: operation lease changed during model call", port.ErrConflict)
				}
				verifying, err := domain.Transition(
					domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
					domain.TransitionInput{Event: domain.EventBeginVerify, Reference: leaseRef},
				)
				if err != nil {
					return fmt.Errorf("begin verify: %w", err)
				}
				op.State = verifying.State
				op.Reevaluation = verifying.Reevaluation
				if err := tx.SaveOperation(op); err != nil {
					return err
				}
				if _, err := tx.AppendEvent(domain.Event{
					SchemaVersion:   domain.SchemaVersionV1,
					ID:              domain.EventID(fmt.Sprintf("%s:model_invoked:%d:%d", op.ID, op.Attempt, budget.ModelCallsUsed)),
					Kind:            EventOperationModelInvoked,
					OccurredAt:      e.Clock.Now().UTC(),
					MissionRevision: op.MissionRevision,
					InquiryID:       op.InquiryID,
					OperationID:     op.ID,
					PayloadRef:      leaseRef + ";model=" + completion.Model + ";call=" + fmt.Sprintf("%d", budget.ModelCallsUsed) + fallbackTag(usingFallback),
				}); err != nil {
					return err
				}
				operation = op
				return nil
			})
			if err != nil {
				return result, err
			}
		} else {
			// Subsequent recovery calls while already VERIFYING — audit only.
			_ = e.Store.Update(ctx, func(tx port.Transaction) error {
				op, err := tx.Operation(operationID)
				if err != nil {
					return err
				}
				if op.State != domain.StateVerifying || op.Reevaluation.Reference != leaseRef {
					return fmt.Errorf("%w: operation lease changed during recovery call", port.ErrConflict)
				}
				_, err = tx.AppendEvent(domain.Event{
					SchemaVersion:   domain.SchemaVersionV1,
					ID:              domain.EventID(fmt.Sprintf("%s:model_recovery:%d:%d", op.ID, op.Attempt, budget.ModelCallsUsed)),
					Kind:            EventOperationModelInvoked,
					OccurredAt:      e.Clock.Now().UTC(),
					MissionRevision: op.MissionRevision,
					InquiryID:       op.InquiryID,
					OperationID:     op.ID,
					PayloadRef:      leaseRef + ";model=" + completion.Model + ";call=" + fmt.Sprintf("%d", budget.ModelCallsUsed) + ";recovery=1" + fallbackTag(usingFallback),
				})
				return err
			})
		}

		commit, processErr := e.Changes.Process(ctx, operationID, lastCompletion)
		if processErr == nil {
			result.CommitID = commit.ID
			lastErr = nil
			// Phase 4: SUCCEED after durable commit.
			err = e.Store.Update(ctx, func(tx port.Transaction) error {
				op, err := tx.Operation(operationID)
				if err != nil {
					return err
				}
				if op.State != domain.StateVerifying || op.Reevaluation.Reference != leaseRef {
					return fmt.Errorf("%w: operation lease changed during verify", port.ErrConflict)
				}
				done, err := domain.Transition(
					domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
					domain.TransitionInput{Event: domain.EventSucceed},
				)
				if err != nil {
					return fmt.Errorf("succeed: %w", err)
				}
				op.State = done.State
				op.Reevaluation = done.Reevaluation
				if err := tx.SaveOperation(op); err != nil {
					return err
				}
				if inquiry, inqErr := tx.Inquiry(op.InquiryID); inqErr == nil && inquiry.State == domain.StateReady {
					inqSnap := domain.OperationalSnapshot{State: inquiry.State, Reevaluation: inquiry.Reevaluation}
					inqRunning, err := domain.Transition(inqSnap, domain.TransitionInput{Event: domain.EventDispatch, Reference: leaseRef})
					if err == nil {
						inqVerifying, err := domain.Transition(inqRunning, domain.TransitionInput{Event: domain.EventBeginVerify, Reference: leaseRef})
						if err == nil {
							inqDone, err := domain.Transition(inqVerifying, domain.TransitionInput{Event: domain.EventSucceed})
							if err == nil {
								inquiry.State = inqDone.State
								inquiry.Reevaluation = inqDone.Reevaluation
								_ = tx.SaveInquiry(inquiry)
							}
						}
					}
				}
				now = e.Clock.Now().UTC()
				payload := leaseRef + ";commit=" + string(commit.ID)
				for _, event := range []domain.Event{
					{
						SchemaVersion: domain.SchemaVersionV1,
						ID:            domain.EventID(fmt.Sprintf("%s:model_verified:%d", op.ID, op.Attempt)),
						Kind:          EventOperationModelVerified,
						OccurredAt:    now, MissionRevision: op.MissionRevision, InquiryID: op.InquiryID,
						OperationID: op.ID, PayloadRef: payload,
					},
					{
						SchemaVersion: domain.SchemaVersionV1,
						ID:            domain.EventID(fmt.Sprintf("%s:succeeded:%d", op.ID, op.Attempt)),
						Kind:          EventOperationSucceeded,
						OccurredAt:    now, MissionRevision: op.MissionRevision, InquiryID: op.InquiryID,
						OperationID: op.ID, PayloadRef: payload,
					},
				} {
					if _, err := tx.AppendEvent(event); err != nil {
						return err
					}
				}
				result.Completed = true
				return nil
			})
			if err != nil {
				return result, err
			}
			return result, nil
		}

		// Process failed with known non-effect: decide recovery (steps 5–8).
		lastErr = processErr
		safeDetail := safeErrorDetail(processErr)
		// FR-MODEL-006: demote enrichment before another Complete when failure
		// class indicates the enriched transport/format is unsafe.
		failClass := domain.ClassifyAdaptationFailure(safeDetail)
		if domain.ShouldDemote(plan.Level, failClass) {
			plan = domain.PlanAfterDemotion(plan, profile)
			_ = e.appendAdaptationEvent(ctx, operation, leaseRef, plan, budget.ModelCallsUsed)
		}
		decision := domain.DecideNextRecovery(budget)
		result.RecoveryStages = append(result.RecoveryStages, decision.Stage)

		switch decision.Disposition {
		case domain.DispositionShortCorrect:
			corr := modeltext.BuildShortCorrection(modeltext.ShortCorrectionInput{
				PreviousOutput: lastRaw,
				SafeError:      safeDetail,
				AnswerFormat:   compileInput.AnswerFormat,
			})
			request = port.CompletionRequest{
				Prompt:          corr.Prompt,
				MaxOutputTokens: compiled.Request.MaxOutputTokens,
				Temperature:     0,
				ResponseFormat:  plan.ResponseFormat,
			}
			budget.ShortCorrectionUsed = true
			_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
			continue
		case domain.DispositionSimplerFormat:
			corr := modeltext.BuildSimplerFormatCorrection(lastRaw, safeDetail)
			// Simpler format recovery always drops enrichment (baseline text).
			plan = domain.PlanAfterDemotion(domain.AdaptationPlan{Level: domain.AdaptationAssistedJSON}, profile)
			request = port.CompletionRequest{
				Prompt:          corr.Prompt,
				MaxOutputTokens: compiled.Request.MaxOutputTokens,
				Temperature:     0,
				ResponseFormat:  domain.ResponseFormatNone,
			}
			budget.SimplerFormatUsed = true
			_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
			continue
		case domain.DispositionFallbackModel:
			if e.FallbackProvider == nil {
				// Policy should not select this without FallbackAvailable; fail safe.
				budget.FallbackModelUsed = true
				_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
				continue
			}
			// One shot on the alternate provider with the original compiled prompt
			// (different model, not a full multi-retry of the same endpoint).
			// Baseline text on fallback — do not re-apply failed enrichment.
			activeProvider = e.FallbackProvider
			usingFallback = true
			plan = domain.AdaptationPlan{
				Level: domain.AdaptationBaseline, ContextTokens: plan.ContextTokens,
				Reason: "fallback_baseline", Reversible: true,
			}
			request = compiled.Request
			request.ResponseFormat = domain.ResponseFormatNone
			budget.FallbackModelUsed = true
			_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
			continue
		case domain.DispositionReplan:
			_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
			return result, e.failVerifying(ctx, operation, leaseRef, lastErr)
		default: // Exhaust
			_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
			result.Exhausted = true
			return result, e.exhaustOperation(ctx, operation, leaseRef, lastErr, decision)
		}
	}

	// Loop exited without success (provider error or call cap).
	if lastErr == nil {
		lastErr = errors.New("model recovery budget exhausted without valid proposal")
	}
	decision := domain.DecideNextRecovery(budget)
	// Force replan/exhaust when text recovery cannot run (transport break or empty).
	// Intra-loop dispositions that need another Complete are not applicable after break.
	switch decision.Disposition {
	case domain.DispositionShortCorrect, domain.DispositionSimplerFormat, domain.DispositionFallbackModel:
		if budget.AllowReplan {
			decision = domain.ModelRecoveryDecision{
				Disposition: domain.DispositionReplan, Stage: domain.RecoveryDefer,
				Reason: "provider_error_replan", RemainingModelCalls: budget.RemainingModelCalls(),
			}
		} else {
			decision = domain.ModelRecoveryDecision{
				Disposition: domain.DispositionExhaust, Stage: domain.RecoveryDefer,
				Reason: "provider_error_exhaust", RemainingModelCalls: budget.RemainingModelCalls(),
			}
		}
	}
	result.RecoveryStages = append(result.RecoveryStages, decision.Stage)
	_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
	if decision.Disposition == domain.DispositionExhaust {
		result.Exhausted = true
		return result, e.exhaustOperation(ctx, operation, leaseRef, lastErr, decision)
	}
	if operation.State == domain.StateVerifying {
		return result, e.failVerifying(ctx, operation, leaseRef, lastErr)
	}
	return result, e.failRunning(ctx, operation, leaseRef, lastErr)
}

func (e ModelExecutor) buildPromptInput(operation domain.Operation, spec domain.OperationSpec, baseCommit domain.CommitID) (prompt.Input, error) {
	task := strings.TrimSpace(operation.ExpectedOutput)
	if task == "" {
		task = "propose a single ProposedChangeSet JSON object"
	}
	facts := []prompt.Fact{
		{ID: "operation_id", Text: string(operation.ID), Required: true, Priority: 100},
		{ID: "mission_revision_id", Text: string(operation.MissionRevision), Required: true, Priority: 100},
		{ID: "idempotency_key", Text: string(operation.IdempotencyKey), Required: true, Priority: 100},
		{ID: "base_commit_id", Text: string(baseCommit), Required: true, Priority: 100},
		{ID: "read_set", Text: strings.Join(operation.ReadSet, ","), Required: false, Priority: 50},
		{ID: "input_refs", Text: strings.Join(operation.InputRefs, ","), Required: false, Priority: 40},
		{ID: "spec_id", Text: string(spec.ID), Required: false, Priority: 30},
		{ID: "validators", Text: strings.Join(spec.Validators, ","), Required: true, Priority: 90},
	}
	return prompt.Input{
		Task:  task,
		Facts: facts,
		Constraints: []string{
			"Respond with exactly one JSON object and no markdown fences.",
			"Do not invent authority: only propose ADD/REPLACE/DEPRECATE changes.",
			"validator_ids must match the operation spec validators exactly.",
			"mission_revision_id, operation_id, base_commit_id, read_set, and idempotency_key must match the facts.",
			"schema_version must be 1.",
		},
		AllowedOutputs: []string{"application/json", "ProposedChangeSet"},
		AnswerFormat:   "single ProposedChangeSet JSON object with canonical snake_case keys",
	}, nil
}

// ensureProposalLineage fills identity fields the model must not invent when
// the response is a JSON object missing them. If the response is already a full
// valid proposal, it is left unchanged. Non-JSON remains unchanged so
// changeset.Process can preserve raw text and reject.
//
// Before interpreting the object, a deterministic local normalization step
// (FR-MODEL-004: trim/BOM/fence/object extract) is applied so weak-model
// fences do not block lineage injection. The original provider text is still
// what Process preserves as RawModelOutput when the executor passes the
// post-lineage completion through — callers that need exact bytes should copy
// them before this helper rewrites the working text.
func ensureProposalLineage(text string, operation domain.Operation, baseCommit domain.CommitID, ids source.IDGenerator, modelName string) (string, error) {
	// Work on a normalized candidate; if normalization finds no object, keep
	// the original bytes untouched for raw preservation + strict rejection.
	candidate := modeltext.BestJSONCandidate(text)
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" || trimmed[0] != '{' {
		return text, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return text, nil
	}
	// If core lineage is already present, do not rewrite. Return the original
	// provider text so RawModelOutput keeps exact bytes; DecodeStrict will
	// re-apply local normalization when parsing the typed proposal.
	if _, ok := fields["operation_id"]; ok {
		if _, ok := fields["idempotency_key"]; ok {
			if _, ok := fields["mission_revision_id"]; ok {
				return text, nil
			}
		}
	}
	// Partial object: inject deterministic lineage for vertical-slice harnesses
	// that return only changes[] / expected_delta skeletons.
	type skeleton struct {
		SchemaVersion   int             `json:"schema_version"`
		ID              string          `json:"id"`
		MissionRevision string          `json:"mission_revision_id"`
		OperationID     string          `json:"operation_id"`
		BaseCommitID    string          `json:"base_commit_id"`
		ReadSet         []string        `json:"read_set"`
		Preconditions   []string        `json:"preconditions"`
		Changes         []domain.Change `json:"changes"`
		ExpectedDelta   string          `json:"expected_delta"`
		ValidatorIDs    []string        `json:"validator_ids"`
		Provenance      string          `json:"provenance"`
		IdempotencyKey  string          `json:"idempotency_key"`
	}
	var partial skeleton
	if err := json.Unmarshal([]byte(trimmed), &partial); err != nil {
		return text, nil
	}
	if len(partial.Changes) == 0 {
		return text, nil
	}
	if partial.ID == "" {
		id, err := ids.NewID("changeset")
		if err != nil {
			return "", err
		}
		partial.ID = id
	}
	if partial.SchemaVersion == 0 {
		partial.SchemaVersion = domain.SchemaVersionV1
	}
	if partial.MissionRevision == "" {
		partial.MissionRevision = string(operation.MissionRevision)
	}
	if partial.OperationID == "" {
		partial.OperationID = string(operation.ID)
	}
	if partial.BaseCommitID == "" {
		partial.BaseCommitID = string(baseCommit)
	}
	if partial.ReadSet == nil {
		partial.ReadSet = append([]string(nil), operation.ReadSet...)
		if partial.ReadSet == nil {
			partial.ReadSet = []string{}
		}
	}
	if partial.Preconditions == nil {
		partial.Preconditions = []string{}
	}
	if partial.ExpectedDelta == "" {
		partial.ExpectedDelta = operation.ExpectedOutput
		if partial.ExpectedDelta == "" {
			partial.ExpectedDelta = "model proposal"
		}
	}
	if len(partial.ValidatorIDs) == 0 {
		// Validators are filled from the stored spec by the caller path via
		// process validation; inject schema as the only known deterministic one
		// when the skeleton omitted them — the processor still checks against
		// the OperationSpec list.
		partial.ValidatorIDs = []string{"schema"}
	}
	if partial.Provenance == "" {
		partial.Provenance = "model:" + modelName
	}
	if partial.IdempotencyKey == "" {
		partial.IdempotencyKey = string(operation.IdempotencyKey)
	}
	encoded, err := json.Marshal(partial)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (e ModelExecutor) failRunning(ctx context.Context, operation domain.Operation, leaseRef string, cause error) error {
	return e.failWith(ctx, operation.ID, leaseRef, domain.StateRunning, cause)
}

func (e ModelExecutor) failVerifying(ctx context.Context, operation domain.Operation, leaseRef string, cause error) error {
	return e.failWith(ctx, operation.ID, leaseRef, domain.StateVerifying, cause)
}

func (e ModelExecutor) failWith(ctx context.Context, operationID domain.OperationID, leaseRef string, expect domain.OperationalState, cause error) error {
	failErr := e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operationID)
		if err != nil {
			return err
		}
		if op.State != expect || op.Reevaluation.Reference != leaseRef {
			// Lease already moved (reaper/crash); surface original cause.
			return nil
		}
		// Known non-effect for model/compile failures: replan to READY via
		// REQUEST_REPLAN rather than SUCCEED. EffectUnknown is reserved for
		// ambiguous transport; here the call clearly did not apply.
		next, err := domain.Transition(
			domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
			domain.TransitionInput{Event: domain.EventRequestReplan, Reference: leaseRef},
		)
		if err != nil {
			return err
		}
		ready, err := domain.Transition(next, domain.TransitionInput{Event: domain.EventResume})
		if err != nil {
			return err
		}
		op.State = ready.State
		op.Reevaluation = ready.Reevaluation
		if err := tx.SaveOperation(op); err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:model_fail:%d:%d", op.ID, op.Attempt, e.Clock.Now().UnixNano())),
			Kind:            "operation.model_failed",
			OccurredAt:      e.Clock.Now().UTC(),
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      leaseRef + ";error_class=model_or_validation",
		})
		return err
	})
	if failErr != nil {
		return fmt.Errorf("%v; also failed to replan operation: %w", cause, failErr)
	}
	return cause
}

// exhaustOperation terminals the operation as EXHAUSTED (FR-MODEL-004 step 8).
// Used when recovery budget is spent so always-invalid models cannot loop.
func (e ModelExecutor) exhaustOperation(ctx context.Context, operation domain.Operation, leaseRef string, cause error, decision domain.ModelRecoveryDecision) error {
	expect := operation.State
	if expect != domain.StateRunning && expect != domain.StateVerifying {
		expect = domain.StateVerifying
	}
	exhaustErr := e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operation.ID)
		if err != nil {
			return err
		}
		if (op.State != domain.StateRunning && op.State != domain.StateVerifying) || op.Reevaluation.Reference != leaseRef {
			return nil
		}
		done, err := domain.Transition(
			domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
			domain.TransitionInput{Event: domain.EventExhaust},
		)
		if err != nil {
			return err
		}
		op.State = done.State
		op.Reevaluation = done.Reevaluation
		if err := tx.SaveOperation(op); err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:model_exhausted:%d:%d", op.ID, op.Attempt, e.Clock.Now().UnixNano())),
			Kind:            "operation.model_exhausted",
			OccurredAt:      e.Clock.Now().UTC(),
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      leaseRef + ";reason=" + decision.Reason + ";disposition=" + string(decision.Disposition),
		})
		return err
	})
	_ = expect
	if exhaustErr != nil {
		return fmt.Errorf("%v; also failed to exhaust operation: %w", cause, exhaustErr)
	}
	return fmt.Errorf("model recovery exhausted: %w", cause)
}

func (e ModelExecutor) appendRecoveryEvent(ctx context.Context, operation domain.Operation, leaseRef string, decision domain.ModelRecoveryDecision, callsUsed int) error {
	return e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operation.ID)
		if err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:model_recovery_decision:%d:%d:%d", op.ID, op.Attempt, callsUsed, e.Clock.Now().UnixNano())),
			Kind:            "operation.model_recovery_decision",
			OccurredAt:      e.Clock.Now().UTC(),
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef: fmt.Sprintf("%s;disposition=%s;stage=%s;reason=%s;calls=%d",
				leaseRef, decision.Disposition, decision.Stage, decision.Reason, callsUsed),
		})
		return err
	})
}

// resolveProfile returns the configured capability snapshot or a conservative
// declared baseline derived from the compiler window (never invents tools/JSON).
func (e ModelExecutor) resolveProfile() domain.ProviderProfile {
	if e.Profile.SchemaVersion == domain.SchemaVersionV1 && strings.TrimSpace(e.Profile.Name) != "" {
		if err := e.Profile.Validate(); err == nil {
			return e.Profile
		}
	}
	ctxTokens := e.Compiler.ProviderContextTokens
	now := time.Unix(0, 0).UTC()
	if e.Clock != nil {
		now = e.Clock.Now().UTC()
	}
	return domain.BaselineDeclaredProfile("model-executor", "", domain.MaxOutputDialectLegacy, ctxTokens, now)
}

func (e ModelExecutor) appendAdaptationEvent(ctx context.Context, operation domain.Operation, leaseRef string, plan domain.AdaptationPlan, callsUsed int) error {
	return e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operation.ID)
		if err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:model_adaptation:%d:%d:%d", op.ID, op.Attempt, callsUsed, e.Clock.Now().UnixNano())),
			Kind:            "operation.model_adaptation",
			OccurredAt:      e.Clock.Now().UTC(),
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      leaseRef + ";" + domain.FormatAdaptationAudit(plan),
		})
		return err
	})
}

// safeErrorDetail redacts a validation/provider error to a short operator-safe string.
func safeErrorDetail(err error) string {
	if err == nil {
		return "unknown validation failure"
	}
	s := err.Error()
	// Drop likely JSON body fragments.
	if i := strings.Index(s, "{"); i >= 0 && i < 80 {
		s = s[:i] + "(payload omitted)"
	}
	s = strings.TrimSpace(s)
	if len(s) > 240 {
		s = s[:240]
	}
	if s == "" {
		return "validation failure"
	}
	return s
}

func fallbackTag(usingFallback bool) string {
	if usingFallback {
		return ";fallback=1"
	}
	return ""
}
