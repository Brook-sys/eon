package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/sqlite"
)

// Crash recovery observable guarantees for FULL vs NORMAL synchronous pragma.
//
// Hypothesis: under process crash (SIGKILL), both FULL and NORMAL preserve
// committed data and discard uncommitted data, because WAL replay is atomic
// regardless of synchronous level. The observable difference between FULL and
// NORMAL appears only under power-loss/OS crash, which this campaign does not
// and cannot simulate. Therefore the campaign proves equivalence of observed
// crash-recovery guarantees, not equivalence of power-loss durability.
//
// What the campaign verifies per synchronous mode:
//   1. Pre-commit crash: uncommitted transaction rolls back; no idempotency
//      record for the crashed key; a subsequent writer commits clean.
//   2. Post-commit crash: committed data survives SIGKILL; a stale handle
//      loses CAS, reloads and merges.
//   3. Reopen: WAL checkpoint replays and the store opens without error.
//
// The campaign does not measure latency; the Fase 178 matrix already
// quantified the ~46× commit cost difference. This is a correctness campaign.

const (
	crashRecoveryHelperEnv = "MOTOR_AUTONOMO_SQLITE_CRASH_RECOVERY_HELPER"
	crashRecoverySyncEnv   = "MOTOR_AUTONOMO_SQLITE_CRASH_RECOVERY_SYNCHRONOUS"
)

type crashRecoveryResult struct {
	Synchronous        string `json:"synchronous"`
	Phase              string `json:"phase"` // "pre_commit_crash" or "post_commit_crash"
	CrashedKey         string `json:"crashed_key"`
	ContenderKey       string `json:"contender_key"`
	PreCommitSurvived  bool   `json:"pre_commit_survived"`
	PostCommitSurvived bool   `json:"post_commit_survived"`
	ContenderConflict  bool   `json:"contender_conflict"`
	ContenderRetry     bool   `json:"contender_retry"`
	ReopenClean        bool   `json:"reopen_clean"`
	UncommittedLost    bool   `json:"uncommitted_lost"`
}

type crashRecoveryMatrix struct {
	SchemaVersion string                `json:"schema_version"`
	Results       []crashRecoveryResult `json:"results"`
}

func TestSQLiteCrashRecoveryPragmaCampaign(t *testing.T) {
	if os.Getenv(crashRecoveryHelperEnv) != "" {
		runCrashRecoveryHelper(t)
		return
	}

	matrix := crashRecoveryMatrix{SchemaVersion: "motor-autonomo.sqlite-crash-recovery-pragma.v1"}
	syncVariants := []string{"FULL", "NORMAL"}

	for _, sync := range syncVariants {
		// Phase 1: crash before durable commit (uncommitted).
		r1 := testCrashRecoveryPreCommit(t, sync)
		matrix.Results = append(matrix.Results, r1)
		t.Logf("crash recovery sync=%s phase=pre_commit_crash uncommitted_lost=%v reopen_clean=%v",
			sync, r1.UncommittedLost, r1.ReopenClean)

		// Phase 2: crash after durable commit (committed data survives).
		r2 := testCrashRecoveryPostCommit(t, sync)
		matrix.Results = append(matrix.Results, r2)
		t.Logf("crash recovery sync=%s phase=post_commit_crash post_commit_survived=%v contender_conflict=%v reopen_clean=%v",
			sync, r2.PostCommitSurvived, r2.ContenderConflict, r2.ReopenClean)
	}

	// Assert observed equivalence: both modes must satisfy the same invariants.
	for _, r := range matrix.Results {
		if r.Phase == "pre_commit_crash" {
			if !r.UncommittedLost {
				t.Fatalf("sync=%s pre_commit: uncommitted data survived (expected lost)", r.Synchronous)
			}
			if !r.PreCommitSurvived {
				t.Fatalf("sync=%s pre_commit: subsequent writer could not commit after crash", r.Synchronous)
			}
		}
		if r.Phase == "post_commit_crash" {
			if !r.PostCommitSurvived {
				t.Fatalf("sync=%s post_commit: committed data did not survive SIGKILL", r.Synchronous)
			}
			if !r.ContenderConflict {
				t.Fatalf("sync=%s post_commit: stale handle did not get ErrConflict", r.Synchronous)
			}
			if !r.ContenderRetry {
				t.Fatalf("sync=%s post_commit: retry after reload failed", r.Synchronous)
			}
		}
		if !r.ReopenClean {
			t.Fatalf("sync=%s %s: reopen not clean", r.Synchronous, r.Phase)
		}
	}

	writeCrashRecoveryReport(t, matrix)
}

