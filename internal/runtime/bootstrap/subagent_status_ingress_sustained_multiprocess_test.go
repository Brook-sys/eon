package bootstrap_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/retry"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/sqlite"
)

const sustainedIngressMultiprocessMode = "MOTOR_AUTONOMO_SUSTAINED_INGRESS_MULTIPROCESS"

const sustainedIngressRecoveryDelay = 100 * time.Millisecond

const sustainedIngressCampaignReportSchema = "motor-autonomo.sustained-ingress-campaign.v2"

type sustainedIngressAttemptTiming struct {
	Cycle         int    `json:"cycle"`
	Worker        int    `json:"worker"`
	Attempt       int    `json:"attempt"`
	ElapsedMicros int64  `json:"elapsed_micros"`
	Outcome       string `json:"outcome"`
}

type sustainedIngressHostLoad struct {
	Source      string  `json:"source"`
	Available   bool    `json:"available"`
	LogicalCPUs int     `json:"logical_cpus"`
	Load1       float64 `json:"load_1,omitempty"`
	Load5       float64 `json:"load_5,omitempty"`
	Load15      float64 `json:"load_15,omitempty"`
}

type sustainedIngressProcessReport struct {
	Cycle          int                             `json:"cycle"`
	Worker         int                             `json:"worker"`
	Leader         bool                            `json:"leader"`
	Processed      int                             `json:"processed"`
	Attempts       int                             `json:"attempts"`
	Retries        int                             `json:"retries"`
	Conflicts      int                             `json:"conflicts"`
	Exhaustions    int                             `json:"exhaustions"`
	SleepMillis    int64                           `json:"sleep_millis"`
	RecoveryMillis int64                           `json:"recovery_millis"`
	ElapsedMillis  int64                           `json:"elapsed_millis"`
	AttemptTimings []sustainedIngressAttemptTiming `json:"attempt_timings"`
	Error          string                          `json:"error,omitempty"`
}

type sustainedIngressObservedStore struct {
	port.Store
	mu      sync.Mutex
	timings []sustainedIngressAttemptTiming
}

func (s *sustainedIngressObservedStore) Update(ctx context.Context, fn func(port.Transaction) error) error {
	started := time.Now()
	err := s.Store.Update(ctx, fn)
	outcome := "success"
	switch {
	case errors.Is(err, port.ErrConflict):
		outcome = "cas_conflict"
	case err != nil && (strings.Contains(strings.ToLower(err.Error()), "busy") || strings.Contains(strings.ToLower(err.Error()), "locked")):
		outcome = "sqlite_busy"
	case err != nil:
		outcome = "error"
	}
	s.mu.Lock()
	s.timings = append(s.timings, sustainedIngressAttemptTiming{Attempt: len(s.timings) + 1, ElapsedMicros: time.Since(started).Microseconds(), Outcome: outcome})
	s.mu.Unlock()
	return err
}

func (s *sustainedIngressObservedStore) snapshot() []sustainedIngressAttemptTiming {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sustainedIngressAttemptTiming(nil), s.timings...)
}

type sustainedIngressCampaignReport struct {
	SchemaVersion       string                          `json:"schema_version"`
	Workers             int                             `json:"workers"`
	Cycles              int                             `json:"cycles"`
	RetryMaxAttempts    int                             `json:"retry_max_attempts"`
	RecoveryDelayMillis int64                           `json:"recovery_delay_millis"`
	WorkerCycles        int                             `json:"worker_cycles"`
	Attempts            int                             `json:"attempts"`
	Exhaustions         int                             `json:"exhaustions"`
	ExhaustionRate      float64                         `json:"exhaustion_rate"`
	PendingDepth        []int                           `json:"pending_depth"`
	ConvergenceMillis   int64                           `json:"convergence_millis"`
	ProcessedByWorker   []int                           `json:"processed_by_worker"`
	ExhaustionsByWorker []int                           `json:"exhaustions_by_worker"`
	AttemptTimings      []sustainedIngressAttemptTiming `json:"attempt_timings"`
	HostLoadStart       sustainedIngressHostLoad        `json:"host_load_start"`
	HostLoadEnd         sustainedIngressHostLoad        `json:"host_load_end"`
	MatrixRunID         string                          `json:"matrix_run_id,omitempty"`
	MatrixOrdinal       int                             `json:"matrix_ordinal,omitempty"`
	MatrixBlock         int                             `json:"matrix_block,omitempty"`
	OrderPosition       int                             `json:"order_position,omitempty"`
	OrderStrategy       string                          `json:"order_strategy,omitempty"`
	MatrixMetadata      bool                            `json:"matrix_metadata_present"`
}

