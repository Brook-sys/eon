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
// LocalExecutor and returns actionable child drafts for the family.
// Structural families may emit multiple split drafts (capped by HorizonPolicy
// max_children); empty result means the static ChildDrafts (if any) should be used.
func PlanChildDraftsFromStore(ctx context.Context, store port.Store, family domain.WorkFamily, mission domain.MissionRevisionID, now time.Time) ([]ChildDraft, error) {
	return PlanChildDraftsFromStoreWithPolicy(ctx, store, family, mission, now, domain.DefaultHorizonPolicy())
}

// PlanChildDraftsFromStoreWithPolicy is the policy-aware planner used when the
// active HorizonPolicy is already resolved (fan-out cap, deterministic order).
func PlanChildDraftsFromStoreWithPolicy(ctx context.Context, store port.Store, family domain.WorkFamily, mission domain.MissionRevisionID, now time.Time, policy domain.HorizonPolicy) ([]ChildDraft, error) {
	if store == nil || mission == "" || !family.Valid() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := policy.Validate(); err != nil {
		policy = domain.DefaultHorizonPolicy()
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

	stamp := now.UTC().Format(time.RFC3339)
	switch family {
	case domain.FamilyGapScan:
		_, withoutObs, withoutFrag, fragsWithoutObs := coverageJoin(sources, versionByID, fragmentByID, observations)
		if withoutObs == 0 && withoutFrag == 0 && fragsWithoutObs == 0 {
			return nil, nil
		}
		drafts := make([]ChildDraft, 0, 3)
		if withoutFrag > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "enumerate sources lacking fragments",
				Origin:         "decompose:gap_scan:split:without_frag",
				ExpectedGain:   fmt.Sprintf("sources_without_fragment=%d", withoutFrag),
				Novelty:        fmt.Sprintf("gap split without_frag at %s", stamp),
				StopCondition:  "sources without fragments listed or deferred",
				DedupSignature: "gap:without_frag",
				Risk:           domain.RiskLow,
				Priority:       26,
				EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
			})
		}
		if withoutObs > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "enumerate sources lacking observations",
				Origin:         "decompose:gap_scan:split:without_obs",
				ExpectedGain:   fmt.Sprintf("sources_without_observation=%d", withoutObs),
				Novelty:        fmt.Sprintf("gap split without_obs at %s", stamp),
				StopCondition:  "sources without observations listed or deferred",
				DedupSignature: "gap:without_obs",
				Risk:           domain.RiskLow,
				Priority:       25,
				EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
			})
		}
		if fragsWithoutObs > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "enumerate fragments lacking observations",
				Origin:         "decompose:gap_scan:split:frags_without_obs",
				ExpectedGain:   fmt.Sprintf("fragments_without_observation=%d", fragsWithoutObs),
				Novelty:        fmt.Sprintf("gap split frags_without_obs at %s", stamp),
				StopCondition:  "fragments without observations listed or deferred",
				DedupSignature: "gap:frags_without_obs",
				Risk:           domain.RiskLow,
				Priority:       24,
				EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
			})
		}
		return capChildDrafts(drafts, policy.MaxChildren), nil

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
		drafts := make([]ChildDraft, 0, 4)
		if withoutFrag > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "map sources without fragment coverage",
				Origin:         "decompose:coverage_scan:split:without_frag",
				ExpectedGain:   fmt.Sprintf("coverage without_frag=%d", withoutFrag),
				Novelty:        fmt.Sprintf("coverage split without_frag at %s", stamp),
				StopCondition:  "sources without fragments covered or deferred",
				DedupSignature: "coverage:without_frag",
				Risk:           domain.RiskLow,
				Priority:       24,
				EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
			})
		}
		if withoutObs > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "map sources without observation coverage",
				Origin:         "decompose:coverage_scan:split:without_obs",
				ExpectedGain:   fmt.Sprintf("coverage without_obs=%d", withoutObs),
				Novelty:        fmt.Sprintf("coverage split without_obs at %s", stamp),
				StopCondition:  "sources without observations covered or deferred",
				DedupSignature: "coverage:without_obs",
				Risk:           domain.RiskLow,
				Priority:       23,
				EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
			})
		}
		if fragsWithoutObs > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "map fragments without observation coverage",
				Origin:         "decompose:coverage_scan:split:frags_without_obs",
				ExpectedGain:   fmt.Sprintf("coverage frags_without_obs=%d", fragsWithoutObs),
				Novelty:        fmt.Sprintf("coverage split frags_without_obs at %s", stamp),
				StopCondition:  "fragments without observations covered or deferred",
				DedupSignature: "coverage:frags_without_obs",
				Risk:           domain.RiskLow,
				Priority:       22,
				EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
			})
		}
		if claimsWithoutEv > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "map claims without evidence coverage",
				Origin:         "decompose:coverage_scan:split:claims_without_ev",
				ExpectedGain:   fmt.Sprintf("coverage claims_without_ev=%d", claimsWithoutEv),
				Novelty:        fmt.Sprintf("coverage split claims_without_ev at %s", stamp),
				StopCondition:  "claims without evidence covered or deferred",
				DedupSignature: "coverage:claims_without_ev",
				Risk:           domain.RiskMedium,
				Priority:       21,
				EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
			})
		}
		return capChildDrafts(drafts, policy.MaxChildren), nil

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
		drafts := make([]ChildDraft, 0, 4)
		if orphanEvidence > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "audit orphan evidence links",
				Origin:         "decompose:integrity_audit:split:orphan_ev",
				ExpectedGain:   fmt.Sprintf("orphan_evidence=%d", orphanEvidence),
				Novelty:        fmt.Sprintf("integrity split orphan_ev at %s", stamp),
				StopCondition:  "orphan evidence listed or deferred",
				DedupSignature: "integrity:orphan_evidence",
				Risk:           domain.RiskLow,
				Priority:       23,
				EstimatedCost:  domain.Budget{Tokens: 40, Attempts: 1},
			})
		}
		if orphanObs > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "audit observations with missing fragment anchors",
				Origin:         "decompose:integrity_audit:split:orphan_obs",
				ExpectedGain:   fmt.Sprintf("orphan_observations=%d", orphanObs),
				Novelty:        fmt.Sprintf("integrity split orphan_obs at %s", stamp),
				StopCondition:  "orphan observations listed or deferred",
				DedupSignature: "integrity:orphan_observations",
				Risk:           domain.RiskLow,
				Priority:       22,
				EstimatedCost:  domain.Budget{Tokens: 40, Attempts: 1},
			})
		}
		if conflicted > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "audit claims with supporting and contradicting evidence",
				Origin:         "decompose:integrity_audit:split:conflicted",
				ExpectedGain:   fmt.Sprintf("conflicted_claims=%d", conflicted),
				Novelty:        fmt.Sprintf("integrity split conflicted at %s", stamp),
				StopCondition:  "conflicted claims listed or deferred",
				DedupSignature: "integrity:conflicted_claims",
				Risk:           domain.RiskMedium,
				Priority:       21,
				EstimatedCost:  domain.Budget{Tokens: 40, Attempts: 1},
			})
		}
		if claimsWithoutEv > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "audit claims without evidence",
				Origin:         "decompose:integrity_audit:split:claims_without_ev",
				ExpectedGain:   fmt.Sprintf("claims_without_evidence=%d", claimsWithoutEv),
				Novelty:        fmt.Sprintf("integrity split claims_without_ev at %s", stamp),
				StopCondition:  "claims without evidence listed or deferred",
				DedupSignature: "integrity:claims_without_ev",
				Risk:           domain.RiskLow,
				Priority:       20,
				EstimatedCost:  domain.Budget{Tokens: 40, Attempts: 1},
			})
		}
		return capChildDrafts(drafts, policy.MaxChildren), nil

	case domain.FamilyConflictReview:
		unopposed, conflicted, claimsWithoutEv := conflictStructuralCounts(claims, evidence)
		if unopposed == 0 && conflicted == 0 && claimsWithoutEv == 0 {
			return nil, nil
		}
		drafts := make([]ChildDraft, 0, 3)
		if conflicted > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "review claims with opposing evidence",
				Origin:         "decompose:conflict:split:conflicted",
				ExpectedGain:   fmt.Sprintf("conflicted=%d", conflicted),
				Novelty:        fmt.Sprintf("conflict split conflicted at %s", stamp),
				StopCondition:  "conflicted claims reviewed or deferred",
				DedupSignature: "conflict:conflicted",
				Risk:           domain.RiskMedium,
				Priority:       26,
				EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
			})
		}
		if unopposed > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "review claims lacking opposing evidence",
				Origin:         "decompose:conflict:split:unopposed",
				ExpectedGain:   fmt.Sprintf("unopposed=%d", unopposed),
				Novelty:        fmt.Sprintf("conflict split unopposed at %s", stamp),
				StopCondition:  "unopposed claims reviewed or deferred",
				DedupSignature: "conflict:unopposed_inventory",
				Risk:           domain.RiskMedium,
				Priority:       24,
				EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
			})
		}
		if claimsWithoutEv > 0 {
			drafts = append(drafts, ChildDraft{
				Title:          "review claims without any evidence",
				Origin:         "decompose:conflict:split:claims_without_ev",
				ExpectedGain:   fmt.Sprintf("claims_without_ev=%d", claimsWithoutEv),
				Novelty:        fmt.Sprintf("conflict split claims_without_ev at %s", stamp),
				StopCondition:  "claims without evidence reviewed or deferred",
				DedupSignature: "conflict:claims_without_ev",
				Risk:           domain.RiskLow,
				Priority:       22,
				EstimatedCost:  domain.Budget{Tokens: 48, Attempts: 1},
			})
		}
		return capChildDrafts(drafts, policy.MaxChildren), nil

	case domain.FamilyHarnessEvaluation:
		// Offline compile inventory is always actionable without a provider.
		return []ChildDraft{{
			Title:          "compile cognitive fixture matrix offline",
			Origin:         "decompose:harness_evaluation:offline",
			ExpectedGain:   "compile-only 2k/4k/8k matrix over cognitive-v1",
			Novelty:        fmt.Sprintf("offline harness plan at %s", now.UTC().Format(time.RFC3339)),
			StopCondition:  "offline compile report persisted",
			DedupSignature: "harness:offline_compile",
			Risk:           domain.RiskLow,
			Priority:       18,
			EstimatedCost:  domain.Budget{Tokens: 96, Attempts: 1},
		}}, nil

	case domain.FamilyFrontierManage:
		open, err := listOpenAndAdmitted(ctx, store, mission)
		if err != nil {
			return nil, err
		}
		dupes, families, depthMax := frontierHygieneCounts(open)
		if len(open) == 0 && dupes == 0 {
			return nil, nil
		}
		return []ChildDraft{{
			Title:          "hygiene open frontier signatures and depth",
			Origin:         "decompose:frontier_manage:findings",
			ExpectedGain:   fmt.Sprintf("frontier open=%d dupes=%d families=%d depth_max=%d", len(open), dupes, families, depthMax),
			Novelty:        fmt.Sprintf("frontier hygiene inventory at %s", now.UTC().Format(time.RFC3339)),
			StopCondition:  "frontier compact report persisted",
			DedupSignature: "frontier:hygiene_inventory",
			Risk:           domain.RiskLow,
			Priority:       14,
			EstimatedCost:  domain.Budget{Tokens: 32, Attempts: 1},
		}}, nil

	default:
		return nil, nil
	}
}

