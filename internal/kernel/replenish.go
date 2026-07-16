package kernel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// Replenisher keeps the executable horizon near target_ready using the durable
// work frontier. It never invents model authority: only OPEN opportunities
// that already passed deterministic validation may be admitted.
type Replenisher struct {
	Store      port.Store
	Clock      source.Clock
	IDs        source.IDGenerator
	Policy     domain.HorizonPolicy
	Admitter   *Admitter
	Decomposer *Decomposer
}

func (r Replenisher) policy() domain.HorizonPolicy {
	if r.Policy.Version == "" && r.Policy.SchemaVersion == 0 {
		return domain.DefaultHorizonPolicy()
	}
	return r.Policy
}

func (r Replenisher) admitter() Admitter {
	if r.Admitter != nil {
		return *r.Admitter
	}
	return Admitter{Store: r.Store, Clock: r.Clock, IDs: r.IDs, Policy: r.policy()}
}

// PreventivelyReplenish admits from the frontier when ready work is at or
// below low_watermark. When no OPEN opportunity exists but DEFERRED or parent
// roots can spawn children, callers should run family strategies first.
func (r Replenisher) PreventivelyReplenish(ctx context.Context, mission domain.MissionRevisionID) (ContinuityResult, domain.ExecutableHorizon, error) {
	if r.Store == nil || r.Clock == nil {
		return ContinuityResult{}, domain.ExecutableHorizon{}, errors.New("replenisher dependencies are incomplete")
	}
	if mission == "" {
		return ContinuityResult{}, domain.ExecutableHorizon{}, errors.New("mission revision is required")
	}
	policy := r.policy()
	if err := policy.Validate(); err != nil {
		return ContinuityResult{}, domain.ExecutableHorizon{}, fmt.Errorf("horizon policy: %w", err)
	}

	horizon, err := observeHorizon(r.Store, r.Clock, mission, policy)
	if err != nil {
		return ContinuityResult{}, domain.ExecutableHorizon{}, err
	}
	if !horizon.NeedsReplenishment() {
		return ContinuityResult{}, horizon, nil
	}
	result, err := r.admitter().AdmitFromFrontier(ctx, mission)
	if err != nil {
		return ContinuityResult{}, horizon, err
	}
	horizon, err = observeHorizon(r.Store, r.Clock, mission, policy)
	if err != nil {
		return result, domain.ExecutableHorizon{}, err
	}
	return result, horizon, nil
}

