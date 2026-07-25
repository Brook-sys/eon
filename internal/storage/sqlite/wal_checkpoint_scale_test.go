package sqlite_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
//   8. Total cost = truncate_ms + reopen_ms (for truncate mode) or reopen_ms
//      (for passive mode), used for paired comparison.

type walCheckpointScaleCell struct {
	Synchronous          string `json:"synchronous"`
	WalAutoCheckpoint    int    `json:"wal_auto_checkpoint"`
	NumCommits           int    `json:"num_commits"`
	CloseMode            string `json:"close_mode"` // "passive" or "truncate"
	CommitDurationMs     int64  `json:"commit_duration_ms"`
	WalSizeBeforeClose   int64  `json:"wal_size_before_close_bytes"`
	TruncateDurationMs   int64  `json:"truncate_duration_ms,omitempty"`
	TruncateBusy         bool   `json:"truncate_busy,omitempty"`
	TruncateLogPages     int    `json:"truncate_log_pages"`
	TruncateCheckpointed int    `json:"truncate_checkpointed_pages"`
	WalSizeAfterTruncate int64  `json:"wal_size_after_truncate_bytes,omitempty"`
	ReopenDurationMs     int64  `json:"reopen_duration_ms"`
	WalSizeAfterReopen   int64  `json:"wal_size_after_reopen_bytes"`
	RecordsVisible       bool   `json:"records_visible"`
}

type walCheckpointScaleMatrix struct {
	SchemaVersion string                   `json:"schema_version"`
	Cells         []walCheckpointScaleCell `json:"cells"`
}

type walCheckpointProgress struct {
	SchemaVersion string                      `json:"schema_version"`
	Quick         bool                        `json:"quick"`
	RepeatCount   int                         `json:"repeat_count"`
	Randomized    bool                        `json:"randomized"`
	Cells         []walCheckpointProgressCell `json:"cells"`
}

type walCheckpointProgressCell struct {
	Run  int                    `json:"run"`
	Cell walCheckpointScaleCell `json:"cell"`
}

type walCheckpointIntDistribution struct {
	Values []int64 `json:"values"`
	P50    int64   `json:"p50"`
	P95    int64   `json:"p95"`
	Min    int64   `json:"min"`
	Max    int64   `json:"max"`
}

type walCheckpointFloatDistribution struct {
	Values []float64 `json:"values"`
	P50    float64   `json:"p50"`
	P95    float64   `json:"p95"`
	Min    float64   `json:"min"`
	Max    float64   `json:"max"`
}

type walCheckpointAggregatePair struct {
	Synchronous       string                         `json:"synchronous"`
	WalAutoCheckpoint int                            `json:"wal_auto_checkpoint"`
	NumCommits        int                            `json:"num_commits"`
	SampleCount       int                            `json:"sample_count"`
	PassiveReopenMs   walCheckpointIntDistribution   `json:"passive_reopen_ms"`
	TruncateReopenMs  walCheckpointIntDistribution   `json:"truncate_reopen_ms"`
	Speedup           walCheckpointFloatDistribution `json:"speedup"`
}

type walCheckpointAggregateReport struct {
	SchemaVersion string                       `json:"schema_version"`
	RepeatCount   int                          `json:"repeat_count"`
	Pairs         []walCheckpointAggregatePair `json:"pairs"`
}

type walCheckpointPairKey struct {
	sync string
	auto int
	n    int
}

type walCheckpointSamples struct{ passive, truncate []int64 }

