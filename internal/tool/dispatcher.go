package tool

import (
	"context"
	"encoding/json"
	"errors"
	"motor-autonomo/internal/port"
)

// FallbackPrompt must be used when sending the result back to the model as an instruction.
type DispatchError struct {
	Err            error
	FallbackPrompt string
}

func (e DispatchError) Error() string {
	return e.Err.Error()
}

func (e DispatchError) Unwrap() error {
	return e.Err
}

type DispatchResult struct {
	CallID string
	Result string
	Error  error
}

type Dispatcher struct {
	catalog Provider
}

func NewDispatcher(catalog Provider) *Dispatcher {
	return &Dispatcher{catalog: catalog}
}

func (d *Dispatcher) Dispatch(ctx context.Context, calls []port.ToolCall) []DispatchResult {
	if d == nil || d.catalog == nil {
		return nil
	}

	results := make([]DispatchResult, 0, len(calls))
	for _, call := range calls {
		target, ok := d.catalog.Find(call.Name)
		if !ok {
			err := DispatchError{
				Err:            errors.New("tool not found"),
				FallbackPrompt: "The tool requested ('" + call.Name + "') does not exist in the current catalog. Use only provided tools.",
			}
			results = append(results, DispatchResult{CallID: call.ID, Error: err})
			continue
		}

		// Pre-flight basic JSON validity check
		var schema map[string]interface{}
		if err := json.Unmarshal([]byte(call.Arguments), &schema); err != nil {
			dispatchErr := DispatchError{
				Err:            errors.New("invalid JSON arguments"),
				FallbackPrompt: "The tool call arguments were not valid JSON. Ensure arguments match the expected schema for '" + call.Name + "'.",
			}
			results = append(results, DispatchResult{CallID: call.ID, Error: dispatchErr})
			continue
		}
	
		res, err := target.Execute(ctx, json.RawMessage(call.Arguments))
		results = append(results, DispatchResult{CallID: call.ID, Result: res, Error: err})
	}
	return results
}
