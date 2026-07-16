package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// WorkFamily identifies a continuity portfolio family. Families are registry
// keys; the scheduler never invents work outside registered families.
type WorkFamily string

const (
	FamilyGapScan           WorkFamily = "gap_scan"
	FamilyConflictReview    WorkFamily = "conflict_evidence_review"
	FamilyArtifactRefresh   WorkFamily = "artifact_refresh"
	FamilyIntegrityAudit    WorkFamily = "integrity_audit"
	FamilyHarnessEvaluation WorkFamily = "harness_evaluation"
	FamilyFrontierManage    WorkFamily = "frontier_management"
	FamilyCoverageScan      WorkFamily = "mission_coverage_scan"
	FamilySourceFreshness   WorkFamily = "source_freshness_scan"
)

func (f WorkFamily) valid() bool {
	switch f {
	case FamilyGapScan, FamilyConflictReview, FamilyArtifactRefresh, FamilyIntegrityAudit,
		FamilyHarnessEvaluation, FamilyFrontierManage, FamilyCoverageScan, FamilySourceFreshness:
		return true
	default:
		return false
	}
}

// Valid reports whether the family is part of the continuity portfolio catalogue.
func (f WorkFamily) Valid() bool { return f.valid() }

// WorkOpportunityStatus is the lifecycle of a frontier opportunity before or
// after admission into the executable agenda.
type WorkOpportunityStatus string

const (
	OpportunityOpen       WorkOpportunityStatus = "OPEN"
	OpportunityAdmitted   WorkOpportunityStatus = "ADMITTED"
	OpportunityDeferred   WorkOpportunityStatus = "DEFERRED"
	OpportunityAbandoned  WorkOpportunityStatus = "ABANDONED"
	OpportunitySuperseded WorkOpportunityStatus = "SUPERSEDED"
)

func (s WorkOpportunityStatus) valid() bool {
	switch s {
	case OpportunityOpen, OpportunityAdmitted, OpportunityDeferred, OpportunityAbandoned, OpportunitySuperseded:
		return true
	default:
		return false
	}
}

// Valid reports whether the status is a known opportunity lifecycle value.
func (s WorkOpportunityStatus) Valid() bool { return s.valid() }

func (s WorkOpportunityStatus) Active() bool {
	return s == OpportunityOpen || s == OpportunityDeferred
}

// HorizonPolicy is a versioned, deterministic replenishment contract. Model
// outputs never override these limits.
type HorizonPolicy struct {
	SchemaVersion    int           `json:"schema_version"`
	Version          string        `json:"version"`
	TargetReady      int           `json:"target_ready"`
	LowWatermark     int           `json:"low_watermark"`
	MaxReady         int           `json:"max_ready"`
	MaxCandidates    int           `json:"max_candidates"`
	MaxChildren      int           `json:"max_children"`
	MaxDepth         int           `json:"max_depth"`
	StrategyCooldown time.Duration `json:"strategy_cooldown"`
}

// DefaultHorizonPolicy returns conservative MVP marks for a short horizon.
func DefaultHorizonPolicy() HorizonPolicy {
	return HorizonPolicy{
		SchemaVersion:    SchemaVersionV1,
		Version:          "horizon.v1",
		TargetReady:      4,
		LowWatermark:     2,
		MaxReady:         8,
		MaxCandidates:    64,
		MaxChildren:      4,
		MaxDepth:         3,
		StrategyCooldown: 5 * time.Minute,
	}
}

func (p HorizonPolicy) Validate() error {
	if p.SchemaVersion != SchemaVersionV1 || strings.TrimSpace(p.Version) == "" {
		return errors.New("horizon policy is incomplete or has unsupported schema version")
	}
	if p.TargetReady <= 0 || p.LowWatermark < 0 || p.MaxReady <= 0 || p.MaxCandidates <= 0 || p.MaxChildren <= 0 || p.MaxDepth < 0 {
		return errors.New("horizon policy marks must be positive or zero only where allowed")
	}
	if p.LowWatermark > p.TargetReady || p.TargetReady > p.MaxReady {
		return errors.New("horizon policy requires low_watermark <= target_ready <= max_ready")
	}
	if p.StrategyCooldown < 0 {
		return errors.New("horizon policy strategy cooldown must not be negative")
	}
	return nil
}

