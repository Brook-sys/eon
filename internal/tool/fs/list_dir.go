package fs

import (
	"context"
	"encoding/json"
	"errors"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/tool"
	"os"
	"path/filepath"
	"strings"
)

type ListDirArgs struct {
	Path string `json:"path"`
}

type ListDirTool struct {
	baseDir string
}

func NewListDirTool(baseDir string) *ListDirTool {
	return &ListDirTool{baseDir: baseDir}
}

func (t *ListDirTool) Definition() port.ToolDefinition {
	return port.ToolDefinition{
		Name:        "list_dir",
		Description: "Lists the contents of a directory within the allowed directory. Use '.' for the root.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Relative path to the directory"}},"required":["path"]}`),
	}
}

func (t *ListDirTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a ListDirArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", tool.DispatchError{Err: err, FallbackPrompt: "Arguments must be a valid JSON object with a 'path' string field."}
	}

	cleanPath := filepath.Clean(a.Path)
	if filepath.IsAbs(cleanPath) || cleanPath == ".." || len(cleanPath) >= 3 && cleanPath[:3] == "../" {
		return "", tool.DispatchError{Err: errors.New("access denied"), FallbackPrompt: "Path must be relative and cannot point outside the base directory."}
	}

	fullPath := filepath.Join(t.baseDir, cleanPath)
	
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", tool.DispatchError{Err: err, FallbackPrompt: "The specified directory does not exist."}
		}
		return "", err
	}

	var sb strings.Builder
	for _, entry := range entries {
		if entry.IsDir() {
			sb.WriteString(entry.Name() + "/\n")
		} else {
			sb.WriteString(entry.Name() + "\n")
		}
	}

	res := sb.String()
	if res == "" {
		res = "(empty directory)"
	}
	return res, nil
}
