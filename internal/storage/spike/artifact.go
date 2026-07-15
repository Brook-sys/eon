package spike

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteArtifacts emits the backend-neutral, reviewable output of one measured
// run. The caller chooses the run directory so environment-specific run IDs do
// not contaminate deterministic workload generation.
func WriteArtifacts(directory string, manifest Manifest, metrics Metrics) error {
	if strings.TrimSpace(directory) == "" {
		return fmt.Errorf("artifact directory is required")
	}
	if manifest.SHA256 == "" || metrics.DatasetSHA256 != manifest.SHA256 {
		return fmt.Errorf("manifest and metrics dataset digests must match")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest artifact: %w", err)
	}
	metricsJSON, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metrics artifact: %w", err)
	}
	report := renderReport(metrics)
	for name, content := range map[string][]byte{
		"manifest.json": append(manifestJSON, '\n'),
		"metrics.json":  append(metricsJSON, '\n'),
		"report.md":     []byte(report),
	} {
		if err := writeAtomic(filepath.Join(directory, name), content); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

// WriteCrashCampaignArtifact persists every trial, not only aggregate counts,
// so a storage decision can be audited against worker exits and classifications.
func WriteCrashCampaignArtifact(path string, result CrashCampaignResult) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("crash campaign artifact path is required")
	}
	if len(result.Trials) < MinCrashCampaignTrials {
		return fmt.Errorf("crash campaign artifact requires at least %d trials", MinCrashCampaignTrials)
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode crash campaign artifact: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create crash campaign artifact directory: %w", err)
	}
	if err := writeAtomic(path, append(encoded, '\n')); err != nil {
		return fmt.Errorf("write crash campaign artifact: %w", err)
	}
	return nil
}

func renderReport(metrics Metrics) string {
	var report strings.Builder
	fmt.Fprintf(&report, "# Storage spike run: %s\n\n", metrics.Backend)
	fmt.Fprintf(&report, "- Dataset SHA-256: `%s`\n", metrics.DatasetSHA256)
	if metrics.BackendVersion != "" {
		fmt.Fprintf(&report, "- Backend version: `%s`\n", metrics.BackendVersion)
	}
	if metrics.DriverVersion != "" {
		fmt.Fprintf(&report, "- Driver version: `%s`\n", metrics.DriverVersion)
	}
	fmt.Fprintf(&report, "- Go/platform: `%s` on `%s/%s`\n", metrics.GoVersion, metrics.GOOS, metrics.GOARCH)
	fmt.Fprintf(&report, "- Batch size: %d\n", metrics.BatchSize)
	fmt.Fprintf(&report, "- Started: %s\n- Finished: %s\n", metrics.StartedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), metrics.FinishedAt.Format("2006-01-02T15:04:05.999999999Z07:00"))
	if metrics.Footprint != nil {
		fmt.Fprintf(&report, "- Disk footprint: %d → %d bytes (delta %+d)\n", metrics.Footprint.BeforeBytes, metrics.Footprint.AfterBytes, metrics.Footprint.DeltaBytes)
	}
	report.WriteString("\n## Phases\n\n")
	for _, phase := range metrics.Phases {
		fmt.Fprintf(&report, "- `%s`: %d operations in %d batches; p50=%s, p95=%s, p99=%s, throughput=%.2f ops/s\n", phase.Name, phase.Operations, phase.Batches, phase.P50, phase.P95, phase.P99, phase.Throughput)
	}
	return report.String()
}

func writeAtomic(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".artifact-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
