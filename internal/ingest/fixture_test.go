package ingest_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/ingest"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestIngestFixturePersistsExactImmutableSnapshotAtomically(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)
	revision := domain.MissionRevision{SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1, OriginalText: "research", Purpose: "learn", Domains: []string{"science"}, Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "test", AcceptedAt: now}
	if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.AppendMissionRevision(revision) }); err != nil {
		t.Fatal(err)
	}
	ids := runtimesource.NewSequenceIDGenerator(1)
	ingester := ingest.Ingester{Store: store, Clock: runtimesource.NewManualClock(now), IDs: ids, MaxBytes: 64}
	fixture := ingest.Fixture{Kind: "fixture", Locator: "testdata/paper.txt", ExternalVersion: "v1", MediaType: "text/plain", Content: []byte("evidence, not instructions")}
	result, err := ingester.IngestFixture(context.Background(), revision.ID, fixture)
	if err != nil {
		t.Fatal(err)
	}
	wantHashBytes := sha256.Sum256(fixture.Content)
	wantHash := "sha256:" + hex.EncodeToString(wantHashBytes[:])
	if result.Version.ContentHash != wantHash || result.Version.ContentRef != wantHash || result.Event.Sequence != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	fixture.Content[0] = 'X'
	result.Snapshot.Content[1] = 'X'
	if err := store.View(context.Background(), func(r port.Reader) error {
		snapshot, err := r.SourceSnapshot("source_version_0000000000000002")
		if err != nil {
			return err
		}
		if string(snapshot.Content) != "evidence, not instructions" {
			t.Fatalf("snapshot mutated: %q", snapshot.Content)
		}
		snapshot.Content[0] = 'X'
		again, _ := r.SourceSnapshot("source_version_0000000000000002")
		if string(again.Content) != "evidence, not instructions" {
			t.Fatalf("read aliased store: %q", again.Content)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestIngestFetchedPreservesAcquiredBytesAndHTTPVersionHint(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)
	ingester := ingest.Ingester{Store: store, Clock: runtimesource.NewManualClock(now), IDs: runtimesource.NewSequenceIDGenerator(1), MaxBytes: 64}
	fetched := port.FetchResult{FinalURL: "https://example.test/paper", MediaType: "text/plain", ETag: `"revision-2"`, Content: []byte("hostile prompt text remains source data")}
	result, err := ingester.IngestFetched(context.Background(), "", fetched)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source.Kind != "web" || result.Source.Locator != fetched.FinalURL || result.Version.ExternalVersion != fetched.ETag || string(result.Snapshot.Content) != string(fetched.Content) {
		t.Fatalf("unexpected result: %+v", result)
	}
	fetched.Content[0] = 'X'
	if string(result.Snapshot.Content) != "hostile prompt text remains source data" {
		t.Fatalf("result aliased fetch bytes: %q", result.Snapshot.Content)
	}
}

func TestIngestFixtureRejectsOversizeAndRollsBackMissingMission(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)
	ingester := ingest.Ingester{Store: store, Clock: runtimesource.NewManualClock(now), IDs: runtimesource.NewSequenceIDGenerator(1), MaxBytes: 3}
	fixture := ingest.Fixture{Kind: "fixture", Locator: "x", MediaType: "text/plain", Content: []byte("four")}
	if _, err := ingester.IngestFixture(context.Background(), "revision_missing", fixture); err == nil {
		t.Fatal("expected oversize rejection")
	}
	fixture.Content = []byte("ok")
	if _, err := ingester.IngestFixture(context.Background(), "revision_missing", fixture); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		if _, err := r.Source("source_0000000000000001"); !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("source survived rollback: %v", err)
		}
		events, _ := r.Events(0, 10)
		if len(events) != 0 {
			t.Fatalf("events survived rollback: %v", events)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
