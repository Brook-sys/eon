package spike

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/sqlite"
)

func TestRunCrashTrialUsesSeparateProcessAndFreshStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	intent := CrashIntent{Event: domain.Event{SchemaVersion: 1, ID: "event_subprocess", Kind: "spike.subprocess", OccurredAt: time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)}}
	open := func() (port.Store, func() error, error) {
		store, err := sqlite.Open(path)
		if err != nil {
			return nil, nil, err
		}
		return store, store.Close, nil
	}
	result, err := RunCrashTrial(context.Background(), CrashCommand{Executable: os.Args[0], Args: []string{"-test.run=TestCrashWorker", "--", path}}, open, intent)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitError != "" || result.Outcome != OutcomeApplied {
		t.Fatalf("trial result = %#v", result)
	}
}

func TestCrashWorker(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	path := os.Args[len(os.Args)-1]
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	intent := CrashIntent{Event: domain.Event{SchemaVersion: 1, ID: "event_subprocess", Kind: "spike.subprocess", OccurredAt: time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)}}
	if err := ApplyCrashIntent(context.Background(), store, intent); err != nil {
		t.Fatal(err)
	}
}