// NeedsReplenishment reports whether the ready horizon has fallen to or below
// the preventive low watermark.
func (p HorizonPolicy) NeedsReplenishment(readyCount int) bool {
	if readyCount < 0 {
		return true
	}
	return readyCount <= p.LowWatermark
}

// AcceptsAdmission reports whether another ready operation may be materialised.
func (p HorizonPolicy) AcceptsAdmission(readyCount int) bool {
	return readyCount >= 0 && readyCount < p.MaxReady
}

// WorkOpportunity is a durable frontier unit before agenda admission.
type WorkOpportunity struct {
	SchemaVersion     int                   `json:"schema_version"`
	ID                WorkOpportunityID     `json:"id"`
	MissionRevision   MissionRevisionID     `json:"mission_revision_id"`
	Family            WorkFamily            `json:"family"`
	Status            WorkOpportunityStatus `json:"status"`
	Title             string                `json:"title"`
	Origin            string                `json:"origin"`
	ExpectedGain      string                `json:"expected_gain"`
	Novelty           string                `json:"novelty"`
	StopCondition     string                `json:"stop_condition"`
	DedupSignature    string                `json:"dedup_signature"`
	ParentID          WorkOpportunityID     `json:"parent_id,omitempty"`
	Depth             int                   `json:"depth"`
	Dependencies      []string              `json:"dependencies,omitempty"`
	EstimatedCost     Budget                `json:"estimated_cost"`
	Risk              RiskLevel             `json:"risk"`
	Priority          uint8                 `json:"priority"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
	AdmittedInquiryID InquiryID             `json:"admitted_inquiry_id,omitempty"`
	AbandonReason     string                `json:"abandon_reason,omitempty"`
}

func (o WorkOpportunity) Validate() error {
	if o.SchemaVersion != SchemaVersionV1 || o.ID == "" || o.MissionRevision == "" || !o.Family.valid() || !o.Status.valid() {
		return errors.New("work opportunity is incomplete or has unsupported schema version")
	}
	if strings.TrimSpace(o.Title) == "" || strings.TrimSpace(o.Origin) == "" || strings.TrimSpace(o.ExpectedGain) == "" ||
		strings.TrimSpace(o.Novelty) == "" || strings.TrimSpace(o.StopCondition) == "" || strings.TrimSpace(o.DedupSignature) == "" {
		return errors.New("work opportunity is missing required descriptive fields")
	}
	if len(o.Title) > 512 || len(o.Origin) > 512 || len(o.ExpectedGain) > 2048 || len(o.Novelty) > 2048 ||
		len(o.StopCondition) > 2048 || len(o.DedupSignature) > 512 || len(o.AbandonReason) > 2048 {
		return errors.New("work opportunity field exceeds byte limit")
	}
	if o.Depth < 0 {
		return errors.New("work opportunity depth must not be negative")
	}
	if o.ParentID == "" && o.Depth != 0 {
		return errors.New("root work opportunity must have depth 0")
	}
	if o.ParentID != "" && o.Depth < 1 {
		return errors.New("child work opportunity requires positive depth")
	}
	if o.ParentID == o.ID {
		return errors.New("work opportunity cannot parent itself")
	}
	if o.Priority == 0 {
		return errors.New("work opportunity priority must be positive")
	}
	if o.CreatedAt.IsZero() || o.UpdatedAt.IsZero() || o.UpdatedAt.Before(o.CreatedAt) {
		return errors.New("work opportunity timestamps are incomplete")
	}
	switch o.Risk {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		return fmt.Errorf("unknown work opportunity risk %q", o.Risk)
	}
	if o.Status == OpportunityAdmitted && o.AdmittedInquiryID == "" {
		return errors.New("admitted work opportunity requires inquiry reference")
	}
	if o.Status != OpportunityAdmitted && o.AdmittedInquiryID != "" {
		return errors.New("non-admitted work opportunity must not reference an inquiry")
	}
	if o.Status == OpportunityAbandoned && strings.TrimSpace(o.AbandonReason) == "" {
		return errors.New("abandoned work opportunity requires a reason")
	}
	if len(o.Dependencies) > 32 {
		return errors.New("work opportunity has too many dependencies")
	}
	for _, dep := range o.Dependencies {
		if strings.TrimSpace(dep) == "" || len(dep) > 256 {
			return errors.New("work opportunity dependency is empty or oversized")
		}
	}
	return o.EstimatedCost.Validate()
}

// CanSpawnChild enforces recursive decomposition limits from policy.
func (o WorkOpportunity) CanSpawnChild(policy HorizonPolicy, existingChildren int) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	if !o.Status.Active() {
		return errors.New("only active opportunities may spawn children")
	}
	if o.Depth >= policy.MaxDepth {
		return fmt.Errorf("work opportunity depth %d reached policy max %d", o.Depth, policy.MaxDepth)
	}
	if existingChildren < 0 {
		return errors.New("existing children count must not be negative")
	}
	if existingChildren >= policy.MaxChildren {
		return fmt.Errorf("work opportunity child fan-out %d reached policy max %d", existingChildren, policy.MaxChildren)
	}
	return nil
}

// DeriveChild builds a validated child opportunity with parent lineage.
func (o WorkOpportunity) DeriveChild(id WorkOpportunityID, title, origin, expectedGain, novelty, stop, signature string, risk RiskLevel, priority uint8, now time.Time, cost Budget) (WorkOpportunity, error) {
	child := WorkOpportunity{
		SchemaVersion:   SchemaVersionV1,
		ID:              id,
		MissionRevision: o.MissionRevision,
		Family:          o.Family,
		Status:          OpportunityOpen,
		Title:           title,
		Origin:          origin,
		ExpectedGain:    expectedGain,
		Novelty:         novelty,
		StopCondition:   stop,
		DedupSignature:  signature,
		ParentID:        o.ID,
		Depth:           o.Depth + 1,
		EstimatedCost:   cost,
		Risk:            risk,
		Priority:        priority,
		CreatedAt:       now.UTC(),
		UpdatedAt:       now.UTC(),
	}
	if err := child.Validate(); err != nil {
		return WorkOpportunity{}, err
	}
	return child, nil
}

// ExecutableHorizon is an observed short-horizon snapshot used by the scheduler.
type ExecutableHorizon struct {
	SchemaVersion   int               `json:"schema_version"`
	MissionRevision MissionRevisionID `json:"mission_revision_id"`
	PolicyVersion   string            `json:"policy_version"`
	ReadyCount      int               `json:"ready_count"`
	OpenCandidates  int               `json:"open_candidates"`
	TargetReady     int               `json:"target_ready"`
	LowWatermark    int               `json:"low_watermark"`
	MaxReady        int               `json:"max_ready"`
	ObservedAt      time.Time         `json:"observed_at"`
}

func (h ExecutableHorizon) Validate() error {
	if h.SchemaVersion != SchemaVersionV1 || h.MissionRevision == "" || strings.TrimSpace(h.PolicyVersion) == "" || h.ObservedAt.IsZero() {
		return errors.New("executable horizon is incomplete or has unsupported schema version")
	}
	if h.ReadyCount < 0 || h.OpenCandidates < 0 || h.TargetReady <= 0 || h.LowWatermark < 0 || h.MaxReady <= 0 {
		return errors.New("executable horizon counts/marks are invalid")
	}
	if h.LowWatermark > h.TargetReady || h.TargetReady > h.MaxReady {
		return errors.New("executable horizon marks are inconsistent")
	}
	return nil
}

func (h ExecutableHorizon) NeedsReplenishment() bool {
	return h.ReadyCount <= h.LowWatermark
}

// ContinuityAction is the non-dispatch branch of a scheduling frontier.
type ContinuityAction string

const (
	ContinuityExpand   ContinuityAction = "EXPAND"
	ContinuityDiagnose ContinuityAction = "DIAGNOSE"
)

func (a ContinuityAction) valid() bool {
	switch a {
	case ContinuityExpand, ContinuityDiagnose:
		return true
	default:
		return false
	}
}

// ContinuityDiagnosis records why an active mission could not continue safely.
type ContinuityDiagnosis struct {
	SchemaVersion           int                   `json:"schema_version"`
	ID                      ContinuityDiagnosisID `json:"id"`
	MissionRevision         MissionRevisionID     `json:"mission_revision_id"`
	OccurredAt              time.Time             `json:"occurred_at"`
	StrategiesTried         []string              `json:"strategies_tried"`
	UnavailableCapabilities []string              `json:"unavailable_capabilities,omitempty"`
	OpenCandidateCount      int                   `json:"open_candidate_count"`
	ReadyCount              int                   `json:"ready_count"`
	LastDeltaRef            string                `json:"last_delta_ref,omitempty"`
	EliminatedAlternatives  []string              `json:"eliminated_alternatives,omitempty"`
	RecoveryConditions      []string              `json:"recovery_conditions"`
	SafeDetail              string                `json:"safe_detail"`
	PolicyVersion           string                `json:"policy_version"`
}

func (d ContinuityDiagnosis) Validate() error {
	if d.SchemaVersion != SchemaVersionV1 || d.ID == "" || d.MissionRevision == "" || d.OccurredAt.IsZero() {
		return errors.New("continuity diagnosis is incomplete or has unsupported schema version")
	}
	if len(d.StrategiesTried) == 0 {
		return errors.New("continuity diagnosis requires strategies tried")
	}
	if len(d.RecoveryConditions) == 0 {
		return errors.New("continuity diagnosis requires recovery conditions")
	}
	if strings.TrimSpace(d.SafeDetail) == "" || strings.TrimSpace(d.PolicyVersion) == "" {
		return errors.New("continuity diagnosis requires safe detail and policy version")
	}
	if d.OpenCandidateCount < 0 || d.ReadyCount < 0 {
		return errors.New("continuity diagnosis counts must not be negative")
	}
	if len(d.SafeDetail) > 4096 || len(d.LastDeltaRef) > 512 || len(d.PolicyVersion) > 128 {
		return errors.New("continuity diagnosis field exceeds byte limit")
	}
	if len(d.StrategiesTried) > 64 || len(d.UnavailableCapabilities) > 32 || len(d.EliminatedAlternatives) > 32 || len(d.RecoveryConditions) > 32 {
		return errors.New("continuity diagnosis list exceeds cardinality limit")
	}
	for _, item := range d.StrategiesTried {
		if strings.TrimSpace(item) == "" {
			return errors.New("continuity diagnosis strategy name must not be empty")
		}
	}
	for _, item := range d.RecoveryConditions {
		if strings.TrimSpace(item) == "" {
			return errors.New("continuity diagnosis recovery condition must not be empty")
		}
	}
	return nil
}

// Event kinds and failure code for continuity degradation.
const (
	EventContinuityGapDetected       = "continuity.gap_detected"
	EventContinuityExpanded          = "continuity.expanded"
	EventContinuityBlocked           = "continuity.blocked"
	EventContinuityFrontierCompacted = "continuity.frontier_compacted"
	EventWorkOpportunityAbandoned    = "continuity.opportunity_abandoned"
	EventWorkOpportunitySuperseded   = "continuity.opportunity_superseded"
	EventWorkOpportunityDeferred     = "continuity.opportunity_deferred"
	EventWorkOpportunityReopened     = "continuity.opportunity_reopened"
	FailureCodeContinuityBlocked     = "CONTINUITY_BLOCKED"
)
