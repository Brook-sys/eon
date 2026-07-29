package domain

import (
	"errors"
	"fmt"
	"time"
)

// ResourceGate pure contracts (ARCHITECTURE §Tempo, rate limits e backpressure,
// FR-RES-001). acquire → Permit | WaitUntil; report updates usage without I/O.

// ResourceID identifies a limited resource (model:…, web:…, file:…).
type ResourceID string

// ResourceCost is the estimated units for one acquire attempt.
type ResourceCost struct {
	// Slots is concurrent occupancy requested (usually 1).
	Slots int `json:"slots"`
	// Calls counts against per-minute / per-day call quotas.
	Calls int `json:"calls"`
	// Tokens counts against token-per-minute quotas (model path).
	Tokens int `json:"tokens"`
	// Bytes is reserved for bandwidth-limited resources.
	Bytes int64 `json:"bytes"`
}

func (c ResourceCost) Validate() error {
	if c.Slots < 0 || c.Calls < 0 || c.Tokens < 0 || c.Bytes < 0 {
		return errors.New("resource cost values must not be negative")
	}
	if c.Slots == 0 && c.Calls == 0 && c.Tokens == 0 && c.Bytes == 0 {
		return errors.New("resource cost must request at least one unit")
	}
	return nil
}

// ResourceLimit is the configured ceiling for one ResourceID.
// Zero on a dimension means that dimension is not enforced (unlike Budget).
// Circuit/cooldown fields drive fail-closed backpressure after errors.
type ResourceLimit struct {
	Resource           ResourceID    `json:"resource"`
	MaxConcurrent      int           `json:"max_concurrent"`
	MaxPerMinute       int           `json:"max_per_minute"`
	MaxPerDay          int           `json:"max_per_day"`
	MaxTokensPerMinute int           `json:"max_tokens_per_minute"`
	FailureThreshold   int           `json:"failure_threshold"`
	CooldownBase       time.Duration `json:"cooldown_base"`
	CooldownMax        time.Duration `json:"cooldown_max"`
	// ReservedForCritical is concurrency held back from normal priority.
	ReservedForCritical int `json:"reserved_for_critical"`
}

func (l ResourceLimit) Validate() error {
	if l.Resource == "" {
		return errors.New("resource id is required")
	}
	if l.MaxConcurrent < 0 || l.MaxPerMinute < 0 || l.MaxPerDay < 0 ||
		l.MaxTokensPerMinute < 0 || l.FailureThreshold < 0 ||
		l.ReservedForCritical < 0 || l.CooldownBase < 0 || l.CooldownMax < 0 {
		return errors.New("resource limit values must not be negative")
	}
	if l.ReservedForCritical > 0 && l.MaxConcurrent > 0 && l.ReservedForCritical >= l.MaxConcurrent {
		return errors.New("reserved_for_critical must be less than max_concurrent")
	}
	if l.CooldownMax > 0 && l.CooldownBase > l.CooldownMax {
		return errors.New("cooldown_base must not exceed cooldown_max")
	}
	return nil
}

// ResourceUsage is the persistable gate state for one resource.
// Windows are wall-clock buckets truncated by the pure helper functions.
type ResourceUsage struct {
	Resource               ResourceID `json:"resource"`
	InFlight               int        `json:"in_flight"`
	MinuteWindowStart      time.Time  `json:"minute_window_start,omitempty"`
	MinuteCount            int        `json:"minute_count"`
	DayWindowStart         time.Time  `json:"day_window_start,omitempty"`
	DayCount               int        `json:"day_count"`
	TokenMinuteWindowStart time.Time  `json:"token_minute_window_start,omitempty"`
	TokenMinuteCount       int        `json:"token_minute_count"`
	ConsecutiveFailures    int        `json:"consecutive_failures"`
	CircuitOpenUntil       *time.Time `json:"circuit_open_until,omitempty"`
	LastFailureAt          *time.Time `json:"last_failure_at,omitempty"`
}

