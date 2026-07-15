package spike

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestRunCrashTrialWithInspectorUsesCompoundClassifier(t *testing.T) {
	store := memory.New()
	called := false
	result, err := RunCrashTrialWithInspector(context.Background(), CrashCommand{Executable: "/bin/sh", Args: []string{"-c", "exit 17"}}, func() (port.Store, func() error, error) {
		return store, nil, nil
	}, func(context.Context, port.Store) (CrashOutcome, error) {
		called = true
		return OutcomeInvalidPartial, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called || !result.WorkerCrashed || result.Outcome != OutcomeInvalidPartial {
		t.Fatalf("unexpected compound classification result: %+v called=%t", result, called)
	}
}

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
