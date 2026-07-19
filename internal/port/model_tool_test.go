package port_test

import (
	"context"
	"motor-autonomo/internal/port"
	"testing"
)

type mockModelToolProvider struct{}

func (m mockModelToolProvider) Complete(context.Context, port.CompletionRequest) (port.CompletionResult, error) {
	return port.CompletionResult{}, nil
}

func (m mockModelToolProvider) CompleteWithTools(context.Context, port.CompletionRequest, []port.ToolDefinition) (port.CompletionResult, error) {
	return port.CompletionResult{}, nil
}

func TestModelToolProviderInterfaceSatisfied(t *testing.T) {
	var _ port.ModelToolProvider = mockModelToolProvider{}
}
