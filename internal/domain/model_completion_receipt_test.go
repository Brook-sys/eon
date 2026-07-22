package domain

import (
	"testing"
	"time"
)

func TestModelCompletionResultHashIsDeterministicAndComplete(t *testing.T) {
	result := ModelCompletionResult{
		Text: "answer", ToolCalls: []ModelCompletionToolCall{{ID: "call", Name: "lookup", Arguments: `{"x":1}`}},
		InputTokens: 5, OutputTokens: 2, Model: "model", FinishReason: "tool_calls",
	}
	first, err := result.Hash()
	if err != nil {
		t.Fatal(err)
	}
	second, err := result.Hash()
	if err != nil || second != first {
		t.Fatalf("second hash = %q err=%v, want %q", second, err, first)
	}
	changed := result
	changed.ToolCalls = append([]ModelCompletionToolCall(nil), result.ToolCalls...)
	changed.ToolCalls[0].Arguments = `{"x":2}`
	other, err := changed.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if other == first {
		t.Fatal("changing a nested completion field did not change the hash")
	}
}

func TestModelCompletionReceiptValidation(t *testing.T) {
	result := ModelCompletionResult{Text: "answer", FinishReason: ""}
	hash, err := result.Hash()
	if err != nil {
		t.Fatal(err)
	}
	receipt := ModelCompletionReceipt{
		SchemaVersion: SchemaVersionV1, OperationID: "op", Attempt: 0, ModelCall: 1,
		Result: result, PayloadHash: hash, RecordedAt: time.Date(2026, 7, 22, 19, 0, 0, 0, time.UTC),
	}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("valid receipt: %v", err)
	}
	receipt.PayloadHash = "sha256:wrong"
	if err := receipt.Validate(); err == nil {
		t.Fatal("receipt accepted a mismatched payload hash")
	}
}
