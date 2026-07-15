package spike

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/storage/memory"
)

func TestRunnerAppliesAndQueriesDataset(t *testing.T) {
	config := DatasetConfig{Seed: 7, Sources: 3, Claims: 5, EvidenceLinks: 11, SnapshotMin: 32, SnapshotMax: 64}
	dataset, manifest, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	calls := 0
	runner := Runner{Now: func() time.Time { calls++; return base.Add(time.Duration(calls) * time.Millisecond) }}
	metrics, err := runner.Run(context.Background(), "memory", memory.New(), dataset, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.DatasetSHA256 != manifest.SHA256 || metrics.Backend != "memory" {
		t.Fatalf("metrics identity = %#v", metrics)
	}
	if len(metrics.Phases) != 3 {
		t.Fatalf("phase count = %d, want 3", len(metrics.Phases))
	}
	want := map[string]int{"load_sources": config.Sources, "load_claims": config.Claims, "query_claims": config.Claims}
	for _, phase := range metrics.Phases {
		if phase.Operations != want[phase.Name] {
			t.Fatalf("phase %s operations = %d, want %d", phase.Name, phase.Operations, want[phase.Name])
		}
		if phase.Duration <= 0 || phase.Throughput <= 0 {
			t.Fatalf("phase %s has invalid timing: %#v", phase.Name, phase)
		}
	}
}

func TestRunnerHonorsCancelledContext(t *testing.T) {
	dataset, manifest, err := Generate(DatasetConfig{Seed: 1, Sources: 1, Claims: 1, EvidenceLinks: 1, SnapshotMin: 16, SnapshotMax: 16})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Runner{}).Run(ctx, "memory", memory.New(), dataset, manifest); err == nil {
		t.Fatal("cancelled context was ignored")
	}
}
