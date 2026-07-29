package kernel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// Capability authorization event kinds (FR-RES-001). Model text never decides policy.
const (
	EventCapabilityAuthorized = "capability.authorized"
	EventCapabilityDenied     = "capability.denied"
	EventResourceThrottled    = "resource.throttled"
	EventResourceReleased     = "resource.released"
)

// DefaultModelCompleteResource is the ResourceGate key for model.complete MVP.
const DefaultModelCompleteResource = domain.ResourceID("model:default")

// CapabilityAuthorizer evaluates PolicyEngine + ResourceGate before an effect.
// Catalog and Limits are process-local configuration; usage is durable via Store.
// When Catalog is nil or empty evaluation is fail-closed (deny).
type CapabilityAuthorizer struct {
	Store   port.Store
	Clock   interface{ Now() time.Time }
	Catalog domain.CapabilityCatalog
	// Limits maps ResourceID → configured ceiling. Missing resource keys deny
	// only when the capability declares a Resource; otherwise gate is skipped.
	Limits map[domain.ResourceID]domain.ResourceLimit
	// GrantedPermissions is the operator/runtime permission set for this process.
	GrantedPermissions []string
	// AuthorizationTTL bounds ALLOW decision reuse (0 = single-use instant).
	AuthorizationTTL time.Duration
	// PolicyVersion labels decisions when Catalog.PolicyVersion is empty.
	PolicyVersion string
}

// AuthorizationOutcome is the result of Reserve before an external effect.
type AuthorizationOutcome struct {
	// Allowed is true only when policy ALLOW and (if gated) resource grant.
	Allowed bool
	// Decision is the pure PolicyEngine outcome (always set when evaluation ran).
	Decision domain.PolicyDecision
	// Permit is non-nil when the ResourceGate admitted cost.
	Permit *domain.ResourcePermit
	// Permits contains every ResourceGate reservation for a composite effect.
	// Permit remains the first entry for backwards-compatible single gates.
	Permits []*domain.ResourcePermit
	// Acquire is set when a gate check ran (including denials).
	Acquire *domain.ResourceAcquireResult
	// Throttled is true when the operation should leave READY without running.
	// The authorizer applies WAIT_UNTIL/THROTTLE and persists usage + events.
	Throttled bool
	// SkipReason is a stable short code for executor SkipReason fields.
	SkipReason string
	// Failure is set when a resource/budget denial produced a FailureRecord.
	Failure *domain.FailureRecord
}

// CapabilityReserveRequest is the generic FR-RES-001 reserve input for any
// catalog capability (model.complete, web.search, web.fetch, …).
type CapabilityReserveRequest struct {
	// Capability is the catalog name (e.g. "web.search", "model.complete").
	Capability string
	// Version is the capability version; 0 selects the latest installed.
	Version uint64
	// ArgsDigest binds the ALLOW decision to a validated args fingerprint.
	ArgsDigest string
	// Operation and Spec identify the READY work item being authorized.
	Operation domain.Operation
	Spec      domain.OperationSpec
	// EstimatedCost is the Budget slice reserved by EvaluateCapability.
	EstimatedCost domain.Budget
	// AvailableBudget is the remaining operation/mission allowance.
	// When zero-value and Spec is valid, Spec.Budget is used.
	AvailableBudget domain.Budget
	// ResourceCost is the ResourceGate cost. When zero and Capability is
	// model.complete, ModelCompleteCost(Spec) is used.
	ResourceCost domain.ResourceCost
	// DefaultResource is used when the decision Resource field is empty.
	DefaultResource domain.ResourceID
	// Resources, when non-empty, replaces the capability/default resource with
	// a composite set of gates acquired under one policy/budget decision.
	Resources []domain.ResourceID
	// Priority feeds ResourceGate reserved-for-critical slots.
	Priority int
}

