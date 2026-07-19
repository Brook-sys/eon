package tool_test

import (
	"context"
	"encoding/json"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/tool"
	"strings"
	"testing"
)

type searchTool struct{}

func (searchTool) Definition() port.ToolDefinition {
	return port.ToolDefinition{
		Name:        "web_search",
		Description: "Search the web",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
	}
}

func (searchTool) Execute(ctx context.Context, payload json.RawMessage) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(payload, &args); err != nil {
		return "", err
	}
	return "Result for " + args.Query, nil
}

type readTool struct{}

func (readTool) Definition() port.ToolDefinition {
	return port.ToolDefinition{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
	}
}

func (readTool) Execute(ctx context.Context, payload json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(payload, &args); err != nil {
		return "", err
	}
	if strings.Contains(args.Path, "..") {
		return "", context.Canceled // simulate error
	}
	return "Content of " + args.Path, nil
}

func TestToolFixtures(t *testing.T) {
	catalog, err := tool.NewCatalog(searchTool{}, readTool{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(catalog.Definitions()) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(catalog.Definitions()))
	}

	search, ok := catalog.Find("web_search")
	if !ok {
		t.Fatal("web_search tool not found")
	}
	res, err := search.Execute(context.Background(), json.RawMessage(`{"query":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "Result for test" {
		t.Fatalf("unexpected result: %q", res)
	}

	read, ok := catalog.Find("read_file")
	if !ok {
		t.Fatal("read_file tool not found")
	}
	res, err = read.Execute(context.Background(), json.RawMessage(`{"path":"main.go"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "Content of main.go" {
		t.Fatalf("unexpected result: %q", res)
	}
}
