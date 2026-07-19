package tool_test

import (
	"context"
	"errors"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/tool"
)

func TestDispatcher_RoutesCorrectlyAndHandlesErrors(t *testing.T) {
	catalog, _ := tool.NewCatalog(fixture("valid_tool"))
	dispatcher := tool.NewDispatcher(catalog)

	calls := []port.ToolCall{
		{ID: "1", Name: "valid_tool", Arguments: `{"arg":"1"}`},
		{ID: "2", Name: "missing_tool", Arguments: `{}`},
	}

	results := dispatcher.Dispatch(context.Background(), calls)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].CallID != "1" || results[0].Error != nil || results[0].Result != "ok" {
		t.Errorf("expected success for valid_tool, got %+v", results[0])
	}
	
	if results[1].CallID != "2" || results[1].Error == nil || results[1].Error.Error() != "tool not found" {
		t.Errorf("expected error for missing_tool, got %+v", results[1])
	}
}
