package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/gatecampaign"
	"motor-autonomo/internal/runtime/source"
)

func TestWaitBeforeNextTrialUsesCompletedAtAndVirtualClock(t *testing.T) {
	start := time.Date(2026, 7, 27, 21, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(start)
	done := make(chan error, 1)
	go func() {
		done <- waitBeforeNextTrial(context.Background(), clock, gatecampaign.RuntimeGateCampaignReport{CompletedAt: start}, 10*time.Second)
	}()
	select {
	case err := <-done:
		t.Fatalf("wait returned early: %v", err)
	default:
	}
	if err := clock.Advance(9 * time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		t.Fatalf("wait returned before deadline: %v", err)
	default:
	}
	if err := clock.Advance(time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWaitBeforeNextTrialHonorsCancellation(t *testing.T) {
	start := time.Date(2026, 7, 27, 21, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitBeforeNextTrial(ctx, source.NewManualClock(start), gatecampaign.RuntimeGateCampaignReport{CompletedAt: start}, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestWaitBeforeNextTrialZeroDelayReturnsImmediately(t *testing.T) {
	if err := waitBeforeNextTrial(context.Background(), source.NewManualClock(time.Time{}), gatecampaign.RuntimeGateCampaignReport{}, 0); err != nil {
		t.Fatal(err)
	}
}

func TestCrashAfterPacingStatePublicationFaultInjection(t *testing.T) {
	if os.Getenv("GO_WANT_PACING_CRASH_HELPER") == "1" {
		crashAfterPacingStatePublication(1)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestCrashAfterPacingStatePublicationFaultInjection")
	cmd.Env = append(os.Environ(), "GO_WANT_PACING_CRASH_HELPER=1", faultAfterPacingStateEnvironment+"=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 86 {
		t.Fatalf("helper error = %v, want exit code 86", err)
	}
}

func TestLoadPacingStateResumesAfterDurableCompletedTrial(t *testing.T) {
	root := t.TempDir()
	trialDir := filepath.Join(root, "trials", "001")
	if err := os.MkdirAll(trialDir, 0o755); err != nil {
		t.Fatal(err)
	}
	completedAt := time.Date(2026, 7, 27, 22, 0, 0, 0, time.UTC)
	report := gatecampaign.RuntimeGateCampaignReport{SchemaVersion: 1, CompletedAt: completedAt, DurableReopen: true}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trialDir, "runtime-gate.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	state := pacingState{SchemaVersion: pacingStateSchemaVersion, CampaignName: "resume", PlannedTrials: 2, CompletedTrials: 1, NextTrialNotBefore: completedAt.Add(30 * time.Second)}
	if err := writePacingState(filepath.Join(root, "pacing-state.json"), state); err != nil {
		t.Fatal(err)
	}

	loaded, reports, err := loadPacingState(filepath.Join(root, "pacing-state.json"), root, "resume", 2)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CompletedTrials != 1 || len(reports) != 1 {
		t.Fatalf("loaded completed=%d reports=%d, want 1 and 1", loaded.CompletedTrials, len(reports))
	}
	clock := source.NewManualClock(completedAt.Add(29 * time.Second))
	done := make(chan error, 1)
	go func() { done <- waitUntilNextTrial(context.Background(), clock, loaded.NextTrialNotBefore) }()
	select {
	case err := <-done:
		t.Fatalf("resumed wait returned before persisted deadline: %v", err)
	default:
	}
	if err := clock.Advance(time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
