package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/sqlite"
)

func TestSaveChannelCursorRejectsStaleRevisionAndPreservesNewerValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	
	now := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)

	// Setup initial cursor via transaction
	initial, err := domain.InitialChannelCursor("telegram", 1000, now)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.SaveChannelCursor(initial, 0)
	}); err != nil {
		t.Fatalf("save initial cursor: %v", err)
	}

	// Read initial cursor
	var cursor1 domain.ChannelCursor
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		cursor1, err = r.ChannelCursor("telegram")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Thread A simulates an advance
	advancedA, err := domain.AdvanceChannelCursor(cursor1, 1050, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}

	// Thread B simulates an older advance based on the SAME initial cursor (Thread B was slower)
	advancedB, err := domain.AdvanceChannelCursor(cursor1, 1020, now.Add(2 * time.Second))
	if err != nil {
		t.Fatal(err)
	}

	// Thread A commits its advance
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.SaveChannelCursor(advancedA, cursor1.Revision) // revision 0
	}); err != nil {
		t.Fatalf("thread A save: %v", err)
	}

	// Thread B tries to commit its advance, but it provides the old expectedRevision (0),
	// which must be rejected with port.ErrConflict.
	err = store.Update(ctx, func(tx port.Transaction) error {
		return tx.SaveChannelCursor(advancedB, cursor1.Revision)
	})
	
	if err == nil {
		t.Fatalf("expected conflict error for stale revision update, got nil")
	}

	// Ensure the store value is still Thread A's correct advance
	if err := store.View(ctx, func(r port.Reader) error {
		final, err := r.ChannelCursor("telegram")
		if err != nil {
			return err
		}
		if final.Cursor != 1050 {
			t.Errorf("expected final cursor to be 1050, got %d", final.Cursor)
		}
		if final.Revision != 1 {
			t.Errorf("expected final revision to be 1, got %d", final.Revision)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
