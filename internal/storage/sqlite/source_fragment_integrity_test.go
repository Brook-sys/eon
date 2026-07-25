package sqlite_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/sqlite"
)

func TestAppendSourceFragmentsRejectsCorruptFragmentsAndFailsAtomic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)

	source := domain.Source{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "source_1",
		Kind:          "document",
		Locator:       "file://test",
		ObservedAt:    now,
	}

	content := []byte("12345678901234567890")
	hash := "sha256:" + hex.EncodeToString(func(b [32]byte) []byte { return b[:] }(sha256.Sum256(content)))

	version := domain.SourceVersion{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "version_1",
		SourceID:      source.ID,
		ContentHash:   hash,
		ContentRef:    hash,
		ObservedAt:    now,
	}

	snapshot := domain.SourceSnapshot{
		SchemaVersion:   domain.SchemaVersionV1,
		SourceVersionID: version.ID,
		MediaType:       "text/plain",
		Content:         content,
	}

	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.AppendSource(source, version, snapshot)
	}); err != nil {
		t.Fatalf("setup source: %v", err)
	}

	hashPart1 := "sha256:" + hex.EncodeToString(func(b [32]byte) []byte { return b[:] }(sha256.Sum256(content[0:10])))
	validFrag := domain.SourceFragment{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              "frag_1",
		SourceVersionID: version.ID,
		Location:        "bytes:0-10",
		StartOffset:     0,
		EndOffset:       10,
		ContentHash:     hashPart1,
		ContentRef:      hashPart1,
	}

	corruptFrag := domain.SourceFragment{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              "frag_2",
		SourceVersionID: version.ID,
		Location:        "bytes:10-10",
		StartOffset:     10,
		EndOffset:       10, // Invalid: must be > StartOffset
		ContentHash:     "hash_part2",
		ContentRef:      "ref_part2",
	}

	err = store.Update(ctx, func(tx port.Transaction) error {
		return tx.AppendSourceFragments(version.ID, []domain.SourceFragment{validFrag, corruptFrag})
	})

	if err == nil {
		t.Fatalf("expected error when appending a batch with a corrupt fragment, got nil")
	}

	if err := store.View(ctx, func(r port.Reader) error {
		frags, err := r.SourceFragments(version.ID)
		if err != nil {
			return err
		}
		if len(frags) > 0 {
			t.Fatalf("atomicity failure: expected 0 fragments, got %d", len(frags))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	hashPart2 := "sha256:" + hex.EncodeToString(func(b [32]byte) []byte { return b[:] }(sha256.Sum256(content[10:20])))
	validFrag2 := domain.SourceFragment{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              "frag_2_valid",
		SourceVersionID: version.ID,
		Location:        "bytes:10-20",
		StartOffset:     10,
		EndOffset:       20,
		ContentHash:     hashPart2,
		ContentRef:      hashPart2,
	}

	err = store.Update(ctx, func(tx port.Transaction) error {
		return tx.AppendSourceFragments(version.ID, []domain.SourceFragment{validFrag, validFrag2})
	})

	if err != nil {
		t.Fatalf("failed to insert valid batch: %v", err)
	}

	if err := store.View(ctx, func(r port.Reader) error {
		frags, err := r.SourceFragments(version.ID)
		if err != nil {
			return err
		}
		if len(frags) != 2 {
			t.Fatalf("expected 2 fragments, got %d", len(frags))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
