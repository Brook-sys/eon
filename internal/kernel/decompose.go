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

// ChildDraft is a deterministic child opportunity proposed by a continuity
// family. Novelty and stop conditions are mandatory so paraphrase-only
// expansions cannot pass validation.
type ChildDraft struct {
	Title          string
	Origin         string
	ExpectedGain   string
	Novelty        string
	StopCondition  string
	DedupSignature string
	Risk           domain.RiskLevel
	Priority       uint8
	EstimatedCost  domain.Budget
	Dependencies   []string
}

func (d ChildDraft) validate() error {
	if strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Origin) == "" ||
		strings.TrimSpace(d.ExpectedGain) == "" || strings.TrimSpace(d.Novelty) == "" ||
		strings.TrimSpace(d.StopCondition) == "" || strings.TrimSpace(d.DedupSignature) == "" {
		return errors.New("child draft is missing required descriptive fields")
	}
	if d.Priority == 0 {
		return errors.New("child draft priority must be positive")
	}
	switch d.Risk {
	case domain.RiskLow, domain.RiskMedium, domain.RiskHigh:
	default:
		return fmt.Errorf("unknown child draft risk %q", d.Risk)
	}
	return d.EstimatedCost.Validate()
}

// Decomposer creates bounded parent→child opportunities under HorizonPolicy
// depth and fan-out limits. It never admits children into the agenda.
type Decomposer struct {
	Store  port.Store
	Clock  source.Clock
	IDs    source.IDGenerator
	Policy domain.HorizonPolicy
}

func (d Decomposer) policy() domain.HorizonPolicy {
	if d.Policy.Version == "" && d.Policy.SchemaVersion == 0 {
		return domain.DefaultHorizonPolicy()
	}
	return d.Policy
}

// SpawnChildren persists validated children of an active parent. The parent
// remains OPEN/DEFERRED; callers admit children separately.
func (d Decomposer) SpawnChildren(ctx context.Context, parentID domain.WorkOpportunityID, drafts []ChildDraft) ([]domain.WorkOpportunity, error) {
	if d.Store == nil || d.Clock == nil || d.IDs == nil {
		return nil, errors.New("decomposer dependencies are incomplete")
	}
	if parentID == "" {
		return nil, errors.New("parent opportunity id is required")
	}
	if len(drafts) == 0 {
		return nil, errors.New("at least one child draft is required")
	}
	policy := d.policy()
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("horizon policy: %w", err)
	}

	var created []domain.WorkOpportunity
	err := d.Store.Update(ctx, func(tx port.Transaction) error {
		parent, err := tx.WorkOpportunity(parentID)
		if err != nil {
			return err
		}
		children, err := tx.WorkOpportunities(parent.MissionRevision, "")
		if err != nil {
			return err
		}
		existing := 0
		activeSignatures := make(map[string]struct{})
		for _, child := range children {
			if child.ParentID == parent.ID {
				existing++
			}
			if child.Status.Active() {
				activeSignatures[child.DedupSignature] = struct{}{}
			}
		}
		now := d.Clock.Now().UTC()
		openCandidates := 0
		for _, child := range children {
			if child.Status.Active() {
				openCandidates++
			}
		}
		out := make([]domain.WorkOpportunity, 0, len(drafts))
		for index, draft := range drafts {
			if err := draft.validate(); err != nil {
				return fmt.Errorf("child draft %d: %w", index, err)
			}
			if err := parent.CanSpawnChild(policy, existing); err != nil {
				return err
			}
			if openCandidates >= policy.MaxCandidates {
				return fmt.Errorf("%w: open candidate frontier at max_candidates=%d", port.ErrConflict, policy.MaxCandidates)
			}
			if _, dup := activeSignatures[draft.DedupSignature]; dup {
				return fmt.Errorf("%w: active work opportunity duplicates semantic signature %q", port.ErrConflict, draft.DedupSignature)
			}
			// Novelty must differ from the parent description to reject pure paraphrase.
			if strings.EqualFold(strings.TrimSpace(draft.Novelty), strings.TrimSpace(parent.Novelty)) &&
				strings.EqualFold(strings.TrimSpace(draft.Title), strings.TrimSpace(parent.Title)) {
				return fmt.Errorf("%w: child must declare novelty beyond parent paraphrase", port.ErrConflict)
			}
			id, err := d.IDs.NewID("opportunity")
			if err != nil {
				return fmt.Errorf("generate child opportunity id: %w", err)
			}
			cost := draft.EstimatedCost
			if cost == (domain.Budget{}) {
				cost = domain.Budget{Tokens: 64, Attempts: 1}
			}
			child, err := parent.DeriveChild(
				domain.WorkOpportunityID(id),
				draft.Title,
				draft.Origin,
				draft.ExpectedGain,
				draft.Novelty,
				draft.StopCondition,
				draft.DedupSignature,
				draft.Risk,
				draft.Priority,
				now,
				cost,
			)
			if err != nil {
				return err
			}
			if len(draft.Dependencies) > 0 {
				child.Dependencies = append([]string(nil), draft.Dependencies...)
				if err := child.Validate(); err != nil {
					return err
				}
			}
			if err := tx.CreateWorkOpportunity(child); err != nil {
				return err
			}
			eventID := domain.EventID(string(child.ID) + ":spawned")
			if _, err := tx.AppendEvent(domain.Event{
				SchemaVersion:   domain.SchemaVersionV1,
				ID:              eventID,
				Kind:            domain.EventContinuityGapDetected,
				OccurredAt:      now.Add(time.Duration(index) * time.Nanosecond),
				MissionRevision: child.MissionRevision,
				PayloadRef:      string(parent.ID) + "->" + string(child.ID),
			}); err != nil {
				return err
			}
			out = append(out, child)
			existing++
			openCandidates++
			activeSignatures[child.DedupSignature] = struct{}{}
		}
		created = out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}
