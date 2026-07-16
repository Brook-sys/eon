package kernel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// PlanChildDraftsFromStore inspects the store with the same model-free joins as
// LocalExecutor and returns at most one actionable child draft for the family.
// Empty result means the static ChildDrafts (if any) should be used.
func PlanChildDraftsFromStore(ctx context.Context, store port.Store, family domain.WorkFamily, mission domain.MissionRevisionID, now time.Time) ([]ChildDraft, error) {
	if store == nil || mission == "" || !family.Valid() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var (
		sources      []domain.Source
		versions     []domain.SourceVersion
		observations []domain.Observation
		claims       []domain.Claim
		evidence     []domain.EvidenceLink
		artifacts    []domain.KnowledgeArtifact
		headID       = domain.GenesisCommitID
	)
	fragmentByID := map[domain.SourceFragmentID]domain.SourceFragment{}
	versionByID := map[domain.SourceVersionID]domain.SourceVersion{}
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		sources, err = r.Sources()
		if err != nil {
			return err
		}
		versions, err = r.SourceVersions("")
		if err != nil {
			return err
		}
		for _, ver := range versions {
			versionByID[ver.ID] = ver
			frags, fragErr := r.SourceFragments(ver.ID)
			if fragErr != nil {
				return fragErr
			}
			for _, frag := range frags {
				fragmentByID[frag.ID] = frag
			}
		}
		observations, err = r.Observations()
		if err != nil {
			return err
		}
		claims, err = r.Claims()
		if err != nil {
			return err
		}
		evidence, err = r.EvidenceLinks()
		if err != nil {
			return err
		}
		artifacts, err = r.KnowledgeArtifacts()
		if err != nil {
			return err
		}
		if head, headErr := r.HeadCommit(mission); headErr == nil {
			headID = head.ID
		}
		return nil
	}); err != nil {
		return nil, err
	}

	switch family {
	case domain.FamilyGapScan:
		_, withoutObs, withoutFrag, fragsWithoutObs := coverageJoin(sources, versionByID, fragmentByID, observations)
		if withoutObs == 0 && withoutFrag == 0 && fragsWithoutObs == 0 {
			return nil, nil
		}
		return []ChildDraft{{
			Title:          "enumerate sources and fragments lacking observations",
			Origin:         "decompose:gap_scan:join",
			ExpectedGain:   fmt.Sprintf("structural gaps without_obs=%d without_frag=%d frags_without_obs=%d", withoutObs, withoutFrag, fragsWithoutObs),
			Novelty:        fmt.Sprintf("gap join inventory at %s", now.UTC().Format(time.RFC3339)),
			StopCondition:  "gap inventory persisted or deferred",
			DedupSignature: "gap:join_inventory",
			Risk:           domain.RiskLow,
			Priority:       25,
			EstimatedCost:  domain.Budget{Tokens: 64, Attempts: 1},
		}}, nil

	case domain.FamilyCoverageScan:
		_, withoutObs, withoutFrag, fragsWithoutObs := coverageJoin(sources, versionByID, fragmentByID, observations)
		claimHasEvidence := map[domain.ClaimID]struct{}{}
		for _, link := range evidence {
			if link.ClaimID != "" {
				claimHasEvidence[link.ClaimID] = struct{}{}
			}
		}
		claimsWithoutEv := 0
		for _, claim := range claims {
			if _, ok := claimHasEvidence[claim.ID]; !ok {
				claimsWithoutEv++
			}
		}
		if withoutObs == 0 && withoutFrag == 0 && fragsWithoutObs == 0 && claimsWithoutEv == 0 {
			return nil, nil
		}
		return []ChildDraft{{
			Title:          "map mission coverage holes from source joins",
			Origin:         "decompose:coverage_scan:join",
			ExpectedGain:   fmt.Sprintf("coverage gaps without_obs=%d claims_without_ev=%d", withoutObs, claimsWithoutEv),
			Novelty:        fmt.Sprintf("coverage join inventory at %s", now.UTC().Format(time.RFC3339)),
			StopCondition:  "coverage inventory persisted",
			DedupSignature: "coverage:join_inventory",
			Risk:           domain.RiskLow,
			Priority:       23,
			EstimatedCost:  domain.Budget{Tokens: 64, Attempts: 1},
		}}, nil

	case domain.FamilySourceFreshness:
		newestBySource := map[domain.SourceID]domain.SourceVersion{}
		for _, ver := range versions {
			prev, ok := newestBySource[ver.SourceID]
			if !ok || ver.ObservedAt.After(prev.ObservedAt) || (ver.ObservedAt.Equal(prev.ObservedAt) && string(ver.ID) > string(prev.ID)) {
				newestBySource[ver.SourceID] = ver
			}
		}
		cutoff := now.Add(-defaultSourceFreshnessMaxAge)
		aging := 0
		var sample domain.SourceID
		for _, src := range sources {
			observed := src.ObservedAt
			if ver, ok := newestBySource[src.ID]; ok {
				observed = ver.ObservedAt
			}
			if observed.Before(cutoff) {
				aging++
				if sample == "" {
					sample = src.ID
				}
			}
		}
		if aging == 0 {
			return nil, nil
		}
		novelty := fmt.Sprintf("aging sources=%d sample=%s window=%s", aging, sample, defaultSourceFreshnessMaxAge)
		return []ChildDraft{{
			Title:          "review aging sources past freshness window",
			Origin:         "decompose:source_freshness:findings",
			ExpectedGain:   fmt.Sprintf("reacquisition candidates aging_count=%d", aging),
			Novelty:        novelty,
			StopCondition:  "aging sources listed or deferred",
			DedupSignature: "freshness:aging_sources",
			Risk:           domain.RiskLow,
			Priority:       20,
			EstimatedCost:  domain.Budget{Tokens: 64, Attempts: 1},
		}}, nil

	case domain.FamilyArtifactRefresh:
		staleCandidates := 0
		var sample domain.ArtifactID
		for _, a := range artifacts {
			if a.Stale || isLocalAuditKind(a.Kind) {
				continue
			}
			if a.BaseCommitID == headID {
				continue
			}
			staleCandidates++
			if sample == "" {
				sample = a.ID
			}
		}
		if staleCandidates == 0 {
			return nil, nil
		}
		return []ChildDraft{{
			Title:          "refresh knowledge artifacts behind mission head",
			Origin:         "decompose:artifact_refresh:findings",
			ExpectedGain:   fmt.Sprintf("stale-mark candidates=%d head=%s", staleCandidates, headID),
			Novelty:        fmt.Sprintf("refresh candidates sample=%s head=%s", sample, headID),
			StopCondition:  "stale candidates marked or deferred",
			DedupSignature: "refresh:stale_candidates",
			Risk:           domain.RiskLow,
			Priority:       22,
			EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
		}}, nil

	case domain.FamilyIntegrityAudit:
		orphanEvidence, orphanObs, conflicted, claimsWithoutEv := integrityStructuralCounts(observations, claims, evidence, fragmentByID)
		if orphanEvidence == 0 && orphanObs == 0 && conflicted == 0 && claimsWithoutEv == 0 {
			return nil, nil
		}
		return []ChildDraft{{
			Title:          "audit structural integrity of knowledge graph",
			Origin:         "decompose:integrity_audit:findings",
			ExpectedGain:   fmt.Sprintf("integrity orphans_ev=%d orphans_obs=%d conflicted=%d claims_without_ev=%d", orphanEvidence, orphanObs, conflicted, claimsWithoutEv),
			Novelty:        fmt.Sprintf("integrity inventory at %s", now.UTC().Format(time.RFC3339)),
			StopCondition:  "integrity inventory persisted or deferred",
			DedupSignature: "integrity:structural_inventory",
			Risk:           domain.RiskLow,
			Priority:       21,
			EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
		}}, nil

	case domain.FamilyConflictReview:
		unopposed, conflicted, claimsWithoutEv := conflictStructuralCounts(claims, evidence)
		if unopposed == 0 && conflicted == 0 && claimsWithoutEv == 0 {
			return nil, nil
		}
		return []ChildDraft{{
			Title:          "review unopposed and conflicted claims",
			Origin:         "decompose:conflict:findings",
			ExpectedGain:   fmt.Sprintf("conflict candidates unopposed=%d conflicted=%d claims_without_ev=%d", unopposed, conflicted, claimsWithoutEv),
			Novelty:        fmt.Sprintf("conflict inventory at %s", now.UTC().Format(time.RFC3339)),
			StopCondition:  "each candidate reviewed or deferred",
			DedupSignature: "conflict:evidence_inventory",
			Risk:           domain.RiskMedium,
			Priority:       24,
			EstimatedCost:  domain.Budget{Tokens: 64, Attempts: 1},
		}}, nil

	default:
		return nil, nil
	}
}

