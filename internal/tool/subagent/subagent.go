package subagent

import (
	"context"
	"encoding/json"
	"errors"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
)

// SessionsSpawnTool delegates work to an isolated child session.
type SessionsSpawnTool struct {
	manager       kernel.SessionManager
	trustedLabels map[string]string
}

func NewSessionsSpawnTool(manager kernel.SessionManager) *SessionsSpawnTool {
	return &SessionsSpawnTool{manager: manager}
}

// NewSessionsSpawnToolWithTrustedLabels binds deployment-authorized routing
// metadata outside model-controlled arguments. Callers cannot override these
// labels through the tool schema.
func NewSessionsSpawnToolWithTrustedLabels(manager kernel.SessionManager, labels map[string]string) *SessionsSpawnTool {
	copy := make(map[string]string, len(labels))
	for key, value := range labels {
		copy[key] = value
	}
	return &SessionsSpawnTool{manager: manager, trustedLabels: copy}
}

func (t *SessionsSpawnTool) Definition() port.ToolDefinition {
	return port.ToolDefinition{
		Name:        "sessions_spawn",
		Description: "Spawn a clean child session/subagent. Use context=\"fork\" only when child needs current transcript.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"task":{"type":"string"},"context":{"type":"string"}},"required":["task"]}`),
	}
}

func (t *SessionsSpawnTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if t.manager == nil {
		return "", errors.New("subagent feature is disabled (manager is nil)")
	}

	var req struct {
		Task        string `json:"task"`
		ContextMode string `json:"context,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", err
	}

	if req.Task == "" {
		return "", errors.New("task is required")
	}
	if req.ContextMode == "" {
		req.ContextMode = "isolated"
	}

	spec := kernel.SubagentSpec{
		Task:        req.Task,
		ContextMode: req.ContextMode,
		Labels:      t.trustedLabels,
	}

	id, err := t.manager.Spawn(ctx, spec)
	if err != nil {
		return "", err
	}

	resp := map[string]string{
		"session_id": string(id),
		"status":     "PENDING",
		"message":    "Subagent spawned successfully.",
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

func (t *SessionsSpawnTool) Capability() *domain.CapabilityDescriptor {
	return &domain.CapabilityDescriptor{
		SchemaVersion:       1,
		Name:                "sessions_spawn",
		Version:             1,
		InputSchema:         `{"type":"object","properties":{"task":{"type":"string"},"context":{"type":"string"}},"required":["task"]}`,
		OutputSchema:        `{"type":"object","properties":{"session_id":{"type":"string"},"status":{"type":"string"},"message":{"type":"string"}}}`,
		SideEffects:         []domain.SideEffectClass{domain.SideEffectWriteLocal},
		Risk:                domain.RiskMedium,
		RequiredPermissions: []string{"subagent:spawn"},
	}
}
