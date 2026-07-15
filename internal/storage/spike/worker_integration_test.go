package spike

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
	worker := buildStorageSpikeWorker(t)
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

func TestStorageSpikeWorkerCrashesOfficialMutationAtomicallyInDolt(t *testing.T) {
	doltBinary := requireDoltBinary(t)
	worker := buildStorageSpikeWorker(t)
	tests := []struct {
		failpoint dolt.Failpoint
		want      CrashOutcome
	}{
		{dolt.FailpointBeforeSQLAndDoltCommit, OutcomeNotApplied},
		{dolt.FailpointAfterSQLAndDoltCommit, OutcomeApplied},
	}
	for index, test := range tests {
		t.Run(string(test.failpoint), func(t *testing.T) {
			root := t.TempDir()
			repositoryPath := filepath.Join(root, "runtime")
			mutationPath := filepath.Join(root, "official.json")
			markerPath := filepath.Join(root, "official.started")
			refs := OfficialMutationRefs{EventID: domain.EventID("event_official_dolt_" + string(rune('a'+index))), CommitID: "commit_official_dolt", ReceiptID: "receipt_official_dolt", MissionRevision: "revision_official_dolt", IdempotencyKey: "idem_official_dolt", CanonicalType: "observation", CanonicalID: "observation_official_dolt"}
			mutation := OfficialMutation{SchemaVersion: 1, Refs: refs, OccurredAt: time.Date(2026, 7, 15, 22, index, 0, 0, time.UTC)}
			encoded, err := json.Marshal(mutation)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(mutationPath, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			open := func() (port.Store, func() error, error) {
				store, err := dolt.Open(doltBinary, repositoryPath)
				if err != nil {
					return nil, nil, err
				}
				return store, store.Close, nil
			}
			result, err := RunCrashTrialWithInspector(context.Background(), CrashCommand{Executable: worker, Args: []string{"-backend", "dolt", "-path", repositoryPath, "-failpoint", string(test.failpoint), "-intent", mutationPath, "-marker", markerPath, "-mutation", "official", "-dolt-bin", doltBinary}}, open, func(ctx context.Context, store port.Store) (CrashOutcome, error) {
				return InspectOfficialMutation(ctx, store, refs)
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.WorkerCrashed || result.Outcome != test.want {
				t.Fatalf("crashed=%v outcome=%s want=%s exit=%q", result.WorkerCrashed, result.Outcome, test.want, result.ExitError)
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

func TestStorageSpikeWorkerCrashesOfficialMutationAcrossDoltServerBoundaries(t *testing.T) {
	doltBinary := requireDoltBinary(t)
	worker := buildStorageSpikeWorker(t)
	tests := []struct {
		failpoint dolt.ServerFailpoint
		want      CrashOutcome
	}{
		{dolt.FailpointBeforeSQLCommit, OutcomeNotApplied},
		{dolt.FailpointAfterSQLCommit, OutcomeInvalidPartial},
		{dolt.FailpointAfterDoltCommit, OutcomeApplied},
	}
	for index, test := range tests {
		t.Run(string(test.failpoint), func(t *testing.T) {
			root := t.TempDir()
			repositoryPath := filepath.Join(root, "runtime")
			mutationPath := filepath.Join(root, "official.json")
			markerPath := filepath.Join(root, "official.started")
			refs := OfficialMutationRefs{EventID: domain.EventID(fmt.Sprintf("event_official_server_%02d", index)), CommitID: domain.CommitID(fmt.Sprintf("commit_official_server_%02d", index)), ReceiptID: domain.ReceiptID(fmt.Sprintf("receipt_official_server_%02d", index)), MissionRevision: domain.MissionRevisionID(fmt.Sprintf("revision_official_server_%02d", index)), IdempotencyKey: domain.IdempotencyKey(fmt.Sprintf("idem_official_server_%02d", index)), CanonicalType: "observation", CanonicalID: fmt.Sprintf("observation_official_server_%02d", index)}
			mutation := OfficialMutation{SchemaVersion: 1, Refs: refs, OccurredAt: time.Date(2026, 7, 15, 23, index, 0, 0, time.UTC)}
			encoded, err := json.Marshal(mutation)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(mutationPath, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			open := func() (port.Store, func() error, error) {
				store, err := dolt.OpenServer(doltBinary, repositoryPath)
				if err != nil {
					return nil, nil, err
				}
				return store, store.Close, nil
			}
			result, err := RunCrashTrialWithInspector(context.Background(), CrashCommand{Executable: worker, Args: []string{"-backend", "dolt-server", "-path", repositoryPath, "-failpoint", string(test.failpoint), "-intent", mutationPath, "-marker", markerPath, "-mutation", "official", "-dolt-bin", doltBinary}}, open, func(ctx context.Context, store port.Store) (CrashOutcome, error) {
				outcome, err := InspectOfficialMutation(ctx, store, refs)
				if err != nil {
					return "", err
				}
				server := store.(*dolt.ServerStore)
				clean, err := server.WorkingSetClean(ctx)
				if err != nil {
					return "", err
				}
				if !clean {
					return OutcomeInvalidPartial, nil
				}
				return outcome, nil
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

func TestDoltServerOfficialCrashCampaigns(t *testing.T) {
	if os.Getenv("STORAGE_SPIKE_FULL") != "1" {
		t.Skip("set STORAGE_SPIKE_FULL=1 to run 30-trial campaigns at every Dolt server boundary")
	}
	doltBinary := requireDoltBinary(t)
	worker := buildStorageSpikeWorker(t)
	root := t.TempDir()
	tests := []struct {
		failpoint dolt.ServerFailpoint
		want      CrashOutcome
	}{
		{dolt.FailpointBeforeSQLCommit, OutcomeNotApplied},
		{dolt.FailpointAfterSQLCommit, OutcomeInvalidPartial},
		{dolt.FailpointAfterDoltCommit, OutcomeApplied},
	}
	for _, test := range tests {
		t.Run(string(test.failpoint), func(t *testing.T) {
			result, err := RunCrashCampaignWithInspector(context.Background(), MinCrashCampaignTrials, func(index int) (CrashCommand, StoreOpener, VisibilityInspector, error) {
				trialRoot := filepath.Join(root, fmt.Sprintf("%s-%02d", test.failpoint, index))
				if err := os.MkdirAll(trialRoot, 0o755); err != nil {
					return CrashCommand{}, nil, nil, err
				}
				repositoryPath := filepath.Join(trialRoot, "runtime")
				mutationPath := filepath.Join(trialRoot, "official.json")
				markerPath := filepath.Join(trialRoot, "official.started")
				refs := OfficialMutationRefs{EventID: domain.EventID(fmt.Sprintf("event_campaign_server_%s_%02d", test.failpoint, index)), CommitID: domain.CommitID(fmt.Sprintf("commit_campaign_server_%s_%02d", test.failpoint, index)), ReceiptID: domain.ReceiptID(fmt.Sprintf("receipt_campaign_server_%s_%02d", test.failpoint, index)), MissionRevision: domain.MissionRevisionID(fmt.Sprintf("revision_campaign_server_%s_%02d", test.failpoint, index)), IdempotencyKey: domain.IdempotencyKey(fmt.Sprintf("idem_campaign_server_%s_%02d", test.failpoint, index)), CanonicalType: "observation", CanonicalID: fmt.Sprintf("observation_campaign_server_%s_%02d", test.failpoint, index)}
				mutation := OfficialMutation{SchemaVersion: 1, Refs: refs, OccurredAt: time.Date(2026, 7, 16, 0, index, 0, 0, time.UTC)}
				encoded, err := json.Marshal(mutation)
				if err != nil {
					return CrashCommand{}, nil, nil, err
				}
				if err := os.WriteFile(mutationPath, encoded, 0o600); err != nil {
					return CrashCommand{}, nil, nil, err
				}
				open := func() (port.Store, func() error, error) {
					store, err := dolt.OpenServer(doltBinary, repositoryPath)
					if err != nil {
						return nil, nil, err
					}
					return store, store.Close, nil
				}
				inspect := func(ctx context.Context, store port.Store) (CrashOutcome, error) {
					outcome, err := InspectOfficialMutation(ctx, store, refs)
					if err != nil {
						return "", err
					}
					clean, err := store.(*dolt.ServerStore).WorkingSetClean(ctx)
					if err != nil {
						return "", err
					}
					if !clean {
						return OutcomeInvalidPartial, nil
					}
					return outcome, nil
				}
				command := CrashCommand{Executable: worker, Args: []string{"-backend", "dolt-server", "-path", repositoryPath, "-failpoint", string(test.failpoint), "-intent", mutationPath, "-marker", markerPath, "-mutation", "official", "-dolt-bin", doltBinary}}
				return command, open, inspect, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Counts.InvalidPartial > 0 && test.want != OutcomeInvalidPartial {
				t.Fatalf("unexpected partial outcomes: %+v", result.Counts)
			}
			if len(result.Trials) != MinCrashCampaignTrials {
				t.Fatalf("trials=%d, want %d", len(result.Trials), MinCrashCampaignTrials)
			}
			for index, trial := range result.Trials {
				if !trial.WorkerCrashed || trial.Outcome != test.want {
					t.Fatalf("trial %d: crashed=%v outcome=%s want=%s exit=%q", index, trial.WorkerCrashed, trial.Outcome, test.want, trial.ExitError)
				}
			}
			if test.want == OutcomeInvalidPartial && result.Passed {
				t.Fatal("campaign with an ambiguous SQL-only commit must fail")
			}
			if directory := os.Getenv("STORAGE_SPIKE_RESULTS_DIR"); directory != "" {
				path := filepath.Join(directory, "dolt-server", "crash", string(test.failpoint)+".json")
				if err := WriteCrashCampaignArtifact(path, result); err != nil {
					t.Fatal(err)
				}
			}
			if test.want != OutcomeInvalidPartial && !result.Passed {
				t.Fatalf("atomic campaign failed: %+v", result.Counts)
			}
		})
	}
}

func TestStorageSpikeWorkerCrashesAtDoltCLIBoundaries(t *testing.T) {
	doltBinary := requireDoltBinary(t)
	worker := buildStorageSpikeWorker(t)

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

func buildStorageSpikeWorker(t *testing.T) string {
	t.Helper()
	worker := filepath.Join(t.TempDir(), "storage-spike-worker")
	build := exec.Command("go", "build", "-o", worker, "./cmd/storage-spike-worker")
	build.Dir = projectRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build worker: %v\n%s", err, output)
	}
	return worker
}

func requireDoltBinary(t *testing.T) string {
	t.Helper()
	binary := os.Getenv("DOLT_BIN")
	if binary == "" {
		t.Skip("DOLT_BIN is not set; Dolt crash trials require an explicit binary")
	}
	return binary
}

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
