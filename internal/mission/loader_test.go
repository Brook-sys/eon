package mission

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

const validSpec = `{
  "schema_version": 1,
  "id": "mission_1",
  "revision": 1,
  "original_text": "Investigate reliable epistemic runtimes.",
  "purpose": "Build cited, recoverable knowledge.",
  "domains": ["runtime", "knowledge"],
  "policies": ["cite sources", "no direct model authority"],
  "budget": {"model_calls": 10, "tokens": 8000, "bytes": 65536, "attempts": 3, "duration": 60000000000},
  "status": "ACTIVE"
}`

func TestLoaderInstallsRevisionAndAuditEventAtomically(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 15, 13, 40, 0, 0, time.UTC)
	loader := Loader{Store: store, Clock: source.NewManualClock(now), IDs: source.NewSequenceIDGenerator(1)}

	revision, err := loader.Load(context.Background(), []byte(validSpec), "user:heartbeat")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if revision.ID != "mission_revision_0000000000000001" || !revision.AcceptedAt.Equal(now) || revision.Provenance != "user:heartbeat" {
		t.Fatalf("revision = %#v", revision)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		active, err := r.ActiveMissionRevision("mission_1")
		if err != nil {
			return err
		}
		if active.ID != revision.ID || active.OriginalText != "Investigate reliable epistemic runtimes." {
			t.Fatalf("active = %#v", active)
		}
		events, err := r.Events(0, 10)
		if err != nil {
			return err
		}
		if len(events) != 1 || events[0].Kind != EventMissionRevisionActivated || events[0].MissionRevision != revision.ID || events[0].PayloadRef != string(revision.ID) {
			t.Fatalf("events = %#v", events)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLoaderRejectsInvalidOrAmbiguousInputWithoutMutation(t *testing.T) {
	tests := map[string]string{
		"unknown field":      strings.Replace(validSpec, `"status": "ACTIVE"`, `"status": "ACTIVE", "authority": "MODEL"`, 1),
		"unsupported schema": strings.Replace(validSpec, `"schema_version": 1`, `"schema_version": 2`, 1),
		"missing revision":   strings.Replace(validSpec, `"revision": 1`, `"revision": 0`, 1),
		"inactive status":    strings.Replace(validSpec, `"status": "ACTIVE"`, `"status": "PAUSED"`, 1),
		"duplicate policy":   strings.Replace(validSpec, `"policies": ["cite sources", "no direct model authority"]`, `"policies": ["cite sources", "cite sources"]`, 1),
		"trailing value":     validSpec + ` {}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			store := memory.New()
			loader := Loader{Store: store, Clock: source.NewManualClock(time.Now()), IDs: source.NewSequenceIDGenerator(1)}
			if _, err := loader.Load(context.Background(), []byte(raw), "user"); err == nil {
				t.Fatal("invalid spec accepted")
			}
			if err := assertEmpty(store); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLoaderEnforcesByteLimitAndRevisionUniqueness(t *testing.T) {
	store := memory.New()
	loader := Loader{Store: store, Clock: source.NewManualClock(time.Date(2026, 7, 15, 13, 40, 0, 0, time.UTC)), IDs: source.NewSequenceIDGenerator(1)}
	if _, err := loader.Load(context.Background(), []byte(validSpec), "user"); err != nil {
		t.Fatal(err)
	}
	_, err := loader.Load(context.Background(), []byte(validSpec), "user")
	if !errors.Is(err, port.ErrConflict) {
		t.Fatalf("duplicate revision error = %v, want ErrConflict", err)
	}

	limited := Loader{Store: memory.New(), Clock: loader.Clock, IDs: source.NewSequenceIDGenerator(1), MaxSpecBytes: int64(len(validSpec) - 1)}
	if _, err := limited.Load(context.Background(), []byte(validSpec), "user"); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversize error = %v", err)
	}
}

func assertEmpty(store port.Store) error {
	return store.View(context.Background(), func(r port.Reader) error {
		_, missionErr := r.ActiveMissionRevision(domain.MissionID("mission_1"))
		events, eventsErr := r.Events(0, 10)
		if !errors.Is(missionErr, port.ErrNotFound) {
			return errors.New("mission mutation survived failed load")
		}
		if eventsErr != nil {
			return eventsErr
		}
		if len(events) != 0 {
			return errors.New("event mutation survived failed load")
		}
		return nil
	})
}
