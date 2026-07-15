package claim_test

import (
	"context"
	"errors"
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
)

func TestProposerPersistsAtomicClaimEvidenceAndEvent(t *testing.T) {
	store, observationID, ids, now := seededObservation(t)
	qualifiers := map[string]string{"scope": "fixture", "modality": "asserted"}
	result, err := (claim.Proposer{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).Propose(context.Background(), "", claim.Candidate{
		Proposition: "The fixture contains a primary statement.", Qualifiers: qualifiers,
		Evidence: []claim.EvidenceCandidate{{ObservationID: observationID, Relation: domain.EvidenceSupports, Rationale: "exact anchored observation"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	qualifiers["scope"] = "mutated"
	if result.Event.Kind != "claim.proposed" || result.Event.Sequence != 4 {
		t.Fatalf("unexpected event: %+v", result.Event)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.Claim(result.Claim.ID)
		if err != nil {
			return err
		}
		if got.Qualifiers["scope"] != "fixture" {
			t.Fatalf("stored qualifiers aliased caller: %+v", got.Qualifiers)
		}
		links, err := r.EvidenceLinksForClaim(got.ID)
		if err != nil {
			return err
		}
		if len(links) != 1 || links[0].ObservationID != observationID || links[0].Relation != domain.EvidenceSupports {
			t.Fatalf("links = %+v", links)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProposerRejectsMissingObservationAndRollsBack(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 15, 17, 0, 0, 0, time.UTC)
	_, err := (claim.Proposer{Store: store, Clock: runtimesource.NewManualClock(now), IDs: runtimesource.NewSequenceIDGenerator(1)}).Propose(context.Background(), "", claim.Candidate{
		Proposition: "orphan", Qualifiers: map[string]string{"scope": "test"}, Evidence: []claim.EvidenceCandidate{{ObservationID: "missing", Relation: domain.EvidenceSupports}},
	})
	if !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		_, claimErr := r.Claim("claim_1")
		events, eventErr := r.Events(0, 10)
		if !errors.Is(claimErr, port.ErrNotFound) || eventErr != nil || len(events) != 0 {
			t.Fatalf("rollback failed: claim=%v events=%v/%d", claimErr, eventErr, len(events))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func seededObservation(t *testing.T) (*memory.Store, domain.ObservationID, *runtimesource.SequenceIDGenerator, time.Time) {
	t.Helper()
	store := memory.New()
	now := time.Date(2026, 7, 15, 17, 0, 0, 0, time.UTC)
	ids := runtimesource.NewSequenceIDGenerator(1)
	ingested, err := (ingest.Ingester{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).IngestFixture(context.Background(), "", ingest.Fixture{Kind: "fixture", Locator: "source.txt", MediaType: "text/plain", Content: []byte("primary statement")})
	if err != nil {
		t.Fatal(err)
	}
	segmented, err := (segment.TextSegmenter{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).Segment(context.Background(), "", ingested.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := (observe.Proposer{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).Propose(context.Background(), "", observe.Candidate{SourceFragmentID: segmented.Fragments[0].ID, Statement: "The source contains a primary statement.", ExactQuote: "primary statement", Provenance: "extractor:test@1"})
	if err != nil {
		t.Fatal(err)
	}
	return store, observed.Observation.ID, ids, now
}
