package segment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/ingest"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/segment"
	"motor-autonomo/internal/storage/memory"
)

func TestTextSegmenterCoversOrdersAndRoundTripsUTF8(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 15, 16, 20, 0, 0, time.UTC)
	revision := domain.MissionRevision{SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1, OriginalText: "research", Purpose: "learn", Domains: []string{"science"}, Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "test", AcceptedAt: now}
	if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.AppendMissionRevision(revision) }); err != nil {
		t.Fatal(err)
	}
	ids := runtimesource.NewSequenceIDGenerator(1)
	content := []byte("alpha áβ gamma")
	ingested, err := (ingest.Ingester{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}).IngestFixture(context.Background(), revision.ID, ingest.Fixture{Kind: "fixture", Locator: "text.txt", MediaType: "text/plain; charset=utf-8", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (segment.TextSegmenter{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids, MaxBytes: 6}).Segment(context.Background(), revision.ID, ingested.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fragments) != 3 || result.Event.Sequence != 2 {
		t.Fatalf("unexpected segmentation result: %+v", result)
	}
	var roundTrip []byte
	if err := store.View(context.Background(), func(r port.Reader) error {
		fragments, err := r.SourceFragments(ingested.Version.ID)
		if err != nil {
			return err
		}
		snapshot, _ := r.SourceSnapshot(ingested.Version.ID)
		for _, fragment := range fragments {
			roundTrip = append(roundTrip, snapshot.Content[fragment.StartOffset:fragment.EndOffset]...)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if string(roundTrip) != string(content) {
		t.Fatalf("round trip = %q, want %q", roundTrip, content)
	}
}

func TestTextSegmenterRejectsNonTextAndRollsBackDuplicate(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 15, 16, 20, 0, 0, time.UTC)
	ids := runtimesource.NewSequenceIDGenerator(1)
	ingester := ingest.Ingester{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids}
	binary, err := ingester.IngestFixture(context.Background(), "", ingest.Fixture{Kind: "fixture", Locator: "x.bin", MediaType: "application/octet-stream", Content: []byte{1, 2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	segmenter := segment.TextSegmenter{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids, MaxBytes: 4}
	if _, err := segmenter.Segment(context.Background(), "", binary.Version.ID); err == nil {
		t.Fatal("expected non-text rejection")
	}
	text, err := ingester.IngestFixture(context.Background(), "", ingest.Fixture{Kind: "fixture", Locator: "x.txt", MediaType: "text/plain", Content: []byte("abcdef")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := segmenter.Segment(context.Background(), "", text.Version.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := segmenter.Segment(context.Background(), "", text.Version.ID); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("duplicate error = %v, want ErrConflict", err)
	}
}
