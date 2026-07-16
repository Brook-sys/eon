package inspect

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/view"
)

// MaxRefreshCandidateRows caps dry-run refresh candidate rows for operators.
const MaxRefreshCandidateRows = 64

// RefreshCandidateSummary is a presentation row for an authorized refresh plan.
type RefreshCandidateSummary struct {
	ArtifactID   domain.ArtifactID `json:"artifact_id"`
	Kind         string            `json:"kind"`
	BaseCommitID domain.CommitID   `json:"base_commit_id,omitempty"`
	Regenerable  bool              `json:"regenerable"`
	Reason       string            `json:"reason,omitempty"`
}

// StoreRetentionProjection is a read-only dry-run of store-retention.v1 posture.
// It never mutates the store and never authorizes event-log prune.
type StoreRetentionProjection struct {
	SchemaVersion          int                       `json:"schema_version"`
	ObservedAt             time.Time                 `json:"observed_at"`
	PolicyVersion          string                    `json:"policy_version"`
	EventHeadSequence      uint64                    `json:"event_head_sequence"`
	EventHeadPressure      string                    `json:"event_head_pressure,omitempty"`
	ArtifactCount          int                       `json:"artifact_count"`
	StaleArtifactCount     int                       `json:"stale_artifact_count"`
	StaleArtifactPressure  string                    `json:"stale_artifact_pressure,omitempty"`
	AllowEventLogPrune     bool                      `json:"allow_event_log_prune"`
	AuthorizedActions      []string                  `json:"authorized_actions"`
	RefreshCandidateLimit  int                       `json:"refresh_candidate_limit"`
	RefreshCandidateCount  int                       `json:"refresh_candidate_count"`
	RefreshCandidatesTrunc int                       `json:"refresh_candidates_truncated,omitempty"`
	RegenerableCitedViews  int                       `json:"regenerable_cited_claim_views"`
	NonRegenerableStale    int                       `json:"non_regenerable_stale"`
	RefreshCandidates      []RefreshCandidateSummary `json:"refresh_candidates"`
	Findings               []string                  `json:"findings,omitempty"`
}

// StoreRetention projects authorized retention posture and refresh candidates.
// missionID is optional; when set, active head is included in findings only.
func (p *Projector) StoreRetention(ctx context.Context, missionID domain.MissionID) (StoreRetentionProjection, error) {
	now := p.Clock().UTC()
	policy := domain.DefaultStoreRetentionPolicy().Normalize()
	var proj StoreRetentionProjection
	err := p.Store.View(ctx, func(r port.Reader) error {
		headSeq, err := eventHead(r)
		if err != nil {
			return err
		}
		artifacts, err := r.KnowledgeArtifacts()
		if err != nil {
			return err
		}
		stale := 0
		for _, a := range artifacts {
			if a.Stale {
				stale++
			}
		}
		// Plan a larger pool then present a capped operator view.
		plannedIDs := domain.PlanStaleArtifactRefresh(artifacts, MaxRefreshCandidateRows*2)
		rows := make([]RefreshCandidateSummary, 0, len(plannedIDs))
		regenerable := 0
		nonRegen := 0
		for _, id := range plannedIDs {
			art, err := r.KnowledgeArtifact(id)
			if err != nil {
				if errors.Is(err, port.ErrNotFound) {
					continue
				}
				return err
			}
			row := RefreshCandidateSummary{
				ArtifactID:   art.ID,
				Kind:         art.Kind,
				BaseCommitID: art.BaseCommitID,
			}
			if art.Kind == view.CitedClaimViewKind {
				// Pure check: claim dependency + claim exists + has evidence.
				if claimID, depErr := claimIDFromArtifactDeps(art.Dependencies); depErr == nil {
					if claim, claimErr := r.Claim(claimID); claimErr == nil && claim.ID != "" {
						if links, linkErr := r.EvidenceLinksForClaim(claimID); linkErr == nil && len(links) > 0 {
							row.Regenerable = true
							row.Reason = "cited_claim_view"
							regenerable++
						} else {
							row.Reason = "missing_evidence"
							nonRegen++
						}
					} else {
						row.Reason = "missing_claim"
						nonRegen++
					}
				} else {
					row.Reason = "missing_claim_dependency"
					nonRegen++
				}
			} else {
				row.Reason = "kind_not_auto_regenerable"
				nonRegen++
			}
			rows = append(rows, row)
		}
		truncated := 0
		if len(rows) > MaxRefreshCandidateRows {
			truncated = len(rows) - MaxRefreshCandidateRows
			rows = rows[:MaxRefreshCandidateRows]
		}
		actions := policy.AuthorizedRetentionActions()
		actionStrs := make([]string, 0, len(actions))
		for _, a := range actions {
			actionStrs = append(actionStrs, string(a))
		}
		findings := []string{
			fmt.Sprintf("retention:policy=%s", policy.Version),
			fmt.Sprintf("retention:event_head=%d", headSeq),
			fmt.Sprintf("retention:stale_artifacts=%d", stale),
			fmt.Sprintf("retention:refresh_candidates=%d", len(plannedIDs)),
			fmt.Sprintf("retention:regenerable_cited=%d", regenerable),
			fmt.Sprintf("retention:event_log_prune_authorized=%t", policy.AllowEventLogPrune),
		}
		if pressure := policy.EventHeadPressure(headSeq); pressure != "" {
			findings = append(findings, "retention:event_head_pressure="+pressure)
		}
		if pressure := policy.StaleArtifactPressure(stale); pressure != "" {
			findings = append(findings, "retention:stale_pressure="+pressure)
		}
		if missionID != "" {
			if active, activeErr := r.ActiveMissionRevision(missionID); activeErr == nil {
				findings = append(findings, "retention:mission_revision="+string(active.ID))
				if head, headErr := r.HeadCommit(active.ID); headErr == nil {
					findings = append(findings, "retention:head_commit="+string(head.ID))
				}
			}
		}
		proj = StoreRetentionProjection{
			SchemaVersion:          domain.SchemaVersionV1,
			ObservedAt:             now,
			PolicyVersion:          policy.Version,
			EventHeadSequence:      headSeq,
			EventHeadPressure:      policy.EventHeadPressure(headSeq),
			ArtifactCount:          len(artifacts),
			StaleArtifactCount:     stale,
			StaleArtifactPressure:  policy.StaleArtifactPressure(stale),
			AllowEventLogPrune:     false,
			AuthorizedActions:      actionStrs,
			RefreshCandidateLimit:  MaxRefreshCandidateRows,
			RefreshCandidateCount:  len(plannedIDs),
			RefreshCandidatesTrunc: truncated,
			RegenerableCitedViews:  regenerable,
			NonRegenerableStale:    nonRegen,
			RefreshCandidates:      rows,
			Findings:               findings,
		}
		return nil
	})
	if err != nil {
		return StoreRetentionProjection{}, err
	}
	return proj, nil
}

func claimIDFromArtifactDeps(deps []string) (domain.ClaimID, error) {
	for _, dependency := range deps {
		kind, id, _, ok := domain.ParseDependencyRef(dependency)
		if ok && kind == domain.DependencyKindClaim && id != "" {
			return domain.ClaimID(id), nil
		}
	}
	return "", errors.New("knowledge artifact has no claim dependency")
}