func testCrashRecoveryPreCommit(t *testing.T, synchronous string) crashRecoveryResult {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	readyPath := filepath.Join(dir, "ready")
	crashedKey := "crash-pre-commit-" + synchronous
	afterKey := "after-pre-crash-" + synchronous

	// Open a contender before the crashing subprocess begins, so we have a
	// handle to test post-crash reopen semantics.
	contender, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open contender: %v", err)
	}
	defer contender.Close()

	// Launch subprocess that holds at FailpointBeforeDurableCommit (never commits).
	crashing := crashRecoveryHelperCommand("pre-crash", synchronous, dbPath, readyPath, crashedKey)
	if err := crashing.Start(); err != nil {
		t.Fatalf("start crashing writer: %v", err)
	}
	waitForFile(t, readyPath, 5*time.Second)

	// Kill the subprocess while it holds an open uncommitted transaction.
	if err := crashing.Process.Kill(); err != nil {
		t.Fatalf("kill pre-commit writer: %v", err)
	}
	if err := crashing.Wait(); err == nil {
		t.Fatal("killed pre-commit writer unexpectedly exited successfully")
	}

	// The contender was pre-opened; the crashed writer never committed so the
	// checkpoint is unchanged. The contender should be able to write clean.
	contenderErr := contender.Update(ctx, reserveIdempotency(afterKey))
	preCommitSurvived := contenderErr == nil

	// Reopen from disk and verify: crashed key absent, after key present.
	reopened, err := sqlite.Open(dbPath)
	reopenClean := err == nil
	if reopenClean {
		defer reopened.Close()
		err = reopened.View(ctx, func(r port.Reader) error {
			if _, err := r.IdempotencyRecord(domain.IdempotencyKey(afterKey)); err != nil {
				return fmt.Errorf("after key missing: %w", err)
			}
			if _, err := r.IdempotencyRecord(domain.IdempotencyKey(crashedKey)); !errors.Is(err, port.ErrNotFound) {
				return fmt.Errorf("crashed key present (expected not found): %v", err)
			}
			return nil
		})
		reopenClean = reopenClean && err == nil
	}

	return crashRecoveryResult{
		Synchronous:       synchronous,
		Phase:             "pre_commit_crash",
		CrashedKey:        crashedKey,
		ContenderKey:      afterKey,
		PreCommitSurvived: preCommitSurvived,
		ReopenClean:       reopenClean,
		UncommittedLost:   true, // verified by absence of crashed key
	}
}

