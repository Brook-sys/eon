package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RecurringKind classifies a mission-declared standing maintenance obligation
// (FR-DUR-011). Kinds map onto continuity WorkFamily values for admission.
type RecurringKind string

const (
	RecurringKindReview       RecurringKind = "review"
	RecurringKindRevalidation RecurringKind = "revalidation"
	RecurringKindUpdate       RecurringKind = "update"
	RecurringKindAudit        RecurringKind = "audit"
	RecurringKindHarness      RecurringKind = "harness_evaluation"
	RecurringKindGapScan      RecurringKind = "gap_scan"
	RecurringKindCoverage     RecurringKind = "mission_coverage_scan"
	RecurringKindFreshness    RecurringKind = "source_freshness_scan"
	RecurringKindConflict     RecurringKind = "conflict_evidence_review"
	RecurringKindFrontier     RecurringKind = "frontier_management"
	RecurringKindArtifact     RecurringKind = "artifact_refresh"
	RecurringKindIntegrity    RecurringKind = "integrity_audit"
)

func (k RecurringKind) Valid() bool {
	switch k {
	case RecurringKindReview, RecurringKindRevalidation, RecurringKindUpdate, RecurringKindAudit,
		RecurringKindHarness, RecurringKindGapScan, RecurringKindCoverage, RecurringKindFreshness,
		RecurringKindConflict, RecurringKindFrontier, RecurringKindArtifact, RecurringKindIntegrity:
		return true
	default:
		return false
	}
}

// DefaultFamily returns the continuity family that executes this kind.
// Review/revalidation/update/audit map to integrity or frontier when no
// dedicated family exists.
func (k RecurringKind) DefaultFamily() WorkFamily {
	switch k {
	case RecurringKindHarness:
		return FamilyHarnessEvaluation
	case RecurringKindGapScan:
		return FamilyGapScan
	case RecurringKindCoverage:
		return FamilyCoverageScan
	case RecurringKindFreshness:
		return FamilySourceFreshness
	case RecurringKindConflict:
		return FamilyConflictReview
	case RecurringKindFrontier:
		return FamilyFrontierManage
	case RecurringKindArtifact:
		return FamilyArtifactRefresh
	case RecurringKindIntegrity, RecurringKindAudit, RecurringKindReview, RecurringKindRevalidation, RecurringKindUpdate:
		return FamilyIntegrityAudit
	default:
		return ""
	}
}

// AntiRepetitionPolicy limits empty re-work within a cadence window.
type AntiRepetitionPolicy string

const (
	// AntiRepSkipWithoutDelta never reseeds the same period without a new
	// state fingerprint (default; fabricates no empty activity).
	AntiRepSkipWithoutDelta AntiRepetitionPolicy = "skip_without_delta"
	// AntiRepRequireStateChange reseeds mid-window only when StateFingerprint
	// differs from every seed already recorded for that period.
	AntiRepRequireStateChange AntiRepetitionPolicy = "require_state_change"
	// AntiRepRequireEvidenceOrCapacity is an alias of require_state_change for
	// operators who phrase the gate as evidence/capacity movement.
	AntiRepRequireEvidenceOrCapacity AntiRepetitionPolicy = "require_evidence_or_capacity"
)

func (p AntiRepetitionPolicy) Valid() bool {
	switch p {
	case AntiRepSkipWithoutDelta, AntiRepRequireStateChange, AntiRepRequireEvidenceOrCapacity:
		return true
	default:
		return false
	}
}

// RecurringObligation is a mission-declared standing duty with cadence, budget,
// delta criterion and anti-repetition (FR-DUR-011). Empty slices on a mission
// mean "no explicit standing duties"; family strategies may still run.
type RecurringObligation struct {
	SchemaVersion int           `json:"schema_version"`
	ID            string        `json:"id"`
	Kind          RecurringKind `json:"kind"`
	// Family overrides Kind.DefaultFamily when set.
	Family         WorkFamily           `json:"family,omitempty"`
	Title          string               `json:"title"`
	Cadence        time.Duration        `json:"cadence"`
	Budget         Budget               `json:"budget"`
	DeltaCriterion string               `json:"delta_criterion"`
	AntiRepetition AntiRepetitionPolicy `json:"anti_repetition"`
	// MaxPerWindow caps seeds per cadence bucket (default 1).
	MaxPerWindow int    `json:"max_per_window,omitempty"`
	Priority     uint8  `json:"priority,omitempty"`
	Enabled      bool   `json:"enabled"`
	Objective    string `json:"objective,omitempty"`
}

func (o RecurringObligation) EffectiveFamily() WorkFamily {
	if o.Family.Valid() {
		return o.Family
	}
	return o.Kind.DefaultFamily()
}