// SeedRootOpportunity creates a single OPEN root when the frontier has no
// active opportunity with the same dedup signature. Used by local strategies.
func (r Replenisher) SeedRootOpportunity(ctx context.Context, opportunity domain.WorkOpportunity) (bool, error) {
	if r.Store == nil || r.Clock == nil {
		return false, errors.New("replenisher dependencies are incomplete")
	}
	if err := opportunity.Validate(); err != nil {
		return false, err
	}
	if opportunity.Status != domain.OpportunityOpen {
		return false, errors.New("seed opportunity must be OPEN")
	}
	if opportunity.ParentID != "" || opportunity.Depth != 0 {
		return false, errors.New("seed opportunity must be a depth-0 root")
	}
	policy := r.policy()
	if err := policy.Validate(); err != nil {
		return false, err
	}

	created := false
	err := r.Store.Update(ctx, func(tx port.Transaction) error {
		if _, err := tx.MissionRevision(opportunity.MissionRevision); err != nil {
			return err
		}
		existing, err := tx.WorkOpportunities(opportunity.MissionRevision, "")
		if err != nil {
			return err
		}
		active := 0
		for _, item := range existing {
			// Any prior opportunity with the same semantic signature blocks reseeding,
			// including ADMITTED/ABANDONED roots. Re-creating the same root after
			// admission would be artificial activity without a new delta.
			if item.DedupSignature == opportunity.DedupSignature || item.ID == opportunity.ID {
				return nil
			}
			if item.Status.Active() {
				active++
			}
		}
		if active >= policy.MaxCandidates {
			return fmt.Errorf("%w: open candidate frontier at max_candidates=%d", port.ErrConflict, policy.MaxCandidates)
		}
		now := r.Clock.Now().UTC()
		opportunity.CreatedAt = now
		opportunity.UpdatedAt = now
		if opportunity.SchemaVersion == 0 {
			opportunity.SchemaVersion = domain.SchemaVersionV1
		}
		if err := opportunity.Validate(); err != nil {
			return err
		}
		if err := tx.CreateWorkOpportunity(opportunity); err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(string(opportunity.ID) + ":seeded"),
			Kind:            domain.EventContinuityGapDetected,
			OccurredAt:      now,
			MissionRevision: opportunity.MissionRevision,
			PayloadRef:      string(opportunity.ID),
		})
		if err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

// EnsureCatalogSpecs appends missing continuity OperationSpecs for the default
// family catalogue. Existing ids are left untouched.
func EnsureCatalogSpecs(ctx context.Context, store port.Store, catalog FamilySpecCatalog) error {
	if store == nil {
		return errors.New("store is required")
	}
	if len(catalog) == 0 {
		catalog = DefaultFamilySpecCatalog()
	}
	return store.Update(ctx, func(tx port.Transaction) error {
		for family, id := range catalog {
			if _, err := tx.OperationSpec(id); err == nil {
				continue
			} else if !errors.Is(err, port.ErrNotFound) {
				return err
			}
			authority := domain.AuthorityProposeOnly
			if family == domain.FamilyIntegrityAudit || family == domain.FamilyFrontierManage {
				authority = domain.AuthorityReadOnly
			}
			spec := ContinuityOperationSpec(id, authority)
			if err := tx.AppendOperationSpec(spec); err != nil {
				return fmt.Errorf("append continuity spec %s: %w", id, err)
			}
		}
		return nil
	})
}

func observeHorizon(store port.Store, clock source.Clock, mission domain.MissionRevisionID, policy domain.HorizonPolicy) (domain.ExecutableHorizon, error) {
	var horizon domain.ExecutableHorizon
	err := store.View(context.Background(), func(r port.Reader) error {
		operations, err := r.Operations(mission)
		if err != nil {
			return err
		}
		ready := 0
		for _, operation := range operations {
			if operation.State == domain.StateReady {
				ready++
			}
		}
		open := 0
		for _, status := range []domain.WorkOpportunityStatus{domain.OpportunityOpen, domain.OpportunityDeferred} {
			items, err := r.WorkOpportunities(mission, status)
			if err != nil {
				return err
			}
			open += len(items)
		}
		horizon = domain.ExecutableHorizon{
			SchemaVersion:   domain.SchemaVersionV1,
			MissionRevision: mission,
			PolicyVersion:   policy.Version,
			ReadyCount:      ready,
			OpenCandidates:  open,
			TargetReady:     policy.TargetReady,
			LowWatermark:    policy.LowWatermark,
			MaxReady:        policy.MaxReady,
			ObservedAt:      clock.Now().UTC(),
		}
		return horizon.Validate()
	})
	if err != nil {
		return domain.ExecutableHorizon{}, fmt.Errorf("observe executable horizon: %w", err)
	}
	return horizon, nil
}

// RootOpportunityFromMission derives a deterministic root seed from mission text.
func RootOpportunityFromMission(mission domain.MissionRevision, family domain.WorkFamily, id domain.WorkOpportunityID, now time.Time) (domain.WorkOpportunity, error) {
	if id == "" || !family.Valid() {
		return domain.WorkOpportunity{}, errors.New("root opportunity requires id and valid family")
	}
	if err := mission.Validate(); err != nil {
		return domain.WorkOpportunity{}, err
	}
	title := strings.TrimSpace(mission.Purpose)
	if title == "" {
		title = "advance mission"
	}
	opp := domain.WorkOpportunity{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              id,
		MissionRevision: mission.ID,
		Family:          family,
		Status:          domain.OpportunityOpen,
		Title:           "continuity: " + title,
		Origin:          "mission:" + string(mission.ID),
		ExpectedGain:    "admit executable work aligned to mission purpose",
		Novelty:         "root seed for family " + string(family) + " on revision " + string(mission.ID),
		StopCondition:   "family produces verified delta or is superseded",
		DedupSignature:  "root:" + string(family) + ":" + string(mission.ID),
		Depth:           0,
		EstimatedCost:   domain.Budget{Tokens: 128, Attempts: 1},
		Risk:            domain.RiskLow,
		Priority:        20,
		CreatedAt:       now.UTC(),
		UpdatedAt:       now.UTC(),
	}
	if err := opp.Validate(); err != nil {
		return domain.WorkOpportunity{}, err
	}
	return opp, nil
}
