package kernel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// LocalFamilyStrategy is a model-free continuity family that seeds a root
// opportunity when needed, optionally decomposes once, and admits from the
// frontier until the ready horizon is healthy or no OPEN work remains.
type LocalFamilyStrategy struct {
	NameValue string
	Family    domain.WorkFamily
	Store     port.Store
	Clock     source.Clock
	IDs       source.IDGenerator
	Policy    domain.HorizonPolicy
	// ChildDrafts, when non-empty, are spawned from the root once if the root
	// is still OPEN and has no children. Pure paraphrase drafts are rejected.
	ChildDrafts []ChildDraft
}

func (s LocalFamilyStrategy) Name() string {
	if strings.TrimSpace(s.NameValue) != "" {
		return s.NameValue
	}
	return string(s.Family)
}

func (s LocalFamilyStrategy) Replenish(ctx context.Context, mission domain.MissionRevisionID) (ContinuityResult, error) {
	if s.Store == nil || s.Clock == nil || s.IDs == nil {
		return ContinuityResult{}, errors.New("local family strategy dependencies are incomplete")
	}
	if mission == "" || !s.Family.Valid() {
		return ContinuityResult{}, errors.New("local family strategy requires mission and valid family")
	}
	policy := s.Policy
	if policy.Version == "" && policy.SchemaVersion == 0 {
		policy = domain.DefaultHorizonPolicy()
	}
	if err := policy.Validate(); err != nil {
		return ContinuityResult{}, err
	}
	if err := EnsureCatalogSpecs(ctx, s.Store, DefaultFamilySpecCatalog()); err != nil {
		return ContinuityResult{}, err
	}

	result := ContinuityResult{}
	replenisher := Replenisher{Store: s.Store, Clock: s.Clock, IDs: s.IDs, Policy: policy}

	// 1. Ensure a root seed for this family exists (idempotent by signature).
	var revision domain.MissionRevision
	if err := s.Store.View(ctx, func(r port.Reader) error {
		var err error
		revision, err = r.MissionRevision(mission)
		return err
	}); err != nil {
		return ContinuityResult{}, err
	}
	rootID, err := s.IDs.NewID("opportunity")
	if err != nil {
		return ContinuityResult{}, fmt.Errorf("generate root opportunity id: %w", err)
	}
	root, err := RootOpportunityFromMission(revision, s.Family, domain.WorkOpportunityID(rootID), s.Clock.Now())
	if err != nil {
		return ContinuityResult{}, err
	}
	// Stable signature keeps reseeding idempotent across strategy calls.
	root.ID = domain.WorkOpportunityID("opp_" + string(s.Family) + "_" + string(mission))
	root.Priority = familyPriority(s.Family)
	seeded, err := replenisher.SeedRootOpportunity(ctx, root)
	if err != nil {
		return ContinuityResult{}, err
	}
	if seeded {
		result.Changed = true
	}

	// 2. Optional single-level decomposition of the still-open root.
	// Prefer store-planned drafts (gap/coverage/freshness/refresh findings);
	// fall back to configured/static drafts so empty missions still fan out once.
	drafts, err := resolveChildDrafts(ctx, s.Store, s.Family, mission, s.Clock.Now().UTC(), s.ChildDrafts, policy)
	if err != nil {
		return ContinuityResult{}, err
	}
	if len(drafts) > 0 {
		decomposer := Decomposer{Store: s.Store, Clock: s.Clock, IDs: s.IDs, Policy: policy}
		var parentID domain.WorkOpportunityID
		var shouldSpawn bool
		if err := s.Store.View(ctx, func(r port.Reader) error {
			parent, err := r.WorkOpportunity(root.ID)
			if err != nil {
				if errors.Is(err, port.ErrNotFound) {
					// Seed may have been a no-op because another id already holds the signature.
					items, listErr := r.WorkOpportunities(mission, domain.OpportunityOpen)
					if listErr != nil {
						return listErr
					}
					for _, item := range items {
						if item.Family == s.Family && item.Depth == 0 {
							parent = item
							err = nil
							break
						}
					}
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}
			if !parent.Status.Active() {
				return nil
			}
			parentID = parent.ID
			children, err := r.WorkOpportunities(mission, "")
			if err != nil {
				return err
			}
			for _, child := range children {
				if child.ParentID == parent.ID {
					return nil
				}
			}
			shouldSpawn = true
			return nil
		}); err != nil {
			return ContinuityResult{}, err
		}
		if shouldSpawn && parentID != "" {
			if _, err := decomposer.SpawnChildren(ctx, parentID, drafts); err != nil {
				// Fan-out/dedup conflicts are non-fatal for the strategy; admission may still proceed.
				if !errors.Is(err, port.ErrConflict) && !strings.Contains(err.Error(), "reached policy max") {
					return ContinuityResult{}, err
				}
			} else {
				result.Changed = true
			}
		}
	}

	// 3. Admit open opportunities into the executable horizon.
	admit, err := Admitter{Store: s.Store, Clock: s.Clock, IDs: s.IDs, Policy: policy}.AdmitFromFrontier(ctx, mission)
	if err != nil {
		return ContinuityResult{}, err
	}
	result.Admitted += admit.Admitted
	if admit.Changed {
		result.Changed = true
	}
	return result, nil
}

func familyPriority(family domain.WorkFamily) uint8 {
	switch family {
	case domain.FamilyGapScan:
		return 30
	case domain.FamilyConflictReview:
		return 28
	case domain.FamilyCoverageScan:
		return 26
	case domain.FamilyArtifactRefresh:
		return 24
	case domain.FamilySourceFreshness:
		return 22
	case domain.FamilyIntegrityAudit:
		return 20
	case domain.FamilyHarnessEvaluation:
		return 18
	case domain.FamilyFrontierManage:
		return 16
	default:
		return 10
	}
}

// RegisterDefaultContinuityFamilies installs the initial local portfolio.
// Strategies remain model-free so continuity works without network or LLM.
// Mission-declared FR-DUR-011 obligations are registered first so cadence seeds
// run before opportunistic family scans.
func RegisterDefaultContinuityFamilies(reg *StrategyRegistry, store port.Store, clock source.Clock, ids source.IDGenerator, policy domain.HorizonPolicy) error {
	if reg == nil {
		return errors.New("strategy registry is required")
	}
	if policy.Version == "" && policy.SchemaVersion == 0 {
		policy = domain.DefaultHorizonPolicy()
	}
	if err := EnsureRecurringStrategy(reg, store, clock, ids, policy); err != nil {
		return err
	}
	families := []struct {
		name     string
		family   domain.WorkFamily
		priority uint8
		local    bool
		children []ChildDraft
	}{
		{
			name: "gap_scan", family: domain.FamilyGapScan, priority: 30, local: true,
			children: []ChildDraft{{
				Title: "map uncovered mission scopes", Origin: "decompose:gap_scan",
				ExpectedGain: "enumerable scopes without inquiries", Novelty: "scope inventory absent from agenda",
				StopCondition: "scope list persisted", DedupSignature: "gap:scopes",
				Risk: domain.RiskLow, Priority: 25, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
			}},
		},
		{
			name: "conflict_evidence_review", family: domain.FamilyConflictReview, priority: 28, local: true,
			children: []ChildDraft{{
				Title: "review claims lacking opposing evidence", Origin: "decompose:conflict",
				ExpectedGain: "conflict or corroboration candidates", Novelty: "unopposed claims scan",
				StopCondition: "each claim reviewed or deferred", DedupSignature: "conflict:unopposed",
				Risk: domain.RiskMedium, Priority: 24, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
			}},
		},
		{
			name: "mission_coverage_scan", family: domain.FamilyCoverageScan, priority: 26, local: true,
			children: []ChildDraft{{
				Title: "map mission areas without inquiries", Origin: "decompose:coverage_scan",
				ExpectedGain: "coverage gaps for mission scopes", Novelty: "mission areas lacking admitted work",
				StopCondition: "coverage inventory persisted", DedupSignature: "coverage:mission_areas",
				Risk: domain.RiskLow, Priority: 23, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
			}},
		},
		{
			name: "artifact_refresh", family: domain.FamilyArtifactRefresh, priority: 24, local: true,
		},
		{
			name: "source_freshness_scan", family: domain.FamilySourceFreshness, priority: 22, local: true,
			children: []ChildDraft{{
				Title: "review aging or stale sources", Origin: "decompose:source_freshness",
				ExpectedGain: "candidates for reacquisition or revalidation", Novelty: "sources past freshness window",
				StopCondition: "stale sources listed or deferred", DedupSignature: "freshness:stale_sources",
				Risk: domain.RiskLow, Priority: 20, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
			}},
		},
		{
			name: "integrity_audit", family: domain.FamilyIntegrityAudit, priority: 20, local: true,
		},
		{
			name: "harness_evaluation", family: domain.FamilyHarnessEvaluation, priority: 18, local: true,
		},
		{
			name: "frontier_management", family: domain.FamilyFrontierManage, priority: 16, local: true,
			children: []ChildDraft{{
				Title: "dedupe and diversify open frontier", Origin: "decompose:frontier_manage",
				ExpectedGain: "merged duplicates and diversified candidates", Novelty: "frontier hygiene without new model calls",
				StopCondition: "frontier compact report", DedupSignature: "frontier:hygiene",
				Risk: domain.RiskLow, Priority: 14, EstimatedCost: domain.Budget{Tokens: 32, Attempts: 1},
			}},
		},
	}
	for _, item := range families {
		strategy := LocalFamilyStrategy{
			NameValue:   item.name,
			Family:      item.family,
			Store:       store,
			Clock:       clock,
			IDs:         ids,
			Policy:      policy,
			ChildDrafts: item.children,
		}
		if err := reg.Register(StrategyDescriptor{
			Name:      item.name,
			Family:    item.family,
			Version:   "v2",
			Priority:  item.priority,
			LocalOnly: item.local,
		}, strategy); err != nil {
			return err
		}
	}
	// Portfolio revision is explicit: recurring obligations land in v3 alongside the v2 family set.
	reg.SetCatalogVersion(DefaultContinuityCatalogVersion)
	return nil
}
