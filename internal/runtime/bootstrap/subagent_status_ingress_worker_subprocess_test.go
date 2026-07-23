package bootstrap_test

import (
	"context"
	"encoding/json"
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

const ingressWorkerSubprocessMode = "MOTOR_AUTONOMO_INGRESS_WORKER_SUBPROCESS"

type multiprocessIngressClock struct{ now time.Time }

func (c multiprocessIngressClock) Now() time.Time { return c.now }

type ingressWorkerSubprocessResult struct {
	Role          string `json:"role"`
	Processed     int    `json:"processed"`
	Attempts      int    `json:"attempts"`
	Retries       int    `json:"retries"`
	Conflicts     int    `json:"conflicts"`
	SleepMillis   int64  `json:"sleep_millis"`
	ElapsedMillis int64  `json:"elapsed_millis"`
	Error         string `json:"error,omitempty"`
}

func TestSubagentStatusIngressWorkerProductionRetryConvergesAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	lockHeldPath := filepath.Join(dir, "leader-lock-held")
	leaderStartPath := filepath.Join(dir, "leader-start")
	followerStartPath := filepath.Join(dir, "follower-start")
	leaderReadyPath := filepath.Join(dir, "leader-ready")
	followerReadyPath := filepath.Join(dir, "follower-ready")
	startedAt := time.Date(2026, 7, 23, 17, 20, 0, 0, time.UTC)

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	record := domain.SubagentRecord{
		SchemaVersion: domain.SchemaVersionV1, ID: "subagent-multiprocess", TaskID: "task-multiprocess",
		MissionID: "mission-multiprocess", State: domain.SubagentStatePending, StartedAt: startedAt,
		UpdatedAt: startedAt, Task: "exercise production ingress retry", ContextMode: "isolated",
		TransportPeerID: "peer-a", MaxAttempts: 2, Deadline: startedAt.Add(time.Minute),
	}
	receipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: domain.SchemaVersionV1, CallerPeerID: "peer-a", DeliveryID: "delivery-multiprocess",
		SessionID: record.ID, Attempt: 0, State: string(kernel.SessionStateComplete), Result: "winner",
		Status: domain.SubagentStatusIngressPending, RecordedAt: startedAt.Add(time.Second),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentStatusIngressReceipt(receipt)
	}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	leaderResult := filepath.Join(dir, "leader.json")
	followerResult := filepath.Join(dir, "follower.json")
	follower := ingressWorkerHelperCommand("follower", dbPath, followerResult, lockHeldPath, followerReadyPath, followerStartPath)
	if err := follower.Start(); err != nil {
		t.Fatal(err)
	}
	defer follower.Process.Kill()
	waitForIngressWorkerFile(t, followerReadyPath, 15*time.Second)
	leader := ingressWorkerHelperCommand("leader", dbPath, leaderResult, lockHeldPath, leaderReadyPath, leaderStartPath)
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	defer leader.Process.Kill()
	waitForIngressWorkerFile(t, leaderReadyPath, 15*time.Second)
	if err := os.WriteFile(leaderStartPath, []byte("start\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForIngressWorkerFile(t, lockHeldPath, 5*time.Second)
	if err := os.WriteFile(followerStartPath, []byte("start\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := follower.Wait(); err != nil {
		t.Fatalf("follower: %v", err)
	}
	if err := leader.Wait(); err != nil {
		t.Fatalf("leader: %v", err)
	}

	leaderReport := readIngressWorkerSubprocessResult(t, leaderResult)
	followerReport := readIngressWorkerSubprocessResult(t, followerResult)
	for _, result := range []ingressWorkerSubprocessResult{leaderReport, followerReport} {
		if result.Error != "" {
			t.Fatalf("%s failed: %+v", result.Role, result)
		}
		if result.Attempts > kernel.DefaultSubagentStatusIngressRetryPolicy().MaxAttempts {
			t.Fatalf("%s attempts outside production budget: %+v", result.Role, result)
		}
	}
	if leaderReport.Processed+followerReport.Processed != 1 {
		t.Fatalf("processed sum=%d leader=%+v follower=%+v", leaderReport.Processed+followerReport.Processed, leaderReport, followerReport)
	}
	if leaderReport.Conflicts+followerReport.Conflicts < 1 || leaderReport.Retries+followerReport.Retries < 1 {
		t.Fatalf("campaign did not exercise retryable checkpoint CAS contention: leader=%+v follower=%+v", leaderReport, followerReport)
	}
	if leaderReport.Attempts+followerReport.Attempts > 2*kernel.DefaultSubagentStatusIngressRetryPolicy().MaxAttempts {
		t.Fatalf("aggregate attempts exceeded production ceiling: leader=%+v follower=%+v", leaderReport, followerReport)
	}
	t.Logf("leader=%+v follower=%+v", leaderReport, followerReport)

	reopened, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.View(context.Background(), func(reader port.Reader) error {
		persisted, err := reader.SubagentStatusIngressReceipt(receipt.CallerPeerID, receipt.DeliveryID)
		if err != nil {
			return err
		}
		if persisted.Status != domain.SubagentStatusIngressApplied || persisted.Result != "winner" {
			return fmt.Errorf("persisted receipt = %+v", persisted)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSubagentStatusIngressWorkerSubprocessHelper(t *testing.T) {
	role := os.Getenv(ingressWorkerSubprocessMode)
	if role == "" {
		t.Skip("subprocess helper")
	}
	dbPath := os.Getenv("MOTOR_AUTONOMO_INGRESS_WORKER_DB")
	resultPath := os.Getenv("MOTOR_AUTONOMO_INGRESS_WORKER_RESULT")
	lockHeldPath := os.Getenv("MOTOR_AUTONOMO_INGRESS_WORKER_LOCK_HELD")
	readyPath := os.Getenv("MOTOR_AUTONOMO_INGRESS_WORKER_READY")
	startPath := os.Getenv("MOTOR_AUTONOMO_INGRESS_WORKER_START")
	if dbPath == "" || resultPath == "" || lockHeldPath == "" || readyPath == "" || startPath == "" {
		t.Fatal("subprocess helper requires db, result, lock-held, ready, and start paths")
	}
	options := sqlite.Options{}
	if role == "leader" {
		options.Failpoint = func(point sqlite.Failpoint) {
			if point != sqlite.FailpointBeforeDurableCommit {
				return
			}
			if err := os.WriteFile(lockHeldPath, []byte("held\n"), 0o600); err != nil {
				panic(err)
			}
			time.Sleep(150 * time.Millisecond)
		}
	}
	store, err := sqlite.OpenWithOptions(dbPath, options)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var record domain.SubagentRecord
	if err := store.View(context.Background(), func(reader port.Reader) error {
		var readErr error
		record, readErr = reader.SubagentRecord("subagent-multiprocess")
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
	clock := multiprocessIngressClock{now: record.StartedAt.Add(2 * time.Second)}
	manager := kernel.NewLocalSessionManager(clock)
	if err := manager.Restore(context.Background(), kernel.SubagentStatus{
		ID: kernel.SessionID(record.ID), Attempt: record.Attempt, State: kernel.SessionStatePending,
		Spec:      kernel.SubagentSpec{Task: record.Task, ContextMode: record.ContextMode, Labels: map[string]string{kernel.SubagentTransportPeerLabel: record.TransportPeerID}},
		StartedAt: record.StartedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForIngressWorkerFile(t, startPath, 5*time.Second)
	worker := kernel.SubagentStatusIngressWorker{
		Store: store, Manager: manager, Clock: clock, Batch: 4,
		RetryPolicy: kernel.DefaultSubagentStatusIngressRetryPolicy(), RetrySleeper: retry.SystemSleeper{}, RetryJitter: source.NewSequenceRandomSource(0, 0),
	}
	started := time.Now()
	processed, report, runErr := worker.ApplyPendingWithRetryReport(context.Background())
	result := ingressWorkerSubprocessResult{
		Role: role, Processed: processed, Attempts: report.Attempts, Retries: report.Retries,
		Conflicts: report.Classes["conflict"], SleepMillis: report.SleepTotal.Milliseconds(), ElapsedMillis: time.Since(started).Milliseconds(),
	}
	if runErr != nil {
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

func ingressWorkerHelperCommand(role, dbPath, resultPath, lockHeldPath, readyPath, startPath string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run", "^TestSubagentStatusIngressWorkerSubprocessHelper$")
	cmd.Env = append(os.Environ(),
		ingressWorkerSubprocessMode+"="+role,
		"MOTOR_AUTONOMO_INGRESS_WORKER_DB="+dbPath,
		"MOTOR_AUTONOMO_INGRESS_WORKER_RESULT="+resultPath,
		"MOTOR_AUTONOMO_INGRESS_WORKER_LOCK_HELD="+lockHeldPath,
		"MOTOR_AUTONOMO_INGRESS_WORKER_READY="+readyPath,
		"MOTOR_AUTONOMO_INGRESS_WORKER_START="+startPath,
	)
	return cmd
}

func readIngressWorkerSubprocessResult(t *testing.T, path string) ingressWorkerSubprocessResult {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result ingressWorkerSubprocessResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func waitForIngressWorkerFile(t *testing.T, path string, timeout time.Duration) {
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
