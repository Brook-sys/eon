package tool

import (
	"context"
	"encoding/json"
	"errors"
	"motor-autonomo/internal/port"
)

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
			results = append(results, DispatchResult{CallID: call.ID, Error: errors.New("tool not found")})
			continue
		}
	
		res, err := target.Execute(ctx, json.RawMessage(call.Arguments))
		results = append(results, DispatchResult{CallID: call.ID, Result: res, Error: err})
	}
	return results
}