func (o RecurringObligation) Validate() error {
	if o.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported recurring obligation schema version %d", o.SchemaVersion)
	}
	if strings.TrimSpace(o.ID) == "" || len(o.ID) > 128 {
		return errors.New("recurring obligation id is required and must be bounded")
	}
	if !o.Kind.Valid() {
		return fmt.Errorf("unknown recurring obligation kind %q", o.Kind)
	}
	if o.Family != "" && !o.Family.Valid() {
		return fmt.Errorf("unknown recurring obligation family %q", o.Family)
	}
	if o.EffectiveFamily() == "" || !o.EffectiveFamily().Valid() {
		return errors.New("recurring obligation does not resolve to a valid work family")
	}
	if strings.TrimSpace(o.Title) == "" || len(o.Title) > 512 {
		return errors.New("recurring obligation title is required and must be bounded")
	}
	if o.Cadence <= 0 {
		return errors.New("recurring obligation cadence must be positive")
	}
	if o.Cadence < time.Second {
		return errors.New("recurring obligation cadence must be at least one second")
	}
	if strings.TrimSpace(o.DeltaCriterion) == "" || len(o.DeltaCriterion) > 2048 {
		return errors.New("recurring obligation delta_criterion is required and must be bounded")
	}
	if !o.AntiRepetition.Valid() {
		return fmt.Errorf("unknown anti_repetition policy %q", o.AntiRepetition)
	}
	if o.MaxPerWindow < 0 {
		return errors.New("recurring obligation max_per_window must not be negative")
	}
	if o.Priority == 0 {
		// zero means "use family default at plan time"; allowed on the type.
	}
	if len(o.Objective) > 2048 {
		return errors.New("recurring obligation objective exceeds bound")
	}
	return o.Budget.Validate()
}