func integrityStructuralCounts(
	observations []domain.Observation,
	claims []domain.Claim,
	evidence []domain.EvidenceLink,
	fragmentByID map[domain.SourceFragmentID]domain.SourceFragment,
) (orphanEvidence, orphanObs, conflicted, claimsWithoutEv int) {
	observationByID := map[domain.ObservationID]struct{}{}
	for _, obs := range observations {
		observationByID[obs.ID] = struct{}{}
		if obs.Anchor.SourceFragmentID != "" {
			if _, ok := fragmentByID[obs.Anchor.SourceFragmentID]; !ok {
				orphanObs++
			}
		}
	}
	claimByID := map[domain.ClaimID]struct{}{}
	for _, claim := range claims {
		claimByID[claim.ID] = struct{}{}
	}
	claimHasEvidence := map[domain.ClaimID]struct{}{}
	support := map[domain.ClaimID]int{}
	contradict := map[domain.ClaimID]int{}
	for _, link := range evidence {
		claimHasEvidence[link.ClaimID] = struct{}{}
		if _, ok := observationByID[link.ObservationID]; !ok {
			orphanEvidence++
		} else if _, ok := claimByID[link.ClaimID]; !ok {
			orphanEvidence++
		}
		switch link.Relation {
		case domain.EvidenceSupports:
			support[link.ClaimID]++
		case domain.EvidenceContradicts:
			contradict[link.ClaimID]++
		}
	}
	for id, n := range support {
		if n > 0 && contradict[id] > 0 {
			conflicted++
		}
	}
	for _, claim := range claims {
		if _, ok := claimHasEvidence[claim.ID]; !ok {
			claimsWithoutEv++
		}
	}
	return orphanEvidence, orphanObs, conflicted, claimsWithoutEv
}

