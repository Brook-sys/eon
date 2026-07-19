package fs_test

import (
	"context"
	"encoding/json"
	"motor-autonomo/internal/tool/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFS(t *testing.T) {
	tempDir := t.TempDir()
	
	writeTool := fs.NewWriteFileTool(tempDir)
	readTool := fs.NewReadFileTool(tempDir)
	listTool := fs.NewListDirTool(tempDir)
	
	// Test Write
	_, err := writeTool.Execute(context.Background(), json.RawMessage(`{"path":"test.txt","content":"hello world"}`))
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	
	// Test Read
	res, err := readTool.Execute(context.Background(), json.RawMessage(`{"path":"test.txt"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if res != "hello world" {
		t.Fatalf("expected 'hello world', got %q", res)
	}
	
	// Test List
	res, err = listTool.Execute(context.Background(), json.RawMessage(`{"path":"."}`))
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if !strings.Contains(res, "test.txt") {
		t.Fatalf("expected list output to contain test.txt, got %q", res)
	}
	
	// Test Directory Traversal Prevention
	_, err = readTool.Execute(context.Background(), json.RawMessage(`{"path":"../outside.txt"}`))
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected traversal error, got %v", err)
	}
	
	_, err = writeTool.Execute(context.Background(), json.RawMessage(`{"path":"/etc/passwd","content":"test"}`))
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
}
