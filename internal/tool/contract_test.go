package tool_test

import (
	"context"
	"encoding/json"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/tool"
)

type fakeTool struct {
	definition port.ToolDefinition
}

func (f fakeTool) Definition() port.ToolDefinition { return f.definition }
func (f fakeTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "ok", nil
}

func fixture(name string) fakeTool {
	return fakeTool{definition: port.ToolDefinition{
		Name: name, Description: "fixture " + name,
		Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}}
}

func TestCatalogIsValidatedOrderedAndDefensivelyCopied(t *testing.T) {
	catalog, err := tool.NewCatalog(fixture("web_search"), fixture("read_file"))
	if err != nil {
		t.Fatal(err)
	}
	definitions := catalog.Definitions()
	if len(definitions) != 2 || definitions[0].Name != "read_file" || definitions[1].Name != "web_search" {
		t.Fatalf("definitions = %+v", definitions)
	}
	definitions[0].Parameters[0] = '['
	if got := catalog.Definitions()[0].Parameters[0]; got != '{' {
		t.Fatalf("catalog schema mutated through result: %q", got)
	}
	if found, ok := catalog.Find("read_file"); !ok || found.Definition().Name != "read_file" {
		t.Fatal("read_file not found")
	}
	if _, ok := catalog.Find("missing"); ok {
		t.Fatal("unexpected missing tool")
	}
}

func TestCatalogRejectsInvalidOrDuplicateDefinitions(t *testing.T) {
	tests := []struct {
		name  string
		tools []tool.Tool
	}{
		{"duplicate", []tool.Tool{fixture("read_file"), fixture("read_file")}},
		{"invalid name", []tool.Tool{fixture("read file")}},
		{"empty description", []tool.Tool{fakeTool{definition: port.ToolDefinition{Name: "read_file", Parameters: json.RawMessage(`{"type":"object"}`)}}}},
		{"invalid json", []tool.Tool{fakeTool{definition: port.ToolDefinition{Name: "read_file", Description: "read", Parameters: json.RawMessage(`{`)}}}},
		{"non-object schema", []tool.Tool{fakeTool{definition: port.ToolDefinition{Name: "read_file", Description: "read", Parameters: json.RawMessage(`{"type":"string"}`)}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := tool.NewCatalog(test.tools...); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