func conflictStructuralCounts(claims []domain.Claim, evidence []domain.EvidenceLink) (unopposed, conflicted, claimsWithoutEv int) {
	hasSupport := map[domain.ClaimID]bool{}
	hasContradict := map[domain.ClaimID]bool{}
	claimHasEvidence := map[domain.ClaimID]struct{}{}
	for _, link := range evidence {
		claimHasEvidence[link.ClaimID] = struct{}{}
		switch link.Relation {
		case domain.EvidenceSupports, domain.EvidenceReplicates:
			hasSupport[link.ClaimID] = true
		case domain.EvidenceContradicts, domain.EvidenceFailsToReplicate:
			hasContradict[link.ClaimID] = true
		}
	}
	for _, claim := range claims {
		if _, ok := claimHasEvidence[claim.ID]; !ok {
			claimsWithoutEv++
		}
		if hasSupport[claim.ID] && !hasContradict[claim.ID] {
			unopposed++
		}
	}
	for id := range hasSupport {
		if hasContradict[id] {
			conflicted++
		}
	}
	return unopposed, conflicted, claimsWithoutEv
}

// staticChildDrafts keeps a non-empty portfolio when the store has no gap yet,
// so strategies still exercise single-level decomposition in empty missions.
func staticChildDrafts(family domain.WorkFamily) []ChildDraft {
	switch family {
	case domain.FamilyGapScan:
		return []ChildDraft{{
			Title: "map uncovered mission scopes", Origin: "decompose:gap_scan",
			ExpectedGain: "enumerable scopes without inquiries", Novelty: "scope inventory absent from agenda",
			StopCondition: "scope list persisted", DedupSignature: "gap:scopes",
			Risk: domain.RiskLow, Priority: 25, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
		}}
	case domain.FamilyConflictReview:
		return []ChildDraft{{
			Title: "review claims lacking opposing evidence", Origin: "decompose:conflict",
			ExpectedGain: "conflict or corroboration candidates", Novelty: "unopposed claims scan",
			StopCondition: "each claim reviewed or deferred", DedupSignature: "conflict:unopposed",
			Risk: domain.RiskMedium, Priority: 24, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
		}}
	case domain.FamilyCoverageScan:
		return []ChildDraft{{
			Title: "map mission areas without inquiries", Origin: "decompose:coverage_scan",
			ExpectedGain: "coverage gaps for mission scopes", Novelty: "mission areas lacking admitted work",
			StopCondition: "coverage inventory persisted", DedupSignature: "coverage:mission_areas",
			Risk: domain.RiskLow, Priority: 23, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
		}}
	case domain.FamilySourceFreshness:
		return []ChildDraft{{
			Title: "review aging or stale sources", Origin: "decompose:source_freshness",
			ExpectedGain: "candidates for reacquisition or revalidation", Novelty: "sources past freshness window",
			StopCondition: "stale sources listed or deferred", DedupSignature: "freshness:stale_sources",
			Risk: domain.RiskLow, Priority: 20, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
		}}
	case domain.FamilyFrontierManage:
		return []ChildDraft{{
			Title: "dedupe and diversify open frontier", Origin: "decompose:frontier_manage",
			ExpectedGain: "merged duplicates and diversified candidates", Novelty: "frontier hygiene without new model calls",
			StopCondition: "frontier compact report", DedupSignature: "frontier:hygiene",
			Risk: domain.RiskLow, Priority: 14, EstimatedCost: domain.Budget{Tokens: 32, Attempts: 1},
		}}
	default:
		return nil
	}
}

// resolveChildDrafts prefers store-planned drafts when the family can derive
// them from findings; otherwise falls back to static catalogue drafts.
func resolveChildDrafts(ctx context.Context, store port.Store, family domain.WorkFamily, mission domain.MissionRevisionID, now time.Time, configured []ChildDraft) ([]ChildDraft, error) {
	planned, err := PlanChildDraftsFromStore(ctx, store, family, mission, now)
	if err != nil {
		return nil, err
	}
	if len(planned) > 0 {
		return planned, nil
	}
	if len(configured) > 0 {
		return configured, nil
	}
	// Empty configured + empty plan: use static defaults for families that need
	// a seed child to avoid pure root-only frontiers.
	return staticChildDrafts(family), nil
}

// draftSignaturePrefix is used by tests to assert planner provenance.
func draftSignaturePrefix(drafts []ChildDraft) string {
	if len(drafts) == 0 {
		return ""
	}
	return strings.SplitN(drafts[0].DedupSignature, ":", 2)[0]
}