func (u ResourceUsage) Validate() error {
	if u.Resource == "" {
		return errors.New("usage resource id is required")
	}
	if u.InFlight < 0 || u.MinuteCount < 0 || u.DayCount < 0 ||
		u.TokenMinuteCount < 0 || u.ConsecutiveFailures < 0 {
		return errors.New("usage counters must not be negative")
	}
	return nil
}

// ResourcePermit is a short-lived grant returned by a successful acquire.
// It is not an authority expansion — only proof that the gate admitted cost.
type ResourcePermit struct {
	Resource  ResourceID   `json:"resource"`
	Cost      ResourceCost `json:"cost"`
	GrantedAt time.Time    `json:"granted_at"`
	// Priority at grant time (critical bypasses reserved slots).
	Priority int `json:"priority"`
}

// ResourceAcquireResult is Permit | WaitUntil with a stable machine reason.
type ResourceAcquireResult struct {
	Allowed   bool            `json:"allowed"`
	Permit    *ResourcePermit `json:"permit,omitempty"`
	WaitUntil *time.Time      `json:"wait_until,omitempty"`
	// Reason is stable, short, and safe for events (no provider bodies).
	Reason string `json:"reason"`
	// FailureCode aligns with FAILURE_TAXONOMY resource/dependency codes.
	FailureCode string `json:"failure_code,omitempty"`
	// Usage is the projected usage if Allowed (in-flight + window counts).
	// Callers persist Usage only after accepting the permit.
	Usage ResourceUsage `json:"usage"`
}

// PriorityCritical may use reserved concurrency slots.
const PriorityCritical = 100

// Acquire evaluates whether cost may proceed now. Pure: no sleep, no I/O.
// On denial, WaitUntil is set when a concrete instant is known (window end,
// circuit open, cooldown); nil WaitUntil means "not now, re-check next cycle".
func Acquire(limit ResourceLimit, usage ResourceUsage, cost ResourceCost, priority int, now time.Time) (ResourceAcquireResult, error) {
	if err := limit.Validate(); err != nil {
		return ResourceAcquireResult{}, err
	}
	if usage.Resource != "" && usage.Resource != limit.Resource {
		return ResourceAcquireResult{}, fmt.Errorf("usage resource %q does not match limit %q", usage.Resource, limit.Resource)
	}
	if err := cost.Validate(); err != nil {
		return ResourceAcquireResult{}, err
	}
	if now.IsZero() {
		return ResourceAcquireResult{}, errors.New("acquire requires now")
	}
	now = now.UTC()
	usage = normalizeUsageWindows(usage, limit.Resource, now)

	if usage.CircuitOpenUntil != nil && now.Before(*usage.CircuitOpenUntil) {
		until := usage.CircuitOpenUntil.UTC()
		return ResourceAcquireResult{
			Allowed:     false,
			WaitUntil:   &until,
			Reason:      "circuit open",
			FailureCode: "DEPENDENCY_CIRCUIT_OPEN",
			Usage:       usage,
		}, nil
	}

	// Concurrency
	if limit.MaxConcurrent > 0 {
		available := limit.MaxConcurrent - usage.InFlight
		if priority < PriorityCritical {
			available -= limit.ReservedForCritical
		}
		need := cost.Slots
		if need == 0 {
			need = 1
		}
		if available < need {
			return ResourceAcquireResult{
				Allowed:     false,
				Reason:      "concurrency saturated",
				FailureCode: "RESOURCE_CONCURRENCY",
				Usage:       usage,
			}, nil
		}
	}

	// Per-minute calls
	if limit.MaxPerMinute > 0 && cost.Calls > 0 {
		if usage.MinuteCount+cost.Calls > limit.MaxPerMinute {
			until := usage.MinuteWindowStart.Add(time.Minute)
			return ResourceAcquireResult{
				Allowed:     false,
				WaitUntil:   &until,
				Reason:      "per-minute quota exhausted",
				FailureCode: "RESOURCE_RATE_LIMIT",
				Usage:       usage,
			}, nil
		}
	}

	// Per-day calls
	if limit.MaxPerDay > 0 && cost.Calls > 0 {
		if usage.DayCount+cost.Calls > limit.MaxPerDay {
			until := usage.DayWindowStart.Add(24 * time.Hour)
			return ResourceAcquireResult{
				Allowed:     false,
				WaitUntil:   &until,
				Reason:      "daily quota exhausted",
				FailureCode: "RESOURCE_DAILY_QUOTA",
				Usage:       usage,
			}, nil
		}
	}

	// Token per-minute
	if limit.MaxTokensPerMinute > 0 && cost.Tokens > 0 {
		if usage.TokenMinuteCount+cost.Tokens > limit.MaxTokensPerMinute {
			until := usage.TokenMinuteWindowStart.Add(time.Minute)
			return ResourceAcquireResult{
				Allowed:     false,
				WaitUntil:   &until,
				Reason:      "token per-minute quota exhausted",
				FailureCode: "RESOURCE_TOKEN_RATE",
				Usage:       usage,
			}, nil
		}
	}

	// Admit: project usage
	slots := cost.Slots
	if slots == 0 {
		slots = 1
	}
	next := usage
	next.InFlight += slots
	if cost.Calls > 0 {
		next.MinuteCount += cost.Calls
		next.DayCount += cost.Calls
	}
	if cost.Tokens > 0 {
		next.TokenMinuteCount += cost.Tokens
	}
	permit := &ResourcePermit{
		Resource:  limit.Resource,
		Cost:      cost,
		GrantedAt: now,
		Priority:  priority,
	}
	return ResourceAcquireResult{
		Allowed: true,
		Permit:  permit,
		Reason:  "granted",
		Usage:   next,
	}, nil
}

