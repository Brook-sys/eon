package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/sqlite"
)

func TestIndependentHandlesRejectStaleCheckpointAndReload(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	first, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("open first: %v", err)
	}
	defer first.Close()
	second, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("open second: %v", err)
	}
	defer second.Close()

	now := time.Date(2026, 7, 23, 14, 30, 0, 0, time.UTC)
	one := domain.IdempotencyRecord{SchemaVersion: 1, Key: "one", OperationID: "operation-one", Intent: "intent-one", Status: domain.IdempotencyReserved, ReservedAt: now}
	if err := first.Update(ctx, func(tx port.Transaction) error { _, err := tx.ReserveIdempotency(one); return err }); err != nil {
		t.Fatalf("first update: %v", err)
	}
	two := domain.IdempotencyRecord{SchemaVersion: 1, Key: "two", OperationID: "operation-two", Intent: "intent-two", Status: domain.IdempotencyReserved, ReservedAt: now}
	if err := second.Update(ctx, func(tx port.Transaction) error { _, err := tx.ReserveIdempotency(two); return err }); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("stale second update = %v, want ErrConflict", err)
	}
	if err := second.Update(ctx, func(tx port.Transaction) error { _, err := tx.ReserveIdempotency(two); return err }); err != nil {
		t.Fatalf("second update after reload: %v", err)
	}

	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if err := reopened.View(ctx, func(r port.Reader) error {
		if _, err := r.IdempotencyRecord("one"); err != nil {
			return err
		}
		_, err := r.IdempotencyRecord("two")
		return err
	}); err != nil {
		t.Fatalf("verify merged checkpoint: %v", err)
	}
}