// ModelCompleteCost derives ResourceCost from an OperationSpec budget for one Complete.
func ModelCompleteCost(spec domain.OperationSpec) domain.ResourceCost {
	tokens := spec.MaxOutputTokens
	if tokens <= 0 {
		tokens = 1
	}
	// Bound gate token accounting by remaining budget tokens when set.
	if spec.Budget.Tokens > 0 && tokens > spec.Budget.Tokens {
		tokens = spec.Budget.Tokens
	}
	return domain.ResourceCost{Slots: 1, Calls: 1, Tokens: tokens}
}

// ModelCompleteEstimatedBudget is the Budget slice reserved by EvaluateCapability.
func ModelCompleteEstimatedBudget(spec domain.OperationSpec) domain.Budget {
	calls := 1
	if spec.Budget.ModelCalls > 0 {
		// Reserve one call at a time; the executor still enforces total ModelCalls.
		calls = 1
	}
	return domain.Budget{
		ModelCalls: calls,
		Tokens:     spec.MaxOutputTokens,
		Attempts:   1,
	}
}

// WebSearchCost is the ResourceGate cost for one web.search call.
func WebSearchCost() domain.ResourceCost {
	return domain.ResourceCost{Slots: 1, Calls: 1}
}

// WebFetchCost is the ResourceGate cost for one web.fetch call.
// expectedBytes contributes to Bytes accounting when known; zero is valid.
func WebFetchCost(expectedBytes int64) domain.ResourceCost {
	if expectedBytes < 0 {
		expectedBytes = 0
	}
	return domain.ResourceCost{Slots: 1, Calls: 1, Bytes: expectedBytes}
}

// WebCapabilityEstimatedBudget is the Budget slice reserved for web search/fetch.
// Prefer Attempts (and Bytes when set on the operation budget).
func WebCapabilityEstimatedBudget(spec domain.OperationSpec) domain.Budget {
	attempts := 1
	if spec.Budget.Attempts > 0 {
		attempts = 1
	}
	var bytes int64
	if spec.Budget.Bytes > 0 {
		// Reserve a conservative slice; executor still enforces total Bytes.
		bytes = spec.Budget.Bytes
		if bytes > 1<<20 {
			bytes = 1 << 20
		}
	}
	return domain.Budget{
		Attempts: attempts,
		Bytes:    bytes,
	}
}