// TestSubagentStatusIngressSustainedContentionCampaign keeps the production
// per-transaction budget fixed while repeatedly presenting four real SQLite
// processes with a bounded contention wave. Leadership rotates each cycle so
// the campaign can measure, rather than assume, progress fairness.
func TestSubagentStatusIngressSustainedContentionCampaign(t *testing.T) {
	policy := kernel.DefaultSubagentStatusIngressRetryPolicy()
	if policy.MaxAttempts != 3 || policy.BaseDelay != 10*time.Millisecond || policy.MaxDelay != 40*time.Millisecond || policy.MaxJitter != 10*time.Millisecond {
		t.Fatalf("production retry policy changed: %+v", policy)
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	seedMixedIngressReceipts(t, dbPath)

	const workers = 4
	recoveryDelay := sustainedIngressCampaignRecoveryDelay(t)
	matrixRunID, matrixOrdinal, matrixBlock, orderPosition, orderStrategy, matrixMetadata := sustainedIngressMatrixMetadata(t)
	pendingDepth := []int{mixedIngressReceiptCount}
	processedByWorker := make([]int, workers)
	exhaustionsByWorker := make([]int, workers)
	workerCycles := make([]int, workers)
	totalAttempts, totalExhaustions := 0, 0
	attemptTimings := make([]sustainedIngressAttemptTiming, 0, workers*mixedIngressReceiptCount*policy.MaxAttempts)
	hostLoadStart := readSustainedIngressHostLoad()
	started := time.Now()

	for cycle := 0; cycle < mixedIngressReceiptCount; cycle++ {
		leader := cycle % workers
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		commands := make([]*exec.Cmd, workers)
		readyPaths := make([]string, workers)
		startPaths := make([]string, workers)
		resultPaths := make([]string, workers)
		lockHeldPath := filepath.Join(dir, fmt.Sprintf("cycle-%d-lock-held", cycle))
		for worker := 0; worker < workers; worker++ {
			readyPaths[worker] = filepath.Join(dir, fmt.Sprintf("cycle-%d-worker-%d-ready", cycle, worker))
			startPaths[worker] = filepath.Join(dir, fmt.Sprintf("cycle-%d-worker-%d-start", cycle, worker))
			resultPaths[worker] = filepath.Join(dir, fmt.Sprintf("cycle-%d-worker-%d.json", cycle, worker))
			commands[worker] = sustainedIngressHelperCommand(ctx, cycle, worker, leader, dbPath, readyPaths[worker], startPaths[worker], resultPaths[worker], lockHeldPath)
			if err := commands[worker].Start(); err != nil {
				cancel()
				t.Fatalf("cycle %d start worker %d: %v", cycle, worker, err)
			}
			waitForMixedIngressFile(t, readyPaths[worker], 5*time.Second)
		}

		if err := os.WriteFile(startPaths[leader], []byte("start\n"), 0o600); err != nil {
			cancel()
			t.Fatal(err)
		}
		waitForMixedIngressFile(t, lockHeldPath, 5*time.Second)
		for worker := 0; worker < workers; worker++ {
			if worker == leader {
				continue
			}
			if err := os.WriteFile(startPaths[worker], []byte("start\n"), 0o600); err != nil {
				cancel()
				t.Fatal(err)
			}
		}
		for worker, cmd := range commands {
			if err := cmd.Wait(); err != nil {
				cancel()
				t.Fatalf("cycle %d worker %d: %v", cycle, worker, err)
			}
		}
		cancel()

		cycleProcessed, cycleExhaustions := 0, 0
		for worker, path := range resultPaths {
			report := readSustainedIngressProcessReport(t, path)
			if report.Cycle != cycle || report.Worker != worker || report.Leader != (worker == leader) || report.Error != "" {
				t.Fatalf("cycle %d worker %d malformed report: %+v", cycle, worker, report)
			}
			if report.Attempts > policy.MaxAttempts || report.Retries > policy.MaxAttempts-1 || report.Exhaustions > 1 {
				t.Fatalf("cycle %d worker %d exceeded production budget: %+v", cycle, worker, report)
			}
			if report.Exhaustions == 1 && report.RecoveryMillis != recoveryDelay.Milliseconds() {
				t.Fatalf("cycle %d worker %d recovery pacing mismatch: %+v", cycle, worker, report)
			}
			if report.Exhaustions == 0 && report.RecoveryMillis != 0 {
				t.Fatalf("cycle %d worker %d scheduled recovery without exhaustion: %+v", cycle, worker, report)
			}
			cycleProcessed += report.Processed
			cycleExhaustions += report.Exhaustions
			totalAttempts += report.Attempts
			totalExhaustions += report.Exhaustions
			processedByWorker[worker] += report.Processed
			exhaustionsByWorker[worker] += report.Exhaustions
			workerCycles[worker]++
			for _, timing := range report.AttemptTimings {
				timing.Cycle = cycle
				timing.Worker = worker
				attemptTimings = append(attemptTimings, timing)
			}
		}
		if cycleProcessed != 1 {
			t.Fatalf("cycle %d processed=%d want exactly one bounded winner", cycle, cycleProcessed)
		}
		pending := pendingMixedIngressCount(t, dbPath)
		pendingDepth = append(pendingDepth, pending)
		if pending != mixedIngressReceiptCount-cycle-1 {
			t.Fatalf("cycle %d pending depth=%d want=%d trace=%v", cycle, pending, mixedIngressReceiptCount-cycle-1, pendingDepth)
		}
	}

	convergence := time.Since(started)
	workerCycleTotal := workers * mixedIngressReceiptCount
	exhaustionRate := float64(totalExhaustions) / float64(workerCycleTotal)
	if exhaustionRate < 0 || exhaustionRate >= 1 {
		t.Fatalf("exhaustion rate %.3f outside bounded range [0,1)", exhaustionRate)
	}
	if totalAttempts > workerCycleTotal*policy.MaxAttempts {
		t.Fatalf("attempts=%d exceeded campaign ceiling=%d", totalAttempts, workerCycleTotal*policy.MaxAttempts)
	}
	if len(attemptTimings) != totalAttempts {
		t.Fatalf("attempt timing count=%d want attempts=%d", len(attemptTimings), totalAttempts)
	}
	if convergence <= 0 || convergence > 20*time.Second {
		t.Fatalf("time to convergence=%s outside bounded window (0,20s]", convergence)
	}
	minProcessed, maxProcessed := mixedIngressReceiptCount, 0
	for worker := 0; worker < workers; worker++ {
		if workerCycles[worker] != mixedIngressReceiptCount || processedByWorker[worker] == 0 {
			t.Fatalf("worker %d starved: cycles=%d processed=%d", worker, workerCycles[worker], processedByWorker[worker])
		}
		if processedByWorker[worker] < minProcessed {
			minProcessed = processedByWorker[worker]
		}
		if processedByWorker[worker] > maxProcessed {
			maxProcessed = processedByWorker[worker]
		}
	}
	if maxProcessed-minProcessed > 1 {
		t.Fatalf("per-worker progress unfair: processed=%v", processedByWorker)
	}
	if pendingDepth[len(pendingDepth)-1] != 0 {
		t.Fatalf("campaign did not converge: pending trace=%v", pendingDepth)
	}
	report := sustainedIngressCampaignReport{
		SchemaVersion: sustainedIngressCampaignReportSchema, Workers: workers, Cycles: mixedIngressReceiptCount,
		RetryMaxAttempts: policy.MaxAttempts, RecoveryDelayMillis: recoveryDelay.Milliseconds(),
		WorkerCycles: workerCycleTotal, Attempts: totalAttempts, Exhaustions: totalExhaustions,
		ExhaustionRate: exhaustionRate, PendingDepth: pendingDepth, ConvergenceMillis: convergence.Milliseconds(),
		ProcessedByWorker: processedByWorker, ExhaustionsByWorker: exhaustionsByWorker,
		AttemptTimings: attemptTimings, HostLoadStart: hostLoadStart, HostLoadEnd: readSustainedIngressHostLoad(),
		MatrixRunID: matrixRunID, MatrixOrdinal: matrixOrdinal, MatrixBlock: matrixBlock,
		OrderPosition: orderPosition, OrderStrategy: orderStrategy, MatrixMetadata: matrixMetadata,
	}
	writeSustainedIngressCampaignReport(t, report)
	t.Logf("sustained ingress: exhaustion_rate=%.3f pending_depth=%v convergence=%s attempts=%d processed_by_worker=%v exhaustions_by_worker=%v", exhaustionRate, pendingDepth, convergence, totalAttempts, processedByWorker, exhaustionsByWorker)
}

func TestSubagentStatusIngressSustainedMultiprocessHelper(t *testing.T) {
	workerText := os.Getenv(sustainedIngressMultiprocessMode)
	if workerText == "" {
		t.Skip("multiprocess helper")
	}
	var worker, cycle, leader int
	if _, err := fmt.Sscanf(workerText, "%d", &worker); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Sscanf(os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_CYCLE"), "%d", &cycle); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Sscanf(os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_LEADER"), "%d", &leader); err != nil {
		t.Fatal(err)
	}
	dbPath := os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_DB")
	readyPath := os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_READY")
	startPath := os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_START")
	resultPath := os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_RESULT")
	lockHeldPath := os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_LOCK_HELD")
	if dbPath == "" || readyPath == "" || startPath == "" || resultPath == "" || lockHeldPath == "" {
		t.Fatal("multiprocess helper paths are incomplete")
	}

	options := sqlite.Options{}
	if worker == leader {
		firstCommit := true
		options.Failpoint = func(point sqlite.Failpoint) {
			if point != sqlite.FailpointBeforeDurableCommit || !firstCommit {
				return
			}
			firstCommit = false
			if err := os.WriteFile(lockHeldPath, []byte("held\n"), 0o600); err != nil {
				panic(err)
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
	store, err := sqlite.OpenWithOptions(dbPath, options)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	clock := mixedIngressClock{now: mixedIngressNow.Add(time.Duration(cycle) * time.Second)}
	manager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: mixedIngressReceiptCount})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < mixedIngressReceiptCount; i++ {
		if err := manager.Restore(context.Background(), kernel.SubagentStatus{
			ID: kernel.SessionID(fmt.Sprintf("mixed-session-%d", i)), Attempt: 0, State: kernel.SessionStatePending,
			Spec: kernel.SubagentSpec{Task: fmt.Sprintf("mixed work %d", i), ContextMode: "isolated", Labels: map[string]string{
				kernel.SubagentTransportPeerLabel: fmt.Sprintf("peer-%d", i),
			}}, StartedAt: mixedIngressNow.Add(-time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForMixedIngressFile(t, startPath, 5*time.Second)

	observedStore := &sustainedIngressObservedStore{Store: store}
	runner := kernel.SubagentStatusIngressWorker{
		Store: observedStore, Manager: manager, Clock: clock, Batch: 1, LeaseTTL: mixedIngressLeaseTTL,
		RetryPolicy: kernel.DefaultSubagentStatusIngressRetryPolicy(), RetrySleeper: retry.SystemSleeper{},
		RetryJitter: source.NewSequenceRandomSource(uint64(worker*3), uint64(worker*3)),
	}
	started := time.Now()
	processed, report, runErr := runner.ApplyPendingWithRetryReport(context.Background())
	result := sustainedIngressProcessReport{
		Cycle: cycle, Worker: worker, Leader: worker == leader, Processed: processed,
		Attempts: report.Attempts, Retries: report.Retries, Conflicts: report.Classes["conflict"],
		Exhaustions: report.Exhaustions, SleepMillis: report.SleepTotal.Milliseconds(),
		AttemptTimings: observedStore.snapshot(),
	}
	if errors.Is(runErr, retry.ErrBudgetExhausted) {
		recoveryDelay := sustainedIngressCampaignRecoveryDelay(t)
		time.Sleep(recoveryDelay)
		result.RecoveryMillis = recoveryDelay.Milliseconds()
	} else if runErr != nil {
		result.Error = runErr.Error()
	}
	result.ElapsedMillis = time.Since(started).Milliseconds()
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readSustainedIngressHostLoad() sustainedIngressHostLoad {
	load := sustainedIngressHostLoad{Source: "unavailable", LogicalCPUs: runtime.NumCPU()}
	body, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return load
	}
	if _, err := fmt.Sscanf(string(body), "%f %f %f", &load.Load1, &load.Load5, &load.Load15); err != nil {
		return load
	}
	load.Source = "proc_loadavg"
	load.Available = true
	return load
}

func TestReadSustainedIngressHostLoad(t *testing.T) {
	load := readSustainedIngressHostLoad()
	if load.Available && (load.Load1 < 0 || load.Load5 < 0 || load.Load15 < 0) {
		t.Fatalf("invalid host load: %+v", load)
	}
}

func sustainedIngressMatrixMetadata(t *testing.T) (string, int, int, int, string, bool) {
	t.Helper()
	runID := os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_MATRIX_RUN_ID")
	if runID == "" {
		return "", 0, 0, 0, "", false
	}
	strategy := os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_ORDER_STRATEGY")
	if strategy == "" {
		t.Fatal("matrix order strategy is required when matrix run id is set")
	}
	values := make([]int, 3)
	for i, name := range []string{"MATRIX_ORDINAL", "MATRIX_BLOCK", "ORDER_POSITION"} {
		text := os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_" + name)
		if _, err := fmt.Sscanf(text, "%d", &values[i]); err != nil || values[i] < 0 {
			t.Fatalf("invalid sustained ingress %s %q", strings.ToLower(name), text)
		}
	}
	if values[2] > 2 {
		t.Fatalf("invalid sustained ingress order position %d: require 0..2", values[2])
	}
	return runID, values[0], values[1], values[2], strategy, true
}

func TestSustainedIngressMatrixMetadata(t *testing.T) {
	if _, _, _, _, _, present := sustainedIngressMatrixMetadata(t); present {
		t.Fatal("matrix metadata unexpectedly present")
	}
	t.Setenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_MATRIX_RUN_ID", "phase172-block-00")
	t.Setenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_MATRIX_ORDINAL", "2")
	t.Setenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_MATRIX_BLOCK", "0")
	t.Setenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_ORDER_POSITION", "2")
	t.Setenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_ORDER_STRATEGY", "rotating_latin_v1")
	runID, ordinal, block, position, strategy, present := sustainedIngressMatrixMetadata(t)
	if !present || runID != "phase172-block-00" || ordinal != 2 || block != 0 || position != 2 || strategy != "rotating_latin_v1" {
		t.Fatalf("matrix metadata mismatch: %q %d %d %d %q %t", runID, ordinal, block, position, strategy, present)
	}
}

func sustainedIngressCampaignRecoveryDelay(t *testing.T) time.Duration {
	t.Helper()
	text := os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_RECOVERY_DELAY_MS")
	if text == "" {
		return sustainedIngressRecoveryDelay
	}
	var milliseconds int64
	if _, err := fmt.Sscanf(text, "%d", &milliseconds); err != nil || milliseconds < 1 || milliseconds > 1000 {
		t.Fatalf("invalid sustained ingress recovery delay %q: require integer 1..1000 ms", text)
	}
	return time.Duration(milliseconds) * time.Millisecond
}

func TestSustainedIngressCampaignRecoveryDelay(t *testing.T) {
	if got := sustainedIngressCampaignRecoveryDelay(t); got != sustainedIngressRecoveryDelay {
		t.Fatalf("default recovery delay=%s want=%s", got, sustainedIngressRecoveryDelay)
	}
	t.Setenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_RECOVERY_DELAY_MS", "250")
	if got := sustainedIngressCampaignRecoveryDelay(t); got != 250*time.Millisecond {
		t.Fatalf("experimental recovery delay=%s want=250ms", got)
	}
}

func TestWriteSustainedIngressCampaignReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "campaign.json")
	t.Setenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_REPORT", path)
	want := sustainedIngressCampaignReport{
		SchemaVersion: sustainedIngressCampaignReportSchema, Workers: 4, Cycles: 6,
		RetryMaxAttempts: 3, RecoveryDelayMillis: 100, WorkerCycles: 24,
		Attempts: 52, Exhaustions: 4, ExhaustionRate: 4.0 / 24.0,
		PendingDepth: []int{6, 5, 4, 3, 2, 1, 0}, ProcessedByWorker: []int{2, 2, 1, 1},
		ExhaustionsByWorker: []int{1, 1, 1, 1}, ConvergenceMillis: 5000,
	}
	writeSustainedIngressCampaignReport(t, want)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got sustainedIngressCampaignReport
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != want.SchemaVersion || got.Attempts != want.Attempts || got.ConvergenceMillis != want.ConvergenceMillis || len(got.PendingDepth) != len(want.PendingDepth) {
		t.Fatalf("round-trip report mismatch: got=%+v want=%+v", got, want)
	}
}

func writeSustainedIngressCampaignReport(t *testing.T, report sustainedIngressCampaignReport) {
	t.Helper()
	path := os.Getenv("MOTOR_AUTONOMO_SUSTAINED_INGRESS_REPORT")
	if path == "" {
		return
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".sustained-ingress-*.json")
	if err != nil {
		t.Fatal(err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		t.Fatal(err)
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		t.Fatal(err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		t.Fatal(err)
	}
}

func sustainedIngressHelperCommand(ctx context.Context, cycle, worker, leader int, dbPath, readyPath, startPath, resultPath, lockHeldPath string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run", "^TestSubagentStatusIngressSustainedMultiprocessHelper$")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=%d", sustainedIngressMultiprocessMode, worker),
		fmt.Sprintf("MOTOR_AUTONOMO_SUSTAINED_INGRESS_CYCLE=%d", cycle),
		fmt.Sprintf("MOTOR_AUTONOMO_SUSTAINED_INGRESS_LEADER=%d", leader),
		"MOTOR_AUTONOMO_SUSTAINED_INGRESS_DB="+dbPath,
		"MOTOR_AUTONOMO_SUSTAINED_INGRESS_READY="+readyPath,
		"MOTOR_AUTONOMO_SUSTAINED_INGRESS_START="+startPath,
		"MOTOR_AUTONOMO_SUSTAINED_INGRESS_RESULT="+resultPath,
		"MOTOR_AUTONOMO_SUSTAINED_INGRESS_LOCK_HELD="+lockHeldPath,
	)
	return cmd
}

func readSustainedIngressProcessReport(t *testing.T, path string) sustainedIngressProcessReport {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report sustainedIngressProcessReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	return report
}