// ReportSuccess releases in-flight slots and clears the failure streak.
// It does not refund quota counters (calls/tokens already spent).
func ReportSuccess(usage ResourceUsage, cost ResourceCost, now time.Time) (ResourceUsage, error) {
	if err := usage.Validate(); err != nil {
		return usage, err
	}
	if now.IsZero() {
		return usage, errors.New("report requires now")
	}
	slots := cost.Slots
	if slots == 0 {
		slots = 1
	}
	next := usage
	next.InFlight = saturatingSubInt(next.InFlight, slots)
	next.ConsecutiveFailures = 0
	next.CircuitOpenUntil = nil
	return next, nil
}

// ReconcileObservedTokens replaces the estimated token charge made at
// acquire time with the provider-observed total for the same successful call.
// It delegates to ReconcileObservedTokensWithGrantedAt with a zero grantedAt.
func ReconcileObservedTokens(usage ResourceUsage, estimated, observed int, now time.Time) (ResourceUsage, error) {
	return ReconcileObservedTokensWithGrantedAt(usage, estimated, observed, time.Time{}, now)
}

// ReconcileObservedTokensWithGrantedAt replaces estimated token charges with
// provider-observed totals, handling window boundary crossings deterministically.
// If grantedAt is non-zero and falls in a previous minute window than now, estimated
// tokens were accounted for in that previous window and are not subtracted from the
// current window; only observed tokens exceeding estimated are charged to now.
func ReconcileObservedTokensWithGrantedAt(usage ResourceUsage, estimated, observed int, grantedAt, now time.Time) (ResourceUsage, error) {
	if err := usage.Validate(); err != nil {
		return usage, err
	}
	if estimated < 0 || observed < 0 {
		return usage, errors.New("token reconciliation values must not be negative")
	}
	if now.IsZero() {
		return usage, errors.New("token reconciliation requires now")
	}
	if observed == 0 {
		return usage, nil
	}
	now = now.UTC()
	next := normalizeUsageWindows(usage, usage.Resource, now)

	if !grantedAt.IsZero() {
		grantedMinute := grantedAt.UTC().Truncate(time.Minute)
		nowMinute := now.Truncate(time.Minute)
		if grantedMinute.Equal(nowMinute) {
			if estimated == observed {
				return next, nil
			}
			next.TokenMinuteCount = saturatingSubInt(next.TokenMinuteCount, estimated)
			next.TokenMinuteCount += observed
			return next, nil
		}
		// Cross-minute boundary: estimated tokens were accounted for in the
		// granted minute window. Only charge excess observed tokens to current window.
		excess := saturatingSubInt(observed, estimated)
		if excess > 0 {
			next.TokenMinuteCount += excess
		}
		return next, nil
	}

	activeWindow := !usage.TokenMinuteWindowStart.IsZero() && usage.TokenMinuteWindowStart.Equal(now.Truncate(time.Minute))
	if usage.TokenMinuteWindowStart.IsZero() && estimated == observed {
		// Preserve the legacy/no-window no-op contract: without a persisted
		// bucket there is no evidence that reconciliation crossed a boundary.
		return usage, nil
	}
	if activeWindow {
		if estimated == observed {
			return next, nil
		}
		next.TokenMinuteCount = saturatingSubInt(next.TokenMinuteCount, estimated)
	}
	next.TokenMinuteCount += observed
	return next, nil
}

