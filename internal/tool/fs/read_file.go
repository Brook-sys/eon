package fs

import (
	"context"
	"encoding/json"
	"errors"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/tool"
	"os"
	"path/filepath"
)

type ReadFileArgs struct {
	Path string `json:"path"`
}

type ReadFileTool struct {
	baseDir string
}

func NewReadFileTool(baseDir string) *ReadFileTool {
	return &ReadFileTool{baseDir: baseDir}
}

func (t *ReadFileTool) Definition() port.ToolDefinition {
	return port.ToolDefinition{
		Name:        "read_file",
		Description: "Reads the content of a file within the allowed directory.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Relative path to the file to read"}},"required":["path"]}`),
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a ReadFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", tool.DispatchError{Err: err, FallbackPrompt: "Arguments must be a valid JSON object with a 'path' string field."}
	}

	cleanPath := filepath.Clean(a.Path)
	if filepath.IsAbs(cleanPath) || cleanPath == ".." || len(cleanPath) >= 3 && cleanPath[:3] == "../" {
		return "", tool.DispatchError{Err: errors.New("access denied"), FallbackPrompt: "Path must be relative and cannot point outside the base directory."}
	}

	fullPath := filepath.Join(t.baseDir, cleanPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", tool.DispatchError{Err: err, FallbackPrompt: "The specified file does not exist. Please check the path."}
		}
		return "", err
	}

	return string(content), nil
}
