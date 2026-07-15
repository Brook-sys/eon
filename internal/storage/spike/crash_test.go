package spike

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/storage/memory"
)

func TestCrashIntentClassification(t *testing.T) {
	store := memory.New()
	intent := CrashIntent{Event: domain.Event{SchemaVersion: 1, ID: "event_crash_1", Kind: "spike.crash_intent", OccurredAt: time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)}}
	outcome, err := InspectCrashIntent(context.Background(), store, intent)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeNotApplied {
		t.Fatalf("before apply outcome = %s", outcome)
	}
	if err := ApplyCrashIntent(context.Background(), store, intent); err != nil {
		t.Fatal(err)
	}
	outcome, err = InspectCrashIntent(context.Background(), store, intent)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeApplied {
		t.Fatalf("after apply outcome = %s", outcome)
	}
}
