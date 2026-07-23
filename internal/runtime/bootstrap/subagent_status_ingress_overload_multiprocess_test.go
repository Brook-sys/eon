package bootstrap_test

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

	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/retry"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/sqlite"
)

const overloadIngressMultiprocessMode = "MOTOR_AUTONOMO_OVERLOAD_INGRESS_MULTIPROCESS"

type overloadIngressProcessReport struct {
	Worker      int    `json:"worker"`
	Processed   int    `json:"processed"`
	Attempts    int    `json:"attempts"`
	Retries     int    `json:"retries"`
	Conflicts   int    `json:"conflicts"`
	SleepMillis int64  `json:"sleep_millis"`
	Exhausted   bool   `json:"exhausted"`
	Error       string `json:"error,omitempty"`
}

func TestSubagentStatusIngressOverloadLeavesPendingAndResumesBoundedly(t *testing.T) {
	policy := kernel.DefaultSubagentStatusIngressRetryPolicy()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	seedMixedIngressReceipts(t, dbPath)

	commands := make([]*exec.Cmd, 4)
	readyPaths := make([]string, 4)
	startPaths := make([]string, 4)
	resultPaths := make([]string, 4)
	lockHeldPath := filepath.Join(dir, "leader-lock-held")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for worker := range commands {
		readyPaths[worker] = filepath.Join(dir, fmt.Sprintf("worker-%d-ready", worker))
		startPaths[worker] = filepath.Join(dir, fmt.Sprintf("worker-%d-start", worker))
		resultPaths[worker] = filepath.Join(dir, fmt.Sprintf("worker-%d.json", worker))
		commands[worker] = overloadIngressHelperCommand(ctx, worker, dbPath, readyPaths[worker], startPaths[worker], resultPaths[worker], lockHeldPath)
		if err := commands[worker].Start(); err != nil {
			t.Fatalf("start worker %d: %v", worker, err)
		}
		defer commands[worker].Process.Kill()
		waitForMixedIngressFile(t, readyPaths[worker], 10*time.Second)
	}

	if err := os.WriteFile(startPaths[0], []byte("start\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForMixedIngressFile(t, lockHeldPath, 5*time.Second)
	for worker := 1; worker < len(commands); worker++ {
		if err := os.WriteFile(startPaths[worker], []byte("start\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for worker, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("worker %d: %v", worker, err)
		}
	}

	reports := make([]overloadIngressProcessReport, 0, len(commands))
	totalProcessed, totalAttempts, attemptCeiling, exhausted, conflicts := 0, 0, 0, 0, 0
	for worker, path := range resultPaths {
		report := readOverloadIngressProcessReport(t, path)
		reports = append(reports, report)
		if report.Worker != worker || report.Error != "" {
			t.Fatalf("worker %d report: %+v", worker, report)
		}
		batch := worker + 1
		if report.Attempts > batch*policy.MaxAttempts || report.Retries > batch*(policy.MaxAttempts-1) {
			t.Fatalf("worker %d exceeded per-receipt production budget: %+v", worker, report)
		}
		totalProcessed += report.Processed
		totalAttempts += report.Attempts
		attemptCeiling += batch * policy.MaxAttempts
		conflicts += report.Conflicts
		if report.Exhausted {
			exhausted++
		}
	}
	if totalProcessed < 1 || totalProcessed >= mixedIngressReceiptCount {
		t.Fatalf("overload processed=%d want partial progress in [1,%d) reports=%+v", totalProcessed, mixedIngressReceiptCount, reports)
	}
	if totalAttempts > attemptCeiling {
		t.Fatalf("overload attempts=%d exceeded aggregate ceiling=%d: %+v", totalAttempts, attemptCeiling, reports)
	}
	if conflicts < len(commands)-1 {
		t.Fatalf("overload did not force every follower through real CAS contention: %+v", reports)
	}

	pendingBefore := pendingMixedIngressCount(t, dbPath)
	if pendingBefore != mixedIngressReceiptCount-totalProcessed || pendingBefore < 1 {
		t.Fatalf("pending after overload=%d processed=%d reports=%+v", pendingBefore, totalProcessed, reports)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	clock := mixedIngressClock{now: mixedIngressNow}
	manager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: mixedIngressReceiptCount})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < mixedIngressReceiptCount; i++ {
		if err := manager.Restore(context.Background(), kernel.SubagentStatus{
			ID: kernel.SessionID(fmt.Sprintf("mixed-session-%d", i)), Attempt: 0, State: kernel.SessionStatePending,
			Spec:      kernel.SubagentSpec{Task: fmt.Sprintf("mixed work %d", i), ContextMode: "isolated", Labels: map[string]string{kernel.SubagentTransportPeerLabel: fmt.Sprintf("peer-%d", i)}},
			StartedAt: mixedIngressNow.Add(-time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	recovery := kernel.SubagentStatusIngressWorker{
		Store: store, Manager: manager, Clock: clock, Batch: mixedIngressReceiptCount,
		LeaseTTL: mixedIngressLeaseTTL, RetryPolicy: policy, RetrySleeper: retry.SystemSleeper{}, RetryJitter: source.NewSequenceRandomSource(0, 0),
	}
	processed, recoveryReport, err := recovery.ApplyPendingWithRetryReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if processed != pendingBefore || recoveryReport.Attempts != pendingBefore || recoveryReport.Retries != 0 {
		t.Fatalf("bounded recovery mismatch processed=%d report=%+v", processed, recoveryReport)
	}
	if pendingAfter := pendingMixedIngressCount(t, dbPath); pendingAfter != 0 {
		t.Fatalf("pending after recovery=%d want=0", pendingAfter)
	}
	t.Logf("overload reports=%+v retry_budget_exhaustions=%d pending_before=%d recovery=%+v", reports, exhausted, pendingBefore, recoveryReport)
}

func TestSubagentStatusIngressOverloadMultiprocessHelper(t *testing.T) {
	workerText := os.Getenv(overloadIngressMultiprocessMode)
	if workerText == "" {
		t.Skip("multiprocess helper")
	}
	var worker int
	if _, err := fmt.Sscanf(workerText, "%d", &worker); err != nil {
		t.Fatal(err)
	}
	dbPath := os.Getenv("MOTOR_AUTONOMO_OVERLOAD_INGRESS_DB")
	readyPath := os.Getenv("MOTOR_AUTONOMO_OVERLOAD_INGRESS_READY")
	startPath := os.Getenv("MOTOR_AUTONOMO_OVERLOAD_INGRESS_START")
	resultPath := os.Getenv("MOTOR_AUTONOMO_OVERLOAD_INGRESS_RESULT")
	lockHeldPath := os.Getenv("MOTOR_AUTONOMO_OVERLOAD_INGRESS_LOCK_HELD")
	if dbPath == "" || readyPath == "" || startPath == "" || resultPath == "" || lockHeldPath == "" {
		t.Fatal("multiprocess helper paths are incomplete")
	}

	options := sqlite.Options{}
	if worker == 0 {
		firstCommit := true
		options.Failpoint = func(point sqlite.Failpoint) {
			if point != sqlite.FailpointBeforeDurableCommit || !firstCommit {
				return
			}
			firstCommit = false
			if err := os.WriteFile(lockHeldPath, []byte("held\n"), 0o600); err != nil {
				panic(err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	store, err := sqlite.OpenWithOptions(dbPath, options)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	clock := mixedIngressClock{now: mixedIngressNow}
	manager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: mixedIngressReceiptCount})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < mixedIngressReceiptCount; i++ {
		if err := manager.Restore(context.Background(), kernel.SubagentStatus{
			ID: kernel.SessionID(fmt.Sprintf("mixed-session-%d", i)), Attempt: 0, State: kernel.SessionStatePending,
			Spec:      kernel.SubagentSpec{Task: fmt.Sprintf("mixed work %d", i), ContextMode: "isolated", Labels: map[string]string{kernel.SubagentTransportPeerLabel: fmt.Sprintf("peer-%d", i)}},
			StartedAt: mixedIngressNow.Add(-time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForMixedIngressFile(t, startPath, 5*time.Second)

	jitterOffset := time.Duration(worker*3) * time.Millisecond
	runner := kernel.SubagentStatusIngressWorker{
		// Different bounded batch windows deliberately fan the four stale handles
		// across the ordered queue after they collide on its head. This models
		// heterogeneous worker configurations without adding a claim/offset API.
		Store: store, Manager: manager, Clock: clock, Batch: worker + 1, LeaseTTL: mixedIngressLeaseTTL,
		RetryPolicy: kernel.DefaultSubagentStatusIngressRetryPolicy(), RetrySleeper: retry.SystemSleeper{},
		RetryJitter: source.NewSequenceRandomSource(uint64(jitterOffset), uint64(jitterOffset)),
	}
	processed, report, runErr := runner.ApplyPendingWithRetryReport(context.Background())
	result := overloadIngressProcessReport{
		Worker: worker, Processed: processed, Attempts: report.Attempts, Retries: report.Retries,
		Conflicts: report.Classes["conflict"], SleepMillis: report.SleepTotal.Milliseconds(), Exhausted: errors.Is(runErr, retry.ErrBudgetExhausted),
	}
	if runErr != nil && !result.Exhausted {
		result.Error = runErr.Error()
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func overloadIngressHelperCommand(ctx context.Context, worker int, dbPath, readyPath, startPath, resultPath, lockHeldPath string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run", "^TestSubagentStatusIngressOverloadMultiprocessHelper$")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=%d", overloadIngressMultiprocessMode, worker),
		"MOTOR_AUTONOMO_OVERLOAD_INGRESS_DB="+dbPath,
		"MOTOR_AUTONOMO_OVERLOAD_INGRESS_READY="+readyPath,
		"MOTOR_AUTONOMO_OVERLOAD_INGRESS_START="+startPath,
		"MOTOR_AUTONOMO_OVERLOAD_INGRESS_RESULT="+resultPath,
		"MOTOR_AUTONOMO_OVERLOAD_INGRESS_LOCK_HELD="+lockHeldPath,
	)
	return cmd
}

func readOverloadIngressProcessReport(t *testing.T, path string) overloadIngressProcessReport {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report overloadIngressProcessReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func pendingMixedIngressCount(t *testing.T, dbPath string) int {
	t.Helper()
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	count := 0
	if err := store.View(context.Background(), func(reader port.Reader) error {
		pending, err := reader.PendingSubagentStatusIngressReceipts(mixedIngressReceiptCount + 1)
		count = len(pending)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return count
}
