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

// RecurringSeeder materialises mission-declared FR-DUR-011 obligations into
// frontier WorkOpportunity roots. Pure planning lives in domain.PlanRecurringSeeds;
// this type only persists and admits when capacity allows.
type RecurringSeeder struct {
	Store  port.Store
	Clock  source.Clock
	IDs    source.IDGenerator
	Policy domain.HorizonPolicy
}

// SeedDue creates OPEN opportunities for due recurring obligations of the
// active mission revision. It is idempotent within a cadence bucket via
// DedupSignature and never invents work without a declared obligation.
func (s RecurringSeeder) SeedDue(ctx context.Context, mission domain.MissionRevisionID) (ContinuityResult, error) {
	if s.Store == nil || s.Clock == nil || s.IDs == nil {
		return ContinuityResult{}, errors.New("recurring seeder dependencies are incomplete")
	}
	if mission == "" {
		return ContinuityResult{}, errors.New("mission revision is required")
	}
	policy := s.Policy
	if policy.Version == "" && policy.SchemaVersion == 0 {
		policy = domain.DefaultHorizonPolicy()
	}
	if err := policy.Validate(); err != nil {
		return ContinuityResult{}, err
	}

	var revision domain.MissionRevision
	var existing []domain.WorkOpportunity
	var fingerprint string
	if err := s.Store.View(ctx, func(r port.Reader) error {
		var err error
		revision, err = r.MissionRevision(mission)
		if err != nil {
			return err
		}
		existing, err = r.WorkOpportunities(mission, "")
		if err != nil {
			return err
		}
		// Head commit (when present) is a cheap state fingerprint for mid-window deltas.
		if head, err := r.HeadCommit(mission); err == nil {
			fingerprint = string(head.ID)
		}
		return nil
	}); err != nil {
		return ContinuityResult{}, err
	}
	if len(revision.RecurringObligations) == 0 {
		return ContinuityResult{}, nil
	}

	plans, err := domain.PlanRecurringSeeds(revision.RecurringObligations, existing, mission, s.Clock.Now().UTC(), fingerprint)
	if err != nil {
		return ContinuityResult{}, err
	}
	if len(plans) == 0 {
		return ContinuityResult{}, nil
	}
	if err := EnsureCatalogSpecs(ctx, s.Store, DefaultFamilySpecCatalog()); err != nil {
		return ContinuityResult{}, err
	}

	result := ContinuityResult{}
	replenisher := Replenisher{Store: s.Store, Clock: s.Clock, IDs: s.IDs, Policy: policy}
	now := s.Clock.Now().UTC()

	for _, plan := range plans {
		id, err := s.IDs.NewID("opportunity")
		if err != nil {
			return result, fmt.Errorf("generate recurring opportunity id: %w", err)
		}
		// Deterministic id from signature keeps crash-replay stable when the
		// ID generator advances; SeedRootOpportunity still dedups by signature.
		stableID := domain.WorkOpportunityID("opp_recurring_" + sanitizeID(plan.DedupSignature))
		if len(stableID) > 200 {
			stableID = domain.WorkOpportunityID(id)
		}
		opp := domain.WorkOpportunity{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              stableID,
			MissionRevision: mission,
			Family:          plan.Family,
			Status:          domain.OpportunityOpen,
			Title:           plan.Title,
			Origin:          plan.Origin,
			ExpectedGain:    plan.ExpectedGain,
			Novelty:         plan.Novelty,
			StopCondition:   plan.StopCondition,
			DedupSignature:  plan.DedupSignature,
			Depth:           0,
			EstimatedCost:   plan.Budget,
			Risk:            domain.RiskLow,
			Priority:        plan.Priority,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := opp.Validate(); err != nil {
			return result, fmt.Errorf("build recurring opportunity %s: %w", plan.ObligationID, err)
		}
		seeded, err := replenisher.SeedRootOpportunity(ctx, opp)
		if err != nil {
			// Max candidates / conflict: stop materialising further seeds this cycle.
			if errors.Is(err, port.ErrConflict) {
				break
			}
			return result, fmt.Errorf("seed recurring %s: %w", plan.ObligationID, err)
		}
		if seeded {
			result.Changed = true
			// Audit event with obligation metadata (SeedRoot already emits gap event;
			// append a dedicated kind via store for inspectability).
			_ = s.Store.Update(ctx, func(tx port.Transaction) error {
				eventID, err := s.IDs.NewID("event")
				if err != nil {
					return err
				}
				_, err = tx.AppendEvent(domain.Event{
					SchemaVersion:   domain.SchemaVersionV1,
					ID:              domain.EventID(eventID),
					Kind:            domain.EventContinuityRecurringSeeded,
					OccurredAt:      now,
					MissionRevision: mission,
					PayloadRef:      string(opp.ID) + "|" + plan.ObligationID + "|" + plan.Reason,
				})
				return err
			})
		}
	}

	// Admit OPEN frontier into the executable horizon after seeds.
	admit, err := Admitter{Store: s.Store, Clock: s.Clock, IDs: s.IDs, Policy: policy}.AdmitFromFrontier(ctx, mission)
	if err != nil {
		return result, err
	}
	result.Admitted += admit.Admitted
	if admit.Changed {
		result.Changed = true
	}
	return result, nil
}

func sanitizeID(sig string) string {
	var b strings.Builder
	for _, r := range sig {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ':' || r == '_' || r == '-':
			b.WriteByte('_')
		default:
			b.WriteByte('x')
		}
		if b.Len() >= 120 {
			break
		}
	}
	if b.Len() == 0 {
		return "seed"
	}
	return b.String()
}