func containsFocusCell(filter, cellID string) bool {
	for _, candidate := range strings.Split(filter, ",") {
		if strings.TrimSpace(candidate) == cellID {
			return true
		}
	}
	return false
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
	if os.Getenv("MOTOR_AUTONOMO_SQLITE_WAL_SCALE_QUICK") == "1" {
		commitCounts = []int{5, 20}
	}
	closeModes := []string{"passive", "truncate"}

	// Optional focus filter: restrict the matrix to a subset of cells for
	// targeted soak campaigns (e.g. only FULL/1000/2000).  The filter is a
	// comma-separated list of "SYNC/AUTOCHK/N/CLOSE_MODE" tuples.  When set,
	// only matching cells are included in the matrix.  This lets us run more
	// repetitions on a single pair without executing all 16 cells.
	focusFilter := os.Getenv("MOTOR_AUTONOMO_SQLITE_WAL_SCALE_FOCUS")

	// Build the full matrix of cell descriptors.
	type cellDesc struct {
		sync      string
		autoChk   int
		n         int
		closeMode string
	}
	var descs []cellDesc
	for _, sync := range syncVariants {
		for _, autoChk := range autoCheckpoints {
			for _, n := range commitCounts {
				for _, closeMode := range closeModes {
					cellID := fmt.Sprintf("%s/%d/%d/%s", sync, autoChk, n, closeMode)
					if focusFilter != "" && !containsFocusCell(focusFilter, cellID) {
						continue
					}
					descs = append(descs, cellDesc{sync, autoChk, n, closeMode})
				}
			}
		}
	}
	if focusFilter != "" && len(descs) == 0 {
		t.Fatalf("focus filter %q matched no cells", focusFilter)
	}

	matrix := walCheckpointScaleMatrix{
		SchemaVersion: "motor-autonomo.sqlite-wal-checkpoint-reopen.v3",
	}
	repeats := envBoundedInt(t, "MOTOR_AUTONOMO_SQLITE_WAL_SCALE_REPEATS", 1, 1, 100)
	quick := os.Getenv("MOTOR_AUTONOMO_SQLITE_WAL_SCALE_QUICK") == "1"
	randomized := os.Getenv("MOTOR_AUTONOMO_SQLITE_WAL_SCALE_RANDOMIZE") == "1"
	progressPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_WAL_SCALE_PROGRESS")
	progress := walCheckpointProgress{SchemaVersion: "motor-autonomo.sqlite-wal-checkpoint-progress.v1", Quick: quick, RepeatCount: repeats, Randomized: randomized}
	if progressPath != "" {
		var err error
		progress, err = loadWalCheckpointProgress(progressPath, progress)
		if err != nil {
			t.Fatal(err)
		}
	}
	completed := make(map[string]walCheckpointScaleCell)
	for _, saved := range progress.Cells {
		completed[walCheckpointProgressKey(saved.Run, saved.Cell)] = saved.Cell
	}
	maxNewCells := envBoundedInt(t, "MOTOR_AUTONOMO_SQLITE_WAL_SCALE_MAX_NEW_CELLS", len(descs)*repeats, 1, len(descs)*repeats)
	newCells := 0
	samples := make(map[walCheckpointPairKey]*walCheckpointSamples)
	for run := 0; run < repeats; run++ {
		runDescs := append([]cellDesc(nil), descs...)
		if randomized {
			rng := rand.New(rand.NewSource(42 + int64(run)))
			rng.Shuffle(len(runDescs), func(i, j int) { runDescs[i], runDescs[j] = runDescs[j], runDescs[i] })
		}
		for _, d := range runDescs {
			identity := walCheckpointScaleCell{Synchronous: d.sync, WalAutoCheckpoint: d.autoChk, NumCommits: d.n, CloseMode: d.closeMode}
			keyString := walCheckpointProgressKey(run+1, identity)
			cell, ok := completed[keyString]
			if !ok {
				if newCells >= maxNewCells {
					continue
				}
				cell = runWalCheckpointScaleCell(t, ctx, d.sync, d.autoChk, d.n, d.closeMode)
				newCells++
				progress.Cells = append(progress.Cells, walCheckpointProgressCell{Run: run + 1, Cell: cell})
				completed[keyString] = cell
				if progressPath != "" {
					if err := writeJSONAtomic(progressPath, progress); err != nil {
						t.Fatal(err)
					}
				}
			}
			if run == 0 {
				matrix.Cells = append(matrix.Cells, cell)
			}
			key := walCheckpointPairKey{d.sync, d.autoChk, d.n}
			if samples[key] == nil {
				samples[key] = &walCheckpointSamples{}
			}
			if d.closeMode == "passive" {
				samples[key].passive = append(samples[key].passive, cell.ReopenDurationMs)
			} else {
				samples[key].truncate = append(samples[key].truncate, cell.TruncateDurationMs+cell.ReopenDurationMs)
			}
			t.Logf("run=%d sync=%s autochk=%d n=%d close=%s commit=%dms trunc=%dms reopen=%dms wal_before=%dB wal_after_trunc=%dB wal_after=%dB visible=%v busy=%v",
				run+1, d.sync, d.autoChk, d.n, d.closeMode,
				cell.CommitDurationMs, cell.TruncateDurationMs, cell.ReopenDurationMs,
				cell.WalSizeBeforeClose, cell.WalSizeAfterTruncate, cell.WalSizeAfterReopen,
				cell.RecordsVisible, cell.TruncateBusy)
		}
	}

	if len(completed) < len(descs)*repeats {
		t.Logf("campaign paused after %d new cells: %d/%d durable cells complete", newCells, len(completed), len(descs)*repeats)
		return
	}

	// Assert all cells preserved all records.
	for _, c := range matrix.Cells {
		if !c.RecordsVisible {
			t.Fatalf("records not visible: sync=%s autochk=%d n=%d close=%s",
				c.Synchronous, c.WalAutoCheckpoint, c.NumCommits, c.CloseMode)
		}
	}

	// Paired comparison: for each (sync, autoChk, n), compare passive vs truncate total cost.
	// Group cells by their shared dimensions.
	type pairKey struct {
		sync    string
		autoChk int
		n       int
	}
	passiveTotal := make(map[pairKey]int64)
	truncateTotal := make(map[pairKey]int64)
	for _, c := range matrix.Cells {
		key := pairKey{c.Synchronous, c.WalAutoCheckpoint, c.NumCommits}
		if c.CloseMode == "passive" {
			passiveTotal[key] = c.ReopenDurationMs
		} else {
			truncateTotal[key] = c.TruncateDurationMs + c.ReopenDurationMs
		}
	}
	t.Logf("paired comparison (reopen-only passive vs truncate+reopen):")
	var pairKeys []pairKey
	for k := range passiveTotal {
		pairKeys = append(pairKeys, k)
	}
	sort.Slice(pairKeys, func(i, j int) bool {
		if pairKeys[i].sync != pairKeys[j].sync {
			return pairKeys[i].sync < pairKeys[j].sync
		}
		if pairKeys[i].autoChk != pairKeys[j].autoChk {
			return pairKeys[i].autoChk < pairKeys[j].autoChk
		}
		return pairKeys[i].n < pairKeys[j].n
	})
	for _, k := range pairKeys {
		passiveMs := passiveTotal[k]
		truncMs := truncateTotal[k]
		ratio := "N/A"
		if truncMs > 0 {
			ratio = fmt.Sprintf("%.2fx", float64(passiveMs)/float64(truncMs))
		}
		t.Logf("  sync=%s autochk=%d n=%d: passive_reopen=%dms truncate+reopen=%dms speedup=%s",
			k.sync, k.autoChk, k.n, passiveMs, truncMs, ratio)
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
	if aggregatePath := os.Getenv("MOTOR_AUTONOMO_SQLITE_WAL_SCALE_AGGREGATE_REPORT"); aggregatePath != "" {
		report := buildWalCheckpointAggregate(repeats, samples)
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(aggregatePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(aggregatePath, append(body, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func walCheckpointProgressKey(run int, cell walCheckpointScaleCell) string {
	return fmt.Sprintf("%d|%s|%d|%d|%s", run, cell.Synchronous, cell.WalAutoCheckpoint, cell.NumCommits, cell.CloseMode)
}

func loadWalCheckpointProgress(path string, expected walCheckpointProgress) (walCheckpointProgress, error) {
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return expected, nil
	}
	if err != nil {
		return walCheckpointProgress{}, err
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	var got walCheckpointProgress
	if err := dec.Decode(&got); err != nil {
		return walCheckpointProgress{}, fmt.Errorf("decode WAL progress: %w", err)
	}
	if dec.Decode(&struct{}{}) == nil {
		return walCheckpointProgress{}, fmt.Errorf("decode WAL progress: trailing JSON")
	}
	if got.SchemaVersion != expected.SchemaVersion || got.Quick != expected.Quick || got.RepeatCount != expected.RepeatCount || got.Randomized != expected.Randomized {
		return walCheckpointProgress{}, fmt.Errorf("WAL progress configuration mismatch")
	}
	seen := map[string]bool{}
	for _, saved := range got.Cells {
		if saved.Run < 1 || saved.Run > got.RepeatCount || !saved.Cell.RecordsVisible {
			return walCheckpointProgress{}, fmt.Errorf("invalid WAL progress cell")
		}
		key := walCheckpointProgressKey(saved.Run, saved.Cell)
		if seen[key] {
			return walCheckpointProgress{}, fmt.Errorf("duplicate WAL progress cell %s", key)
		}
		seen[key] = true
	}
	return got, nil
}

func writeJSONAtomic(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".wal-progress-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
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
		cell.TruncateLogPages = result.LogPages
		cell.TruncateCheckpointed = result.CheckpointedPages
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

func envBoundedInt(t *testing.T, key string, defVal, minVal, maxVal int) int {
	t.Helper()
	v := defVal
	if raw := os.Getenv(key); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("%s: not an integer: %q", key, raw)
		}
		v = parsed
	}
	if v < minVal || v > maxVal {
		t.Fatalf("%s: value %d out of [%d, %d]", key, v, minVal, maxVal)
	}
	return v
}

// intPercentile returns the p-th percentile (0–100) of a sorted copy of vals.
// Uses nearest-rank: p=50 → median, p=95 → 95th percentile.
func intPercentile(vals []int64, p float64) int64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := append([]int64(nil), vals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(math.Ceil((p/100.0)*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// intDistribution builds a walCheckpointIntDistribution from a slice of int64 samples.
func intDistribution(vals []int64) walCheckpointIntDistribution {
	sorted := append([]int64(nil), vals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return walCheckpointIntDistribution{
		Values: sorted,
		P50:    intPercentile(vals, 50),
		P95:    intPercentile(vals, 95),
		Min:    minInt64(vals),
		Max:    maxInt64(vals),
	}
}

// floatPercentile returns the p-th percentile of a sorted copy of vals.
func floatPercentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	idx := int(math.Ceil((p/100.0)*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func floatDistribution(vals []float64) walCheckpointFloatDistribution {
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	return walCheckpointFloatDistribution{
		Values: sorted,
		P50:    floatPercentile(vals, 50),
		P95:    floatPercentile(vals, 95),
		Min:    minFloat64(vals),
		Max:    maxFloat64(vals),
	}
}

func buildWalCheckpointAggregate(repeats int, samples map[walCheckpointPairKey]*walCheckpointSamples) walCheckpointAggregateReport {
	var keys []walCheckpointPairKey
	for k := range samples {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].sync != keys[j].sync {
			return keys[i].sync < keys[j].sync
		}
		if keys[i].auto != keys[j].auto {
			return keys[i].auto < keys[j].auto
		}
		return keys[i].n < keys[j].n
	})
	var pairs []walCheckpointAggregatePair
	for _, k := range keys {
		s := samples[k]
		var speedups []float64
		minLen := len(s.passive)
		if len(s.truncate) < minLen {
			minLen = len(s.truncate)
		}
		for i := 0; i < minLen; i++ {
			if s.truncate[i] > 0 {
				speedups = append(speedups, float64(s.passive[i])/float64(s.truncate[i]))
			} else {
				speedups = append(speedups, 0)
			}
		}
		pairs = append(pairs, walCheckpointAggregatePair{
			Synchronous:       k.sync,
			WalAutoCheckpoint: k.auto,
			NumCommits:        k.n,
			SampleCount:       minLen,
			PassiveReopenMs:   intDistribution(s.passive),
			TruncateReopenMs:  intDistribution(s.truncate),
			Speedup:           floatDistribution(speedups),
		})
	}
	return walCheckpointAggregateReport{
		SchemaVersion: "motor-autonomo.sqlite-wal-checkpoint-aggregate.v1",
		RepeatCount:   repeats,
		Pairs:         pairs,
	}
}

func minInt64(vals []int64) int64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxInt64(vals []int64) int64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func minFloat64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxFloat64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func TestContainsFocusCell(t *testing.T) {
	const target = "FULL/1000/2000/passive"
	for _, tc := range []struct {
		name   string
		filter string
		want   bool
	}{
		{name: "exact", filter: target, want: true},
		{name: "list with whitespace", filter: "NORMAL/1000/500/passive, " + target, want: true},
		{name: "absent", filter: "FULL/1000/2000/truncate", want: false},
		{name: "no partial match", filter: "FULL/1000/2000", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := containsFocusCell(tc.filter, target); got != tc.want {
				t.Fatalf("containsFocusCell(%q, %q) = %v, want %v", tc.filter, target, got, tc.want)
			}
		})
	}
}
