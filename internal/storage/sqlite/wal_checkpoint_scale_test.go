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

// WAL checkpoint scale campaign — extends phase180 with N=500/5000 to cross
// the default 1000-page autocheckpoint threshold, and compares explicit
// PRAGMA wal_checkpoint(TRUNCATE) before close against passive WAL replay
// on reopen.
//
// Hypothesis: at N=5000 with wal_autocheckpoint=1000 (default), the WAL has
// been auto-checkpointed multiple times during the commit phase; the WAL
// file at close should be smaller than with autocheckpoint=-1 (disabled),
// reducing reopen replay cost. An explicit TRUNCATE before close should
// eliminate WAL replay entirely on reopen, at the cost of the truncate
// operation itself. The difference should be most pronounced at N=5000
// with checkpointing disabled (large WAL) and FULL synchronous (slow
// writes make each replay page costlier).
//
// What this campaign measures per cell:
//   synchronous × wal_autocheckpoint × N × close_mode
//
// close_mode:
//   "passive"  — close the store normally; the WAL remains and is replayed
//                on the next Open.
//   "truncate" — issue PRAGMA wal_checkpoint(TRUNCATE) before close; the
//                WAL is reset to zero and reopen reads only the main DB.
//
// Metrics per cell:
//   1. Time to perform N sequential durable commits.
//   2. WAL file size before close.
//   3. Time to execute PRAGMA wal_checkpoint(TRUNCATE) (only for truncate mode).
//   4. WAL file size after truncate (should be 0 for truncate mode).
//   5. Time to close and reopen the store.
//   6. Whether all N idempotency records are visible after reopen.
//   7. WAL file size after reopen (should be the initial empty WAL or absent).

type walCheckpointScaleCell struct {
	Synchronous          string `json:"synchronous"`
	WalAutoCheckpoint    int    `json:"wal_auto_checkpoint"`
	NumCommits           int    `json:"num_commits"`
	CloseMode            string `json:"close_mode"` // "passive" or "truncate"
	CommitDurationMs     int64  `json:"commit_duration_ms"`
	WalSizeBeforeClose   int64  `json:"wal_size_before_close_bytes"`
	TruncateDurationMs   int64  `json:"truncate_duration_ms,omitempty"`
	WalSizeAfterTruncate int64  `json:"wal_size_after_truncate_bytes,omitempty"`
	ReopenDurationMs     int64  `json:"reopen_duration_ms"`
	WalSizeAfterReopen   int64  `json:"wal_size_after_reopen_bytes"`
	RecordsVisible       bool   `json:"records_visible"`
	TruncateBusy         bool   `json:"truncate_busy,omitempty"`
}

type walCheckpointScaleMatrix struct {
	SchemaVersion string                   `json:"schema_version"`
	Cells         []walCheckpointScaleCell `json:"cells"`
}

func TestSQLiteWalCheckpointScaleCampaign(t *testing.T) {
	ctx := context.Background()

	// Matrix dimensions:
	//   synchronous: FULL, NORMAL
	//   wal_autocheckpoint: 1000 (SQLite default), -1 (disabled)
	//   N commits: 500, 2000  (500 crosses ~half a default-threshold;
	//                           2000 crosses the threshold ~2× and
	//                           stays within a bounded wall-clock budget)
	//   close_mode: passive, truncate
	//
	// Total: 2 × 2 × 2 × 2 = 16 cells.
	syncVariants := []string{"FULL", "NORMAL"}
	autoCheckpoints := []int{1000, -1}
	commitCounts := []int{500, 2000}
	closeModes := []string{"passive", "truncate"}

	matrix := walCheckpointScaleMatrix{
		SchemaVersion: "motor-autonomo.sqlite-wal-checkpoint-reopen.v2",
	}

	for _, sync := range syncVariants {
		for _, autoChk := range autoCheckpoints {
			for _, n := range commitCounts {
				for _, closeMode := range closeModes {
					cell := runWalCheckpointScaleCell(t, ctx, sync, autoChk, n, closeMode)
					matrix.Cells = append(matrix.Cells, cell)
					t.Logf("sync=%s autochk=%d n=%d close=%s commit=%dms reopen=%dms wal_before=%dB wal_after=%dB visible=%v",
						sync, autoChk, n, closeMode,
						cell.CommitDurationMs, cell.ReopenDurationMs,
						cell.WalSizeBeforeClose, cell.WalSizeAfterReopen,
						cell.RecordsVisible)
				}
			}
		}
	}

	// Assert all cells preserved all records.
	for _, c := range matrix.Cells {
		if !c.RecordsVisible {
			t.Fatalf("records not visible: sync=%s autochk=%d n=%d close=%s",
				c.Synchronous, c.WalAutoCheckpoint, c.NumCommits, c.CloseMode)
		}
	}

	// Write versioned report if requested.
	reportPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_WAL_SCALE_REPORT")
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

func runWalCheckpointScaleCell(t *testing.T, ctx context.Context, synchronous string, walAutoCheckpoint, numCommits int, closeMode string) walCheckpointScaleCell {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	walPath := dbPath + "-wal"

	opts := sqlite.Options{
		Synchronous:       synchronous,
		WalAutoCheckpoint: walAutoCheckpoint,
	}
	store, err := sqlite.OpenWithOptions(dbPath, opts)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	// Phase 1: perform N commits.
	// Use a short key prefix to avoid OOM with 5000 keys.
	keys := make([]string, numCommits)
	commitStart := time.Now()
	for i := 0; i < numCommits; i++ {
		key := fmt.Sprintf("k-%d-%d-%d-%d", walAutoCheckpoint, numCommits, len(synchronous), i)
		keys[i] = key
		if err := store.Update(ctx, reserveIdempotency(key)); err != nil {
			store.Close()
			t.Fatalf("commit %d: %v", i, err)
		}
	}
	commitDuration := time.Since(commitStart)

	// Measure WAL file size before any close/truncate.
	walSizeBefore := fileStatSize(walPath)

	cell := walCheckpointScaleCell{
		Synchronous:        synchronous,
		WalAutoCheckpoint:  walAutoCheckpoint,
		NumCommits:         numCommits,
		CloseMode:          closeMode,
		CommitDurationMs:   commitDuration.Milliseconds(),
		WalSizeBeforeClose: walSizeBefore,
	}

	// Phase 2: optionally truncate the WAL before close.
	if closeMode == "truncate" {
		truncStart := time.Now()
		result, err := store.Checkpoint()
		truncDuration := time.Since(truncStart)
		if err != nil {
			store.Close()
			t.Fatalf("checkpoint truncate: %v", err)
		}
		cell.TruncateDurationMs = truncDuration.Milliseconds()
		cell.TruncateBusy = result.Busy
		cell.WalSizeAfterTruncate = fileStatSize(walPath)
	}

	// Phase 3: close and reopen, measuring duration.
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

	cell.ReopenDurationMs = reopenDuration.Milliseconds()

	// Phase 4: verify all records visible after reopen.
	cell.RecordsVisible = true
	err = reopened.View(ctx, func(r port.Reader) error {
		for _, key := range keys {
			if _, err := r.IdempotencyRecord(domain.IdempotencyKey(key)); err != nil {
				return fmt.Errorf("key %s missing after reopen: %w", key, err)
			}
		}
		return nil
	})
	if err != nil {
		cell.RecordsVisible = false
	}

	// Measure WAL size after reopen.
	cell.WalSizeAfterReopen = fileStatSize(walPath)

	return cell
}

func fileStatSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}
