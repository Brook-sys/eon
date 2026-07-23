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

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/retry"
	"motor-autonomo/internal/storage/sqlite"
)

const runningIngressMultiprocessMode = "MOTOR_AUTONOMO_RUNNING_INGRESS_MULTIPROCESS"

const runningIngressReceiptCount = 6

var runningIngressNow = time.Date(2026, 7, 23, 18, 0, 0, 0, time.UTC)

const runningIngressLeaseTTL = 3 * time.Minute

type runningIngressClock struct{ now time.Time }

func (c runningIngressClock) Now() time.Time { return c.now }

type zeroIngressJitter struct{}

func (zeroIngressJitter) Uint64() (uint64, error) { return 0, nil }

type runningIngressProcessReport struct {
	Worker      int    `json:"worker"`
	Cycles      int    `json:"cycles"`
	Processed   int    `json:"processed"`
	Attempts    int    `json:"attempts"`
	Retries     int    `json:"retries"`
	Conflicts   int    `json:"conflicts"`
	SleepMillis int64  `json:"sleep_millis"`
	ElapsedMS   int64  `json:"elapsed_millis"`
	Error       string `json:"error,omitempty"`
}

func TestSubagentStatusIngressRunningReceiptsConvergeAcrossFourSQLiteProcesses(t *testing.T) {
	policy := kernel.DefaultSubagentStatusIngressRetryPolicy()
	if policy.MaxAttempts != 3 || policy.BaseDelay != 10*time.Millisecond || policy.MaxDelay != 40*time.Millisecond || policy.MaxJitter != 10*time.Millisecond {
		t.Fatalf("production retry policy changed: %+v", policy)
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	seedRunningIngressReceipts(t, dbPath)

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
		commands[worker] = runningIngressHelperCommand(ctx, worker, dbPath, readyPaths[worker], startPaths[worker], resultPaths[worker], lockHeldPath)
		if err := commands[worker].Start(); err != nil {
			t.Fatalf("start worker %d: %v", worker, err)
		}
		defer commands[worker].Process.Kill()
		// Serialize only process bootstrap. sqlite.Open writes connection setup
		// metadata, so racing four opens adds unrelated SQLITE_BUSY noise before
		// the fire-test barrier. Every handle is still open on the same stale
		// checkpoint before any worker is released to process receipts.
		waitForRunningIngressFile(t, readyPaths[worker], 10*time.Second)
	}

	// Give worker zero the first write lock, then release the other three while
	// its durable checkpoint transaction is deliberately held open. They each
	// operate through an independent process, SQLite connection, and store.
	if err := os.WriteFile(startPaths[0], []byte("start\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForRunningIngressFile(t, lockHeldPath, 5*time.Second)
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

	reports := make([]runningIngressProcessReport, 0, len(commands))
	totalProcessed, totalRetries, totalConflicts := 0, 0, 0
	for worker, resultPath := range resultPaths {
		report := readRunningIngressProcessReport(t, resultPath)
		reports = append(reports, report)
		if report.Worker != worker || report.Error != "" {
			t.Fatalf("worker %d report: %+v", worker, report)
		}
		if report.Cycles < 1 || report.Cycles > 12 {
			t.Fatalf("worker %d cycles not bounded: %+v", worker, report)
		}
		if report.Attempts < report.Retries || report.Conflicts < report.Retries {
			t.Fatalf("worker %d malformed aggregate retry telemetry: %+v", worker, report)
		}
		if report.Attempts > report.Cycles*runningIngressReceiptCount*policy.MaxAttempts {
			t.Fatalf("worker %d exceeded production retry ceiling: %+v", worker, report)
		}
		totalProcessed += report.Processed
		totalRetries += report.Retries
		totalConflicts += report.Conflicts
	}
	if totalProcessed != runningIngressReceiptCount {
		t.Fatalf("APPLIED transition count=%d want=%d reports=%+v", totalProcessed, runningIngressReceiptCount, reports)
	}
	if totalRetries < 1 || totalConflicts < 1 {
		t.Fatalf("fire test did not exercise cross-process checkpoint contention: %+v", reports)
	}
	t.Logf("four-process retry telemetry: %+v", reports)

	reopened, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.View(context.Background(), func(reader port.Reader) error {
		pending, err := reader.PendingSubagentStatusIngressReceipts(runningIngressReceiptCount + 1)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			return fmt.Errorf("pending receipts after bounded convergence: %+v", pending)
		}
		wantLease := runningIngressNow.Add(runningIngressLeaseTTL)
		for i := 0; i < runningIngressReceiptCount; i++ {
			caller := fmt.Sprintf("peer-%d", i)
			delivery := fmt.Sprintf("running-%d", i)
			receipt, err := reader.SubagentStatusIngressReceipt(caller, delivery)
			if err != nil {
				return err
			}
			if receipt.Status != domain.SubagentStatusIngressApplied || !receipt.AppliedAt.Equal(runningIngressNow) || receipt.RejectionCode != "" {
				return fmt.Errorf("receipt %d has terminal-election or duplicate-apply evidence: %+v", i, receipt)
			}
			record, err := reader.SubagentRecord(fmt.Sprintf("running-session-%d", i))
			if err != nil {
				return err
			}
			if record.State != domain.SubagentStatePending || record.Result != "" || record.ErrorCode != "" {
				return fmt.Errorf("RUNNING ingress elected a durable terminal for record %d: %+v", i, record)
			}
			if !record.LeaseExpiresAt.Equal(wantLease) || !record.UpdatedAt.Equal(runningIngressNow) {
				return fmt.Errorf("record %d lease=%v updated=%v want lease=%v updated=%v", i, record.LeaseExpiresAt, record.UpdatedAt, wantLease, runningIngressNow)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSubagentStatusIngressRunningMultiprocessHelper(t *testing.T) {
	workerText := os.Getenv(runningIngressMultiprocessMode)
	if workerText == "" {
		t.Skip("multiprocess helper")
	}
	var worker int
	if _, err := fmt.Sscanf(workerText, "%d", &worker); err != nil {
		t.Fatal(err)
	}
	dbPath := os.Getenv("MOTOR_AUTONOMO_RUNNING_INGRESS_DB")
	readyPath := os.Getenv("MOTOR_AUTONOMO_RUNNING_INGRESS_READY")
	startPath := os.Getenv("MOTOR_AUTONOMO_RUNNING_INGRESS_START")
	resultPath := os.Getenv("MOTOR_AUTONOMO_RUNNING_INGRESS_RESULT")
	lockHeldPath := os.Getenv("MOTOR_AUTONOMO_RUNNING_INGRESS_LOCK_HELD")
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
	manager, err := kernel.NewLocalSessionManagerWithPolicy(runningIngressClock{now: runningIngressNow}, kernel.SessionPolicy{MaxConcurrent: runningIngressReceiptCount})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < runningIngressReceiptCount; i++ {
		if err := manager.Restore(context.Background(), kernel.SubagentStatus{
			ID:        kernel.SessionID(fmt.Sprintf("running-session-%d", i)),
			Attempt:   0,
			State:     kernel.SessionStatePending,
			Spec:      kernel.SubagentSpec{Task: fmt.Sprintf("running work %d", i), ContextMode: "isolated", Labels: map[string]string{kernel.SubagentTransportPeerLabel: fmt.Sprintf("peer-%d", i)}},
			StartedAt: runningIngressNow.Add(-time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForRunningIngressFile(t, startPath, 5*time.Second)

	clock := runningIngressClock{now: runningIngressNow}
	result := runningIngressProcessReport{Worker: worker}
	started := time.Now()
	for result.Cycles < 12 {
		result.Cycles++
		workerRunner := kernel.SubagentStatusIngressWorker{
			Store: store, Manager: manager, Clock: clock, Batch: runningIngressReceiptCount,
			LeaseTTL: runningIngressLeaseTTL, RetryPolicy: kernel.DefaultSubagentStatusIngressRetryPolicy(),
			RetrySleeper: retry.SystemSleeper{}, RetryJitter: zeroIngressJitter{},
		}
		processed, report, runErr := workerRunner.ApplyPendingWithRetryReport(context.Background())
		result.Processed += processed
		result.Attempts += report.Attempts
		result.Retries += report.Retries
		result.Conflicts += report.Classes["conflict"]
		result.SleepMillis += report.SleepTotal.Milliseconds()
		if runErr != nil && !errors.Is(runErr, retry.ErrBudgetExhausted) {
			result.Error = runErr.Error()
			break
		}
		var pending []domain.SubagentStatusIngressReceipt
		if err := store.View(context.Background(), func(reader port.Reader) error {
			var viewErr error
			pending, viewErr = reader.PendingSubagentStatusIngressReceipts(1)
			return viewErr
		}); err != nil {
			result.Error = err.Error()
			break
		}
		if len(pending) == 0 {
			break
		}
	}
	result.ElapsedMS = time.Since(started).Milliseconds()
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func seedRunningIngressReceipts(t *testing.T, dbPath string) {
	t.Helper()
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	startedAt := runningIngressNow.Add(-time.Minute)
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		for i := 0; i < runningIngressReceiptCount; i++ {
			record := domain.SubagentRecord{
				SchemaVersion: domain.SchemaVersionV1, ID: fmt.Sprintf("running-session-%d", i), TaskID: fmt.Sprintf("task-%d", i),
				MissionID: "mission-running-ingress", State: domain.SubagentStatePending, StartedAt: startedAt, UpdatedAt: startedAt,
				Task: fmt.Sprintf("running work %d", i), ContextMode: "isolated", TransportPeerID: fmt.Sprintf("peer-%d", i),
				MaxAttempts: 2, Deadline: startedAt.Add(10 * time.Minute), LeaseExpiresAt: startedAt.Add(time.Minute),
			}
			if err := tx.CreateSubagentRecord(record); err != nil {
				return err
			}
			receipt := domain.SubagentStatusIngressReceipt{
				SchemaVersion: domain.SchemaVersionV1, CallerPeerID: fmt.Sprintf("peer-%d", i), DeliveryID: fmt.Sprintf("running-%d", i),
				SessionID: record.ID, Attempt: 0, State: string(kernel.SessionStateRunning), Status: domain.SubagentStatusIngressPending,
				RecordedAt: runningIngressNow.Add(-time.Second).Add(time.Duration(i) * time.Millisecond),
			}
			if err := tx.CreateSubagentStatusIngressReceipt(receipt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func runningIngressHelperCommand(ctx context.Context, worker int, dbPath, readyPath, startPath, resultPath, lockHeldPath string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run", "^TestSubagentStatusIngressRunningMultiprocessHelper$")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=%d", runningIngressMultiprocessMode, worker),
		"MOTOR_AUTONOMO_RUNNING_INGRESS_DB="+dbPath,
		"MOTOR_AUTONOMO_RUNNING_INGRESS_READY="+readyPath,
		"MOTOR_AUTONOMO_RUNNING_INGRESS_START="+startPath,
		"MOTOR_AUTONOMO_RUNNING_INGRESS_RESULT="+resultPath,
		"MOTOR_AUTONOMO_RUNNING_INGRESS_LOCK_HELD="+lockHeldPath,
	)
	return cmd
}

func readRunningIngressProcessReport(t *testing.T, path string) runningIngressProcessReport {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report runningIngressProcessReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func waitForRunningIngressFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
