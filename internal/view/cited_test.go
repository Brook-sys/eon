package view_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/claim"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/ingest"
	"motor-autonomo/internal/observe"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/segment"
	"motor-autonomo/internal/storage/memory"
	"motor-autonomo/internal/view"
)

func TestGenerateProducesDeterministicCitedArtifact(t *testing.T) {
	store, claimID, _, ids := seededClaimAndSecondObservation(t)
	artifact, err := (view.Generator{Store: store, IDs: ids}).Generate(context.Background(), claimID, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Kind != "cited_claim_view" || artifact.Stale || !strings.HasPrefix(artifact.ContentRef, "sha256:") {
		t.Fatalf("artifact = %+v", artifact)
	}
	for _, want := range []string{"The first fixture is authoritative.", "**SUPPORTS**", "“first statement”", "[first.txt, bytes:0-15]"} {
		if !strings.Contains(artifact.Content, want) {
			t.Fatalf("content missing %q:\n%s", want, artifact.Content)
		}
	}
	artifact.Dependencies[0] = "mutated"
	if err := store.View(context.Background(), func(r port.Reader) error {
		persisted, err := r.KnowledgeArtifact(artifact.ID)
		if err != nil {
			return err
		}
		if persisted.Dependencies[0] == "mutated" {
			t.Fatal("stored artifact aliases caller dependencies")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPatchAppendsEvidenceMarksPriorStaleAndCreatesSuccessor(t *testing.T) {
	store, claimID, secondObservation, ids := seededClaimAndSecondObservation(t)
	generator := view.Generator{Store: store, IDs: ids}
	prior, err := generator.Generate(context.Background(), claimID, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	successor, err := (view.Patcher{Store: store, IDs: ids}).Apply(context.Background(), prior.ID, view.EvidencePatch{ObservationID: secondObservation, Relation: domain.EvidenceContradicts, Rationale: "independent fixture delta"}, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	if successor.ID == prior.ID || successor.Stale || !strings.Contains(successor.Content, "second statement") || !strings.Contains(successor.Content, "**CONTRADICTS**") {
		t.Fatalf("successor = %+v", successor)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		old, err := r.KnowledgeArtifact(prior.ID)
		if err != nil {
			return err
		}
		if !old.Stale {
			t.Fatal("prior artifact was not marked stale")
		}
		links, err := r.EvidenceLinksForClaim(claimID)
		if err != nil {
			return err
		}
		if len(links) != 2 {
			t.Fatalf("evidence links = %d, want 2", len(links))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPatchRollsBackWhenObservationIsMissing(t *testing.T) {
	store, claimID, _, ids := seededClaimAndSecondObservation(t)
	prior, err := (view.Generator{Store: store, IDs: ids}).Generate(context.Background(), claimID, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (view.Patcher{Store: store, IDs: ids}).Apply(context.Background(), prior.ID, view.EvidencePatch{ObservationID: "missing", Relation: domain.EvidenceSupports}, domain.GenesisCommitID)
	if !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		old, err := r.KnowledgeArtifact(prior.ID)
		if err != nil {
			return err
		}
		if old.Stale {
			t.Fatal("failed patch marked prior artifact stale")
		}
		links, err := r.EvidenceLinksForClaim(claimID)
		if err != nil {
			return err
		}
		if len(links) != 1 {
			t.Fatalf("failed patch persisted evidence: %d links", len(links))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func seededClaimAndSecondObservation(t *testing.T) (*memory.Store, domain.ClaimID, domain.ObservationID, *runtimesource.SequenceIDGenerator) {
	t.Helper()
	store := memory.New()
	now := time.Date(2026, 7, 15, 17, 20, 0, 0, time.UTC)
	clock := runtimesource.NewManualClock(now)
	ids := runtimesource.NewSequenceIDGenerator(1)
	first := seedObservation(t, store, clock, ids, "first.txt", "first statement", "The first fixture contains a statement.")
	second := seedObservation(t, store, clock, ids, "second.txt", "second statement", "The second fixture contains a statement.")
	result, err := (claim.Proposer{Store: store, Clock: clock, IDs: ids}).Propose(context.Background(), "", claim.Candidate{Proposition: "The first fixture is authoritative.", Qualifiers: map[string]string{"scope": "fixture", "status": "proposed"}, Evidence: []claim.EvidenceCandidate{{ObservationID: first, Relation: domain.EvidenceSupports, Rationale: "exact quote"}}})
	if err != nil {
		t.Fatal(err)
	}
	return store, result.Claim.ID, second, ids
}

func seedObservation(t *testing.T, store port.Store, clock runtimesource.Clock, ids runtimesource.IDGenerator, locator, content, statement string) domain.ObservationID {
	t.Helper()
	ingested, err := (ingest.Ingester{Store: store, Clock: clock, IDs: ids}).IngestFixture(context.Background(), "", ingest.Fixture{Kind: "fixture", Locator: locator, MediaType: "text/plain", Content: []byte(content)})
	if err != nil {
		t.Fatal(err)
	}
	segmented, err := (segment.TextSegmenter{Store: store, Clock: clock, IDs: ids}).Segment(context.Background(), "", ingested.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := (observe.Proposer{Store: store, Clock: clock, IDs: ids}).Propose(context.Background(), "", observe.Candidate{SourceFragmentID: segmented.Fragments[0].ID, Statement: statement, ExactQuote: content, Provenance: "extractor:test@1"})
	if err != nil {
		t.Fatal(err)
	}
	return observed.Observation.ID
}
