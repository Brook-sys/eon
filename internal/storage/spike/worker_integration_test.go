package spike

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/dolt"
	"motor-autonomo/internal/storage/sqlite"
)

func TestStorageSpikeWorkerCrashesAtSQLiteDurabilityBoundaries(t *testing.T) {
	worker := filepath.Join(t.TempDir(), "storage-spike-worker")
	build := exec.Command("go", "build", "-o", worker, "./cmd/storage-spike-worker")
	build.Dir = projectRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build worker: %v\n%s", err, output)
	}

	tests := []struct {
		name      string
		failpoint sqlite.Failpoint
		want      CrashOutcome
	}{
		{name: "before durable commit", failpoint: sqlite.FailpointBeforeDurableCommit, want: OutcomeNotApplied},
		{name: "after durable commit", failpoint: sqlite.FailpointAfterDurableCommit, want: OutcomeApplied},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			databasePath := filepath.Join(root, "runtime.sqlite")
			intentPath := filepath.Join(root, "intent.json")
			markerPath := filepath.Join(root, "intent.started")
			intent := CrashIntent{Event: domain.Event{
				SchemaVersion: 1,
				ID:            domain.EventID("event_worker_" + test.name),
				Kind:          "spike.worker_crash",
				OccurredAt:    time.Date(2026, 7, 15, 19, index, 0, 0, time.UTC),
			}}
			encoded, err := json.Marshal(intent)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(intentPath, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			open := func() (port.Store, func() error, error) {
				store, err := sqlite.Open(databasePath)
				if err != nil {
					return nil, nil, err
				}
				return store, store.Close, nil
			}
			result, err := RunCrashTrial(context.Background(), CrashCommand{
				Executable: worker,
				Args: []string{
					"-backend", "sqlite",
					"-path", databasePath,
					"-failpoint", string(test.failpoint),
					"-intent", intentPath,
					"-marker", markerPath,
				},
			}, open, intent)
			if err != nil {
				t.Fatal(err)
			}
			if result.ExitError == "" {
				t.Fatal("worker returned normally; wanted abrupt process termination")
			}
			if result.Outcome != test.want {
				t.Fatalf("outcome = %s, want %s (exit %q)", result.Outcome, test.want, result.ExitError)
			}
			marker, err := os.ReadFile(markerPath)
			if err != nil {
				t.Fatalf("read durable intention marker: %v", err)
			}
			if string(marker) != string(encoded) {
				t.Fatalf("marker = %s, want %s", marker, encoded)
			}
		})
	}
}

func TestStorageSpikeWorkerCrashesOfficialMutationAtomicallyInSQLite(t *testing.T) {
	worker := filepath.Join(t.TempDir(), "storage-spike-worker")
	build := exec.Command("go", "build", "-o", worker, "./cmd/storage-spike-worker")
	build.Dir = projectRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build worker: %v\n%s", err, output)
	}
	tests := []struct {
		failpoint sqlite.Failpoint
		want      CrashOutcome
	}{
		{sqlite.FailpointBeforeDurableCommit, OutcomeNotApplied},
		{sqlite.FailpointAfterDurableCommit, OutcomeApplied},
	}
	for index, test := range tests {
		t.Run(string(test.failpoint), func(t *testing.T) {
			root := t.TempDir()
			databasePath := filepath.Join(root, "runtime.sqlite")
			mutationPath := filepath.Join(root, "official.json")
			markerPath := filepath.Join(root, "official.started")
			refs := OfficialMutationRefs{EventID: domain.EventID("event_official_worker_" + string(rune('a'+index))), CommitID: "commit_official_worker", ReceiptID: "receipt_official_worker", MissionRevision: "revision_official_worker", IdempotencyKey: "idem_official_worker", CanonicalType: "observation", CanonicalID: "observation_official_worker"}
			mutation := OfficialMutation{SchemaVersion: 1, Refs: refs, OccurredAt: time.Date(2026, 7, 15, 21, index, 0, 0, time.UTC)}
			encoded, err := json.Marshal(mutation)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(mutationPath, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			open := func() (port.Store, func() error, error) {
				store, err := sqlite.Open(databasePath)
				if err != nil {
					return nil, nil, err
				}
				return store, store.Close, nil
			}
			result, err := RunCrashTrialWithInspector(context.Background(), CrashCommand{Executable: worker, Args: []string{"-backend", "sqlite", "-path", databasePath, "-failpoint", string(test.failpoint), "-intent", mutationPath, "-marker", markerPath, "-mutation", "official"}}, open, func(ctx context.Context, store port.Store) (CrashOutcome, error) {
				return InspectOfficialMutation(ctx, store, refs)
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.WorkerCrashed || result.Outcome != test.want {
				t.Fatalf("crashed=%v outcome=%s want=%s exit=%q", result.WorkerCrashed, result.Outcome, test.want, result.ExitError)
			}
		})
	}
}

func TestStorageSpikeWorkerCrashesAtDoltCLIBoundaries(t *testing.T) {
	doltBinary := os.Getenv("DOLT_BIN")
	if doltBinary == "" {
		t.Skip("DOLT_BIN is not set; Dolt crash trials require an explicit binary")
	}
	worker := filepath.Join(t.TempDir(), "storage-spike-worker")
	build := exec.Command("go", "build", "-o", worker, "./cmd/storage-spike-worker")
	build.Dir = projectRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build worker: %v\n%s", err, output)
	}

	tests := []struct {
		name      string
		failpoint dolt.Failpoint
		want      CrashOutcome
	}{
		{name: "before SQL and Dolt commit", failpoint: dolt.FailpointBeforeSQLAndDoltCommit, want: OutcomeNotApplied},
		{name: "after SQL and Dolt commit", failpoint: dolt.FailpointAfterSQLAndDoltCommit, want: OutcomeApplied},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			repositoryPath := filepath.Join(root, "runtime")
			intentPath := filepath.Join(root, "intent.json")
			markerPath := filepath.Join(root, "intent.started")
			intent := CrashIntent{Event: domain.Event{
				SchemaVersion: 1,
				ID:            domain.EventID("event_dolt_worker_" + test.name),
				Kind:          "spike.dolt_worker_crash",
				OccurredAt:    time.Date(2026, 7, 15, 20, index, 0, 0, time.UTC),
			}}
			encoded, err := json.Marshal(intent)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(intentPath, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			open := func() (port.Store, func() error, error) {
				store, err := dolt.Open(doltBinary, repositoryPath)
				if err != nil {
					return nil, nil, err
				}
				return store, store.Close, nil
			}
			result, err := RunCrashTrial(context.Background(), CrashCommand{
				Executable: worker,
				Args: []string{
					"-backend", "dolt",
					"-path", repositoryPath,
					"-failpoint", string(test.failpoint),
					"-intent", intentPath,
					"-marker", markerPath,
					"-dolt-bin", doltBinary,
				},
			}, open, intent)
			if err != nil {
				t.Fatal(err)
			}
			if result.ExitError == "" {
				t.Fatal("worker returned normally; wanted abrupt process termination")
			}
			if result.Outcome != test.want {
				t.Fatalf("outcome = %s, want %s (exit %q)", result.Outcome, test.want, result.ExitError)
			}
			if _, err := os.Stat(markerPath); err != nil {
				t.Fatalf("durable intention marker: %v", err)
			}
		})
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
