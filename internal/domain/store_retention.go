package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Store retention is intentionally conservative for the MVP (ADR-0003):
// the canonical event log is append-only and MUST NOT be pruned by automated
// maintenance. Authorized actions are limited to (1) marking derived knowledge
// artifacts stale for refresh, (2) frontier hygiene / projection compaction,
// and (3) disposable export buffers outside the store (see observability).

const (
	// StoreRetentionPolicyVersion stamps inspect/docs projections.
	StoreRetentionPolicyVersion = "store-retention.v1"

	// Soft operator thresholds for presentation-only alerts (not GC triggers).
	DefaultEventHeadWarnSequence  uint64 = 50_000
	DefaultEventHeadInfoSequence  uint64 = 10_000
	DefaultStaleArtifactWarnCount        = 100
	DefaultStaleArtifactInfoCount        = 20
)

// RetentionActionKind classifies what maintenance is authorized to do.
type RetentionActionKind string

const (
	// RetentionActionNone: no maintenance; observe only.
	RetentionActionNone RetentionActionKind = "none"
	// RetentionActionRefreshCandidates: plan refresh of stale/derived artifacts.
	RetentionActionRefreshCandidates RetentionActionKind = "refresh_candidates"
	// RetentionActionFrontierHygiene: compact/supersede WorkOpportunity reservoir.
	RetentionActionFrontierHygiene RetentionActionKind = "frontier_hygiene"
	// RetentionActionExportBufferTrim: disposable OTLP queues only (not store).
	RetentionActionExportBufferTrim RetentionActionKind = "export_buffer_trim"
	// RetentionActionEventLogPrune is NEVER authorized in MVP.
	RetentionActionEventLogPrune RetentionActionKind = "event_log_prune"
)

// StoreRetentionPolicy is a pure, versioned posture for authorized maintenance.
// Zero thresholds mean defaults when Normalize is called.
type StoreRetentionPolicy struct {
	Version string `json:"version"`

	// EventHeadInfoSequence / EventHeadWarnSequence are soft alert thresholds
	// on the append-only log head. They do not authorize deletion.
	EventHeadInfoSequence  uint64 `json:"event_head_info_sequence"`
	EventHeadWarnSequence  uint64 `json:"event_head_warn_sequence"`
	StaleArtifactInfoCount int    `json:"stale_artifact_info_count"`
	StaleArtifactWarnCount int    `json:"stale_artifact_warn_count"`

	// AllowEventLogPrune is always forced false by Validate/Normalize in MVP.
	AllowEventLogPrune bool `json:"allow_event_log_prune"`
}

// DefaultStoreRetentionPolicy returns the MVP-safe defaults.
func DefaultStoreRetentionPolicy() StoreRetentionPolicy {
	return StoreRetentionPolicy{
		Version:                StoreRetentionPolicyVersion,
		EventHeadInfoSequence:  DefaultEventHeadInfoSequence,
		EventHeadWarnSequence:  DefaultEventHeadWarnSequence,
		StaleArtifactInfoCount: DefaultStaleArtifactInfoCount,
		StaleArtifactWarnCount: DefaultStaleArtifactWarnCount,
		AllowEventLogPrune:     false,
	}
}

// Normalize applies defaults and hard-disables event-log prune.
func (p StoreRetentionPolicy) Normalize() StoreRetentionPolicy {
	out := p
	if strings.TrimSpace(out.Version) == "" {
		out.Version = StoreRetentionPolicyVersion
	}
	if out.EventHeadInfoSequence == 0 {
		out.EventHeadInfoSequence = DefaultEventHeadInfoSequence
	}
	if out.EventHeadWarnSequence == 0 {
		out.EventHeadWarnSequence = DefaultEventHeadWarnSequence
	}
	if out.StaleArtifactInfoCount <= 0 {
		out.StaleArtifactInfoCount = DefaultStaleArtifactInfoCount
	}
	if out.StaleArtifactWarnCount <= 0 {
		out.StaleArtifactWarnCount = DefaultStaleArtifactWarnCount
	}
	// Warn must be >= info for event head.
	if out.EventHeadWarnSequence < out.EventHeadInfoSequence {
		out.EventHeadWarnSequence = out.EventHeadInfoSequence
	}
	if out.StaleArtifactWarnCount < out.StaleArtifactInfoCount {
		out.StaleArtifactWarnCount = out.StaleArtifactInfoCount
	}
	out.AllowEventLogPrune = false
	return out
}

// Validate rejects inverted thresholds and any attempt to enable event-log prune.
func (p StoreRetentionPolicy) Validate() error {
	if p.AllowEventLogPrune {
		return errors.New("event log prune is not authorized in store-retention.v1 (append-only MVP)")
	}
	if p.EventHeadWarnSequence != 0 && p.EventHeadInfoSequence != 0 && p.EventHeadWarnSequence < p.EventHeadInfoSequence {
		return fmt.Errorf("event head warn sequence %d must be >= info sequence %d", p.EventHeadWarnSequence, p.EventHeadInfoSequence)
	}
	if p.StaleArtifactWarnCount != 0 && p.StaleArtifactInfoCount != 0 && p.StaleArtifactWarnCount < p.StaleArtifactInfoCount {
		return fmt.Errorf("stale artifact warn count %d must be >= info count %d", p.StaleArtifactWarnCount, p.StaleArtifactInfoCount)
	}
	return nil
}

// AuthorizedRetentionActions lists maintenance kinds allowed under this policy.
func (p StoreRetentionPolicy) AuthorizedRetentionActions() []RetentionActionKind {
	// Sorted for deterministic projections.
	actions := []RetentionActionKind{
		RetentionActionRefreshCandidates,
		RetentionActionFrontierHygiene,
		RetentionActionExportBufferTrim,
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i] < actions[j] })
	return actions
}

// IsRetentionActionAuthorized reports whether kind is allowed under MVP policy.
func (p StoreRetentionPolicy) IsRetentionActionAuthorized(kind RetentionActionKind) bool {
	switch kind {
	case RetentionActionNone, RetentionActionRefreshCandidates, RetentionActionFrontierHygiene, RetentionActionExportBufferTrim:
		return true
	case RetentionActionEventLogPrune:
		return false
	default:
		return false
	}
}

// EventHeadPressure classifies soft pressure on the append-only event log head.
// Empty string means below info threshold.
func (p StoreRetentionPolicy) EventHeadPressure(headSequence uint64) string {
	p = p.Normalize()
	if headSequence >= p.EventHeadWarnSequence {
		return "warn"
	}
	if headSequence >= p.EventHeadInfoSequence {
		return "info"
	}
	return ""
}

// StaleArtifactPressure classifies soft pressure from stale derived artifacts.
func (p StoreRetentionPolicy) StaleArtifactPressure(staleCount int) string {
	p = p.Normalize()
	if staleCount < 0 {
		staleCount = 0
	}
	if staleCount >= p.StaleArtifactWarnCount {
		return "warn"
	}
	if staleCount >= p.StaleArtifactInfoCount {
		return "info"
	}
	return ""
}

// PlanStaleArtifactRefresh selects non-audit stale artifact IDs for authorized
// refresh candidates. Sorted by ID. Does not mutate state.
func PlanStaleArtifactRefresh(artifacts []KnowledgeArtifact, limit int) []ArtifactID {
	if limit <= 0 {
		return nil
	}
	var out []ArtifactID
	for _, artifact := range artifacts {
		if !artifact.Stale {
			continue
		}
		if IsLocalAuditArtifactKind(artifact.Kind) {
			continue
		}
		out = append(out, artifact.ID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
