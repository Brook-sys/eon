package spike_test

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
	"motor-autonomo/internal/storage/spike"
)

// In phase 145 we must ensure that intentional SQLite durability fallbacks during
// spike crash campaigns do not cause context leakage (e.g., leaving cancelled context
// references or failing to isolate the error handling boundaries).

func TestCrashIntentClassifierDetectsFallbackWithoutContextLeakage(t *testing.T) {
	t.Parallel()
	
	// Ensure we use an isolatable timeout that naturally expires
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()

	store := memory.New()
	
	intent := spike.CrashIntent{
		Event: domain.Event{
			SchemaVersion: 1, 
			ID: "event_fallback_leak", 
			Kind: "spike.crash_intent.fallback", 
			OccurredAt: time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC),
		},
	}
	
	// Wait for context to expire to simulate a timeout/fallback boundary
	<-ctx.Done()
	
	// Simulating the intent application over an already expired context 
	// must result in a natural rollback and NOT_APPLIED, without panic or leaking context errors.
	err := spike.ApplyCrashIntent(ctx, store, intent)
	if err == nil {
		t.Fatalf("expected error applying on expired context, got nil")
	}

	// We must inspect using a fresh context to mimic restart/recovery.
	// If the crash harness fails to isolate boundaries, inspecting here could panic or block.
	freshCtx := context.Background()
	outcome, inspectErr := spike.InspectCrashIntent(freshCtx, store, intent)
	if inspectErr != nil {
		t.Fatalf("inspect error on fallback recovery boundary: %v", inspectErr)
	}

	if outcome != spike.OutcomeNotApplied {
		t.Errorf("expected outcome NOT_APPLIED for fallback-terminated trial, got %v", outcome)
	}
	
	// Verify that the event log remains completely clean and atomic
	err = store.View(freshCtx, func(r port.Reader) error {
		events, err := r.Events(0, 5)
		if err != nil {
			return err
		}
		if len(events) > 0 {
			t.Errorf("expected empty event log after fallback rollback, got %d events", len(events))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify store: %v", err)
	}
}
