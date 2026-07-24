package sqlite_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/sqlite"
)

// WAL auto-checkpoint isolation campaign.
//
// Hypothesis: the contribution of WAL auto-checkpoint to reopen time is
// observable only when enough pages accumulate in the WAL before the
// autocheckpoint threshold triggers. With a small number of commits, the
// WAL stays small and reopen cost is dominated by connection setup and
// WAL replay overhead, not by checkpoint work. Increasing the autocheckpoint
// threshold (or disabling it) should not measurably change reopen time
// for a small N. The synchronous level (FULL vs NORMAL) affects commit
// latency but not the volume of WAL frames committed, so its effect on
// reopen time should also be negligible for small N.
//
// What this campaign measures per cell (synchronous × wal_autocheckpoint × N):
//   1. Time to perform N sequential durable commits.
//   2. Time to close and reopen the store after those N commits.
//   3. WAL file size at the moment before reopen.
//   4. Whether all N idempotency records are visible after reopen.
//
// The campaign does NOT modify production retry/pacing or the default
// wal_autocheckpoint (1000 pages). It uses OpenWithOptions with an explicit
// WalAutoCheckpoint parameter only in the test harness.

type walCheckpointReopenCell struct {
	Synchronous       string `json:"synchronous"`
	WalAutoCheckpoint int    `json:"wal_auto_checkpoint"` // pages, -1 = disabled
	NumCommits        int    `json:"num_commits"`
	CommitDurationMs  int64  `json:"commit_duration_ms"`
	ReopenDurationMs  int64  `json:"reopen_duration_ms"`
	WalSizeBytes      int64  `json:"wal_size_bytes"`
	RecordsVisible    bool   `json:"records_visible"`
}

type walCheckpointReopenMatrix struct {
	SchemaVersion string                    `json:"schema_version"`
	Cells         []walCheckpointReopenCell `json:"cells"`
}

func TestSQLiteRejectsInvalidWalAutoCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	if _, err := sqlite.OpenWithOptions(path, sqlite.Options{WalAutoCheckpoint: -2}); err == nil {
		t.Fatal("expected invalid negative wal autocheckpoint to be rejected")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid option must not create database, stat error = %v", err)
	}
}

func TestSQLiteWalCheckpointReopenCampaign(t *testing.T) {
	ctx := context.Background()

	// Matrix dimensions:
	//   synchronous: FULL, NORMAL
	//   wal_autocheckpoint: 100 (aggressive), 1000 (SQLite default), -1 (disabled)
	//   N commits: 5, 50
	//
	// Total: 2 × 3 × 2 = 12 cells. Each cell is a self-contained test
	// with its own temp directory and database file.
	syncVariants := []string{"FULL", "NORMAL"}
	autoCheckpoints := []int{100, 1000, -1} // 1000 is SQLite default
	commitCounts := []int{5, 50}

	matrix := walCheckpointReopenMatrix{SchemaVersion: "motor-autonomo.sqlite-wal-checkpoint-reopen.v1"}

	for _, sync := range syncVariants {
		for _, autoChk := range autoCheckpoints {
			for _, n := range commitCounts {
				cell := runWalCheckpointReopenCell(t, ctx, sync, autoChk, n)
				matrix.Cells = append(matrix.Cells, cell)
				t.Logf("sync=%s autochk=%d n=%d commit=%dms reopen=%dms wal=%dB visible=%v",
					sync, autoChk, n, cell.CommitDurationMs, cell.ReopenDurationMs, cell.WalSizeBytes, cell.RecordsVisible)
			}
		}
	}

	// Assert all cells preserved all records.
	for _, c := range matrix.Cells {
		if !c.RecordsVisible {
			t.Fatalf("sync=%s autochk=%d n=%d: records not visible after reopen", c.Synchronous, c.WalAutoCheckpoint, c.NumCommits)
		}
	}

	// Write versioned report if requested.
	reportPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_WAL_REOPEN_REPORT")
	if reportPath != "" {
		body, err := json.MarshalIndent(matrix, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(reportPath, append(body, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func runWalCheckpointReopenCell(t *testing.T, ctx context.Context, synchronous string, walAutoCheckpoint, numCommits int) walCheckpointReopenCell {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")

	// Phase 1: open the store and perform N commits.
	opts := sqlite.Options{
		Synchronous:       synchronous,
		WalAutoCheckpoint: walAutoCheckpoint,
	}
	store, err := sqlite.OpenWithOptions(dbPath, opts)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	keys := make([]string, numCommits)
	commitStart := time.Now()
	for i := 0; i < numCommits; i++ {
		key := fmt.Sprintf("reopen-test-%s-%d-%d-%d", synchronous, walAutoCheckpoint, numCommits, i)
		keys[i] = key
		if err := store.Update(ctx, reserveIdempotency(key)); err != nil {
			store.Close()
			t.Fatalf("commit %d: %v", i, err)
		}
	}
	commitDuration := time.Since(commitStart)

	// Measure WAL file size before closing.
	walPath := dbPath + "-wal"
	walSize := int64(0)
	if info, err := os.Stat(walPath); err == nil {
		walSize = info.Size()
	}

	// Phase 2: close and reopen, measuring duration.
	reopenStart := time.Now()
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened, err := sqlite.OpenWithOptions(dbPath, opts)
	reopenDuration := time.Since(reopenStart)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()

	// Phase 3: verify all records are visible after reopen.
	recordsVisible := true
	err = reopened.View(ctx, func(r port.Reader) error {
		for _, key := range keys {
			if _, err := r.IdempotencyRecord(domain.IdempotencyKey(key)); err != nil {
				return fmt.Errorf("key %s missing after reopen: %w", key, err)
			}
		}
		return nil
	})
	if err != nil {
		recordsVisible = false
	}

	return walCheckpointReopenCell{
		Synchronous:       synchronous,
		WalAutoCheckpoint: walAutoCheckpoint,
		NumCommits:        numCommits,
		CommitDurationMs:  commitDuration.Milliseconds(),
		ReopenDurationMs:  reopenDuration.Milliseconds(),
		WalSizeBytes:      walSize,
		RecordsVisible:    recordsVisible,
	}
}