// ReportFailure releases in-flight slots, increments the failure streak, and
// may open the circuit. retryAfter, when set (e.g. HTTP Retry-After), is the
// earliest reopen instant and wins over computed cooldown if later.
func ReportFailure(usage ResourceUsage, limit ResourceLimit, cost ResourceCost, retryAfter *time.Time, now time.Time) (ResourceUsage, error) {
	if err := usage.Validate(); err != nil {
		return usage, err
	}
	if err := limit.Validate(); err != nil {
		return usage, err
	}
	if usage.Resource != "" && usage.Resource != limit.Resource {
		return usage, fmt.Errorf("usage resource %q does not match limit %q", usage.Resource, limit.Resource)
	}
	if now.IsZero() {
		return usage, errors.New("report requires now")
	}
	now = now.UTC()
	slots := cost.Slots
	if slots == 0 {
		slots = 1
	}
	next := usage
	next.Resource = limit.Resource
	next.InFlight = saturatingSubInt(next.InFlight, slots)
	next.ConsecutiveFailures++
	t := now
	next.LastFailureAt = &t

	var openUntil time.Time
	// Reporting another failure must never shorten an already active circuit.
	// Keep the latest authenticated/persisted lower bound, then extend it when
	// the computed cooldown or provider Retry-After is later.
	if next.CircuitOpenUntil != nil && next.CircuitOpenUntil.After(now) {
		openUntil = next.CircuitOpenUntil.UTC()
	}
	if limit.FailureThreshold > 0 && next.ConsecutiveFailures >= limit.FailureThreshold {
		computed := now.Add(cooldownFor(next.ConsecutiveFailures, limit))
		if openUntil.IsZero() || computed.After(openUntil) {
			openUntil = computed
		}
	}
	if retryAfter != nil && !retryAfter.IsZero() {
		ra := retryAfter.UTC()
		if ra.After(now) && (openUntil.IsZero() || ra.After(openUntil)) {
			openUntil = ra
		}
	}
	if !openUntil.IsZero() {
		next.CircuitOpenUntil = &openUntil
	}
	return next, nil
}

// ThrottleTransitionInput builds a pure TransitionInput for capacity waits.
// Prefer WAIT_UNTIL when WaitUntil is known so not_before is persisted;
// otherwise THROTTLE with a capacity reference (ResourceID + reason).
func ThrottleTransitionInput(result ResourceAcquireResult, resource ResourceID) (TransitionInput, error) {
	if result.Allowed {
		return TransitionInput{}, errors.New("throttle input requires a denied acquire")
	}
	ref := string(resource)
	if result.FailureCode != "" {
		ref = ref + ":" + result.FailureCode
	}
	if result.WaitUntil != nil && !result.WaitUntil.IsZero() {
		until := result.WaitUntil.UTC()
		return TransitionInput{
			Event:     EventWaitUntil,
			NotBefore: &until,
			Reference: ref,
		}, nil
	}
	return TransitionInput{
		Event:     EventThrottle,
		Reference: ref,
	}, nil
}

