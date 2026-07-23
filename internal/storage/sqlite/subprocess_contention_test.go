package sqlite_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	modernsqlite "modernc.org/sqlite"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/sqlite"
)

const sqliteSubprocessHelperEnv = "MOTOR_AUTONOMO_SQLITE_SUBPROCESS_HELPER"

func TestSQLiteSubprocessContentionCrashAndStaleCAS(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	readyPath := filepath.Join(dir, "writer-ready")

	// Open the contender before the subprocess commits so it deliberately keeps
	// an independent checkpoint generation in memory.
	contender, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open contender: %v", err)
	}
	defer contender.Close()

	crashing := sqliteHelperCommand("hold-before-commit", dbPath, readyPath, "crashing-writer")
	if err := crashing.Start(); err != nil {
		t.Fatalf("start crashing writer: %v", err)
	}
	waitForFile(t, readyPath, 5*time.Second)

	busyCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	err = contender.Update(busyCtx, reserveIdempotency("contended-writer"))
	if err == nil {
		t.Fatal("contended update unexpectedly succeeded while subprocess held write transaction")
	}
	if errors.Is(err, port.ErrConflict) {
		t.Fatalf("write-lock contention misclassified as stale checkpoint CAS: %v", err)
	}
	if !isSQLiteBusy(err) && !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(strings.ToLower(err.Error()), "interrupt") {
		t.Fatalf("contended update error = %v, want SQLITE_BUSY or context interruption", err)
	}

	if err := crashing.Process.Kill(); err != nil {
		t.Fatalf("kill writer at pre-commit boundary: %v", err)
	}
	if err := crashing.Wait(); err == nil {
		t.Fatal("killed writer unexpectedly exited successfully")
	}

	// The killed subprocess never committed. Its transaction must roll back and
	// the pre-opened contender can publish from the unchanged checkpoint.
	if err := contender.Update(ctx, reserveIdempotency("after-crash")); err != nil {
		t.Fatalf("update after crash recovery: %v", err)
	}

	committed := sqliteHelperCommand("commit", dbPath, "", "subprocess-commit")
	if output, err := committed.CombinedOutput(); err != nil {
		t.Fatalf("committing subprocess: %v output=%s", err, output)
	}

	// The contender is now stale relative to the independently committed child.
	// The first write must lose the checkpoint CAS and reload; an explicit retry
	// then merges with the winner rather than overwriting it.
	if err := contender.Update(ctx, reserveIdempotency("after-conflict")); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("stale contender update = %v, want ErrConflict", err)
	}
	if err := contender.Update(ctx, reserveIdempotency("after-conflict")); err != nil {
		t.Fatalf("retry after stale checkpoint reload: %v", err)
	}

	reopened, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen after subprocess race: %v", err)
	}
	defer reopened.Close()
	if err := reopened.View(ctx, func(r port.Reader) error {
		for _, key := range []string{"after-crash", "subprocess-commit", "after-conflict"} {
			if _, err := r.IdempotencyRecord(domain.IdempotencyKey(key)); err != nil {
				return fmt.Errorf("idempotency record %q: %w", key, err)
			}
		}
		if _, err := r.IdempotencyRecord(domain.IdempotencyKey("crashing-writer")); !errors.Is(err, port.ErrNotFound) {
			return fmt.Errorf("crashed writer record err=%v, want not found", err)
		}
		if _, err := r.IdempotencyRecord(domain.IdempotencyKey("contended-writer")); !errors.Is(err, port.ErrNotFound) {
			return fmt.Errorf("contended writer record err=%v, want not found", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteSubprocessCrashAfterDurableCommitAndStaleCAS(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	readyPath := filepath.Join(dir, "writer-committed")

	// This handle deliberately loads the empty generation before the child
	// commits and dies at the post-commit boundary.
	contender, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open stale contender: %v", err)
	}
	defer contender.Close()

	crashing := sqliteHelperCommand("hold-after-commit", dbPath, readyPath, "committed-before-crash")
	if err := crashing.Start(); err != nil {
		t.Fatalf("start post-commit writer: %v", err)
	}
	waitForFile(t, readyPath, 5*time.Second)
	if err := crashing.Process.Kill(); err != nil {
		t.Fatalf("kill writer at post-commit boundary: %v", err)
	}
	if err := crashing.Wait(); err == nil {
		t.Fatal("killed post-commit writer unexpectedly exited successfully")
	}

	// The child commit survived, so this pre-opened handle must lose the CAS,
	// reload that winner, and merge only after an explicit retry.
	if err := contender.Update(ctx, reserveIdempotency("after-postcommit-conflict")); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("post-commit stale update = %v, want ErrConflict", err)
	}
	if err := contender.Update(ctx, reserveIdempotency("after-postcommit-conflict")); err != nil {
		t.Fatalf("post-commit retry after reload: %v", err)
	}

	reopened, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen post-commit crash database: %v", err)
	}
	defer reopened.Close()
	if err := reopened.View(ctx, func(r port.Reader) error {
		for _, key := range []domain.IdempotencyKey{"committed-before-crash", "after-postcommit-conflict"} {
			if _, err := r.IdempotencyRecord(key); err != nil {
				return fmt.Errorf("idempotency record %q: %w", key, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteSubprocessHelper(t *testing.T) {
	mode := os.Getenv(sqliteSubprocessHelperEnv)
	if mode == "" {
		t.Skip("subprocess helper")
	}
	runSQLiteSubprocessHelper(t, mode)
}

func runSQLiteSubprocessHelper(t *testing.T, mode string) {
	dbPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_SUBPROCESS_DB")
	key := os.Getenv("MOTOR_AUTONOMO_SQLITE_SUBPROCESS_KEY")
	if dbPath == "" || key == "" {
		t.Fatal("sqlite subprocess helper requires database path and key")
	}
	options := sqlite.Options{}
	if mode == "hold-before-commit" || mode == "hold-after-commit" {
		readyPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_SUBPROCESS_READY")
		if readyPath == "" {
			t.Fatal("hold helper requires ready path")
		}
		boundary := sqlite.FailpointBeforeDurableCommit
		if mode == "hold-after-commit" {
			boundary = sqlite.FailpointAfterDurableCommit
		}
		options.Failpoint = func(point sqlite.Failpoint) {
			if point != boundary {
				return
			}
			if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
				panic(err)
			}
			select {}
		}
	} else if mode != "commit" {
		t.Fatalf("unknown sqlite subprocess helper mode %q", mode)
	}
	store, err := sqlite.OpenWithOptions(dbPath, options)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Update(context.Background(), reserveIdempotency(key)); err != nil {
		t.Fatal(err)
	}
}

func sqliteHelperCommand(mode, dbPath, readyPath, key string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run", "^TestSQLiteSubprocessHelper$")
	cmd.Env = append(os.Environ(),
		sqliteSubprocessHelperEnv+"="+mode,
		"MOTOR_AUTONOMO_SQLITE_SUBPROCESS_DB="+dbPath,
		"MOTOR_AUTONOMO_SQLITE_SUBPROCESS_READY="+readyPath,
		"MOTOR_AUTONOMO_SQLITE_SUBPROCESS_KEY="+key,
	)
	return cmd
}

func isSQLiteBusy(err error) bool {
	var sqliteErr *modernsqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == 5
}

func reserveIdempotency(key string) func(port.Transaction) error {
	return func(tx port.Transaction) error {
		_, err := tx.ReserveIdempotency(domain.IdempotencyRecord{
			SchemaVersion: 1,
			Key:           domain.IdempotencyKey(key),
			OperationID:   domain.OperationID("operation-" + key),
			Intent:        "intent-" + key,
			Status:        domain.IdempotencyReserved,
			ReservedAt:    time.Date(2026, 7, 23, 15, 0, 0, 0, time.UTC),
		})
		return err
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for subprocess boundary file %s", path)
}