// ValidateRecurringObligations checks the slice for structural rules and unique ids.
func ValidateRecurringObligations(items []RecurringObligation) error {
	if len(items) > 64 {
		return errors.New("mission may declare at most 64 recurring obligations")
	}
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("recurring_obligations[%d]: %w", i, err)
		}
		if _, ok := seen[item.ID]; ok {
			return fmt.Errorf("duplicate recurring obligation id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

// ValidateStandingObjectives bounds free-text standing labels (ARCHITECTURE).
func ValidateStandingObjectives(items []string) error {
	if len(items) > 32 {
		return errors.New("mission may declare at most 32 standing objectives")
	}
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || len(item) > 512 {
			return fmt.Errorf("standing_objectives[%d] is empty or oversized", i)
		}
		if _, ok := seen[item]; ok {
			return fmt.Errorf("duplicate standing objective %q", item)
		}
		seen[item] = struct{}{}
	}
	return nil
}

// CadenceBucket returns the discrete period index for now under cadence.
// Bucket 0 is the Unix epoch aligned window; virtual clocks advance buckets
// deterministically without wall-clock dependence beyond the injected now.
func CadenceBucket(now time.Time, cadence time.Duration) int64 {
	if cadence <= 0 {
		return 0
	}
	sec := cadence / time.Second
	if sec <= 0 {
		sec = 1
	}
	return now.UTC().Unix() / int64(sec)
}

// RecurringDedupSignature is stable across restarts for a given obligation
// period (and optional state fingerprint for mid-window deltas).
func RecurringDedupSignature(obligationID string, mission MissionRevisionID, bucket int64, stateFingerprint string) string {
	base := fmt.Sprintf("recurring:%s:%s:%d", obligationID, mission, bucket)
	fp := strings.TrimSpace(stateFingerprint)
	if fp == "" {
		return base
	}
	// Bound fingerprint contribution to keep DedupSignature within 512.
	if len(fp) > 64 {
		fp = fp[:64]
	}
	return base + ":delta:" + fp
}

// RecurringSeedPlan is a pure planner output; the kernel materialises it as a
// WorkOpportunity without inventing model authority.
type RecurringSeedPlan struct {
	ObligationID   string     `json:"obligation_id"`
	Family         WorkFamily `json:"family"`
	Title          string     `json:"title"`
	Origin         string     `json:"origin"`
	ExpectedGain   string     `json:"expected_gain"`
	Novelty        string     `json:"novelty"`
	StopCondition  string     `json:"stop_condition"`
	DedupSignature string     `json:"dedup_signature"`
	Budget         Budget     `json:"budget"`
	Priority       uint8      `json:"priority"`
	PeriodBucket   int64      `json:"period_bucket"`
	Reason         string     `json:"reason"`
	DeltaCriterion string     `json:"delta_criterion"`
}

// PlanRecurringSeeds decides which mission obligations are due. It never
// fabricates empty activity: same cadence bucket is suppressed unless the
// anti-repetition policy and a new state fingerprint authorize a delta seed.
//
// stateFingerprint is an opaque, caller-supplied snapshot of evidence/capacity
// (for example head commit id or continuity finding hash). Empty fingerprint
// disables mid-window delta reseeds.
func PlanRecurringSeeds(
	obligations []RecurringObligation,
	existing []WorkOpportunity,
	mission MissionRevisionID,
	now time.Time,
	stateFingerprint string,
) ([]RecurringSeedPlan, error) {
	if mission == "" {
		return nil, errors.New("mission revision is required for recurring seed plan")
	}
	if err := ValidateRecurringObligations(obligations); err != nil {
		return nil, err
	}
	now = now.UTC()
	fp := strings.TrimSpace(stateFingerprint)

	// Index existing signatures for O(1) anti-duplication.
	sigCount := make(map[string]int, len(existing))
	activeFamily := make(map[WorkFamily]int)
	for _, item := range existing {
		if item.MissionRevision != mission {
			continue
		}
		sigCount[item.DedupSignature]++
		if item.Status.Active() || item.Status == OpportunityAdmitted {
			activeFamily[item.Family]++
		}
	}

	plans := make([]RecurringSeedPlan, 0)
	for _, ob := range obligations {
		if !ob.Enabled {
			continue
		}
		family := ob.EffectiveFamily()
		bucket := CadenceBucket(now, ob.Cadence)
		max := ob.MaxPerWindow
		if max == 0 {
			max = 1
		}
		priority := ob.Priority
		if priority == 0 {
			priority = 18
		}

		baseSig := RecurringDedupSignature(ob.ID, mission, bucket, "")
		baseCount := sigCount[baseSig]

		// Mid-window delta seed only when policy allows and fingerprint is new.
		deltaSig := ""
		if fp != "" && (ob.AntiRepetition == AntiRepRequireStateChange || ob.AntiRepetition == AntiRepRequireEvidenceOrCapacity) {
			deltaSig = RecurringDedupSignature(ob.ID, mission, bucket, fp)
		}

		reason := ""
		sig := ""
		switch {
		case baseCount < max:
			// Cadence-due primary slot still open for this period.
			sig = baseSig
			reason = "cadence_due"
		case deltaSig != "" && sigCount[deltaSig] == 0 && baseCount >= max:
			// Period already used; allow one state-driven increment.
			sig = deltaSig
			reason = "state_delta"
		default:
			// Anti-repetition: no empty reseed.
			continue
		}

		// Avoid piling active work for the same family beyond a soft gate:
		// if there is already active work with this exact signature, skip.
		if sigCount[sig] > 0 {
			continue
		}

		title := strings.TrimSpace(ob.Title)
		gain := strings.TrimSpace(ob.Objective)
		if gain == "" {
			gain = "fulfill standing " + string(ob.Kind) + " obligation"
		}
		novelty := fmt.Sprintf("period_bucket=%d reason=%s", bucket, reason)
		if reason == "state_delta" {
			novelty += " fingerprint=" + fp
			if len(novelty) > 2048 {
				novelty = novelty[:2048]
			}
		}
		plans = append(plans, RecurringSeedPlan{
			ObligationID:   ob.ID,
			Family:         family,
			Title:          title,
			Origin:         "recurring:" + ob.ID,
			ExpectedGain:   gain,
			Novelty:        novelty,
			StopCondition:  "delta_criterion:" + strings.TrimSpace(ob.DeltaCriterion),
			DedupSignature: sig,
			Budget:         ob.Budget,
			Priority:       priority,
			PeriodBucket:   bucket,
			Reason:         reason,
			DeltaCriterion: ob.DeltaCriterion,
		})
		sigCount[sig]++
		_ = activeFamily // reserved for future portfolio pressure signals
	}

	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].Priority != plans[j].Priority {
			return plans[i].Priority > plans[j].Priority
		}
		return plans[i].ObligationID < plans[j].ObligationID
	})
	return plans, nil
}

// RecurringObligationsFingerprint is a deterministic diff key for amendments.
func RecurringObligationsFingerprint(items []RecurringObligation) string {
	if len(items) == 0 {
		return ""
	}
	cp := append([]RecurringObligation(nil), items...)
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].ID < cp[j].ID })
	parts := make([]string, 0, len(cp))
	for _, o := range cp {
		parts = append(parts, strings.Join([]string{
			o.ID,
			string(o.Kind),
			string(o.Family),
			o.Title,
			o.Cadence.String(),
			fmt.Sprintf("mc=%d,t=%d,b=%d,a=%d,d=%s", o.Budget.ModelCalls, o.Budget.Tokens, o.Budget.Bytes, o.Budget.Attempts, o.Budget.Duration),
			o.DeltaCriterion,
			string(o.AntiRepetition),
			fmt.Sprintf("%d", o.MaxPerWindow),
			fmt.Sprintf("%d", o.Priority),
			fmt.Sprintf("%t", o.Enabled),
			o.Objective,
		}, "|"))
	}
	return strings.Join(parts, "\n")
}
