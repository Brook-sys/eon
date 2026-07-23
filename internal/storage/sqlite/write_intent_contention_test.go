package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

const writeIntentHelperEnv = "MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_HELPER"

type writeIntentAttemptReport struct {
	Mode                  string `json:"mode"`
	Cycle                 int    `json:"cycle"`
	Worker                int    `json:"worker"`
	Leader                bool   `json:"leader"`
	Outcome               string `json:"outcome"`
	ElapsedMicros         int64  `json:"elapsed_micros"`
	CallbackMicros        int64  `json:"callback_micros"`
	BeginMicros           int64  `json:"begin_micros"`
	WriteCASMicros        int64  `json:"write_cas_micros"`
	CommitMicros          int64  `json:"commit_micros"`
	ConflictReloadMicros  int64  `json:"conflict_reload_micros"`
	TransactionOpenMicros int64  `json:"transaction_open_micros"`
	Error                 string `json:"error,omitempty"`
}

type writeIntentDistribution struct {
	Count int   `json:"count"`
	P50   int64 `json:"p50_micros"`
	P95   int64 `json:"p95_micros"`
	Max   int64 `json:"max_micros"`
}

type writeIntentModeSummary struct {
	Mode            string                     `json:"mode"`
	Attempts        int                        `json:"attempts"`
	Successes       int                        `json:"successes"`
	Conflicts       int                        `json:"conflicts"`
	WinsByWorker    []int                      `json:"wins_by_worker"`
	Elapsed         writeIntentDistribution    `json:"elapsed"`
	Begin           writeIntentDistribution    `json:"begin"`
	WriteCAS        writeIntentDistribution    `json:"write_cas"`
	TransactionOpen writeIntentDistribution    `json:"transaction_open"`
	ConflictBegin   writeIntentDistribution    `json:"conflict_begin"`
	ConflictWrite   writeIntentDistribution    `json:"conflict_write_cas"`
	ConflictReload  writeIntentDistribution    `json:"conflict_reload"`
	Raw             []writeIntentAttemptReport `json:"attempts_raw"`
}

type writeIntentCampaignReport struct {
	SchemaVersion string                   `json:"schema_version"`
	Workers       int                      `json:"workers"`
	CyclesPerMode int                      `json:"cycles_per_mode"`
	HoldMillis    int64                    `json:"leader_hold_millis"`
	Modes         []writeIntentModeSummary `json:"modes"`
}

// TestSQLiteWriteIntentContentionCampaign is an isolated experiment. It does
// not change Store.Update: it compares the current deferred transaction with a
// test-only path that acquires BEGIN IMMEDIATE before cloning/callback work.
func TestSQLiteWriteIntentContentionCampaign(t *testing.T) {
	if os.Getenv(writeIntentHelperEnv) != "" {
		runWriteIntentHelper(t)
		return
	}

	const workers = 4
	const cycles = 6
	const hold = 300 * time.Millisecond
	report := writeIntentCampaignReport{
		SchemaVersion: "motor-autonomo.sqlite-write-intent-campaign.v1",
		Workers:       workers, CyclesPerMode: cycles, HoldMillis: hold.Milliseconds(),
	}
	for _, mode := range []string{"deferred", "immediate_before_callback"} {
		attempts := make([]writeIntentAttemptReport, 0, workers*cycles)
		for cycle := 0; cycle < cycles; cycle++ {
			attempts = append(attempts, runWriteIntentCycle(t, mode, cycle, cycle%workers, workers, hold)...)
		}
		report.Modes = append(report.Modes, summarizeWriteIntentMode(t, mode, workers, cycles, attempts))
	}
	writeWriteIntentCampaignReport(t, report)
	for _, summary := range report.Modes {
		t.Logf("write intent mode=%s attempts=%d conflicts=%d begin_conflict_p50=%dus write_conflict_p50=%dus lock_held_p95=%dus wins=%v",
			summary.Mode, summary.Attempts, summary.Conflicts, summary.ConflictBegin.P50,
			summary.ConflictWrite.P50, summary.TransactionOpen.P95, summary.WinsByWorker)
	}
}

