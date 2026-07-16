package view_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/view"
)

func TestRefreshCitedRegeneratesStaleView(t *testing.T) {
	store, claimID, secondObservation, ids := seededClaimAndSecondObservation(t)
	generator := view.Generator{Store: store, IDs: ids}
	prior, err := generator.Generate(context.Background(), claimID, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	// Cascade-stale via evidence delta (no patcher).
	link := domain.EvidenceLink{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "evidence_refresh_1",
		ObservationID: secondObservation,
		ClaimID:       claimID,
		Relation:      domain.EvidenceSupports,
		Rationale:     "refresh fixture",
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.AppendEvidenceLinks(claimID, []domain.EvidenceLink{link})
	}); err != nil {
		t.Fatal(err)
	}

	refresher := view.Refresher{Store: store, IDs: ids}
	successor, err := refresher.RefreshCited(context.Background(), prior.ID, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	if successor.ID == prior.ID || successor.Stale || successor.Kind != view.CitedClaimViewKind {
		t.Fatalf("successor = %+v", successor)
	}
	if !strings.Contains(successor.Content, "second statement") {
		t.Fatalf("successor missing new evidence:\n%s", successor.Content)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		old, err := r.KnowledgeArtifact(prior.ID)
		if err != nil {
			return err
		}
		if !old.Stale {
			t.Fatal("prior should remain stale after refresh")
		}
		fresh, err := r.KnowledgeArtifact(successor.ID)
		if err != nil {
			return err
		}
		if fresh.Stale || fresh.Content != successor.Content {
			t.Fatalf("persisted successor = %+v", fresh)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshCitedRejectsNonStaleAndAuditKinds(t *testing.T) {
	store, claimID, _, ids := seededClaimAndSecondObservation(t)
	prior, err := (view.Generator{Store: store, IDs: ids}).Generate(context.Background(), claimID, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	refresher := view.Refresher{Store: store, IDs: ids}
	if _, err := refresher.RefreshCited(context.Background(), prior.ID, domain.GenesisCommitID); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("non-stale refresh error = %v, want conflict", err)
	}
	audit := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "artifact_audit_refresh",
		Kind:          "gap_scan_report",
		BaseCommitID:  domain.GenesisCommitID,
		Dependencies:  []string{domain.FormatClaimDependency(claimID, 1)},
		ContentRef:    "sha256:audit",
		Content:       "audit",
		Stale:         true,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.AppendKnowledgeArtifact(audit)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := refresher.RefreshCited(context.Background(), audit.ID, domain.GenesisCommitID); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("audit refresh error = %v, want conflict", err)
	}
}

func TestRefreshCitedBatchSelectsStaleCitedOnly(t *testing.T) {
	store, claimID, secondObservation, ids := seededClaimAndSecondObservation(t)
	prior, err := (view.Generator{Store: store, IDs: ids}).Generate(context.Background(), claimID, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	// Force prior stale without changing content for batch selection.
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		art, err := tx.KnowledgeArtifact(prior.ID)
		if err != nil {
			return err
		}
		art.Stale = true
		return tx.SaveKnowledgeArtifact(art)
	}); err != nil {
		t.Fatal(err)
	}
	// Unrelated non-cited stale artifact must be ignored.
	other := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "artifact_other_kind",
		Kind:          "summary_view",
		BaseCommitID:  domain.GenesisCommitID,
		Dependencies:  []string{domain.FormatClaimDependency(claimID, 1)},
		ContentRef:    "sha256:other",
		Content:       "other",
		Stale:         true,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.AppendKnowledgeArtifact(other)
	}); err != nil {
		t.Fatal(err)
	}
	// Evidence delta so regenerated content is observably different when claim has more links.
	_ = secondObservation
	created, err := (view.Refresher{Store: store, IDs: ids}).RefreshCitedBatch(context.Background(), domain.GenesisCommitID, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 {
		t.Fatalf("created = %d, want 1", len(created))
	}
	if created[0].Kind != view.CitedClaimViewKind || created[0].Stale {
		t.Fatalf("created = %+v", created[0])
	}
}

func TestRefreshCitedRequiresDeps(t *testing.T) {
	_, err := (view.Refresher{}).RefreshCited(context.Background(), "a", domain.GenesisCommitID)
	if err == nil {
		t.Fatal("expected dependency error")
	}
	// Satisfy unused import if toolchain trims; keep time available for future fixtures.
	_ = time.Time{}
	_ = runtimesource.NewSequenceIDGenerator(1)
}
