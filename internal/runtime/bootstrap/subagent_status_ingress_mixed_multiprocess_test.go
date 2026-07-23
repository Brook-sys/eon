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
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/sqlite"
)

const mixedIngressMultiprocessMode = "MOTOR_AUTONOMO_MIXED_INGRESS_MULTIPROCESS"

const (
	mixedIngressReceiptCount = 6
	mixedIngressRunningCount = 3
)

var mixedIngressNow = time.Date(2026, 7, 23, 19, 0, 0, 0, time.UTC)

const mixedIngressLeaseTTL = 3 * time.Minute

type mixedIngressClock struct{ now time.Time }

func (c mixedIngressClock) Now() time.Time { return c.now }

type mixedIngressLeaderBarrierSleeper struct {
	leaderDone string
	waited     bool
}

func (s *mixedIngressLeaderBarrierSleeper) Sleep(ctx context.Context, delay time.Duration) error {
	if !s.waited {
		s.waited = true
		for {
			if _, err := os.Stat(s.leaderDone); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond):
			}
		}
	}
	return (retry.SystemSleeper{}).Sleep(ctx, delay)
}

type mixedIngressProcessReport struct {
	Worker                int    `json:"worker"`
	Cycles                int    `json:"cycles"`
	Processed             int    `json:"processed"`
	Attempts              int    `json:"attempts"`
	Retries               int    `json:"retries"`
	Conflicts             int    `json:"conflicts"`
	SleepMillis           int64  `json:"sleep_millis"`
	Reconciled            int    `json:"reconciled"`
	CapacityBlockedBefore bool   `json:"capacity_blocked_before"`
	CapacityOpened        int    `json:"capacity_opened"`
	CapacityBlockedAfter  bool   `json:"capacity_blocked_after"`
	Error                 string `json:"error,omitempty"`
}

