package spike

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteArtifactsEmitsManifestMetricsAndReport(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "results", "sqlite", "run-1")
	manifest := Manifest{SchemaVersion: 1, SHA256: "abc"}
	metrics := Metrics{
		SchemaVersion: 3, Backend: "sqlite", BackendVersion: "SQLite test", DriverVersion: "driver test",
		GoVersion: "go-test", GOOS: "test-os", GOARCH: "test-arch", BatchSize: 10, DatasetSHA256: "abc",
		StartedAt: time.Unix(1, 0).UTC(), FinishedAt: time.Unix(2, 0).UTC(),
		Footprint: &FootprintMetric{BeforeBytes: 10, AfterBytes: 15, DeltaBytes: 5},
		Phases:    []PhaseMetric{{Name: "load", Operations: 2, Batches: 1, P50: time.Millisecond, P95: 2 * time.Millisecond, P99: 3 * time.Millisecond, Throughput: 4}},
	}
	if err := WriteArtifacts(directory, manifest, metrics); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"manifest.json", "metrics.json", "report.md"} {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if len(content) == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
	report, _ := os.ReadFile(filepath.Join(directory, "report.md"))
	if !strings.Contains(string(report), "10 → 15 bytes") || !strings.Contains(string(report), "`load`") || !strings.Contains(string(report), "SQLite test") {
		t.Fatalf("unexpected report:\n%s", report)
	}
}

func TestWriteArtifactsRejectsDatasetMismatch(t *testing.T) {
	err := WriteArtifacts(t.TempDir(), Manifest{SHA256: "one"}, Metrics{DatasetSHA256: "two"})
	if err == nil {
		t.Fatal("dataset mismatch was accepted")
	}
}

func TestWriteCrashCampaignArtifactPreservesTrials(t *testing.T) {
	trials := make([]CrashTrialResult, MinCrashCampaignTrials)
	for index := range trials {
		trials[index] = CrashTrialResult{WorkerCrashed: true, Outcome: OutcomeApplied, ExitError: "signal: killed"}
	}
	result := CrashCampaignResult{Trials: trials, Counts: CrashOutcomeCounts{Applied: MinCrashCampaignTrials}, Passed: true}
	path := filepath.Join(t.TempDir(), "crash", "after_dolt_commit.json")
	if err := WriteCrashCampaignArtifact(path, result); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got CrashCampaignResult
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Trials) != MinCrashCampaignTrials || got.Counts.Applied != MinCrashCampaignTrials || !got.Passed {
		t.Fatalf("unexpected artifact: %+v", got)
	}
}
