package yield

import (
	"context"
	"encoding/json"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type SessionsYieldTool struct{}

func NewSessionsYieldTool() *SessionsYieldTool {
	return &SessionsYieldTool{}
}

func (t *SessionsYieldTool) Definition() port.ToolDefinition {
	return port.ToolDefinition{
		Name:        "sessions_yield",
		Description: "End current turn. Use after spawning subagents; results arrive as next message.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}}}`),
	}
}

func (t *SessionsYieldTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var req struct {
		Message string `json:"message,omitempty"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return "", err
		}
	}
	resp := map[string]string{
		"status":  "YIELDED",
		"message": "Turn yielded successfully. Awaiting external events or child completions.",
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

func (t *SessionsYieldTool) Capability() *domain.CapabilityDescriptor {
	return &domain.CapabilityDescriptor{
		SchemaVersion:       1,
		Name:                "sessions_yield",
		Version:             1,
		InputSchema:         `{"type":"object","properties":{"message":{"type":"string"}}}`,
		OutputSchema:        `{"type":"object","properties":{"status":{"type":"string"},"message":{"type":"string"}}}`,
		SideEffects:         []domain.SideEffectClass{domain.SideEffectWriteLocal},
		Risk:                domain.RiskLow,
		RequiredPermissions: []string{"subagent:yield"},
	}
}