// ReserveCapability authorizes an arbitrary catalog capability for a READY
// operation and, when denied by the gate with a capacity wait, transitions the
// operation out of READY via ThrottleTransitionInput without dispatching a lease.
//
// On ALLOW it persists projected ResourceUsage (in-flight) so concurrent admits
// see the reservation. Call ReportCapability after the effect to release the slot.
//
// When authorizer is nil, ReserveCapability returns Allowed=true with no I/O
// so unit tests without resource wiring keep working (opt-in enforcement).
func (a *CapabilityAuthorizer) ReserveCapability(
	ctx context.Context,
	req CapabilityReserveRequest,
) (AuthorizationOutcome, error) {
	if a == nil {
		return AuthorizationOutcome{Allowed: true, SkipReason: "authorizer_disabled"}, nil
	}
	if a.Store == nil || a.Clock == nil {
		return AuthorizationOutcome{}, errors.New("capability authorizer requires store and clock")
	}
	if err := req.Operation.Validate(); err != nil {
		return AuthorizationOutcome{}, fmt.Errorf("operation: %w", err)
	}
	if err := req.Spec.Validate(); err != nil {
		return AuthorizationOutcome{}, fmt.Errorf("operation spec: %w", err)
	}
	capability := strings.TrimSpace(req.Capability)
	if capability == "" {
		return AuthorizationOutcome{}, errors.New("capability name is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := a.Clock.Now().UTC()

	estimated := req.EstimatedCost
	if estimated.IsZero() {
		// Callers should always set estimated cost; empty means no units.
		return a.denyBudget(ctx, req.Operation, capability, estimated, now, "estimated cost is zero")
	}
	available := req.AvailableBudget
	if available.IsZero() {
		available = req.Spec.Budget
	}
	// Capability-specific hard zeros (fail closed before PolicyEngine).
	if capability == "model.complete" && available.ModelCalls <= 0 {
		return a.denyBudget(ctx, req.Operation, capability, estimated, now, "operation model_calls budget is zero")
	}
	if (capability == "web.search" || capability == "web.fetch" ||
		capability == "file.discover" || capability == "file.read") &&
		available.Attempts <= 0 && available.IsZero() {
		return a.denyBudget(ctx, req.Operation, capability, estimated, now, "operation budget is zero")
	}

	argsDigest := strings.TrimSpace(req.ArgsDigest)
	if argsDigest == "" {
		argsDigest = capability + ":" + string(req.Spec.ID)
	}

	authReq := domain.AuthorizationRequest{
		Capability:         capability,
		Version:            req.Version,
		ArgsDigest:         argsDigest,
		OperationID:        req.Operation.ID,
		MissionRevision:    req.Operation.MissionRevision,
		EstimatedCost:      estimated,
		GrantedPermissions: a.GrantedPermissions,
		AvailableBudget:    available,
		Now:                now,
		AuthorizationTTL:   a.AuthorizationTTL,
	}
	decision, err := domain.EvaluateCapability(a.Catalog, authReq)
	if err != nil {
		return AuthorizationOutcome{}, err
	}
	out := AuthorizationOutcome{Decision: decision}
	if decision.Decision != domain.PolicyAllow {
		out.SkipReason = "policy_" + strings.ToLower(string(decision.Decision))
		if err := a.persistPolicyDeny(ctx, req.Operation, decision, now); err != nil {
			return out, err
		}
		return out, nil
	}
	if !decision.UsableAt(now, req.Operation.ID, req.Operation.MissionRevision, authReq.ArgsDigest) {
		out.SkipReason = "policy_permit_unusable"
		return out, nil
	}

	resource := domain.ResourceID(decision.Resource)
	if resource == "" {
		resource = req.DefaultResource
	}
	if resource == "" && capability == "model.complete" {
		resource = DefaultModelCompleteResource
	}
	resources := req.Resources
	if len(resources) == 0 && resource != "" {
		resources = []domain.ResourceID{resource}
	}
	if len(resources) == 0 {
		out.Allowed = true
		if err := a.persistAuthorized(ctx, req.Operation, decision, nil, now); err != nil {
			return out, err
		}
		return out, nil
	}
	limit, hasLimit := a.Limits[resources[0]]
	if !hasLimit || resource == "" {
		// No configured gate for this resource: policy ALLOW is enough.
		out.Allowed = true
		if err := a.persistAuthorized(ctx, req.Operation, decision, nil, now); err != nil {
			return out, err
		}
		return out, nil
	}

	cost := req.ResourceCost
	if cost.Slots == 0 && cost.Calls == 0 && cost.Tokens == 0 && cost.Bytes == 0 {
		if capability == "model.complete" {
			cost = ModelCompleteCost(req.Spec)
		} else {
			cost = domain.ResourceCost{Slots: 1, Calls: 1}
		}
	}

	acquires := make([]domain.ResourceAcquireResult, 0, len(resources))
	for _, gatedResource := range resources {
		limit, hasLimit = a.Limits[gatedResource]
		if !hasLimit {
			continue
		}
		var usage domain.ResourceUsage
		err = a.Store.View(ctx, func(r port.Reader) error {
			u, readErr := r.ResourceUsage(gatedResource)
			if errors.Is(readErr, port.ErrNotFound) {
				usage = domain.ResourceUsage{Resource: gatedResource}
				return nil
			}
			usage = u
			return readErr
		})
		if err != nil {
			return out, err
		}
		acquire, acquireErr := domain.Acquire(limit, usage, cost, req.Priority, now)
		if acquireErr != nil {
			return out, acquireErr
		}
		out.Acquire = &acquire
		if !acquire.Allowed {
			out.Throttled = true
			out.SkipReason = "resource_" + strings.ToLower(acquire.FailureCode)
			if out.SkipReason == "resource_" {
				out.SkipReason = "resource_denied"
			}
			// READY callers can be moved to WAIT_UNTIL. Model recovery attempts
			// authorize under an existing RUNNING/VERIFYING lease, where changing
			// state here would violate the lease transition contract.
			if req.Operation.State == domain.StateReady {
				if err := a.applyThrottle(ctx, req.Operation, gatedResource, decision, acquire, now); err != nil {
					return out, err
				}
			}
			return out, nil
		}
		acquires = append(acquires, acquire)
	}

	// Persist projected usage (in-flight reservation) before effect.
	if err := a.Store.Update(ctx, func(tx port.Transaction) error {
		for _, acquire := range acquires {
			if err := tx.SaveResourceUsage(acquire.Usage); err != nil {
				return err
			}
		}
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:cap_auth:%s:%s:%s:%d", req.Operation.ID, decision.CapabilityRef, resourceEventKey(resources), acquireEventKey(acquires), now.UnixNano())),
			Kind:            EventCapabilityAuthorized,
			OccurredAt:      now,
			MissionRevision: req.Operation.MissionRevision,
			InquiryID:       req.Operation.InquiryID,
			OperationID:     req.Operation.ID,
			PayloadRef:      decision.CapabilityRef + ";resources=" + joinResourceIDs(resources) + ";reason=" + decision.Reason,
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return out, err
	}
	out.Allowed = true
	for _, acquire := range acquires {
		out.Permits = append(out.Permits, acquire.Permit)
	}
	if len(out.Permits) > 0 {
		out.Permit = out.Permits[0]
	}
	return out, nil
}

func resourceEventKey(resources []domain.ResourceID) string {
	if len(resources) == 0 {
		return "ungated"
	}
	var b strings.Builder
	for i, resource := range resources {
		if i > 0 {
			b.WriteByte('+')
		}
		for _, r := range string(resource) {
			if r == ':' {
				b.WriteByte('_')
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func acquireEventKey(acquires []domain.ResourceAcquireResult) string {
	if len(acquires) == 0 {
		return "0"
	}
	var b strings.Builder
	for i, acquire := range acquires {
		if i > 0 {
			b.WriteByte('+')
		}
		fmt.Fprintf(&b, "%d", acquire.Usage.MinuteCount)
	}
	return b.String()
}

func joinResourceIDs(resources []domain.ResourceID) string {
	values := make([]string, len(resources))
	for i, resource := range resources {
		values[i] = string(resource)
	}
	return strings.Join(values, ",")
}

// ReportCapability releases the ResourceGate slot after an authorized effect.
// success=true clears the failure streak; success=false may open the circuit.
// retryAfter is optional (HTTP Retry-After). No-op when authorizer is nil or
// permit is nil (policy-only allow without a gate).
func (a *CapabilityAuthorizer) ReportCapability(
	ctx context.Context,
	operation domain.Operation,
	permit *domain.ResourcePermit,
	success bool,
	retryAfter *time.Time,
) error {
	return a.ReportCapabilityObserved(ctx, operation, permit, success, retryAfter, 0)
}

// ReportCapabilityObserved additionally reconciles a successful model call's
// provider-reported token total against the estimate charged on acquire.
func (a *CapabilityAuthorizer) ReportCapabilityObserved(
	ctx context.Context,
	operation domain.Operation,
	permit *domain.ResourcePermit,
	success bool,
	retryAfter *time.Time,
	observedTokens int,
) error {
	if a == nil || permit == nil {
		return nil
	}
	if a.Store == nil || a.Clock == nil {
		return errors.New("capability authorizer requires store and clock")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := a.Clock.Now().UTC()
	resource := permit.Resource
	limit, hasLimit := a.Limits[resource]
	if !hasLimit {
		// Usage may still exist; release in-flight with a synthetic limit.
		limit = domain.ResourceLimit{Resource: resource}
	}
	return a.Store.Update(ctx, func(tx port.Transaction) error {
		usage, err := tx.ResourceUsage(resource)
		if err != nil {
			if errors.Is(err, port.ErrNotFound) {
				// Nothing to release.
				return nil
			}
			return err
		}
		var next domain.ResourceUsage
		if success {
			next, err = domain.ReportSuccess(usage, permit.Cost, now)
			if err == nil && observedTokens > 0 {
				next, err = domain.ReconcileObservedTokensWithGrantedAt(next, permit.Cost.Tokens, observedTokens, permit.GrantedAt, now)
			}
		} else {
			next, err = domain.ReportFailure(usage, limit, permit.Cost, retryAfter, now)
		}
		if err != nil {
			return err
		}
		if err := tx.SaveResourceUsage(next); err != nil {
			return err
		}
		outcome := "success"
		if !success {
			outcome = "failure"
		}
		var err2 error
		_, err2 = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:resource_released:%s:%d:%d", operation.ID, resource, next.MinuteCount, now.UnixNano())),
			Kind:            EventResourceReleased,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      string(resource) + ";outcome=" + outcome,
		})
		return err2
	})
}

// ReserveModelComplete authorizes model.complete for a READY operation.
// Thin wrapper over ReserveCapability for call-site compatibility.
func (a *CapabilityAuthorizer) ReserveModelComplete(
	ctx context.Context,
	operation domain.Operation,
	spec domain.OperationSpec,
	priority int,
	providerID string,
	bindingID string,
) (AuthorizationOutcome, error) {
	var resources []domain.ResourceID
	if providerID != "" && bindingID != "" {
		resources = []domain.ResourceID{
			domain.ModelProviderResource(providerID),
			domain.ModelBindingResource(bindingID),
		}
	}
	return a.ReserveCapability(ctx, CapabilityReserveRequest{
		Capability:      "model.complete",
		ArgsDigest:      "model.complete:" + string(spec.ID),
		Operation:       operation,
		Spec:            spec,
		EstimatedCost:   ModelCompleteEstimatedBudget(spec),
		AvailableBudget: spec.Budget,
		ResourceCost:    ModelCompleteCost(spec),
		DefaultResource: DefaultModelCompleteResource,
		Resources:       resources,
		Priority:        priority,
	})
}

// ReportModelComplete releases the ResourceGate slot after Complete finishes.
// Thin wrapper over ReportCapability for call-site compatibility.
func (a *CapabilityAuthorizer) ReportModelComplete(
	ctx context.Context,
	operation domain.Operation,
	permits []*domain.ResourcePermit,
	success bool,
	retryAfter *time.Time,
) error {
	return a.ReportModelCompleteObserved(ctx, operation, permits, success, retryAfter, 0)
}

// ReportModelCompleteObserved releases all composite permits and reconciles
// the same observed token total into each independently limited bucket.
func (a *CapabilityAuthorizer) ReportModelCompleteObserved(
	ctx context.Context,
	operation domain.Operation,
	permits []*domain.ResourcePermit,
	success bool,
	retryAfter *time.Time,
	observedTokens int,
) error {
	return a.reportModelCompleteBatch(ctx, operation, permits, func(*domain.ResourcePermit) bool { return success }, retryAfter, observedTokens)
}

// SettleModelCompletionReceipt atomically applies the receipt's durable permit
// snapshot and marks it settled. Replays are no-ops, so normal execution and
// crash recovery share one receipt-keyed accounting boundary.
func (a *CapabilityAuthorizer) SettleModelCompletionReceipt(ctx context.Context, operation domain.Operation, receipt domain.ModelCompletionReceipt) error {
	if a == nil || len(receipt.Permits) == 0 {
		return nil
	}
	if a.Store == nil || a.Clock == nil {
		return errors.New("capability authorizer requires store and clock")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := a.Clock.Now().UTC()
	observed := receipt.Result.InputTokens + receipt.Result.OutputTokens
	return a.Store.Update(ctx, func(tx port.Transaction) error {
		current, err := tx.ModelCompletionReceipt(receipt.OperationID, receipt.Attempt, receipt.ModelCall)
		if err != nil {
			return err
		}
		if current.SettledAt != nil {
			return nil
		}
		resources := make([]domain.ResourceID, 0, len(current.Permits))
		for i := range current.Permits {
			permit := &current.Permits[i]
			usage, err := tx.ResourceUsage(permit.Resource)
			if errors.Is(err, port.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			next, err := domain.ReportSuccess(usage, permit.Cost, now)
			if err == nil && observed > 0 {
				next, err = domain.ReconcileObservedTokensWithGrantedAt(next, permit.Cost.Tokens, observed, permit.GrantedAt, now)
			}
			if err != nil {
				return err
			}
			if err := tx.SaveResourceUsage(next); err != nil {
				return err
			}
			resources = append(resources, permit.Resource)
		}
		if err := tx.MarkModelCompletionReceiptSettled(current.OperationID, current.Attempt, current.ModelCall, now); err != nil {
			return err
		}
		if len(resources) == 0 {
			return nil
		}
		_, err = tx.AppendEvent(domain.Event{SchemaVersion: domain.SchemaVersionV1, ID: domain.EventID(fmt.Sprintf("%s:resource_released:receipt:%d:%d", operation.ID, current.Attempt, current.ModelCall)), Kind: EventResourceReleased, OccurredAt: now, MissionRevision: operation.MissionRevision, InquiryID: operation.InquiryID, OperationID: operation.ID, PayloadRef: joinResourceIDs(resources) + ";outcome=success"})
		return err
	})
}

// ReconcileModelCompletionReceipts settles at most limit receipts.
func (a *CapabilityAuthorizer) ReconcileModelCompletionReceipts(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	var receipts []domain.ModelCompletionReceipt
	if err := a.Store.View(ctx, func(r port.Reader) error {
		var err error
		receipts, err = r.UnsettledModelCompletionReceipts(limit)
		return err
	}); err != nil {
		return 0, err
	}
	settled := 0
	for _, receipt := range receipts {
		var op domain.Operation
		if err := a.Store.View(ctx, func(r port.Reader) error { var err error; op, err = r.Operation(receipt.OperationID); return err }); err != nil {
			return settled, err
		}
		if err := a.SettleModelCompletionReceipt(context.WithoutCancel(ctx), op, receipt); err != nil {
			return settled, err
		}
		settled++
	}
	return settled, nil
}

// ReportModelCompleteScopedFailure atomically releases every composite model
// permit while applying the failure only to the classified provider/binding
// scope. A model attempt therefore produces one durable release event and one
// store transaction instead of one of each per ResourceGate bucket.
func (a *CapabilityAuthorizer) ReportModelCompleteScopedFailure(
	ctx context.Context,
	operation domain.Operation,
	permits []*domain.ResourcePermit,
	scope string,
	retryAfter *time.Time,
) error {
	return a.reportModelCompleteBatch(ctx, operation, permits, func(permit *domain.ResourcePermit) bool {
		switch scope {
		case "provider":
			return !strings.HasPrefix(string(permit.Resource), "model-provider:") && !strings.HasPrefix(string(permit.Resource), "model:provider:")
		case "binding":
			return !strings.HasPrefix(string(permit.Resource), "model-binding:") && !strings.HasPrefix(string(permit.Resource), "model:binding:")
		default:
			return false
		}
	}, retryAfter, 0)
}

func (a *CapabilityAuthorizer) reportModelCompleteBatch(
	ctx context.Context,
	operation domain.Operation,
	permits []*domain.ResourcePermit,
	succeeded func(*domain.ResourcePermit) bool,
	retryAfter *time.Time,
	observedTokens int,
) error {
	if a == nil || len(permits) == 0 {
		return nil
	}
	if a.Store == nil || a.Clock == nil {
		return errors.New("capability authorizer requires store and clock")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := a.Clock.Now().UTC()
	return a.Store.Update(ctx, func(tx port.Transaction) error {
		outcomes := make([]string, 0, len(permits))
		resources := make([]domain.ResourceID, 0, len(permits))
		for _, permit := range permits {
			if permit == nil {
				continue
			}
			resource := permit.Resource
			usage, err := tx.ResourceUsage(resource)
			if errors.Is(err, port.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			limit, hasLimit := a.Limits[resource]
			if !hasLimit {
				limit = domain.ResourceLimit{Resource: resource}
			}
			success := succeeded(permit)
			var next domain.ResourceUsage
			if success {
				next, err = domain.ReportSuccess(usage, permit.Cost, now)
				if err == nil && observedTokens > 0 {
					next, err = domain.ReconcileObservedTokensWithGrantedAt(next, permit.Cost.Tokens, observedTokens, permit.GrantedAt, now)
				}
			} else {
				next, err = domain.ReportFailure(usage, limit, permit.Cost, retryAfter, now)
			}
			if err != nil {
				return err
			}
			if err := tx.SaveResourceUsage(next); err != nil {
				return err
			}
			outcome := "failure"
			if success {
				outcome = "success"
			}
			resources = append(resources, resource)
			outcomes = append(outcomes, string(resource)+"="+outcome)
		}
		if len(resources) == 0 {
			return nil
		}
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:resource_released:%s:%d", operation.ID, resourceEventKey(resources), now.UnixNano())),
			Kind:            EventResourceReleased,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      strings.Join(outcomes, ","),
		})
		return err
	})
}

// DefaultMVPLimits returns conservative ResourceGate ceilings for MVP resources.
func DefaultMVPLimits() map[domain.ResourceID]domain.ResourceLimit {
	return map[domain.ResourceID]domain.ResourceLimit{
		"model:default": {
			Resource:            "model:default",
			MaxConcurrent:       2,
			MaxPerMinute:        30,
			MaxPerDay:           500,
			MaxTokensPerMinute:  200_000,
			FailureThreshold:    5,
			CooldownBase:        2 * time.Second,
			CooldownMax:         2 * time.Minute,
			ReservedForCritical: 0,
		},
		"web:searxng": {
			Resource:         "web:searxng",
			MaxConcurrent:    4,
			MaxPerMinute:     60,
			MaxPerDay:        2000,
			FailureThreshold: 3,
			CooldownBase:     2 * time.Second,
			CooldownMax:      time.Minute,
		},
		"web:http": {
			Resource:         "web:http",
			MaxConcurrent:    4,
			MaxPerMinute:     60,
			MaxPerDay:        2000,
			FailureThreshold: 3,
			CooldownBase:     2 * time.Second,
			CooldownMax:      time.Minute,
		},
		"file:authorized-root": {
			Resource:      "file:authorized-root",
			MaxConcurrent: 8,
		},
		"store:knowledge": {
			Resource:      "store:knowledge",
			MaxConcurrent: 16,
		},
		"store:artifact": {
			Resource:      "store:artifact",
			MaxConcurrent: 16,
		},
	}
}

// NewMVPCapabilityAuthorizer builds a process-local authorizer with MVP catalog
// and default resource limits. Callers must still inject Store and Clock.
func NewMVPCapabilityAuthorizer(store port.Store, clock interface{ Now() time.Time }, policyVersion string) (*CapabilityAuthorizer, error) {
	if strings.TrimSpace(policyVersion) == "" {
		policyVersion = "policy@mvp-1"
	}
	cat, err := domain.NewCapabilityCatalog(policyVersion, domain.MVPCapabilityDescriptors())
	if err != nil {
		return nil, err
	}
	return &CapabilityAuthorizer{
		Store:   store,
		Clock:   clock,
		Catalog: cat,
		Limits:  DefaultMVPLimits(),
		GrantedPermissions: []string{
			"file:authorized-root",
			"web:search",
			"web:fetch",
			"source:snapshot",
			"model:complete",
			"artifact:render",
		},
		AuthorizationTTL: 0,
		PolicyVersion:    policyVersion,
	}, nil
}

func (a *CapabilityAuthorizer) denyBudget(
	ctx context.Context,
	operation domain.Operation,
	capability string,
	estimated domain.Budget,
	now time.Time,
	reason string,
) (AuthorizationOutcome, error) {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		capability = "unknown"
	}
	version := uint64(1)
	if a != nil {
		if desc, ok := a.Catalog.Latest(capability); ok {
			version = desc.Version
		}
	}
	dec := domain.PolicyDecision{
		SchemaVersion:     domain.SchemaVersionV1,
		Decision:          domain.PolicyDeny,
		Reason:            reason,
		PolicyVersion:     a.catalogPolicyVersion(),
		Capability:        capability,
		CapabilityVersion: version,
		CapabilityRef:     fmt.Sprintf("%s@%d", capability, version),
		OperationID:       operation.ID,
		MissionRevision:   operation.MissionRevision,
		IssuedAt:          now,
		ExpiresAt:         now,
		ReservedCost:      domain.Budget{},
	}
	out := AuthorizationOutcome{Decision: dec, SkipReason: "policy_deny"}
	if err := a.persistPolicyDeny(ctx, operation, dec, now); err != nil {
		return out, err
	}
	_ = estimated
	return out, nil
}

func (a *CapabilityAuthorizer) persistPolicyDeny(
	ctx context.Context,
	operation domain.Operation,
	decision domain.PolicyDecision,
	now time.Time,
) error {
	return a.Store.Update(ctx, func(tx port.Transaction) error {
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:cap_deny:%s:%d", operation.ID, decision.Capability, now.UnixNano())),
			Kind:            EventCapabilityDenied,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      decision.CapabilityRef + ";decision=" + string(decision.Decision) + ";reason=" + decision.Reason,
		})
		return err
	})
}

func (a *CapabilityAuthorizer) persistAuthorized(
	ctx context.Context,
	operation domain.Operation,
	decision domain.PolicyDecision,
	permit *domain.ResourcePermit,
	now time.Time,
) error {
	ref := decision.CapabilityRef + ";reason=" + decision.Reason
	if permit != nil {
		ref += ";resource=" + string(permit.Resource)
	}
	return a.Store.Update(ctx, func(tx port.Transaction) error {
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:cap_auth:%s:%d", operation.ID, decision.CapabilityRef, now.UnixNano())),
			Kind:            EventCapabilityAuthorized,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      ref,
		})
		return err
	})
}

func (a *CapabilityAuthorizer) applyThrottle(
	ctx context.Context,
	operation domain.Operation,
	resource domain.ResourceID,
	decision domain.PolicyDecision,
	acquire domain.ResourceAcquireResult,
	now time.Time,
) error {
	input, err := domain.ThrottleTransitionInput(acquire, resource)
	if err != nil {
		return err
	}
	return a.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operation.ID)
		if err != nil {
			return err
		}
		if op.State != domain.StateReady {
			// Concurrent dispatch won; do not force throttle over RUNNING.
			return nil
		}
		snap := domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation}
		next, err := domain.Transition(snap, input)
		if err != nil {
			return fmt.Errorf("throttle transition: %w", err)
		}
		op.State = next.State
		op.Reevaluation = next.Reevaluation
		if err := tx.SaveOperation(op); err != nil {
			return err
		}
		// Persist usage as observed (no in-flight increase on denial).
		if err := tx.SaveResourceUsage(acquire.Usage); err != nil {
			return err
		}
		payload := string(resource) + ";code=" + acquire.FailureCode + ";reason=" + acquire.Reason
		if acquire.WaitUntil != nil {
			payload += ";wait_until=" + acquire.WaitUntil.UTC().Format(time.RFC3339Nano)
		}
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:resource_throttled:%d", op.ID, now.UnixNano())),
			Kind:            EventResourceThrottled,
			OccurredAt:      now,
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      payload,
		}); err != nil {
			return err
		}
		// Audit policy ALLOW that still could not run (gate denial).
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:cap_auth_throttled:%s:%d", op.ID, decision.CapabilityRef, now.UnixNano())),
			Kind:            EventCapabilityDenied,
			OccurredAt:      now,
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      decision.CapabilityRef + ";decision=THROTTLED;reason=" + acquire.Reason,
		})
		return err
	})
}

func (a *CapabilityAuthorizer) catalogPolicyVersion() string {
	if a != nil && a.Catalog.PolicyVersion != "" {
		return a.Catalog.PolicyVersion
	}
	if a != nil && a.PolicyVersion != "" {
		return a.PolicyVersion
	}
	return "policy@unknown"
}
