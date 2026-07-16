package inspect

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// MaxContinuityFindingsPerReport caps presentation-time finding lines.
const MaxContinuityFindingsPerReport = 32

// ContinuityFindingSummary is a safe, compact projection of one model-free
// local continuity audit artifact (gap/coverage/freshness/integrity/…).
// Content is derived; the canonical KnowledgeArtifact remains authoritative.
type ContinuityFindingSummary struct {
	ArtifactID          domain.ArtifactID        `json:"artifact_id"`
	Kind                string                   `json:"kind"`
	Family              string                   `json:"family,omitempty"`
	OperationID         domain.OperationID       `json:"operation_id,omitempty"`
	SpecID              domain.OperationSpecID   `json:"spec_id,omitempty"`
	MissionRevision     domain.MissionRevisionID `json:"mission_revision_id,omitempty"`
	VerifiedAt          time.Time                `json:"verified_at,omitempty"`
	HeadCommitID        domain.CommitID          `json:"head_commit_id,omitempty"`
	Findings            []string                 `json:"findings,omitempty"`
	SourcesWithoutObs   int                      `json:"sources_without_observation_count,omitempty"`
	SourcesWithoutFrag  int                      `json:"sources_without_fragment_count,omitempty"`
	FragmentsWithoutObs int                      `json:"fragments_without_observation_count,omitempty"`
	ClaimsWithoutEv     int                      `json:"claims_without_evidence_count,omitempty"`
	AgingSourceCount    int                      `json:"aging_source_count,omitempty"`
	StaleMarked         int                      `json:"stale_artifacts_marked,omitempty"`
	OrphanEvidence      int                      `json:"orphan_evidence_links,omitempty"`
	OrphanObsAnchors    int                      `json:"orphan_observation_anchors,omitempty"`
	OpenOpps            int                      `json:"open_opportunities,omitempty"`
	ReadyCount          int                      `json:"ready_count,omitempty"`
	Stale               bool                     `json:"stale"`
}

// ContinuityFindingsProjection aggregates the latest local continuity audits.
type ContinuityFindingsProjection struct {
	SchemaVersion  int                        `json:"schema_version"`
	TotalReports   int                        `json:"total_reports"`
	StaleReports   int                        `json:"stale_reports"`
	ActiveReports  int                        `json:"active_reports"`
	Latest         *ContinuityFindingSummary  `json:"latest,omitempty"`
	LatestByFamily []ContinuityFindingSummary `json:"latest_by_family,omitempty"`
}

// localAuditBodyMirror matches kernel local-operation-audit-v1 without importing kernel.
type localAuditBodyMirror struct {
	Schema              string                   `json:"schema"`
	OperationID         domain.OperationID       `json:"operation_id"`
	SpecID              domain.OperationSpecID   `json:"spec_id"`
	Mission             domain.MissionRevisionID `json:"mission_revision_id"`
	ReadyCount          int                      `json:"ready_count"`
	OpenOpps            int                      `json:"open_opportunities"`
	VerifiedAt          time.Time                `json:"verified_at"`
	Family              string                   `json:"family,omitempty"`
	HeadCommitID        domain.CommitID          `json:"head_commit_id,omitempty"`
	SourcesWithoutObs   int                      `json:"sources_without_observation_count"`
	ClaimsWithoutEv     int                      `json:"claims_without_evidence_count"`
	SourcesWithoutFrag  int                      `json:"sources_without_fragment_count,omitempty"`
	FragmentsWithoutObs int                      `json:"fragments_without_observation_count,omitempty"`
	StaleMarked         int                      `json:"stale_artifacts_marked,omitempty"`
	OrphanEvidence      int                      `json:"orphan_evidence_links,omitempty"`
	OrphanObsAnchors    int                      `json:"orphan_observation_anchors,omitempty"`
	AgingSourceCount    int                      `json:"aging_source_count,omitempty"`
	Findings            []string                 `json:"findings,omitempty"`
}

func isLocalAuditReportKind(kind string) bool {
	switch kind {
	case "local_operation_audit", "integrity_audit_report", "frontier_manage_report",
		"gap_scan_report", "coverage_scan_report", "source_freshness_report",
		"artifact_refresh_report", "conflict_review_report":
		return true
	default:
		return false
	}
}