// NewResourceBudgetFailure builds a FailureRecord for gate denial (class RESOURCE
// or DEPENDENCY for circuit/rate from provider).
func NewResourceBudgetFailure(
	id FailureID,
	operation Operation,
	attempt uint32,
	code string,
	class FailureClass,
	disposition RetryDisposition,
	retryAt *time.Time,
	safeDetail string,
	policyVersion string,
	now time.Time,
) (FailureRecord, error) {
	if id == "" || operation.ID == "" || code == "" || policyVersion == "" {
		return FailureRecord{}, errors.New("failure id, operation, code, and policy version are required")
	}
	if now.IsZero() {
		return FailureRecord{}, errors.New("failure occurred_at is required")
	}
	switch class {
	case FailureResource, FailureDependency:
	default:
		return FailureRecord{}, fmt.Errorf("resource gate failure class must be RESOURCE or DEPENDENCY, got %s", class)
	}
	detail := safeDetail
	if len(detail) > 512 {
		detail = detail[:512]
	}
	rec := FailureRecord{
		SchemaVersion:    SchemaVersionV1,
		ID:               id,
		Code:             code,
		Class:            class,
		Locus:            FailureLocus("CAPABILITY"),
		RetryDisposition: disposition,
		EffectState:      EffectNotApplied,
		Scope:            ScopeAttempt,
		MissionRevision:  operation.MissionRevision,
		InquiryID:        operation.InquiryID,
		OperationID:      operation.ID,
		Attempt:          attempt,
		OccurredAt:       now.UTC(),
		SafeDetail:       detail,
		PolicyVersion:    policyVersion,
	}
	if disposition == RetryAfter {
		if retryAt == nil || retryAt.IsZero() {
			return FailureRecord{}, errors.New("RETRY_AFTER requires retry_at")
		}
		t := retryAt.UTC()
		rec.RetryAt = &t
	}
	if err := rec.Validate(); err != nil {
		return FailureRecord{}, err
	}
	return rec, nil
}

func normalizeUsageWindows(u ResourceUsage, resource ResourceID, now time.Time) ResourceUsage {
	u.Resource = resource
	minuteStart := now.Truncate(time.Minute)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if u.MinuteWindowStart.IsZero() || u.MinuteWindowStart.Before(minuteStart) {
		u.MinuteWindowStart = minuteStart
		u.MinuteCount = 0
	}
	if u.DayWindowStart.IsZero() || u.DayWindowStart.Before(dayStart) {
		u.DayWindowStart = dayStart
		u.DayCount = 0
	}
	if u.TokenMinuteWindowStart.IsZero() || u.TokenMinuteWindowStart.Before(minuteStart) {
		u.TokenMinuteWindowStart = minuteStart
		u.TokenMinuteCount = 0
	}
	// An elapsed circuit deadline is no longer active state. Clear it while
	// normalizing so an admitted acquire does not persist a stale deadline.
	// The failure streak remains intact and can still drive the next failure's
	// escalation; only the time-bounded denial expires here.
	if u.CircuitOpenUntil != nil && !now.Before(*u.CircuitOpenUntil) {
		u.CircuitOpenUntil = nil
	}
	return u
}

// cooldownFor is exponential in the failure streak, capped by CooldownMax.
// No wall-clock jitter here — inject jitter at the adapter if desired so pure
// tests remain deterministic (FR-RES-001).
func cooldownFor(failures int, limit ResourceLimit) time.Duration {
	base := limit.CooldownBase
	if base <= 0 {
		base = time.Second
	}
	max := limit.CooldownMax
	if max <= 0 {
		max = base * 32
	}
	// failures at threshold → 1×base; each extra failure doubles.
	shift := failures - limit.FailureThreshold
	if shift < 0 {
		shift = 0
	}
	if shift > 16 {
		shift = 16
	}
	d := base << shift
	if d > max || d <= 0 {
		return max
	}
	return d
}
