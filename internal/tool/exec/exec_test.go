package exec_test

import (
	"context"
	"encoding/json"
	"motor-autonomo/internal/tool/exec"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecTool(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("hello workspace"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	t.Run("allowed execution within workspace", func(t *testing.T) {
		tool := exec.NewExecTool(true, tempDir)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"command":["cat","test.txt"],"work_dir":"."}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "hello workspace") {
			t.Errorf("expected output to contain 'hello workspace', got: %s", res)
		}
		if !strings.Contains(res, "EXIT_CODE: 0") {
			t.Errorf("expected EXIT_CODE: 0, got: %s", res)
		}
	})

	t.Run("execution disabled", func(t *testing.T) {
		tool := exec.NewExecTool(false, tempDir)
		_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":["echo","hello"]}`))
		if err == nil || !strings.Contains(err.Error(), "execution disabled") {
			t.Errorf("expected execution disabled error, got: %v", err)
		}
	})

	t.Run("invalid command execution failure", func(t *testing.T) {
		tool := exec.NewExecTool(true, tempDir)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"command":["non_existent_command_12345"]}`))
		if err != nil {
			t.Fatalf("unexpected error returned from Execute (expected internal exit/error): %v", err)
		}
		if !strings.Contains(res, "ERROR:") {
			t.Errorf("expected ERROR in output for invalid command, got: %s", res)
		}
	})

	t.Run("missing arguments", func(t *testing.T) {
		tool := exec.NewExecTool(true, tempDir)
		_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":[]}`))
		if err == nil || !strings.Contains(err.Error(), "empty command") {
			t.Errorf("expected empty command error, got: %v", err)
		}
	})

	t.Run("working directory escape", func(t *testing.T) {
		tool := exec.NewExecTool(true, tempDir)
		_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":["echo","hello"],"work_dir":"../"}`))
		if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
			t.Errorf("expected escape workspace error, got: %v", err)
		}
		_, err = tool.Execute(context.Background(), json.RawMessage(`{"command":["echo","hello"],"work_dir":"/tmp"}`))
		if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
			t.Errorf("expected escape workspace error, got: %v", err)
		}
	})

	t.Run("truncated output", func(t *testing.T) {
		tool := exec.NewExecTool(true, tempDir)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"command":["sh","-c","yes 12345 | head -n 20000"]}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "[truncated at 65536 bytes]") {
			t.Errorf("expected output to indicate truncation, got: %s", res)
		}
	})
}
