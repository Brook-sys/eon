package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/tool"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const maxCapturedOutput = 64 * 1024

type ExecArgs struct {
	Command []string `json:"command"`
	WorkDir string   `json:"work_dir,omitempty"`
}

type ExecTool struct {
	allowExec bool
	baseDir   string
}

func NewExecTool(allowExec bool, baseDir string) *ExecTool {
	return &ExecTool{allowExec: allowExec, baseDir: baseDir}
}

func (t *ExecTool) Definition() port.ToolDefinition {
	return port.ToolDefinition{
		Name:        "exec_command",
		Description: "Executes a binary directly without a shell, within the configured workspace. Captures bounded stdout, stderr, and exit code. Requires allow_exec config.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"array","items":{"type":"string"},"description":"Executable and arguments as an array; no shell expansion is performed"},"work_dir":{"type":"string","description":"Optional workspace-relative working directory"}},"required":["command"]}`),
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
	decoder := json.NewDecoder(strings.NewReader(string(args)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&a); err != nil {
		return "", tool.DispatchError{Err: err, FallbackPrompt: "Arguments must be a valid JSON object with a 'command' array of strings and optional workspace-relative 'work_dir'."}
	}

	if len(a.Command) == 0 || strings.TrimSpace(a.Command[0]) == "" {
		return "", tool.DispatchError{Err: errors.New("empty command"), FallbackPrompt: "The 'command' array must not be empty and the first element must be the executable."}
	}

	workDir, err := t.resolveWorkDir(a.WorkDir)
	if err != nil {
		return "", tool.DispatchError{Err: err, FallbackPrompt: "The working directory must be an existing directory inside the configured workspace."}
	}

	cmd := osexec.CommandContext(ctx, a.Command[0], a.Command[1:]...)
	cmd.Dir = workDir

	stdout := &limitedBuffer{limit: maxCapturedOutput}
	stderr := &limitedBuffer{limit: maxCapturedOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		exitCode = -1
		var exitErr *osexec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}

	var result strings.Builder
	result.WriteString("EXIT_CODE: ")
	result.WriteString(strconv.Itoa(exitCode))
	result.WriteString("\nSTDOUT:\n")
	result.WriteString(stdout.String())
	if stdout.truncated {
		result.WriteString("\n[truncated at 65536 bytes]")
	}
	result.WriteString("\nSTDERR:\n")
	result.WriteString(stderr.String())
	if stderr.truncated {
		result.WriteString("\n[truncated at 65536 bytes]")
	}
	if runErr != nil {
		result.WriteString("\nERROR:\n")
		result.WriteString(runErr.Error())
	}
	return strings.TrimSpace(result.String()), nil
}

func (t *ExecTool) resolveWorkDir(requested string) (string, error) {
	base, err := filepath.Abs(t.baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return "", fmt.Errorf("resolve workspace symlinks: %w", err)
	}

	clean := filepath.Clean(requested)
	if requested == "" || clean == "." {
		return base, nil
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("working directory escapes workspace")
	}

	candidate, err := filepath.EvalSymlinks(filepath.Join(base, clean))
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	rel, err := filepath.Rel(base, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("working directory escapes workspace")
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", fmt.Errorf("stat working directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("working directory is not a directory")
	}
	return candidate, nil
}

type limitedBuffer struct {
	data      []byte
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
	if original > remaining {
		b.truncated = true
	}
	return original, nil
}

func (b *limitedBuffer) String() string { return string(b.data) }