func TestSubagentStatusIngressMixedReceiptsPreserveCapacityAcrossFourSQLiteProcesses(t *testing.T) {
	policy := kernel.DefaultSubagentStatusIngressRetryPolicy()
	if policy.MaxAttempts != 3 || policy.BaseDelay != 10*time.Millisecond || policy.MaxDelay != 40*time.Millisecond || policy.MaxJitter != 10*time.Millisecond {
		t.Fatalf("production retry policy changed: %+v", policy)
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	seedMixedIngressReceipts(t, dbPath)

	commands := make([]*exec.Cmd, 4)
	readyPaths := make([]string, 4)
	startPaths := make([]string, 4)
	donePaths := make([]string, 4)
	resultPaths := make([]string, 4)
	lockHeldPath := filepath.Join(dir, "leader-lock-held")
	supervisePath := filepath.Join(dir, "supervise")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for worker := range commands {
		readyPaths[worker] = filepath.Join(dir, fmt.Sprintf("worker-%d-ready", worker))
		startPaths[worker] = filepath.Join(dir, fmt.Sprintf("worker-%d-start", worker))
		donePaths[worker] = filepath.Join(dir, fmt.Sprintf("worker-%d-ingress-done", worker))
		resultPaths[worker] = filepath.Join(dir, fmt.Sprintf("worker-%d.json", worker))
		predecessorDonePath := donePaths[worker]
		if worker > 0 {
			predecessorDonePath = donePaths[worker-1]
		}
		commands[worker] = mixedIngressHelperCommand(ctx, worker, dbPath, readyPaths[worker], startPaths[worker], donePaths[worker], predecessorDonePath, resultPaths[worker], lockHeldPath, supervisePath)
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
	for _, donePath := range donePaths {
		waitForMixedIngressFile(t, donePath, 10*time.Second)
	}
	// Only worker zero retains its process-local terminal observations for the
	// post-ingress Supervisor boundary. Other writers have stopped touching the
	// checkpoint, so this phase isolates capacity acknowledgement from CAS noise.
	if err := os.WriteFile(supervisePath, []byte("supervise\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for worker, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("worker %d: %v", worker, err)
		}
	}

	reports := make([]mixedIngressProcessReport, 0, len(commands))
	totalProcessed, totalRetries, totalConflicts := 0, 0, 0
	for worker, resultPath := range resultPaths {
		report := readMixedIngressProcessReport(t, resultPath)
		reports = append(reports, report)
		if report.Worker != worker || report.Error != "" {
			t.Fatalf("worker %d report: %+v", worker, report)
		}
		if report.Cycles != 1 {
			t.Fatalf("worker %d did not use exactly one ingress cycle: %+v", worker, report)
		}
		if report.Attempts < report.Retries || report.Conflicts < report.Retries {
			t.Fatalf("worker %d malformed retry telemetry: %+v", worker, report)
		}
		if report.Attempts > report.Cycles*mixedIngressReceiptCount*policy.MaxAttempts {
			t.Fatalf("worker %d exceeded production retry ceiling: %+v", worker, report)
		}
		totalProcessed += report.Processed
		totalRetries += report.Retries
		totalConflicts += report.Conflicts
	}
	if totalProcessed != mixedIngressReceiptCount {
		t.Fatalf("APPLIED transition count=%d want=%d reports=%+v", totalProcessed, mixedIngressReceiptCount, reports)
	}
	if totalRetries < 1 || totalConflicts < 1 {
		t.Fatalf("fire test did not exercise checkpoint contention: %+v", reports)
	}
	leader := reports[0]
	terminalCount := mixedIngressReceiptCount - mixedIngressRunningCount
	if leader.Reconciled != mixedIngressReceiptCount || !leader.CapacityBlockedBefore || leader.CapacityOpened != terminalCount || !leader.CapacityBlockedAfter {
		t.Fatalf("terminal capacity retention/release mismatch: %+v", leader)
	}
	t.Logf("four-process mixed ingress telemetry: %+v", reports)

	reopened, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.View(context.Background(), func(reader port.Reader) error {
		pending, err := reader.PendingSubagentStatusIngressReceipts(mixedIngressReceiptCount + 1)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			return fmt.Errorf("pending receipts after bounded convergence: %+v", pending)
		}
		wantLease := mixedIngressNow.Add(mixedIngressLeaseTTL)
		for i := 0; i < mixedIngressReceiptCount; i++ {
			receipt, err := reader.SubagentStatusIngressReceipt(fmt.Sprintf("peer-%d", i), fmt.Sprintf("mixed-%d", i))
			if err != nil {
				return err
			}
			if receipt.Status != domain.SubagentStatusIngressApplied || !receipt.AppliedAt.Equal(mixedIngressNow) || receipt.RejectionCode != "" {
				return fmt.Errorf("receipt %d not uniquely applied: %+v", i, receipt)
			}
			record, err := reader.SubagentRecord(fmt.Sprintf("mixed-session-%d", i))
			if err != nil {
				return err
			}
			switch {
			case i < mixedIngressRunningCount:
				if record.State != domain.SubagentStateRunning || record.Result != "" || record.ErrorCode != "" || !record.LeaseExpiresAt.Equal(wantLease) {
					return fmt.Errorf("RUNNING record %d mismatch: %+v", i, record)
				}
			case i < mixedIngressReceiptCount-1:
				if record.State != domain.SubagentStateComplete || record.Result != fmt.Sprintf("result-%d", i) || record.ErrorCode != "" {
					return fmt.Errorf("COMPLETE record %d mismatch: %+v", i, record)
				}
			default:
				if record.State != domain.SubagentStateError || record.Result != "" || record.ErrorCode != "remote_failed" {
					return fmt.Errorf("FAILED record %d mismatch: %+v", i, record)
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSubagentStatusIngressMixedMultiprocessHelper(t *testing.T) {
	workerText := os.Getenv(mixedIngressMultiprocessMode)
	if workerText == "" {
		t.Skip("multiprocess helper")
	}
	var worker int
	if _, err := fmt.Sscanf(workerText, "%d", &worker); err != nil {
		t.Fatal(err)
	}
	dbPath := os.Getenv("MOTOR_AUTONOMO_MIXED_INGRESS_DB")
	readyPath := os.Getenv("MOTOR_AUTONOMO_MIXED_INGRESS_READY")
	startPath := os.Getenv("MOTOR_AUTONOMO_MIXED_INGRESS_START")
	donePath := os.Getenv("MOTOR_AUTONOMO_MIXED_INGRESS_DONE")
	predecessorDonePath := os.Getenv("MOTOR_AUTONOMO_MIXED_INGRESS_PREDECESSOR_DONE")
	resultPath := os.Getenv("MOTOR_AUTONOMO_MIXED_INGRESS_RESULT")
	lockHeldPath := os.Getenv("MOTOR_AUTONOMO_MIXED_INGRESS_LOCK_HELD")
	supervisePath := os.Getenv("MOTOR_AUTONOMO_MIXED_INGRESS_SUPERVISE")
	if dbPath == "" || readyPath == "" || startPath == "" || donePath == "" || predecessorDonePath == "" || resultPath == "" || lockHeldPath == "" || supervisePath == "" {
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
			// Hold the first durable commit long enough to force stale checkpoints,
			// but within the production caller's 10+20 ms retry sleep budget. A
			// 200 ms hold is intentionally outside that contract and was observed to
			// exhaust one-cycle workers rather than characterize normal contention.
			time.Sleep(20 * time.Millisecond)
		}
	}
	store, err := sqlite.OpenWithOptions(dbPath, options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	clock := mixedIngressClock{now: mixedIngressNow}
	manager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: mixedIngressReceiptCount})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := mixedIngressNow.Add(-time.Minute)
	for i := 0; i < mixedIngressReceiptCount; i++ {
		if err := manager.Restore(context.Background(), kernel.SubagentStatus{
			ID: kernel.SessionID(fmt.Sprintf("mixed-session-%d", i)), Attempt: 0, State: kernel.SessionStatePending,
			Spec:      kernel.SubagentSpec{Task: fmt.Sprintf("mixed work %d", i), ContextMode: "isolated", Labels: map[string]string{kernel.SubagentTransportPeerLabel: fmt.Sprintf("peer-%d", i)}},
			StartedAt: startedAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForMixedIngressFile(t, startPath, 5*time.Second)

	result := mixedIngressProcessReport{Worker: worker, Cycles: 1}
	// Followers first collide with an earlier writer, then wait for their direct
	// predecessor to finish before spending the next attempt. This deterministic
	// release chain gives each stale handle one quiet checkpoint reload. Releasing
	// all three followers together requires four serial CAS winners, which cannot
	// fit a three-attempt caller budget and is characterized as overload instead.
	jitterOffset := time.Duration(worker*3) * time.Millisecond
	jitter := source.NewSequenceRandomSource(uint64(jitterOffset), uint64(jitterOffset))
	var sleeper retry.Sleeper = retry.SystemSleeper{}
	if worker != 0 {
		sleeper = &mixedIngressLeaderBarrierSleeper{leaderDone: predecessorDonePath}
	}
	runner := kernel.SubagentStatusIngressWorker{
		Store: store, Manager: manager, Clock: clock, Batch: mixedIngressReceiptCount,
		LeaseTTL: mixedIngressLeaseTTL, RetryPolicy: kernel.DefaultSubagentStatusIngressRetryPolicy(),
		RetrySleeper: sleeper, RetryJitter: jitter,
	}
	processed, report, runErr := runner.ApplyPendingWithRetryReport(context.Background())
	result.Processed = processed
	result.Attempts = report.Attempts
	result.Retries = report.Retries
	result.Conflicts = report.Classes["conflict"]
	result.SleepMillis = report.SleepTotal.Milliseconds()
	if runErr != nil {
		result.Error = runErr.Error()
	}
	if err := os.WriteFile(donePath, []byte("done\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if worker == 0 && result.Error == "" {
		waitForMixedIngressFile(t, supervisePath, 10*time.Second)
		// Refresh the process checkpoint after the competing writers have exited.
		// The ingress retry already measured stale-CAS convergence; supervision is
		// a subsequent control-cycle boundary, not another contention experiment.
		if err := store.Close(); err != nil {
			result.Error = err.Error()
		}
		if result.Error == "" {
			store, err = sqlite.Open(dbPath)
			if err != nil {
				result.Error = err.Error()
			}
		}
		_, err := manager.Spawn(context.Background(), kernel.SubagentSpec{Task: "must remain blocked before durable terminal ack", ContextMode: "isolated"})
		result.CapacityBlockedBefore = errors.Is(err, kernel.ErrSessionLimit)
		supervisor := kernel.Supervisor{Store: store, Manager: manager, Clock: clock, LeaseTTL: mixedIngressLeaseTTL}
		result.Reconciled, err = supervisor.Reconcile(context.Background())
		if err != nil {
			result.Error = err.Error()
		} else {
			for i := 0; i < mixedIngressReceiptCount-mixedIngressRunningCount; i++ {
				if _, spawnErr := manager.Spawn(context.Background(), kernel.SubagentSpec{Task: fmt.Sprintf("replacement-%d", i), ContextMode: "isolated"}); spawnErr != nil {
					result.Error = spawnErr.Error()
					break
				}
				result.CapacityOpened++
			}
			if result.Error == "" {
				_, err = manager.Spawn(context.Background(), kernel.SubagentSpec{Task: "must remain bounded after replacements", ContextMode: "isolated"})
				result.CapacityBlockedAfter = errors.Is(err, kernel.ErrSessionLimit)
			}
		}
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func seedMixedIngressReceipts(t *testing.T, dbPath string) {
	t.Helper()
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	startedAt := mixedIngressNow.Add(-time.Minute)
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		for i := 0; i < mixedIngressReceiptCount; i++ {
			record := domain.SubagentRecord{
				SchemaVersion: domain.SchemaVersionV1, ID: fmt.Sprintf("mixed-session-%d", i), TaskID: fmt.Sprintf("mixed-task-%d", i),
				MissionID: "mission-mixed-ingress", State: domain.SubagentStatePending, StartedAt: startedAt, UpdatedAt: startedAt,
				Task: fmt.Sprintf("mixed work %d", i), ContextMode: "isolated", TransportPeerID: fmt.Sprintf("peer-%d", i),
				MaxAttempts: 1, Deadline: startedAt.Add(10 * time.Minute), LeaseExpiresAt: startedAt.Add(time.Minute),
			}
			if err := tx.CreateSubagentRecord(record); err != nil {
				return err
			}
			state, result, failure := kernel.SessionStateRunning, "", ""
			if i >= mixedIngressRunningCount && i < mixedIngressReceiptCount-1 {
				state, result = kernel.SessionStateComplete, fmt.Sprintf("result-%d", i)
			}
			if i == mixedIngressReceiptCount-1 {
				state, failure = kernel.SessionStateFailed, "remote_failed"
			}
			receipt := domain.SubagentStatusIngressReceipt{
				SchemaVersion: domain.SchemaVersionV1, CallerPeerID: fmt.Sprintf("peer-%d", i), DeliveryID: fmt.Sprintf("mixed-%d", i),
				SessionID: record.ID, Attempt: 0, State: string(state), Result: result, Failure: failure,
				Status: domain.SubagentStatusIngressPending, RecordedAt: mixedIngressNow.Add(-time.Second).Add(time.Duration(i) * time.Millisecond),
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

func mixedIngressHelperCommand(ctx context.Context, worker int, dbPath, readyPath, startPath, donePath, predecessorDonePath, resultPath, lockHeldPath, supervisePath string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run", "^TestSubagentStatusIngressMixedMultiprocessHelper$")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=%d", mixedIngressMultiprocessMode, worker),
		"MOTOR_AUTONOMO_MIXED_INGRESS_DB="+dbPath,
		"MOTOR_AUTONOMO_MIXED_INGRESS_READY="+readyPath,
		"MOTOR_AUTONOMO_MIXED_INGRESS_START="+startPath,
		"MOTOR_AUTONOMO_MIXED_INGRESS_DONE="+donePath,
		"MOTOR_AUTONOMO_MIXED_INGRESS_PREDECESSOR_DONE="+predecessorDonePath,
		"MOTOR_AUTONOMO_MIXED_INGRESS_RESULT="+resultPath,
		"MOTOR_AUTONOMO_MIXED_INGRESS_LOCK_HELD="+lockHeldPath,
		"MOTOR_AUTONOMO_MIXED_INGRESS_SUPERVISE="+supervisePath,
	)
	return cmd
}

func readMixedIngressProcessReport(t *testing.T, path string) mixedIngressProcessReport {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report mixedIngressProcessReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func waitForMixedIngressFile(t *testing.T, path string, timeout time.Duration) {
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