func listOpenAndAdmitted(ctx context.Context, store port.Store, mission domain.MissionRevisionID) ([]domain.WorkOpportunity, error) {
	var items []domain.WorkOpportunity
	err := store.View(ctx, func(r port.Reader) error {
		open, err := r.WorkOpportunities(mission, domain.OpportunityOpen)
		if err != nil {
			return err
		}
		admitted, err := r.WorkOpportunities(mission, domain.OpportunityAdmitted)
		if err != nil {
			return err
		}
		items = append(items, open...)
		items = append(items, admitted...)
		return nil
	})
	return items, err
}

func frontierHygieneCounts(items []domain.WorkOpportunity) (dupes, familyCount, depthMax int) {
	sigCount := map[string]int{}
	families := map[domain.WorkFamily]struct{}{}
	for _, opp := range items {
		sigCount[opp.DedupSignature]++
		families[opp.Family] = struct{}{}
		if opp.Depth > depthMax {
			depthMax = opp.Depth
		}
	}
	for _, n := range sigCount {
		if n > 1 {
			dupes += n - 1
		}
	}
	return dupes, len(families), depthMax
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
	case domain.FamilyHarnessEvaluation:
		return []ChildDraft{{
			Title: "compile cognitive fixture matrix offline", Origin: "decompose:harness_evaluation",
			ExpectedGain: "compile-only matrix without provider", Novelty: "offline harness seed",
			StopCondition: "offline compile report", DedupSignature: "harness:offline_compile",
			Risk: domain.RiskLow, Priority: 18, EstimatedCost: domain.Budget{Tokens: 96, Attempts: 1},
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
// policy caps fan-out for multi-draft structural splits (max_children).
func resolveChildDrafts(ctx context.Context, store port.Store, family domain.WorkFamily, mission domain.MissionRevisionID, now time.Time, configured []ChildDraft, policy domain.HorizonPolicy) ([]ChildDraft, error) {
	planned, err := PlanChildDraftsFromStoreWithPolicy(ctx, store, family, mission, now, policy)
	if err != nil {
		return nil, err
	}
	if len(planned) > 0 {
		return planned, nil
	}
	if len(configured) > 0 {
		return capChildDrafts(configured, policy.MaxChildren), nil
	}
	// Empty configured + empty plan: use static defaults for families that need
	// a seed child to avoid pure root-only frontiers.
	return capChildDrafts(staticChildDrafts(family), policy.MaxChildren), nil
}

// capChildDrafts enforces HorizonPolicy.MaxChildren on planned drafts.
// Order is preserved (callers emit highest-priority splits first).
// max <= 0 returns nil so invalid policy never implies unbounded fan-out.
func capChildDrafts(drafts []ChildDraft, max int) []ChildDraft {
	if len(drafts) == 0 || max <= 0 {
		return nil
	}
	if len(drafts) <= max {
		return drafts
	}
	return append([]ChildDraft(nil), drafts[:max]...)
}

// draftSignaturePrefix is used by tests to assert planner provenance.
func draftSignaturePrefix(drafts []ChildDraft) string {
	if len(drafts) == 0 {
		return ""
	}
	return strings.SplitN(drafts[0].DedupSignature, ":", 2)[0]
}
