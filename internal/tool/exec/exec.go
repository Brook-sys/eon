package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/tool"
	"os/exec"
	"strings"
)

type ExecArgs struct {
	Command []string `json:"command"`
	WorkDir string   `json:"work_dir,omitempty"`
}

type ExecTool struct {
	allowExec bool
}

func NewExecTool(allowExec bool) *ExecTool {
	return &ExecTool{allowExec: allowExec}
}

func (t *ExecTool) Definition() port.ToolDefinition {
	return port.ToolDefinition{
		Name:        "exec_command",
		Description: "Executes a binary command in a non-interactive shell environment. Captures stdout, stderr, and exit code. Requires allow_exec config.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"array","items":{"type":"string"},"description":"Command and arguments as array"},"work_dir":{"type":"string","description":"Optional working directory"}},"required":["command"]}`),
	}
}

func (t *ExecTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if !t.allowExec {
		return "", tool.DispatchError{
			Err:            errors.New("execution disabled by policy"),
			FallbackPrompt: "Execution of commands is disabled by security policy (allow_exec=false).",
		}
	}

	var a ExecArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", tool.DispatchError{Err: err, FallbackPrompt: "Arguments must be a valid JSON object with a 'command' array of strings."}
	}

	if len(a.Command) == 0 || strings.TrimSpace(a.Command[0]) == "" {
		return "", tool.DispatchError{Err: errors.New("empty command"), FallbackPrompt: "The 'command' array must not be empty and the first element must be the executable."}
	}

	cmd := exec.CommandContext(ctx, a.Command[0], a.Command[1:]...)
	if a.WorkDir != "" {
		cmd.Dir = a.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	
	res := "STDOUT:\n" + stdout.String() + "\nSTDERR:\n" + stderr.String()
	if err != nil {
		res += "\nERROR:\n" + err.Error()
	}

	res = strings.TrimSpace(res)
	return res, nil
}
