package subagent

import (
	"context"
	"encoding/json"
	"fmt"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

func RemoteToolDefinition() port.ToolDefinition {
	return port.ToolDefinition{
		Name:        "sessions_spawn_remote",
		Description: "Delegates an operation to a remote authorized peer.",
		Parameters: []byte(`{
			"type": "object",
			"properties": {
				"peer_id": {
					"type": "string",
					"description": "The unique ID of the target peer."
				},
				"capability": {
					"type": "string",
					"description": "The name of the capability to invoke (e.g., 'exec', 'search_memory')."
				},
				"payload": {
					"type": "object",
					"description": "JSON payload specific to the requested capability."
				}
			},
			"required": ["peer_id", "capability", "payload"]
		}`),
	}
}

// RemoteTool implements tool.Tool for remote delegation.
type RemoteTool struct {
	Delegator *SubagentDelegator
	CallerID  string // Derived from current agent's identity
	IDGen     source.IDGenerator
}

func (t *RemoteTool) Definition() port.ToolDefinition {
	return RemoteToolDefinition()
}

func (t *RemoteTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		PeerID     string          `json:"peer_id"`
		Capability string          `json:"capability"`
		Payload    json.RawMessage `json:"payload"`
	}

	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.PeerID == "" || args.Capability == "" || len(args.Payload) == 0 {
		return "", fmt.Errorf("missing required fields in input")
	}

	requestID, err := t.IDGen.NewID("req")
	if err != nil {
		return "", fmt.Errorf("failed to generate request id: %w", err)
	}

	respPayload, err := t.Delegator.Delegate(ctx, requestID, args.PeerID, t.CallerID, args.Capability, args.Payload)
	if err != nil {
		return "", fmt.Errorf("delegation failed: %w", err)
	}

	return string(respPayload), nil
}
