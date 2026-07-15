package spike

import (
	"context"
	"fmt"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestRunCrashCampaignRepeatsAndAggregates(t *testing.T) {
	result, err := RunCrashCampaign(context.Background(), MinCrashCampaignTrials, func(index int) (CrashCommand, StoreOpener, CrashIntent, error) {
		store := memory.New()
		intent := CrashIntent{Event: domain.Event{
			SchemaVersion: 1,
			ID:            domain.EventID(fmt.Sprintf("event_campaign_%02d", index)),
			Kind:          "spike.campaign",
			OccurredAt:    time.Date(2026, 7, 15, 20, index, 0, 0, time.UTC),
		}}
		open := func() (port.Store, func() error, error) { return store, nil, nil }
		return CrashCommand{Executable: "/bin/sh", Args: []string{"-c", "exit 17"}}, open, intent, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatal("campaign failed despite abrupt workers and atomic outcomes")
	}
	if len(result.Trials) != MinCrashCampaignTrials || result.Counts.NotApplied != MinCrashCampaignTrials {
		t.Fatalf("unexpected aggregate: trials=%d counts=%+v", len(result.Trials), result.Counts)
	}
	if result.Counts.Applied != 0 || result.Counts.InvalidPartial != 0 {
		t.Fatalf("unexpected outcomes: %+v", result.Counts)
	}
}

func TestRunCrashCampaignRejectsInsufficientRepetition(t *testing.T) {
	_, err := RunCrashCampaign(context.Background(), MinCrashCampaignTrials-1, func(int) (CrashCommand, StoreOpener, CrashIntent, error) {
		return CrashCommand{}, nil, CrashIntent{}, nil
	})
	if err == nil {
		t.Fatal("expected minimum trial validation")
	}
}

func TestRunCrashCampaignFailsWhenWorkerReturnsNormally(t *testing.T) {
	result, err := RunCrashCampaign(context.Background(), MinCrashCampaignTrials, func(index int) (CrashCommand, StoreOpener, CrashIntent, error) {
		store := memory.New()
		intent := CrashIntent{Event: domain.Event{
			SchemaVersion: 1,
			ID:            domain.EventID(fmt.Sprintf("event_normal_%02d", index)),
			Kind:          "spike.normal_exit",
			OccurredAt:    time.Date(2026, 7, 15, 21, index, 0, 0, time.UTC),
		}}
		open := func() (port.Store, func() error, error) { return store, nil, nil }
		return CrashCommand{Executable: "/bin/sh", Args: []string{"-c", "exit 0"}}, open, intent, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed {
		t.Fatal("normal worker exit must invalidate a crash campaign")
	}
}
