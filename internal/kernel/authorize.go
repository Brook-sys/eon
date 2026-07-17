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

// ReserveModelComplete authorizes model.complete for a READY operation and, when
// denied by the gate with a capacity wait, transitions the operation out of READY
// via ThrottleTransitionInput (WAIT_UNTIL or THROTTLE) without dispatching a lease.
//
// On ALLOW it persists projected ResourceUsage (in-flight) so concurrent admits
// see the reservation. Call ReportModelComplete after the Complete attempt to
// release the slot (success or failure).
//
// When authorizer is nil, ReserveModelComplete returns Allowed=true with no I/O
// so unit tests without resource wiring keep working (opt-in enforcement).
func (a *CapabilityAuthorizer) ReserveModelComplete(
	ctx context.Context,
	operation domain.Operation,
	spec domain.OperationSpec,
	priority int,
) (AuthorizationOutcome, error) {
	if a == nil {
		return AuthorizationOutcome{Allowed: true, SkipReason: "authorizer_disabled"}, nil
	}
	if a.Store == nil || a.Clock == nil {
		return AuthorizationOutcome{}, errors.New("capability authorizer requires store and clock")
	}
	if err := operation.Validate(); err != nil {
		return AuthorizationOutcome{}, fmt.Errorf("operation: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return AuthorizationOutcome{}, fmt.Errorf("operation spec: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := a.Clock.Now().UTC()
	estimated := ModelCompleteEstimatedBudget(spec)
	// AvailableBudget is the operation's remaining allowance; for model path the
	// full spec budget is the ceiling (attempt-local reserve of one call).
	available := spec.Budget
	if available.ModelCalls <= 0 {
		// Zero model_calls means no Complete authorized — pure budget deny.
		return a.denyBudget(ctx, operation, estimated, now, "operation model_calls budget is zero")
	}

	req := domain.AuthorizationRequest{
		Capability:         "model.complete",
		Version:            0,
		ArgsDigest:         "model.complete:" + string(operation.SpecID),
		OperationID:        operation.ID,
		MissionRevision:    operation.MissionRevision,
		EstimatedCost:      estimated,
		GrantedPermissions: a.GrantedPermissions,
		AvailableBudget:    available,
		Now:                now,
		AuthorizationTTL:   a.AuthorizationTTL,
	}
	decision, err := domain.EvaluateCapability(a.Catalog, req)
	if err != nil {
		return AuthorizationOutcome{}, err
	}
	out := AuthorizationOutcome{Decision: decision}
	if decision.Decision != domain.PolicyAllow {
		out.SkipReason = "policy_" + strings.ToLower(string(decision.Decision))
		if err := a.persistPolicyDeny(ctx, operation, decision, now); err != nil {
			return out, err
		}
		return out, nil
	}
	if !decision.UsableAt(now, operation.ID, operation.MissionRevision, req.ArgsDigest) {
		out.SkipReason = "policy_permit_unusable"
		return out, nil
	}

	resource := domain.ResourceID(decision.Resource)
	if resource == "" {
		resource = DefaultModelCompleteResource
	}
	limit, hasLimit := a.Limits[resource]
	if !hasLimit {
		// No configured gate for this resource: policy ALLOW is enough.
		out.Allowed = true
		if err := a.persistAuthorized(ctx, operation, decision, nil, now); err != nil {
			return out, err
		}
		return out, nil
	}

	cost := ModelCompleteCost(spec)
	var usage domain.ResourceUsage
	err = a.Store.View(ctx, func(r port.Reader) error {
		u, err := r.ResourceUsage(resource)
		if err != nil {
			if errors.Is(err, port.ErrNotFound) {
				usage = domain.ResourceUsage{Resource: resource}
				return nil
			}
			return err
		}
		usage = u
		return nil
	})
	if err != nil {
		return out, err
	}

	acquire, err := domain.Acquire(limit, usage, cost, priority, now)
	if err != nil {
		return out, err
	}
	out.Acquire = &acquire
	if !acquire.Allowed {
		out.Throttled = true
		out.SkipReason = "resource_" + strings.ToLower(acquire.FailureCode)
		if out.SkipReason == "resource_" {
			out.SkipReason = "resource_denied"
		}
		if err := a.applyThrottle(ctx, operation, resource, decision, acquire, now); err != nil {
			return out, err
		}
		return out, nil
	}

	// Persist projected usage (in-flight reservation) before effect.
	if err := a.Store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.SaveResourceUsage(acquire.Usage); err != nil {
			return err
		}
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:cap_auth:%s:%d", operation.ID, decision.CapabilityRef, now.UnixNano())),
			Kind:            EventCapabilityAuthorized,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      decision.CapabilityRef + ";resource=" + string(resource) + ";reason=" + decision.Reason,
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return out, err
	}
	out.Allowed = true
	out.Permit = acquire.Permit
	return out, nil
}

// ReportModelComplete releases the ResourceGate slot after Complete finishes.
// success=true clears the failure streak; success=false may open the circuit.
// retryAfter is optional (HTTP Retry-After). No-op when authorizer is nil or
// permit is nil (policy-only allow without a gate).
func (a *CapabilityAuthorizer) ReportModelComplete(
	ctx context.Context,
	operation domain.Operation,
	permit *domain.ResourcePermit,
	success bool,
	retryAfter *time.Time,
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
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:resource_released:%s:%d", operation.ID, resource, now.UnixNano())),
			Kind:            EventResourceReleased,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      string(resource) + ";outcome=" + outcome,
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
	estimated domain.Budget,
	now time.Time,
	reason string,
) (AuthorizationOutcome, error) {
	dec := domain.PolicyDecision{
		SchemaVersion:     domain.SchemaVersionV1,
		Decision:          domain.PolicyDeny,
		Reason:            reason,
		PolicyVersion:     a.catalogPolicyVersion(),
		Capability:        "model.complete",
		CapabilityVersion: 1,
		CapabilityRef:     "model.complete@1",
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
