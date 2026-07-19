package yield_test

import (
	"context"
	"encoding/json"
	"testing"

	"motor-autonomo/internal/tool/yield"
)

func TestSessionsYieldTool(t *testing.T) {
	tool := yield.NewSessionsYieldTool()
	def := tool.Definition()
	if def.Name != "sessions_yield" {
		t.Errorf("expected name sessions_yield, got %s", def.Name)
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"message":"waiting"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "YIELDED" {
		t.Errorf("expected status YIELDED, got %s", resp["status"])
	}
}

