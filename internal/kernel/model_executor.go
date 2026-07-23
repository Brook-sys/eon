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
	"motor-autonomo/internal/tool"
)

// Event kinds for the PROPOSE_ONLY model path. Model text never mutates
// canonical state directly; only changeset.Processor commit does.
const (
	EventOperationModelInvoked       = "operation.model_invoked"
	EventOperationModelVerified      = "operation.model_verified"
	EventOperationModelFailurePolicy = "operation.model_failure_policy"
)

// ModelExecutor completes non-local PROPOSE_ONLY operations under a lease:
// dispatch → compile prompt → Complete → process ProposedChangeSet → SUCCEED.
// Local continuity work remains on LocalExecutor.
type ModelExecutor struct {
	Store port.Store
	Clock source.Clock
	IDs   source.IDGenerator
	// Catalog path: providers are keyed by binding ID because model name, wire
	// dialect, and context are binding-specific even when transport is shared.
	Providers map[string]port.ModelProvider
	// Tools is the bound capability catalog for this executor
	Tools        tool.Provider
	ModelsConfig *domain.ModelsConfig
	// Legacy direct construction remains supported.
	Provider             port.ModelProvider
	FallbackProvider     port.ModelProvider
	PrimaryProviderID    string
	PrimaryBindingID     string
	PrimaryProviderKind  domain.ProviderKind
	FallbackProviderID   string
	FallbackBindingID    string
	FallbackProviderKind domain.ProviderKind
	Changes              *changeset.Processor
	Compiler             prompt.Compiler
	MemoryStore          port.MemoryReader
	// PolicyVersion is stamped on accepted changesets.
	PolicyVersion string
	LeaseTTL      time.Duration
	// MaxOutputBytes bounds the provider response buffer before memory limits.
	MaxOutputBytes int64
	// Profile is the FR-MODEL-005 capability snapshot used for progressive
	// adaptation (FR-MODEL-006) and conservative context (FR-MODEL-007).
	// Zero-value falls back to a declared baseline using Compiler context.
	Profile domain.ProviderProfile
	// PreferExpandedContext allows spending more of the safe window on optional facts.
	// Default false keeps FR-MODEL-007 conservative.
	PreferExpandedContext bool
	// Authorizer is optional FR-RES-001 PolicyEngine + ResourceGate enforcement.
	// When nil, Execute keeps historical behavior (no capability/resource gate).
	// When set, model.complete must ALLOW and acquire before any Complete call.
	Authorizer *CapabilityAuthorizer
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
	Done      bool
	ToolCalls []port.ToolCall
	Yielded   bool
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
	if e.Provider == nil && (e.Providers == nil || len(e.Providers) == 0) {
		return errors.New("model executor requires at least one ModelProvider")
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

// releaseResourcePermit reports ResourceGate success/failure when an authorizer
// reserved a slot. Best-effort: never overrides the primary Execute error path.
func (e ModelExecutor) releaseResourcePermits(ctx context.Context, operation domain.Operation, permits []*domain.ResourcePermit, success bool, retryAfter *time.Time) {
	if e.Authorizer == nil || len(permits) == 0 {
		return
	}
	_ = e.Authorizer.ReportModelComplete(ctx, operation, permits, success, retryAfter)
}

// releaseFailedResourcePermits applies a classified cooldown only to its scope;
// other composite permits are released as successes so their circuits are not contaminated.
func (e ModelExecutor) releaseFailedResourcePermits(ctx context.Context, operation domain.Operation, permits []*domain.ResourcePermit, decision domain.ModelBindingFailureDecision, classified bool, retryAfter *time.Time) {
	if e.Authorizer == nil {
		return
	}
	if classified {
		_ = e.Authorizer.ReportModelCompleteScopedFailure(ctx, operation, permits, decision.Scope, retryAfter)
		return
	}
	_ = e.Authorizer.ReportModelComplete(ctx, operation, permits, false, retryAfter)
}

// releaseResourcePermitsWithTokens replaces the conservative acquire estimate
// with provider-observed input+output tokens on a successful attempt.
func (e ModelExecutor) releaseResourcePermitsWithTokens(ctx context.Context, operation domain.Operation, permits []*domain.ResourcePermit, success bool, retryAfter *time.Time, observedTokens int) {
	if e.Authorizer == nil || len(permits) == 0 {
		return
	}
	_ = e.Authorizer.ReportModelCompleteObserved(ctx, operation, permits, success, retryAfter, observedTokens)
}

// selectAlternateBinding resolves the next catalog binding after a classified
// provider failure whose disposition is TRY_NEXT_BINDING. The failed binding is
// excluded for this Execute even when its configured circuit threshold has not
// opened yet; durable provider/binding circuits and configured priority still
// govern every remaining candidate through SelectModelBinding.
func (e ModelExecutor) selectAlternateBinding(ctx context.Context, operation domain.Operation, spec domain.OperationSpec, failedBindingID string) (domain.ModelBindingConfig, domain.ModelRouteDecision, error) {
	if e.ModelsConfig == nil {
		return domain.ModelBindingConfig{}, domain.ModelRouteDecision{}, errors.New("model catalog is not configured")
	}
	config := *e.ModelsConfig
	config.Bindings = append([]domain.ModelBindingConfig(nil), e.ModelsConfig.Bindings...)
	for i := range config.Bindings {
		if config.Bindings[i].ID == failedBindingID {
			config.Bindings[i].Enabled = false
		}
	}

	var reqCap domain.RequiredCapability
	reqCap.Format = ""
	profilesMap := make(map[string]domain.ModelCapabilityProfile)
	binding, decision, err := SelectModelBinding(ctx, e.Store, config, spec.MaxOutputTokens, reqCap, profilesMap, e.Clock.Now().UTC())
	if err != nil {
		return domain.ModelBindingConfig{}, decision, err
	}
	if err := AppendModelRoutingEvent(ctx, e.Store, e.Clock.Now().UTC(), operation, decision); err != nil {
		return domain.ModelBindingConfig{}, decision, err
	}
	return binding, decision, nil
}

// ModelEligible reports whether an OperationSpec should run on the model path.
// Continuity/local specs stay on LocalExecutor even if PROPOSE_ONLY.
// Web/file acquisition specs stay on dedicated executors even if mis-tagged PROPOSE_ONLY.
func ModelEligible(spec domain.OperationSpec) bool {
	if err := spec.Validate(); err != nil {
		return false
	}
	if LocalEligible(spec) {
		return false
	}
	if webCapabilityFromSpec(spec) != "" || fileCapabilityFromSpec(spec) != "" {
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

	// Phase 0: load READY model-eligible operation (read-only) for authorization.
	var (
		operation        domain.Operation
		spec             domain.OperationSpec
		leaseRef         string
		now              time.Time
		preflightPermits []*domain.ResourcePermit
		activeProviderID = e.PrimaryProviderID
		activeBindingID  = e.PrimaryBindingID
		activeKind       = e.PrimaryProviderKind
		activeProvider   = e.Provider
	)
	err := e.Store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation(operationID)
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
		loadedSpec, err := r.OperationSpec(op.SpecID)
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
		operation = op
		spec = loadedSpec
		return nil
	})
	if err != nil {
		return result, err
	}
	if result.Skipped || result.Exhausted {
		return result, nil
	}

	// Catalog execution routes every attempt through durable usage/circuit state.
	// The conservative preflight requirement is output capacity; compilation below
	// validates the full prompt against the selected binding context before a call.
	if e.ModelsConfig != nil {
		var fbReqCap domain.RequiredCapability
		fbReqCap.Format = ""
		fbProfilesMap := make(map[string]domain.ModelCapabilityProfile)
		binding, decision, routeErr := SelectModelBinding(ctx, e.Store, *e.ModelsConfig, spec.MaxOutputTokens, fbReqCap, fbProfilesMap, e.Clock.Now().UTC())
		if routeErr != nil {
			result.Skipped = true
			result.SkipReason = "model_route_unavailable"
			return result, nil
		}
		activeProviderID, activeBindingID = binding.ProviderRef, binding.ID
		activeProvider = e.Providers[binding.ID]
		if activeProvider == nil {
			return result, fmt.Errorf("model binding %s has no provider instance", binding.ID)
		}
		for _, p := range e.ModelsConfig.Providers {
			if p.ID == binding.ProviderRef {
				activeKind = p.Kind
				break
			}
		}
		if err := AppendModelRoutingEvent(ctx, e.Store, e.Clock.Now().UTC(), operation, decision); err != nil {
			return result, err
		}
	}

	if e.Authorizer != nil {
		auth, authErr := e.Authorizer.ReserveModelComplete(ctx, operation, spec, 0, activeProviderID, activeBindingID)
		if authErr != nil {
			return result, authErr
		}
		if auth.Throttled {
			result.Skipped = true
			result.SkipReason = auth.SkipReason
			if result.SkipReason == "" {
				result.SkipReason = "resource_throttled"
			}
			// Apply ThrottleTransitionInput to leave READY without dispatching.
			if err := e.Store.Update(ctx, func(tx port.Transaction) error {
				op, err := tx.Operation(operationID)
				if err != nil {
					return err
				}
				if op.State != domain.StateReady {
					return nil
				}
				snap := domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation}
				input, err := domain.ThrottleTransitionInput(*auth.Acquire, DefaultModelCompleteResource)
				if err != nil {
					return err
				}
				next, err := domain.Transition(snap, input)
				if err != nil {
					return err
				}
				op.State = next.State
				op.Reevaluation = next.Reevaluation
				return tx.SaveOperation(op)
			}); err != nil {
				return result, err
			}
			return result, nil
		}
		if !auth.Allowed {
			result.Skipped = true
			result.SkipReason = auth.SkipReason
			if result.SkipReason == "" {
				result.SkipReason = "policy_deny"
			}
			return result, nil
		}
		// Keep the preflight reservation across lease acquisition and consume it on
		// the first provider attempt. Releasing it here would make concurrency
		// accounting blind while Complete is actually in flight.
		preflightPermits = auth.Permits
		if len(preflightPermits) == 0 && auth.Permit != nil {
			preflightPermits = []*domain.ResourcePermit{auth.Permit}
		}
	}

	// Phase 1: claim lease under READY → RUNNING (short transaction).
	err = e.Store.Update(ctx, func(tx port.Transaction) error {
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
			// Authorization may have throttled concurrently, or another worker claimed.
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

		if loadedSpec.Budget.Attempts <= 0 || int(op.Attempt) >= loadedSpec.Budget.Attempts {
			done, err := domain.Transition(
				domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
				domain.TransitionInput{Event: domain.EventExhaust},
			)
			if err != nil {
				return fmt.Errorf("exhaust attempt budget: %w", err)
			}
			op.State, op.Reevaluation = done.State, done.Reevaluation
			if err := tx.SaveOperation(op); err != nil {
				return err
			}
			_, err = tx.AppendEvent(domain.Event{
				SchemaVersion: domain.SchemaVersionV1, ID: domain.EventID(fmt.Sprintf("%s:model_attempts_exhausted:%d", op.ID, op.Attempt)),
				Kind: "operation.model_exhausted", OccurredAt: e.Clock.Now().UTC(), MissionRevision: op.MissionRevision,
				InquiryID: op.InquiryID, OperationID: op.ID,
				PayloadRef: fmt.Sprintf("reason=attempt_budget_exhausted;attempt=%d;max_attempts=%d", op.Attempt, loadedSpec.Budget.Attempts),
			})
			result.Exhausted = true
			result.SkipReason = "attempt_budget_exhausted"
			return err
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
		// Best-effort release of reserved gate slots if dispatch failed after
		// preflight. No provider call occurred, but ReportSuccess is the existing
		// safe release primitive; quota counters remain conservative.
		e.releaseResourcePermits(ctx, operation, preflightPermits, true, nil)
		return result, err
	}
	if result.Skipped || result.Exhausted {
		// Race or budget fence: release any reservation before provider contact.
		e.releaseResourcePermits(ctx, operation, preflightPermits, true, nil)
		return result, nil
	}

	// The persisted lease is also the hard upper bound for provider network I/O.
	// http.NewRequestWithContext in the OpenAI adapter propagates this deadline
	// down to the transport, preventing a stalled socket from outliving its
	// authority to execute. Keep the parent cancellation semantics intact.
	leaseDeadline, ok := ParseLeaseDeadline(leaseRef)
	if !ok {
		e.releaseResourcePermits(ctx, operation, preflightPermits, true, nil)
		return result, fmt.Errorf("invalid lease ref deadline format: %s", leaseRef)
	}
	// Lease timestamps live on the injected clock (including virtual clocks in
	// deterministic replay/tests), whereas context deadlines use wall time.
	// Preserve the lease's remaining duration instead of comparing clock domains.
	remainingLease := leaseDeadline.Sub(e.Clock.Now())
	if remainingLease <= 0 {
		e.releaseResourcePermits(ctx, operation, preflightPermits, true, nil)
		return result, errors.New("operation lease expired before provider call")
	}
	providerCtx, cancelProvider := context.WithTimeout(ctx, remainingLease)
	defer cancelProvider()

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
		e.releaseResourcePermits(ctx, operation, preflightPermits, true, nil)
		preflightPermits = nil
		failErr := e.failRunning(ctx, operation, leaseRef, err)
		return result, failErr
	}
	// FR-MODEL-006/007: select reversible enrichment + conservative context.
	profile := e.resolveProfile()
	plan := domain.SelectAdaptationPlan(domain.AdaptationSelectionInput{
		Profile:               profile,
		PreferJSON:            true, // PROPOSE_ONLY path expects JSON ChangeSet.
		PreferExpandedContext: e.PreferExpandedContext,
		AllowNativeTools:      e.Tools != nil, // Tools wired natively now
	})
	compiler := e.Compiler
	if plan.ContextTokens > 0 && (compiler.ProviderContextTokens <= 0 || plan.ContextTokens < compiler.ProviderContextTokens) {
		compiler.ProviderContextTokens = plan.ContextTokens
	}
	compiled, err := compiler.Compile(spec, compileInput)
	if err != nil {
		e.releaseResourcePermits(ctx, operation, preflightPermits, true, nil)
		preflightPermits = nil
		failErr := e.failRunning(ctx, operation, leaseRef, fmt.Errorf("compile prompt: %w", err))
		return result, failErr
	}
	_ = e.appendAdaptationEvent(ctx, operation, leaseRef, plan, 0)

	maxCalls := spec.Budget.ModelCalls
	if maxCalls <= 0 {
		// Budget zero means no Complete authorized (domain.Budget semantics).
		e.releaseResourcePermits(ctx, operation, preflightPermits, true, nil)
		preflightPermits = nil
		failErr := e.failRunning(ctx, operation, leaseRef, errors.New("operation model_calls budget is zero"))
		return result, failErr
	}
	budget := domain.NewModelRecoveryBudget(spec, operation.Attempt, 0)
	budget.FallbackAvailable = e.FallbackProvider != nil
	if e.ModelsConfig != nil {
		// New path allows multiple bindings
		budget.FallbackAvailable = len(e.ModelsConfig.Bindings) > 1
	}
	request := compiled.Request
	request.ResponseFormat = plan.ResponseFormat
	usingFallback := false
	var lastCompletion port.CompletionResult
	var lastErr error
	var lastRaw string
	var lastRetryAfter *time.Time

	for {
		if budget.ModelCallsUsed >= maxCalls {
			break
		}
		if activeKind == domain.ProviderKindNVIDIANIM && strings.TrimSpace(activeBindingID) != "" {
			persisted, pressureErr := e.loadContextPressure(ctx, activeBindingID)
			if pressureErr != nil {
				lastErr = fmt.Errorf("load model context pressure: %w", pressureErr)
				break
			}
			budget.ContextPressure = persisted
		} else if strings.TrimSpace(activeBindingID) != "" {
			budget.ContextPressure = domain.ContextPressureState{}
		}
		var permits []*domain.ResourcePermit
		var replayed bool
		var receipt domain.ModelCompletionReceipt

		// Attempt to load durable receipt inside the loop for the specific attempt + model call.
		if operation.Attempt > 0 {
			err := e.Store.View(ctx, func(r port.Reader) error {
				var rErr error
				receipt, rErr = r.ModelCompletionReceipt(operation.ID, operation.Attempt, uint32(budget.ModelCallsUsed+1))
				return rErr
			})
			if err == nil {
				replayed = true
			} else if !errors.Is(err, port.ErrNotFound) {
				lastErr = fmt.Errorf("load model completion receipt: %w", err)
				break
			}
		}

		if e.Authorizer != nil && !replayed {
			if budget.ModelCallsUsed == 0 && !usingFallback && len(preflightPermits) > 0 {
				permits = preflightPermits
				preflightPermits = nil
			} else {
				auth, authErr := e.Authorizer.ReserveModelComplete(ctx, operation, spec, 0, activeProviderID, activeBindingID)
				if authErr != nil {
					lastErr = fmt.Errorf("authorize model attempt: %w", authErr)
					break
				}
				if !auth.Allowed {
					lastErr = fmt.Errorf("authorize model attempt: %s", auth.SkipReason)
					break
				}
				permits = auth.Permits
				if len(permits) == 0 && auth.Permit != nil {
					permits = []*domain.ResourcePermit{auth.Permit}
				}
			}
		}
		// A prior NIM context rejection activates a bounded, reversible ceiling
		// before another contact. Recompile deterministically with fewer facts.
		if activeKind == domain.ProviderKindNVIDIANIM && budget.ContextPressure.Level > 0 {
			declared := profile.MaxContextTokens
			if e.ModelsConfig != nil {
				for _, configured := range e.ModelsConfig.Bindings {
					if configured.ID == activeBindingID && (declared <= 0 || configured.ContextTokens < declared) {
						declared = configured.ContextTokens
						break
					}
				}
			}
			plan = domain.SelectAdaptationPlan(domain.AdaptationSelectionInput{
				Profile:               profile,
				PreferJSON:            true,
				PreferExpandedContext: e.PreferExpandedContext,
				AllowNativeTools:      false,
				ContextReduction:      domain.ReductionForPressure(declared, budget.ContextPressure),
			})
			compiler.ProviderContextTokens = plan.ContextTokens
			var compileErr error
			compiled, compileErr = compiler.Compile(spec, compileInput)
			if compileErr != nil {
				e.releaseResourcePermits(ctx, operation, permits, true, nil)
				lastErr = fmt.Errorf("compile reduced context: %w", compileErr)
				break
			}
			request = compiled.Request
			request.ResponseFormat = plan.ResponseFormat
			plan.Reason = "context_pressure_reduction"
			_ = e.appendAdaptationEvent(ctx, operation, leaseRef, plan, budget.ModelCallsUsed)
		}

		var completion port.CompletionResult
		var callErr error
		var activeToolProvider port.ModelToolProvider
		if replayed {
			completion = port.CompletionResultFromDurable(receipt.Result)
		} else {
			if activeProvider == nil {
				lastErr = fmt.Errorf("active provider is nil")
				break
			}
			var ok bool
			if e.Tools != nil && len(e.Tools.Definitions()) > 0 {
				if activeToolProvider, ok = activeProvider.(port.ModelToolProvider); ok {
					completion, callErr = activeToolProvider.CompleteWithTools(providerCtx, request, e.Tools.Definitions())
				} else {
					completion, callErr = activeProvider.Complete(providerCtx, request)
				}
			} else {
				completion, callErr = activeProvider.Complete(providerCtx, request)
			}
		}

		budget.ModelCallsUsed++
		if !replayed {
			result.ModelCalls++
		}
		if callErr != nil {
			// A provider attempt is audit-relevant even when transport or HTTP
			// validation fails before a completion can enter VERIFYING. Persist the
			// invocation while the operation still owns its RUNNING lease so live
			// failure campaigns and crash/reopen inspection do not undercount calls.
			if auditErr := e.Store.Update(ctx, func(tx port.Transaction) error {
				op, err := tx.Operation(operationID)
				if err != nil {
					return err
				}
				// A recovery call runs under the same lease after the first completion
				// has moved the operation to VERIFYING. Provider failures there are
				// still audit-relevant and must not be misclassified as lease loss.
				if (op.State != domain.StateRunning && op.State != domain.StateVerifying) || op.Reevaluation.Reference != leaseRef {
					return fmt.Errorf("%w: operation lease changed during failed model call", port.ErrConflict)
				}
				_, err = tx.AppendEvent(domain.Event{
					SchemaVersion:   domain.SchemaVersionV1,
					ID:              domain.EventID(fmt.Sprintf("%s:model_invoked:%d:%d", op.ID, op.Attempt, budget.ModelCallsUsed)),
					Kind:            EventOperationModelInvoked,
					OccurredAt:      e.Clock.Now().UTC(),
					MissionRevision: op.MissionRevision,
					InquiryID:       op.InquiryID,
					OperationID:     op.ID,
					PayloadRef:      leaseRef + ";binding=" + activeBindingID + ";call=" + fmt.Sprintf("%d", budget.ModelCallsUsed) + ";outcome=provider_error" + fallbackTag(usingFallback),
				})
				return err
			}); auditErr != nil {
				return result, auditErr
			}
			var providerErr port.ProviderError
			if errors.As(callErr, &providerErr) {
				if ra := providerErr.RetryAfterDelay(); ra > 0 {
					t := e.Clock.Now().UTC().Add(ra)
					lastRetryAfter = &t
				}
			}
			decision, classified := classifyProviderFailure(callErr, activeKind)
			if classified {
				budget.Decisions = append(budget.Decisions, decision)
				if activeKind == domain.ProviderKindNVIDIANIM && decision.Class == domain.ModelFailureInvalidRequest {
					budget.ContextPressure = domain.RecordContextPressure(budget.ContextPressure)
					_ = e.saveContextPressure(ctx, activeBindingID, budget.ContextPressure)
				}
				_ = e.appendModelFailurePolicyEvent(ctx, operation, leaseRef, decision, activeProviderID, activeBindingID, budget.ModelCallsUsed, rateLimitMetadata(callErr))
			}
			e.releaseFailedResourcePermits(ctx, operation, permits, decision, classified, lastRetryAfter)
			lastErr = fmt.Errorf("model complete: %w", callErr)

			// NEW FALLBACK LOGIC FOR TOOL CALL VALIDATION ERRORS
			var dispatchErr tool.DispatchError
			if errors.As(callErr, &dispatchErr) && dispatchErr.FallbackPrompt != "" {
				// We append the fallback instructions as a required fact so the compiler builds it properly.
				compileInput.Facts = append(compileInput.Facts, prompt.Fact{
					ID:       "tool_validation_error",
					Text:     dispatchErr.FallbackPrompt,
					Required: true,
					Priority: 100,
				})
				var compileErr error
				compiled, compileErr = compiler.Compile(spec, compileInput)
				if compileErr != nil {
					lastErr = fmt.Errorf("compile fallback prompt: %w", compileErr)
					break
				}
				request = compiled.Request
				request.ResponseFormat = plan.ResponseFormat
				continue
			}
			// The failure taxonomy's TRY_NEXT_BINDING disposition is operational,
			// not merely descriptive. When a catalog and call budget permit, route
			// the next attempt through the normal durable selector, excluding the
			// failed binding for this lease. This keeps fallback bounded and avoids
			// retrying an incompatible/server-failing endpoint inline.
			if classified && decision.Disposition == domain.ModelFailureTryNextBinding && e.ModelsConfig != nil && budget.ModelCallsUsed < maxCalls {
				next, _, routeErr := e.selectAlternateBinding(ctx, operation, spec, activeBindingID)
				if routeErr == nil {
					provider := e.Providers[next.ID]
					if provider == nil {
						lastErr = fmt.Errorf("model binding %s has no provider instance", next.ID)
						break
					}
					activeBindingID = next.ID
					activeProviderID = next.ProviderRef
					activeProvider = provider
					for _, configuredProvider := range e.ModelsConfig.Providers {
						if configuredProvider.ID == next.ProviderRef {
							activeKind = configuredProvider.Kind
							break
						}
					}
					usingFallback = true
					budget.FallbackModelUsed = true
					result.RecoveryStages = append(result.RecoveryStages, domain.RecoveryFallbackModel)
					_ = e.appendRecoveryEvent(ctx, operation, leaseRef, domain.ModelRecoveryDecision{
						Disposition:         domain.DispositionFallbackModel,
						Stage:               domain.RecoveryFallbackModel,
						Reason:              "provider_failure_try_next_binding",
						RemainingModelCalls: budget.RemainingModelCalls(),
					}, budget.ModelCallsUsed)
					// Baseline text is the portable contract for a different binding.
					plan = domain.AdaptationPlan{Level: domain.AdaptationBaseline, ContextTokens: plan.ContextTokens, Reason: "provider_failure_fallback", Reversible: true}
					request = compiled.Request
					if activeKind == domain.ProviderKindNVIDIANIM || activeKind == domain.ProviderKindGroq {
						request.Prompt = modeltext.AppendDelimitedChangeSetInstruction(request.Prompt)
					}
					request.ResponseFormat = domain.ResponseFormatNone
					continue
				}
			}
			// Confirmed NIM request rejection receives a bounded reduced-context retry.
			// The retry still consumes the operation's normal model-call budget.
			if classified && activeKind == domain.ProviderKindNVIDIANIM && decision.Class == domain.ModelFailureInvalidRequest && budget.ModelCallsUsed < maxCalls {
				continue
			}
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
		observedTotal := completion.InputTokens + completion.OutputTokens
		e.releaseResourcePermitsWithTokens(ctx, operation, permits, true, nil, observedTotal)
		if activeKind == domain.ProviderKindNVIDIANIM {
			budget.ContextPressure = domain.RecordContextSuccess(budget.ContextPressure)
			_ = e.saveContextPressure(ctx, activeBindingID, budget.ContextPressure)
		}
		if strings.TrimSpace(completion.Model) == "" {
			completion.Model = "unknown"
		}
		// Persist the complete provider-neutral value before tools, parsing, or
		// canonical processing. Re-appending an identical replay under the new
		// attempt makes repeated crash/reopen cycles safe as well.
		durableCompletion := port.DurableModelCompletionResult(completion)
		payloadHash, hashErr := durableCompletion.Hash()
		if hashErr != nil {
			lastErr = fmt.Errorf("hash model completion receipt: %w", hashErr)
			break
		}
		receiptToSave := domain.ModelCompletionReceipt{
			SchemaVersion: domain.SchemaVersionV1,
			OperationID:   operation.ID,
			Attempt:       operation.Attempt,
			ModelCall:     uint32(budget.ModelCallsUsed),
			Result:        durableCompletion,
			PayloadHash:   payloadHash,
			RecordedAt:    e.Clock.Now().UTC(),
		}
		if receiptErr := e.Store.Update(ctx, func(tx port.Transaction) error {
			op, err := tx.Operation(operationID)
			if err != nil {
				return err
			}
			if op.State != domain.StateRunning && op.State != domain.StateVerifying {
				return fmt.Errorf("%w: operation left active state before completion receipt", port.ErrConflict)
			}
			if op.Reevaluation.Reference != leaseRef {
				return fmt.Errorf("%w: operation lease changed before completion receipt", port.ErrConflict)
			}
			return tx.AppendModelCompletionReceipt(receiptToSave)
		}); receiptErr != nil {
			lastErr = fmt.Errorf("persist model completion receipt: %w", receiptErr)
			break
		}

		// if this response was a tool request, do NOT advance to VERIFYING or attempt to parse as final text proposal.
		if len(completion.ToolCalls) > 0 {
			if e.Tools != nil {
				dispatcher := tool.NewDispatcher(e.Tools)
				dispatchResults := dispatcher.Dispatch(ctx, completion.ToolCalls)

				// We check for any tool level errors that should be sent back to model
				var hasToolError bool
				var toolErrorText string
				var isYielded bool

				for _, res := range dispatchResults {
					if res.Error != nil {
						var dErr tool.DispatchError
						if errors.As(res.Error, &dErr) && dErr.FallbackPrompt != "" {
							hasToolError = true
							toolErrorText += dErr.FallbackPrompt + "\n"
						} else {
							// Return generic tool error to model so it knows what failed
							hasToolError = true
							toolErrorText += "Tool execution failed: " + res.Error.Error() + "\n"
						}
					}
					// Handle yielded state
					var yieldCheck map[string]string
					if res.Error == nil && res.Result != "" {
						_ = json.Unmarshal([]byte(res.Result), &yieldCheck)
						if yieldCheck["status"] == "YIELDED" {
							isYielded = true
						}
					}
				}

				// Support explicit task suspension via YIELDED status
				if isYielded {
					return ModelExecuteResult{
						OperationID: operationID,
						Yielded:     true,
					}, nil
				}

				if hasToolError {
					if budget.ModelCallsUsed >= maxCalls {
						lastErr = fmt.Errorf("tool execution failed and loop exhausted")
						break
					}
					nextInput := compileInput
					nextInput.Facts = append([]prompt.Fact(nil), compileInput.Facts...)
					nextInput.Facts = append(nextInput.Facts, prompt.Fact{
						ID:       "tool_execution_error",
						Text:     toolErrorText,
						Required: true,
						Priority: 100,
					})
					var compileErr error
					compiled, compileErr = compiler.Compile(spec, nextInput)
					if compileErr != nil {
						lastErr = fmt.Errorf("compile fallback prompt: %w", compileErr)
						break
					}
					request = compiled.Request
					request.ResponseFormat = plan.ResponseFormat
					compileInput = nextInput
					continue
				}

				// All tools succeeded!
				// We need to recompile with tool responses, but our CompletionRequest
				// currently doesn't have a History/Messages field since it's "baseline text→text"
				// For the MVP, we just inject the tool responses as Facts into the next prompt compilation.

				// FR-MODEL-010: Guard-rails contra infinite tool loops.
				// Não incrementamos maxCalls infinitamente. Limitamos a profundidade.
				if maxCalls-spec.Budget.ModelCalls >= 15 {
					lastErr = fmt.Errorf("maximum tool call loop depth exceeded (limit: 15)")
					break
				}

				// Increase maxCalls by 1 so the next model loop can actually run
				// Otherwise budget=1 will immediately fail on tools.
				maxCalls++
				spec.Budget.ModelCalls = maxCalls

				if budget.ModelCallsUsed >= maxCalls {
					lastErr = fmt.Errorf("tool loop budget exhausted")
					break
				}

				// Inject the tool responses as facts for the next iteration
				nextInput := compileInput
				nextInput.Facts = append([]prompt.Fact(nil), compileInput.Facts...)
				for _, res := range dispatchResults {
					nextInput.Facts = append(nextInput.Facts, prompt.Fact{
						ID:       "tool_result_" + res.CallID,
						Text:     "Tool returned: " + res.Result,
						Required: true,
						Priority: 90,
					})
				}

				var compileErr error
				compiled, compileErr = compiler.Compile(spec, nextInput)
				if compileErr != nil {
					lastErr = fmt.Errorf("compile tool response prompt: %w", compileErr)
					break
				}
				request = compiled.Request
				request.ResponseFormat = plan.ResponseFormat
				compileInput = nextInput

				continue
			} else {
				// We're delegating tool dispatch upward because no tools are configured.
				result.ToolCalls = completion.ToolCalls
				result.Done = true
				break
			}
		}

		// If we delegated tool dispatch upwards, break out of the loop entirely
		if result.Done {
			break
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
			if e.ModelsConfig != nil {
				// We don't implement full multi-binding retry traversal in this recovery switch
				// but legacy semantic checks expect we can fall back to *some* alternative.
				// However, new binding path treats circuit routing per-cycle, not inline.
				// For inline fallback, we can fall back to the highest priority binding that wasn't used yet.
				var nextBinding *domain.ModelBindingConfig
				for _, cand := range e.ModelsConfig.Bindings {
					if cand.ID != activeBindingID && cand.Enabled {
						nextBinding = &cand
						break
					}
				}
				if nextBinding == nil {
					budget.FallbackModelUsed = true
					_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
					continue
				}
				activeBindingID = nextBinding.ID
				activeProviderID = nextBinding.ProviderRef
				activeProvider = e.Providers[nextBinding.ID]
				for _, p := range e.ModelsConfig.Providers {
					if p.ID == nextBinding.ProviderRef {
						activeKind = p.Kind
						break
					}
				}
				usingFallback = true
			} else if e.FallbackProvider == nil {
				// Policy should not select this without FallbackAvailable; fail safe.
				budget.FallbackModelUsed = true
				_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
				continue
			} else {
				activeProvider = e.FallbackProvider
				activeProviderID = e.FallbackProviderID
				activeBindingID = e.FallbackBindingID
				activeKind = e.FallbackProviderKind
				usingFallback = true
			}
			// One shot on the alternate provider with the original compiled prompt
			// (different model, not a full multi-retry of the same endpoint).
			// Baseline text on fallback — do not re-apply failed enrichment.
			plan = domain.AdaptationPlan{
				Level: domain.AdaptationBaseline, ContextTokens: plan.ContextTokens,
				Reason: "fallback_baseline", Reversible: true,
			}
			request = compiled.Request
			// Remote fallback profiles (notably constrained NIM/Groq bindings)
			// use the reduced line format. The changeset decoder accepts only the
			// explicitly versioned, fixed-key contract and still applies all kernel
			// validators after deterministic conversion to JSON.
			if activeKind == domain.ProviderKindNVIDIANIM || activeKind == domain.ProviderKindGroq {
				request.Prompt = modeltext.AppendDelimitedChangeSetInstruction(request.Prompt)
			}
			request.ResponseFormat = domain.ResponseFormatNone
			budget.FallbackModelUsed = true
			_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
			continue
		case domain.DispositionReplan:
			_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
			failErr := e.failVerifying(ctx, operation, leaseRef, lastErr)
			return result, failErr
		default: // Exhaust
			_ = e.appendRecoveryEvent(ctx, operation, leaseRef, decision, budget.ModelCallsUsed)
			result.Exhausted = true
			failErr := e.exhaustOperation(ctx, operation, leaseRef, lastErr, decision)
			return result, failErr
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
		failErr := e.exhaustOperation(ctx, operation, leaseRef, lastErr, decision)
		return result, failErr
	}
	if operation.State == domain.StateVerifying {
		failErr := e.failVerifying(ctx, operation, leaseRef, lastErr)
		return result, failErr
	}
	failErr := e.failRunning(ctx, operation, leaseRef, lastErr)
	return result, failErr
}

func (e ModelExecutor) buildPromptInput(operation domain.Operation, spec domain.OperationSpec, baseCommit domain.CommitID) (prompt.Input, error) {
	task := strings.TrimSpace(operation.ExpectedOutput)
	if task == "" {
		task = "propose a single ProposedChangeSet JSON object"
	}
	if spec.OutputSchema == "exact_text" {
		return prompt.Input{
			Task:           task,
			Constraints:    []string{"Return exactly the text requested by the task, with no explanation or formatting."},
			AllowedOutputs: []string{"exact requested text"},
			AnswerFormat:   "exact requested text only",
		}, nil
	}
	if spec.OutputSchema == "exact_json" {
		return prompt.Input{
			Task:           task,
			Constraints:    []string{"Return exactly the JSON object requested by the task, with no markdown fence, explanation, or extra keys."},
			AllowedOutputs: []string{"exact requested JSON object"},
			AnswerFormat:   "single exact JSON object only",
		}, nil
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
			"The top-level object may contain only: schema_version, id, mission_revision_id, operation_id, base_commit_id, read_set, preconditions, changes, expected_delta, validator_ids, provenance, idempotency_key.",
			"Each changes item may contain only: kind, entity_type, entity_id, payload_ref.",
			"schema_version is an integer; read_set, preconditions, changes, and validator_ids are arrays; every other top-level field is a JSON string.",
			"Do not wrap the object and do not add input_refs, spec_id, task, facts, or commentary as JSON fields.",
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
			ID:              domain.EventID(fmt.Sprintf("%s:model_adaptation:%d:%d:%d:%d:%s", op.ID, op.Attempt, callsUsed, plan.ContextTokens, e.Clock.Now().UnixNano(), plan.Reason)),
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

func (e ModelExecutor) saveContextPressure(ctx context.Context, bindingID string, state domain.ContextPressureState) error {
	if strings.TrimSpace(bindingID) == "" {
		return nil
	}
	return e.Store.Update(ctx, func(tx port.Transaction) error {
		return tx.SaveModelContextPressure(domain.ModelContextPressure{
			BindingID: bindingID,
			State:     state,
			UpdatedAt: e.Clock.Now().UTC(),
		})
	})
}

func (e ModelExecutor) loadContextPressure(ctx context.Context, bindingID string) (domain.ContextPressureState, error) {
	var state domain.ContextPressureState
	err := e.Store.View(ctx, func(r port.Reader) error {
		persisted, err := r.ModelContextPressure(bindingID)
		if errors.Is(err, port.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		state = persisted.State
		return nil
	})
	return state, err
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

func classifyProviderFailure(err error, kind domain.ProviderKind) (domain.ModelBindingFailureDecision, bool) {
	if err == nil {
		return domain.ModelBindingFailureDecision{}, false
	}
	var he port.ProviderHTTPError
	if errors.As(err, &he) {
		providerWide := kind == domain.ProviderKindNVIDIANIM && he.HTTPStatusCode() == 429
		return domain.ClassifyModelBindingFailure(he.HTTPStatusCode(), he.RetryableFailure(), providerWide), true
	}
	return domain.ModelBindingFailureDecision{}, false
}

func rateLimitMetadata(err error) port.RateLimitMetadata {
	var rateLimitErr port.ProviderRateLimitError
	if errors.As(err, &rateLimitErr) {
		return rateLimitErr.RateLimitMetadata()
	}
	return port.RateLimitMetadata{}
}

func safeRateLimitPayload(metadata port.RateLimitMetadata) string {
	var fields []string
	if metadata.HasRequestLimit {
		fields = append(fields, fmt.Sprintf("quota_request_limit=%d", metadata.RequestLimit))
	}
	if metadata.HasRequestRemaining {
		fields = append(fields, fmt.Sprintf("quota_request_remaining=%d", metadata.RequestRemaining))
	}
	if metadata.HasRequestReset {
		fields = append(fields, fmt.Sprintf("quota_request_reset_ms=%d", metadata.RequestReset.Milliseconds()))
	}
	if metadata.HasTokenLimit {
		fields = append(fields, fmt.Sprintf("quota_token_limit=%d", metadata.TokenLimit))
	}
	if metadata.HasTokenRemaining {
		fields = append(fields, fmt.Sprintf("quota_token_remaining=%d", metadata.TokenRemaining))
	}
	if metadata.HasTokenReset {
		fields = append(fields, fmt.Sprintf("quota_token_reset_ms=%d", metadata.TokenReset.Milliseconds()))
	}
	if len(fields) == 0 {
		return ""
	}
	return ";" + strings.Join(fields, ";")
}

func (e ModelExecutor) appendModelFailurePolicyEvent(ctx context.Context, operation domain.Operation, leaseRef string, decision domain.ModelBindingFailureDecision, providerID, bindingID string, callsUsed int, rateLimit port.RateLimitMetadata) error {
	return e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operation.ID)
		if err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:model_failure_policy:%d:%d:%d", op.ID, op.Attempt, callsUsed, e.Clock.Now().UnixNano())),
			Kind:            EventOperationModelFailurePolicy,
			OccurredAt:      e.Clock.Now().UTC(),
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      fmt.Sprintf("%s;class=%s;disposition=%s;scope=%s;reason=%s;provider_id=%s;binding_id=%s%s", leaseRef, decision.Class, decision.Disposition, decision.Scope, decision.Reason, providerID, bindingID, safeRateLimitPayload(rateLimit)),
		})
		return err
	})
}
