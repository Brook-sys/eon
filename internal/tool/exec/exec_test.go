package exec_test

import (
	"context"
	"encoding/json"
	"motor-autonomo/internal/tool/exec"
	"strings"
	"testing"
)

func TestExecTool(t *testing.T) {
	// Test allowed execution
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

	// Test execution disabled
	t.Run("execution disabled", func(t *testing.T) {
		tool := exec.NewExecTool(false)
		_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":["echo","hello"]}`))
		if err == nil || !strings.Contains(err.Error(), "execution disabled") {
			t.Errorf("expected execution disabled error, got: %v", err)
		}
	})

	// Test command execution error (e.g. invalid command)
	t.Run("invalid command", func(t *testing.T) {
		tool := exec.NewExecTool(true)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"command":["non_existent_command"]}`))
		// exec.Cmd.Run() returns an error, but the tool captures it in the result string and returns a nil error from the Execute method itself.
		if err != nil {
			t.Fatalf("unexpected error returned from Execute, the error should be captured in output: %v", err)
		}
		if !strings.Contains(res, "ERROR:") {
			t.Errorf("expected ERROR in output for invalid command, got: %s", res)
		}
	})
}
