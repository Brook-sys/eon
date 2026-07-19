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

type WriteFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type WriteFileTool struct {
	baseDir string
}

func NewWriteFileTool(baseDir string) *WriteFileTool {
	return &WriteFileTool{baseDir: baseDir}
}

func (t *WriteFileTool) Definition() port.ToolDefinition {
	return port.ToolDefinition{
		Name:        "write_file",
		Description: "Writes content to a file, creating it if it doesn't exist, within the allowed directory.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Relative path to the file"},"content":{"type":"string","description":"Content to write to the file"}},"required":["path","content"]}`),
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a WriteFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", tool.DispatchError{Err: err, FallbackPrompt: "Arguments must be a valid JSON object with 'path' and 'content' string fields."}
	}

	cleanPath := filepath.Clean(a.Path)
	if filepath.IsAbs(cleanPath) || cleanPath == ".." || len(cleanPath) >= 3 && cleanPath[:3] == "../" {
		return "", tool.DispatchError{Err: errors.New("access denied"), FallbackPrompt: "Path must be relative and cannot point outside the base directory."}
	}

	fullPath := filepath.Join(t.baseDir, cleanPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", err
	}

	if err := os.WriteFile(fullPath, []byte(a.Content), 0644); err != nil {
		return "", err
	}

	return "File written successfully.", nil
}