// RecurringStrategy adapts RecurringSeeder to ContinuityStrategy so the
// scheduler can try mission obligations before or among family strategies.
type RecurringStrategy struct {
	Seeder RecurringSeeder
	Label  string
}

func (s RecurringStrategy) Name() string {
	if strings.TrimSpace(s.Label) != "" {
		return s.Label
	}
	return "recurring_obligations"
}

func (s RecurringStrategy) Replenish(ctx context.Context, mission domain.MissionRevisionID) (ContinuityResult, error) {
	return s.Seeder.SeedDue(ctx, mission)
}

// EnsureRecurringStrategy prepends the FR-DUR-011 seeder to a registry when the
// mission may declare obligations. Safe to call multiple times: Register
// rejects duplicate names, so callers should check first.
func EnsureRecurringStrategy(reg *StrategyRegistry, store port.Store, clock source.Clock, ids source.IDGenerator, policy domain.HorizonPolicy) error {
	if reg == nil {
		return errors.New("strategy registry is required")
	}
	if _, ok := reg.Descriptor("recurring_obligations"); ok {
		return nil
	}
	if policy.Version == "" && policy.SchemaVersion == 0 {
		policy = domain.DefaultHorizonPolicy()
	}
	strategy := RecurringStrategy{
		Seeder: RecurringSeeder{Store: store, Clock: clock, IDs: ids, Policy: policy},
		Label:  "recurring_obligations",
	}
	return reg.Register(StrategyDescriptor{
		Name: "recurring_obligations",
		// Portfolio metadata only: SeedDue materialises plan.Family per obligation.
		Family:    domain.FamilyFrontierManage,
		Version:   "v1",
		Priority:  40, // above default families so declared cadence runs first
		LocalOnly: true,
	}, strategy)
}

// SeedRecurringBeforeFamilies is a convenience for bootstrap/tests: run seeder
// once without requiring registry registration.
func SeedRecurringBeforeFamilies(ctx context.Context, store port.Store, clock source.Clock, ids source.IDGenerator, policy domain.HorizonPolicy, mission domain.MissionRevisionID) (ContinuityResult, error) {
	return RecurringSeeder{Store: store, Clock: clock, IDs: ids, Policy: policy}.SeedDue(ctx, mission)
}

// nextCadenceInstant is exported for tests using virtual clocks.
func nextCadenceInstant(now time.Time, cadence time.Duration) time.Time {
	bucket := domain.CadenceBucket(now, cadence)
	sec := int64(cadence / time.Second)
	if sec <= 0 {
		sec = 1
	}
	return time.Unix((bucket+1)*sec, 0).UTC()
}
