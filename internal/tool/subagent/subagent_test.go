package subagent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/tool/subagent"
)

type mockClock struct {
	now time.Time
}

func TestSessionsSpawnToolInjectsTrustedPeerBinding(t *testing.T) {
	clock := &mockClock{now: time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)}
	sm := kernel.NewLocalSessionManager(clock)
	spawnTool := subagent.NewSessionsSpawnToolWithTrustedLabels(sm, map[string]string{kernel.SubagentTransportPeerLabel: "peer-a"})

	out, err := spawnTool.Execute(context.Background(), []byte(`{"task":"remote work","context":"isolated","transport_peer_id":"peer-evil"}`))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatal(err)
	}
	status, err := sm.Status(context.Background(), kernel.SessionID(response.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	if got := status.Spec.Labels[kernel.SubagentTransportPeerLabel]; got != "peer-a" {
		t.Fatalf("transport peer = %q, want trusted peer-a", got)
	}
}

func (c *mockClock) Now() time.Time {
	return c.now
}

func TestSessionsSpawnTool_Execute(t *testing.T) {
	clock := &mockClock{now: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)}
	sm := kernel.NewLocalSessionManager(clock)
	spawnTool := subagent.NewSessionsSpawnTool(sm)

	if spawnTool.Definition().Name != "sessions_spawn" {
		t.Errorf("expected name sessions_spawn, got %s", spawnTool.Definition().Name)
	}

	ctx := context.Background()
	args := []byte(`{"task": "Analyze data", "context": "isolated"}`)

	out, err := spawnTool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	result := string(out)
	if !strings.Contains(result, `"session_id"`) || !strings.Contains(result, `"PENDING"`) {
		t.Errorf("unexpected output: %s", result)
	}

	// Test default context fallback
	args2 := []byte(`{"task": "Fallback test"}`)
	out2, err := spawnTool.Execute(ctx, args2)
	if err != nil {
		t.Fatalf("unexpected execution error with omitted context: %v", err)
	}
	if !strings.Contains(string(out2), `"session_id"`) {
		t.Errorf("unexpected output for fallback: %s", out2)
	}

	cap := spawnTool.Capability()
	if cap.Name != "sessions_spawn" {
		t.Errorf("expected capability Name sessions_spawn, got %s", cap.Name)
	}
}

func TestSessionsSpawnTool_ExecuteDisabled(t *testing.T) {
	spawnTool := subagent.NewSessionsSpawnTool(nil)

	ctx := context.Background()
	args := []byte(`{"task": "Analyze data"}`)

	_, err := spawnTool.Execute(ctx, args)
	if err == nil {
		t.Fatal("expected error for nil manager, got none")
	}
	if !strings.Contains(err.Error(), "subagent feature is disabled") {
		t.Errorf("expected disabled error, got: %v", err)
	}
}