// ProjectContinuityFindings builds a read-only aggregation of local continuity
// audit artifacts. When missionRevision is non-empty, only reports bound to
// that revision (via audit body) are kept; empty missionRevision returns all.
func ProjectContinuityFindings(r port.Reader, missionRevision domain.MissionRevisionID) (ContinuityFindingsProjection, error) {
	if r == nil {
		return ContinuityFindingsProjection{}, errors.New("reader is required")
	}
	artifacts, err := r.KnowledgeArtifacts()
	if err != nil {
		return ContinuityFindingsProjection{}, err
	}
	out := ContinuityFindingsProjection{SchemaVersion: domain.SchemaVersionV1}
	summaries := make([]ContinuityFindingSummary, 0)
	for _, artifact := range artifacts {
		if !isLocalAuditReportKind(artifact.Kind) {
			continue
		}
		summary, ok := summarizeLocalAuditArtifact(artifact)
		if !ok {
			continue
		}
		if missionRevision != "" && summary.MissionRevision != "" && summary.MissionRevision != missionRevision {
			continue
		}
		out.TotalReports++
		if artifact.Stale {
			out.StaleReports++
		} else {
			out.ActiveReports++
		}
		summaries = append(summaries, summary)
	}
	if len(summaries) == 0 {
		return out, nil
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		ti, tj := summaries[i].VerifiedAt, summaries[j].VerifiedAt
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return string(summaries[i].ArtifactID) > string(summaries[j].ArtifactID)
	})
	latest := summaries[0]
	out.Latest = &latest

	// Prefer non-stale reports when selecting the latest per family.
	byFamily := map[string]ContinuityFindingSummary{}
	for _, s := range summaries {
		family := s.Family
		if family == "" {
			family = s.Kind
		}
		prev, ok := byFamily[family]
		if !ok {
			byFamily[family] = s
			continue
		}
		// Prefer non-stale; then newer verified_at; then higher artifact id.
		if prev.Stale && !s.Stale {
			byFamily[family] = s
			continue
		}
		if prev.Stale == s.Stale {
			if s.VerifiedAt.After(prev.VerifiedAt) ||
				(s.VerifiedAt.Equal(prev.VerifiedAt) && string(s.ArtifactID) > string(prev.ArtifactID)) {
				byFamily[family] = s
			}
		}
	}
	families := make([]string, 0, len(byFamily))
	for family := range byFamily {
		families = append(families, family)
	}
	sort.Strings(families)
	out.LatestByFamily = make([]ContinuityFindingSummary, 0, len(families))
	for _, family := range families {
		out.LatestByFamily = append(out.LatestByFamily, byFamily[family])
	}
	return out, nil
}

// ContinuityFindingsForMission projects continuity audits for a mission's active revision.
func (p *Projector) ContinuityFindingsForMission(ctx context.Context, missionID domain.MissionID) (ContinuityFindingsProjection, error) {
	if missionID == "" {
		return ContinuityFindingsProjection{}, errors.New("mission ID is required")
	}
	var out ContinuityFindingsProjection
	err := p.Store.View(ctx, func(r port.Reader) error {
		active, err := r.ActiveMissionRevision(missionID)
		if err != nil {
			return err
		}
		proj, err := ProjectContinuityFindings(r, active.ID)
		if err != nil {
			return err
		}
		out = proj
		return nil
	})
	return out, err
}

func summarizeLocalAuditArtifact(artifact domain.KnowledgeArtifact) (ContinuityFindingSummary, bool) {
	if strings.TrimSpace(artifact.Content) == "" {
		return ContinuityFindingSummary{}, false
	}
	var body localAuditBodyMirror
	if err := json.Unmarshal([]byte(artifact.Content), &body); err != nil {
		// Non-JSON or foreign schema: still surface kind/id without findings.
		return ContinuityFindingSummary{
			ArtifactID: artifact.ID,
			Kind:       artifact.Kind,
			Stale:      artifact.Stale,
		}, true
	}
	if body.Schema != "" && body.Schema != "local-operation-audit-v1" {
		return ContinuityFindingSummary{
			ArtifactID: artifact.ID,
			Kind:       artifact.Kind,
			Stale:      artifact.Stale,
		}, true
	}
	findings := body.Findings
	if len(findings) > MaxContinuityFindingsPerReport {
		findings = append([]string(nil), findings[:MaxContinuityFindingsPerReport]...)
		findings = append(findings, "findings:truncated")
	} else if findings != nil {
		findings = append([]string(nil), findings...)
	}
	// Presentation-time redaction of free-text finding lines (defensive).
	for i, line := range findings {
		safe, _ := RedactSensitiveText(line)
		bounded, _ := BoundUTF8(safe, 512)
		findings[i] = bounded
	}
	return ContinuityFindingSummary{
		ArtifactID:          artifact.ID,
		Kind:                artifact.Kind,
		Family:              body.Family,
		OperationID:         body.OperationID,
		SpecID:              body.SpecID,
		MissionRevision:     body.Mission,
		VerifiedAt:          body.VerifiedAt.UTC(),
		HeadCommitID:        body.HeadCommitID,
		Findings:            findings,
		SourcesWithoutObs:   body.SourcesWithoutObs,
		SourcesWithoutFrag:  body.SourcesWithoutFrag,
		FragmentsWithoutObs: body.FragmentsWithoutObs,
		ClaimsWithoutEv:     body.ClaimsWithoutEv,
		AgingSourceCount:    body.AgingSourceCount,
		StaleMarked:         body.StaleMarked,
		OrphanEvidence:      body.OrphanEvidence,
		OrphanObsAnchors:    body.OrphanObsAnchors,
		OpenOpps:            body.OpenOpps,
		ReadyCount:          body.ReadyCount,
		Stale:               artifact.Stale,
	}, true
}