func testCrashRecoveryPostCommit(t *testing.T, synchronous string) crashRecoveryResult {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	readyPath := filepath.Join(dir, "ready")
	crashedKey := "crash-post-commit-" + synchronous
	afterKey := "after-post-crash-" + synchronous

	// Open a stale contender before the subprocess commits.
	contender, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open stale contender: %v", err)
	}
	defer contender.Close()

	// Launch subprocess that commits and then holds at FailpointAfterDurableCommit.
	crashing := crashRecoveryHelperCommand("post-crash", synchronous, dbPath, readyPath, crashedKey)
	if err := crashing.Start(); err != nil {
		t.Fatalf("start post-commit writer: %v", err)
	}
	waitForFile(t, readyPath, 5*time.Second)

	// Kill after commit has landed. SIGKILL simulates process crash but NOT
	// power-loss: the OS flushes file system buffers, so WAL data on disk
	// reflects the committed state regardless of synchronous level.
	if err := crashing.Process.Kill(); err != nil {
		t.Fatalf("kill post-commit writer: %v", err)
	}
	if err := crashing.Wait(); err == nil {
		t.Fatal("killed post-commit writer unexpectedly exited successfully")
	}

	// The stale contender must lose CAS against the committed generation.
	conflictErr := contender.Update(ctx, reserveIdempotency(afterKey))
	contenderConflict := errors.Is(conflictErr, port.ErrConflict)

	// Retry after reload should merge.
	retryErr := contender.Update(ctx, reserveIdempotency(afterKey))
	contenderRetry := retryErr == nil

	// Reopen and verify: crashed key present (committed), after key present (merged).
	reopened, err := sqlite.Open(dbPath)
	reopenClean := err == nil
	postCommitSurvived := false
	if reopenClean {
		defer reopened.Close()
		err = reopened.View(ctx, func(r port.Reader) error {
			if _, err := r.IdempotencyRecord(domain.IdempotencyKey(crashedKey)); err != nil {
				return fmt.Errorf("committed key missing: %w", err)
			}
			if _, err := r.IdempotencyRecord(domain.IdempotencyKey(afterKey)); err != nil {
				return fmt.Errorf("after key missing: %w", err)
			}
			return nil
		})
		postCommitSurvived = err == nil
		reopenClean = reopenClean && err == nil
	}

	return crashRecoveryResult{
		Synchronous:        synchronous,
		Phase:              "post_commit_crash",
		CrashedKey:         crashedKey,
		ContenderKey:       afterKey,
		PostCommitSurvived: postCommitSurvived,
		ContenderConflict:  contenderConflict,
		ContenderRetry:     contenderRetry,
		ReopenClean:        reopenClean,
	}
}

func runCrashRecoveryHelper(t *testing.T) {
	mode := os.Getenv(crashRecoveryHelperEnv)
	synchronous := os.Getenv(crashRecoverySyncEnv)
	if synchronous != "FULL" && synchronous != "NORMAL" {
		t.Fatalf("unknown synchronous mode %q", synchronous)
	}
	dbPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_SUBPROCESS_DB")
	key := os.Getenv("MOTOR_AUTONOMO_SQLITE_SUBPROCESS_KEY")
	readyPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_SUBPROCESS_READY")
	if dbPath == "" || key == "" || readyPath == "" {
		t.Fatal("crash recovery helper requires db, key, and ready path")
	}

	boundary := sqlite.FailpointBeforeDurableCommit
	if mode == "post-crash" {
		boundary = sqlite.FailpointAfterDurableCommit
	} else if mode != "pre-crash" {
		t.Fatalf("unknown crash recovery helper mode %q", mode)
	}

	options := sqlite.Options{Synchronous: synchronous}
	options.Failpoint = func(point sqlite.Failpoint) {
		if point != boundary {
			return
		}
		if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
			panic(err)
		}
		select {} // block forever; the parent will SIGKILL.
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

func crashRecoveryHelperCommand(mode, synchronous, dbPath, readyPath, key string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run", "^TestSQLiteCrashRecoveryPragmaCampaign$")
	cmd.Env = append(os.Environ(),
		crashRecoveryHelperEnv+"="+mode,
		crashRecoverySyncEnv+"="+synchronous,
		"MOTOR_AUTONOMO_SQLITE_SUBPROCESS_DB="+dbPath,
		"MOTOR_AUTONOMO_SQLITE_SUBPROCESS_READY="+readyPath,
		"MOTOR_AUTONOMO_SQLITE_SUBPROCESS_KEY="+key,
	)
	return cmd
}

func writeCrashRecoveryReport(t *testing.T, matrix crashRecoveryMatrix) {
	t.Helper()
	path := os.Getenv("MOTOR_AUTONOMO_SQLITE_CRASH_RECOVERY_REPORT")
	if path == "" {
		return
	}
	body, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
