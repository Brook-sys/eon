package exec_test

import (
	"context"
	"encoding/json"
	"motor-autonomo/internal/tool/exec"
	"strings"
	"testing"
)

func TestExecTool(t *testing.T) {
	t.Run("allowed execution", func(t *testing.T) {
		tool := exec.NewExecTool(true)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"command":["echo","hello"]}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "hello") {
			t.Errorf("expected output to contain 'hello', got: %s", res)
		}
	})

	t.Run("execution disabled", func(t *testing.T) {
		tool := exec.NewExecTool(false)
		_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":["echo","hello"]}`))
		if err == nil || !strings.Contains(err.Error(), "execution disabled") {
			t.Errorf("expected execution disabled error, got: %v", err)
		}
	})

	t.Run("invalid command", func(t *testing.T) {
		tool := exec.NewExecTool(true)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"command":["non_existent_command_12345"]}`))
		if err != nil {
			t.Fatalf("unexpected error returned from Execute: %v", err)
		}
		if !strings.Contains(res, "ERROR:") {
			t.Errorf("expected ERROR in output for invalid command, got: %s", res)
		}
	})

	t.Run("missing arguments", func(t *testing.T) {
		tool := exec.NewExecTool(true)
		_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":[]}`))
		if err == nil || !strings.Contains(err.Error(), "empty command") {
			t.Errorf("expected empty command error, got: %v", err)
		}
	})
}
