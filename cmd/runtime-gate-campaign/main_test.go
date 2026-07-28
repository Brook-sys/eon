package main

import (
	"context"
	"errors"
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