func runWriteIntentCycle(t *testing.T, mode string, cycle, leader, workers int, hold time.Duration) []writeIntentAttemptReport {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	seed, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	commands := make([]*exec.Cmd, workers)
	ready := make([]string, workers)
	start := make([]string, workers)
	result := make([]string, workers)
	lockHeld := filepath.Join(dir, "lock-held")
	for worker := 0; worker < workers; worker++ {
		ready[worker] = filepath.Join(dir, fmt.Sprintf("ready-%d", worker))
		start[worker] = filepath.Join(dir, fmt.Sprintf("start-%d", worker))
		result[worker] = filepath.Join(dir, fmt.Sprintf("result-%d.json", worker))
		commands[worker] = writeIntentHelperCommand(mode, cycle, worker, leader, hold, dbPath, ready[worker], start[worker], result[worker], lockHeld)
		if err := commands[worker].Start(); err != nil {
			t.Fatalf("mode %s cycle %d start worker %d: %v", mode, cycle, worker, err)
		}
		waitForWriteIntentFile(t, ready[worker], 5*time.Second)
	}
	if err := os.WriteFile(start[leader], []byte("start\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForWriteIntentFile(t, lockHeld, 5*time.Second)
	for worker := 0; worker < workers; worker++ {
		if worker == leader {
			continue
		}
		if err := os.WriteFile(start[worker], []byte("start\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	attempts := make([]writeIntentAttemptReport, workers)
	for worker, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("mode %s cycle %d worker %d: %v", mode, cycle, worker, err)
		}
		body, err := os.ReadFile(result[worker])
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &attempts[worker]); err != nil {
			t.Fatal(err)
		}
		if attempts[worker].Error != "" {
			t.Fatalf("mode %s cycle %d worker %d: %s", mode, cycle, worker, attempts[worker].Error)
		}
	}
	return attempts
}

func runWriteIntentHelper(t *testing.T) {
	mode := os.Getenv(writeIntentHelperEnv)
	var cycle, worker, leader int
	if _, err := fmt.Sscanf(os.Getenv("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_CYCLE"), "%d", &cycle); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Sscanf(os.Getenv("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_WORKER"), "%d", &worker); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Sscanf(os.Getenv("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_LEADER"), "%d", &leader); err != nil {
		t.Fatal(err)
	}
	var holdMillis int64
	if _, err := fmt.Sscanf(os.Getenv("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_HOLD_MS"), "%d", &holdMillis); err != nil {
		t.Fatal(err)
	}
	dbPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_DB")
	readyPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_READY")
	startPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_START")
	resultPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_RESULT")
	lockHeldPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_LOCK_HELD")
	if dbPath == "" || readyPath == "" || startPath == "" || resultPath == "" || lockHeldPath == "" {
		t.Fatal("write-intent helper paths are incomplete")
	}

	dsn := dbPath
	if mode == "immediate_before_callback" {
		dsn = "file:" + filepath.ToSlash(dbPath) + "?_txlock=immediate"
	} else if mode != "deferred" {
		t.Fatalf("unknown write-intent mode %q", mode)
	}
	var observed UpdateTiming
	options := Options{ObserveUpdate: func(timing UpdateTiming) { observed = timing }}
	if worker == leader {
		options.Failpoint = func(point Failpoint) {
			if point != FailpointBeforeDurableCommit {
				return
			}
			if err := os.WriteFile(lockHeldPath, []byte("held\n"), 0o600); err != nil {
				panic(err)
			}
			time.Sleep(time.Duration(holdMillis) * time.Millisecond)
		}
	}
	store, err := OpenWithOptions(dsn, options)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForWriteIntentFile(t, startPath, 5*time.Second)

	started := time.Now()
	var updateErr error
	var transactionOpen time.Duration
	fn := reserveWriteIntentKey(cycle, worker)
	if mode == "deferred" {
		updateErr = store.Update(context.Background(), fn)
		transactionOpen = observed.WriteCAS + observed.Commit
	} else {
		updateErr, observed, transactionOpen = updateImmediateBeforeCallback(context.Background(), store, fn)
	}
	outcome := "success"
	if errors.Is(updateErr, port.ErrConflict) {
		outcome = "cas_conflict"
	} else if updateErr != nil {
		outcome = "error"
	}
	attempt := writeIntentAttemptReport{
		Mode: mode, Cycle: cycle, Worker: worker, Leader: worker == leader, Outcome: outcome,
		ElapsedMicros: time.Since(started).Microseconds(), CallbackMicros: observed.Callback.Microseconds(),
		BeginMicros: observed.Begin.Microseconds(), WriteCASMicros: observed.WriteCAS.Microseconds(),
		CommitMicros: observed.Commit.Microseconds(), ConflictReloadMicros: observed.ConflictReload.Microseconds(),
		TransactionOpenMicros: transactionOpen.Microseconds(),
	}
	if updateErr != nil && !errors.Is(updateErr, port.ErrConflict) {
		attempt.Error = updateErr.Error()
	}
	body, err := json.Marshal(attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// updateImmediateBeforeCallback duplicates only the experimental ordering that
// cannot be expressed through the production Store.Update contract. The DSN's
// _txlock=immediate makes BeginTx acquire SQLite write intent.
func updateImmediateBeforeCallback(ctx context.Context, s *Store, fn func(port.Transaction) error) (err error, timing UpdateTiming, transactionOpen time.Duration) {
	if err := ctx.Err(); err != nil {
		return err, timing, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	started := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	timing.Begin = time.Since(started)
	if err != nil {
		return fmt.Errorf("begin immediate sqlite checkpoint: %w", err), timing, 0
	}
	lockStarted := time.Now()
	defer tx.Rollback()

	before, err := s.core.MarshalBinary()
	if err != nil {
		return err, timing, time.Since(lockStarted)
	}
	working, err := memory.NewFromBinary(before)
	if err != nil {
		return err, timing, time.Since(lockStarted)
	}
	started = time.Now()
	err = working.Update(ctx, fn)
	timing.Callback = time.Since(started)
	if err != nil {
		return err, timing, time.Since(lockStarted)
	}
	payload, err := working.MarshalBinary()
	if err != nil {
		return err, timing, time.Since(lockStarted)
	}
	started = time.Now()
	result, err := tx.ExecContext(ctx, `INSERT INTO runtime_checkpoint(id, format_version, payload)
		VALUES(?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET format_version=excluded.format_version, payload=excluded.payload
		WHERE runtime_checkpoint.format_version = ? AND runtime_checkpoint.payload = ?`,
		checkpointID, memory.CheckpointFormatVersion, payload, s.persistedFormat, s.persistedPayload)
	timing.WriteCAS = time.Since(started)
	if err != nil {
		return fmt.Errorf("write immediate sqlite checkpoint: %w", err), timing, time.Since(lockStarted)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect immediate sqlite checkpoint write: %w", err), timing, time.Since(lockStarted)
	}
	if rows != 1 {
		if err := tx.Rollback(); err != nil {
			return fmt.Errorf("rollback stale immediate sqlite checkpoint: %w", err), timing, time.Since(lockStarted)
		}
		transactionOpen = time.Since(lockStarted)
		started = time.Now()
		latest, format, persisted, err := loadCheckpoint(s.db)
		timing.ConflictReload = time.Since(started)
		if err != nil {
			return fmt.Errorf("reload immediate sqlite checkpoint after conflict: %w", err), timing, transactionOpen
		}
		s.core = latest
		s.persistedFormat = format
		s.persistedPayload = persisted
		return port.ErrConflict, timing, transactionOpen
	}
	if s.failpoint != nil {
		s.failpoint(FailpointBeforeDurableCommit)
	}
	started = time.Now()
	if err := tx.Commit(); err != nil {
		timing.Commit = time.Since(started)
		return fmt.Errorf("commit immediate sqlite checkpoint: %w", err), timing, time.Since(lockStarted)
	}
	timing.Commit = time.Since(started)
	transactionOpen = time.Since(lockStarted)
	if s.failpoint != nil {
		s.failpoint(FailpointAfterDurableCommit)
	}
	s.core = working
	s.persistedFormat = memory.CheckpointFormatVersion
	s.persistedPayload = append(s.persistedPayload[:0], payload...)
	return nil, timing, transactionOpen
}

func reserveWriteIntentKey(cycle, worker int) func(port.Transaction) error {
	return func(tx port.Transaction) error {
		key := domain.IdempotencyKey(fmt.Sprintf("write-intent-%d-%d", cycle, worker))
		_, err := tx.ReserveIdempotency(domain.IdempotencyRecord{
			SchemaVersion: 1, Key: key, OperationID: domain.OperationID("operation-" + string(key)),
			Intent: "write-intent experiment", Status: domain.IdempotencyReserved,
			ReservedAt: time.Date(2026, 7, 23, 20, 40, 0, 0, time.UTC),
		})
		return err
	}
}

func summarizeWriteIntentMode(t *testing.T, mode string, workers, cycles int, attempts []writeIntentAttemptReport) writeIntentModeSummary {
	t.Helper()
	summary := writeIntentModeSummary{Mode: mode, Attempts: len(attempts), WinsByWorker: make([]int, workers), Raw: attempts}
	var elapsed, begin, writeCAS, transactionOpen, conflictBegin, conflictWrite, conflictReload []int64
	winsByCycle := make([]int, cycles)
	for _, attempt := range attempts {
		if attempt.Mode != mode || attempt.Cycle < 0 || attempt.Cycle >= cycles || attempt.Worker < 0 || attempt.Worker >= workers {
			t.Fatalf("malformed write-intent attempt: %+v", attempt)
		}
		elapsed = append(elapsed, attempt.ElapsedMicros)
		begin = append(begin, attempt.BeginMicros)
		writeCAS = append(writeCAS, attempt.WriteCASMicros)
		transactionOpen = append(transactionOpen, attempt.TransactionOpenMicros)
		switch attempt.Outcome {
		case "success":
			summary.Successes++
			summary.WinsByWorker[attempt.Worker]++
			winsByCycle[attempt.Cycle]++
			if !attempt.Leader {
				t.Fatalf("mode %s cycle %d non-leader worker %d won", mode, attempt.Cycle, attempt.Worker)
			}
		case "cas_conflict":
			summary.Conflicts++
			conflictBegin = append(conflictBegin, attempt.BeginMicros)
			conflictWrite = append(conflictWrite, attempt.WriteCASMicros)
			conflictReload = append(conflictReload, attempt.ConflictReloadMicros)
		default:
			t.Fatalf("mode %s unexpected outcome: %+v", mode, attempt)
		}
	}
	if summary.Successes != cycles || summary.Conflicts != cycles*(workers-1) {
		t.Fatalf("mode %s successes=%d conflicts=%d want=%d/%d", mode, summary.Successes, summary.Conflicts, cycles, cycles*(workers-1))
	}
	for cycle, wins := range winsByCycle {
		if wins != 1 {
			t.Fatalf("mode %s cycle %d wins=%d want=1", mode, cycle, wins)
		}
	}
	minWins, maxWins := cycles, 0
	for _, wins := range summary.WinsByWorker {
		if wins < minWins {
			minWins = wins
		}
		if wins > maxWins {
			maxWins = wins
		}
	}
	if maxWins-minWins > 1 {
		t.Fatalf("mode %s unfair wins: %v", mode, summary.WinsByWorker)
	}
	summary.Elapsed = distribution(elapsed)
	summary.Begin = distribution(begin)
	summary.WriteCAS = distribution(writeCAS)
	summary.TransactionOpen = distribution(transactionOpen)
	summary.ConflictBegin = distribution(conflictBegin)
	summary.ConflictWrite = distribution(conflictWrite)
	summary.ConflictReload = distribution(conflictReload)
	return summary
}

func distribution(values []int64) writeIntentDistribution {
	if len(values) == 0 {
		return writeIntentDistribution{}
	}
	values = append([]int64(nil), values...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	percentile := func(p float64) int64 {
		index := int(float64(len(values)-1) * p)
		return values[index]
	}
	return writeIntentDistribution{Count: len(values), P50: percentile(0.50), P95: percentile(0.95), Max: values[len(values)-1]}
}

func writeWriteIntentCampaignReport(t *testing.T, report writeIntentCampaignReport) {
	t.Helper()
	path := os.Getenv("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_REPORT")
	if path == "" {
		return
	}
	body, err := json.MarshalIndent(report, "", "  ")
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

func writeIntentHelperCommand(mode string, cycle, worker, leader int, hold time.Duration, dbPath, readyPath, startPath, resultPath, lockHeldPath string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run", "^TestSQLiteWriteIntentContentionCampaign$")
	cmd.Env = append(os.Environ(),
		writeIntentHelperEnv+"="+mode,
		fmt.Sprintf("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_CYCLE=%d", cycle),
		fmt.Sprintf("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_WORKER=%d", worker),
		fmt.Sprintf("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_LEADER=%d", leader),
		fmt.Sprintf("MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_HOLD_MS=%d", hold.Milliseconds()),
		"MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_DB="+dbPath,
		"MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_READY="+readyPath,
		"MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_START="+startPath,
		"MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_RESULT="+resultPath,
		"MOTOR_AUTONOMO_SQLITE_WRITE_INTENT_LOCK_HELD="+lockHeldPath,
	)
	return cmd
}

func waitForWriteIntentFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
