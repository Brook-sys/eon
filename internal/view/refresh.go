package view

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
)

// CitedClaimViewKind is the regenerable derived view produced by Generator/Patcher.
const CitedClaimViewKind = "cited_claim_view"

// DefaultRefreshBatch caps how many stale cited views one authorized refresh
// call regenerates. Keeps local continuity budgets bounded (FR-DUR-008).
const DefaultRefreshBatch = 8

// Refresher regenerates authorized derived views that are already stale.
// It never mutates claims/evidence/event log; it only appends a fresh artifact
// successor. Prior rows remain stale for auditability (FR-KNOW-005).
type Refresher struct {
	Store port.Store
	IDs   runtimesource.IDGenerator
}

// RefreshCited regenerates one stale cited_claim_view under a new artifact ID.
// base should be the current mission head (or Genesis when no head exists).
func (r Refresher) RefreshCited(ctx context.Context, priorID domain.ArtifactID, base domain.CommitID) (domain.KnowledgeArtifact, error) {
	if r.Store == nil || r.IDs == nil {
		return domain.KnowledgeArtifact{}, errors.New("cited view refresher requires store and ID generator")
	}
	if priorID == "" {
		return domain.KnowledgeArtifact{}, errors.New("prior artifact id is required")
	}
	if base == "" {
		return domain.KnowledgeArtifact{}, errors.New("refresh requires base commit")
	}
	var successor domain.KnowledgeArtifact
	err := r.Store.Update(ctx, func(tx port.Transaction) error {
		var err error
		successor, err = RefreshCitedInTx(tx, r.IDs, priorID, base)
		return err
	})
	if err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("refresh cited view: %w", err)
	}
	return successor, nil
}

// RefreshCitedBatch regenerates up to limit stale cited_claim_view artifacts.
// Selection is deterministic via domain.PlanStaleArtifactRefresh + kind filter.
func (r Refresher) RefreshCitedBatch(ctx context.Context, base domain.CommitID, limit int) ([]domain.KnowledgeArtifact, error) {
	if r.Store == nil || r.IDs == nil {
		return nil, errors.New("cited view refresher requires store and ID generator")
	}
	if base == "" {
		return nil, errors.New("refresh requires base commit")
	}
	if limit <= 0 {
		limit = DefaultRefreshBatch
	}
	var created []domain.KnowledgeArtifact
	err := r.Store.Update(ctx, func(tx port.Transaction) error {
		var err error
		created, err = RefreshCitedBatchInTx(tx, r.IDs, base, limit)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("refresh cited batch: %w", err)
	}
	return created, nil
}

// RefreshCitedInTx regenerates one stale cited_claim_view inside an open transaction.
func RefreshCitedInTx(tx port.Transaction, ids runtimesource.IDGenerator, priorID domain.ArtifactID, base domain.CommitID) (domain.KnowledgeArtifact, error) {
	if tx == nil || ids == nil {
		return domain.KnowledgeArtifact{}, errors.New("refresh cited in-tx requires transaction and ID generator")
	}
	prior, err := tx.KnowledgeArtifact(priorID)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	if prior.Kind != CitedClaimViewKind {
		return domain.KnowledgeArtifact{}, fmt.Errorf("%w: refresh supports only %s (got %q)", port.ErrConflict, CitedClaimViewKind, prior.Kind)
	}
	if !prior.Stale {
		return domain.KnowledgeArtifact{}, fmt.Errorf("%w: cannot refresh non-stale knowledge artifact", port.ErrConflict)
	}
	if domain.IsLocalAuditArtifactKind(prior.Kind) {
		return domain.KnowledgeArtifact{}, fmt.Errorf("%w: cannot refresh local audit artifact", port.ErrConflict)
	}
	claimID, err := claimDependency(prior.Dependencies)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	artifactID, err := ids.NewID("artifact")
	if err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("generate artifact ID: %w", err)
	}
	if artifactID == "" {
		return domain.KnowledgeArtifact{}, errors.New("generated artifact id must not be empty")
	}
	successor, err := render(tx, domain.ArtifactID(artifactID), claimID, base)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	if err := tx.AppendKnowledgeArtifact(successor); err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	return successor, nil
}

// RefreshCitedBatchInTx plans and regenerates up to limit stale cited views.
// Candidates that are not regenerable (wrong kind, missing claim dependency,
// missing claim entity) are skipped so local continuity ops stay partial-success.
func RefreshCitedBatchInTx(tx port.Transaction, ids runtimesource.IDGenerator, base domain.CommitID, limit int) ([]domain.KnowledgeArtifact, error) {
	if tx == nil || ids == nil {
		return nil, errors.New("refresh batch in-tx requires transaction and ID generator")
	}
	if limit <= 0 {
		limit = DefaultRefreshBatch
	}
	artifacts, err := tx.KnowledgeArtifacts()
	if err != nil {
		return nil, err
	}
	// Oversample then filter by kind so audit/other kinds do not starve the batch.
	planned := domain.PlanStaleArtifactRefresh(artifacts, limit*4)
	created := make([]domain.KnowledgeArtifact, 0, limit)
	for _, id := range planned {
		if len(created) >= limit {
			break
		}
		prior, err := tx.KnowledgeArtifact(id)
		if err != nil {
			// Catalog race / missing row: skip this candidate.
			if errors.Is(err, port.ErrNotFound) {
				continue
			}
			return nil, err
		}
		if prior.Kind != CitedClaimViewKind || !prior.Stale {
			continue
		}
		successor, err := RefreshCitedInTx(tx, ids, id, base)
		if err != nil {
			// Skip unregenerable fixtures (synthetic deps, missing claims).
			if errors.Is(err, port.ErrNotFound) || errors.Is(err, port.ErrConflict) || isUnregenerableCitedError(err) {
				continue
			}
			return nil, err
		}
		created = append(created, successor)
	}
	return created, nil
}

func isUnregenerableCitedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "knowledge artifact has no claim dependency") ||
		strings.Contains(msg, "cited view requires at least one evidence link") ||
		strings.Contains(msg, "first cited view supports only source-fragment")
}
