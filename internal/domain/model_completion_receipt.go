package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ModelCompletionReceipt is the durable, append-only record of one successful
// provider completion. OperationID, Attempt, and ModelCall form its natural key.
type ModelCompletionReceipt struct {
	SchemaVersion int                   `json:"schema_version"`
	OperationID   OperationID           `json:"operation_id"`
	Attempt       uint32                `json:"attempt"`
	ModelCall     uint32                `json:"model_call"`
	Result        ModelCompletionResult `json:"result"`
	PayloadHash   string                `json:"payload_hash"`
	RecordedAt    time.Time             `json:"recorded_at"`
}

// ModelCompletionResult mirrors the provider-neutral completion value without
// importing port from domain. Keep it lossless when converting at the boundary.
type ModelCompletionResult struct {
	Text         string                    `json:"text,omitempty"`
	ToolCalls    []ModelCompletionToolCall `json:"tool_calls,omitempty"`
	InputTokens  int                       `json:"input_tokens"`
	OutputTokens int                       `json:"output_tokens"`
	Model        string                    `json:"model"`
	FinishReason string                    `json:"finish_reason"`
}

type ModelCompletionToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

func (r ModelCompletionReceipt) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("model completion receipt schema version must be %d", SchemaVersionV1)
	}
	if strings.TrimSpace(string(r.OperationID)) == "" {
		return errors.New("model completion receipt operation id is required")
	}
	if r.ModelCall == 0 {
		return errors.New("model completion receipt model call must be positive")
	}
	if r.RecordedAt.IsZero() {
		return errors.New("model completion receipt recorded time is required")
	}
	if err := r.Result.Validate(); err != nil {
		return fmt.Errorf("model completion receipt result: %w", err)
	}
	hash, err := r.Result.Hash()
	if err != nil {
		return err
	}
	if r.PayloadHash != hash {
		return errors.New("model completion receipt payload hash mismatch")
	}
	return nil
}

func (r ModelCompletionResult) Validate() error {
	if r.InputTokens < 0 || r.OutputTokens < 0 {
		return errors.New("token counts must not be negative")
	}
	for i, call := range r.ToolCalls {
		if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
			return fmt.Errorf("tool call %d id and name are required", i)
		}
	}
	switch r.FinishReason {
	case "", "unknown", "stop", "length", "tool_calls", "content_filter", "other":
	default:
		return fmt.Errorf("unsupported finish reason %q", r.FinishReason)
	}
	return nil
}

// Hash returns a deterministic digest of the complete provider-neutral result.
func (r ModelCompletionResult) Hash() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal model completion result: %w", err)
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
