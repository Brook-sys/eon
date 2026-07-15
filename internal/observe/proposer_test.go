package observe_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/ingest"
	"motor-autonomo/internal/observe"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/segment"
	"motor-autonomo/internal/storage/memory"
)

func TestProposerPersistsExactlyAnchoredObservationAndEvent(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 15, 16, 40, 0, 0, time.UTC)
	ids := runtimesource.NewSequenceIDGenerator(1)
	content := []byte("primary source statement")
	ingested, err := (ingest.Ingester{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).IngestFixture(context.Background(), "", ingest.Fixture{Kind: "fixture", Locator: "source.txt", MediaType: "text/plain", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	segmented, err := (segment.TextSegmenter{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).Segment(context.Background(), "", ingested.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (observe.Proposer{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).Propose(context.Background(), "", observe.Candidate{SourceFragmentID: segmented.Fragments[0].ID, Statement: "The source states a primary statement.", ExactQuote: string(content), Provenance: "extractor:test@1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Event.Kind != "observation.proposed" || result.Event.Sequence != 3 {
		t.Fatalf("unexpected event: %+v", result.Event)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.Observation(result.Observation.ID)
		if err != nil {
			return err
		}
		if got.ExactQuote != string(content) || got.Anchor.SourceFragmentID != segmented.Fragments[0].ID {
			t.Fatalf("unexpected observation: %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProposerRejectsHallucinatedQuoteAndRollsBackEvent(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 15, 16, 40, 0, 0, time.UTC)
	ids := runtimesource.NewSequenceIDGenerator(1)
	content := []byte("source truth")
	ingested, err := (ingest.Ingester{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).IngestFixture(context.Background(), "", ingest.Fixture{Kind: "fixture", Locator: "source.txt", MediaType: "text/plain", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	segmented, err := (segment.TextSegmenter{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).Segment(context.Background(), "", ingested.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (observe.Proposer{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).Propose(context.Background(), "", observe.Candidate{SourceFragmentID: segmented.Fragments[0].ID, Statement: "invented", ExactQuote: "not in source", Provenance: "extractor:test@1"})
	if !errors.Is(err, port.ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		events, err := r.Events(0, 10)
		if err != nil {
			return err
		}
		if len(events) != 2 {
			t.Fatalf("events after rollback = %d, want 2", len(events))
		}
		_, err = r.Observation("observation_6")
		if !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("observation survived rollback: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
