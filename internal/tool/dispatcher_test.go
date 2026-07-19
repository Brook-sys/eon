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
		{ID: "3", Name: "valid_tool", Arguments: `{"arg": malformed}`},
		{ID: "4", Name: "valid_tool", Arguments: `{"arg": "fail_validation"}`},
	}

	results := dispatcher.Dispatch(context.Background(), calls)
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	if results[0].CallID != "1" || results[0].Error != nil || results[0].Result != "ok" {
		t.Errorf("expected success for valid_tool, got %+v", results[0])
	}

	if results[1].CallID != "2" || results[1].Error == nil {
		t.Errorf("expected error for missing_tool, got %+v", results[1])
	} else {
		var dispatchErr tool.DispatchError
		if errors.As(results[1].Error, &dispatchErr) {
			if dispatchErr.FallbackPrompt == "" {
				t.Errorf("expected missing_tool error to provide a fallback prompt")
			}
		} else {
			t.Errorf("expected missing_tool error to be of type tool.DispatchError, got %T: %v", results[1].Error, results[1].Error)
		}
	}

	if results[2].CallID != "3" || results[2].Error == nil {
		t.Errorf("expected error for malformed json, got %+v", results[2])
	} else {
		var dispatchErr tool.DispatchError
		if errors.As(results[2].Error, &dispatchErr) {
			if dispatchErr.FallbackPrompt == "" {
				t.Errorf("expected malformed json error to provide a fallback prompt")
			}
		} else {
			t.Errorf("expected malformed json error to be of type tool.DispatchError, got %T: %v", results[2].Error, results[2].Error)
		}
	}

	if results[3].CallID != "4" || results[3].Error == nil {
		t.Errorf("expected error for fail_validation, got %+v", results[3])
	} else {
		var dispatchErr tool.DispatchError
		if errors.As(results[3].Error, &dispatchErr) {
			if dispatchErr.FallbackPrompt == "" {
				t.Errorf("expected fail_validation error to provide a fallback prompt")
			}
		} else {
			t.Errorf("expected fail_validation error to be of type tool.DispatchError, got %T: %v", results[3].Error, results[3].Error)
		}
	}
}
