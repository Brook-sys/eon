package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOperatorCommandValidation(t *testing.T) {
	now := time.Date(2026, 7, 16, 0, 20, 0, 0, time.UTC)
	revision := uint64(12)
	valid := OperatorCommand{
		SchemaVersion: SchemaVersionV1, ID: "cmd_1", IdempotencyKey: "idem_1",
		ActorType: ActorOperator, ActorID: "operator_1", Kind: CommandPauseMission,
		Target: CommandTarget{MissionID: "mission_1"}, ExpectedRevision: &revision,
		Reason: "maintenance", SubmittedAt: now,
	}
	tests := []struct {
		name      string
		edit      func(*OperatorCommand)
		wantError bool
	}{
		{name: "valid mission command"},
		{name: "unknown kind", edit: func(c *OperatorCommand) { c.Kind = "EXECUTE_TEXT" }, wantError: true},
		{name: "missing optimistic revision", edit: func(c *OperatorCommand) { c.ExpectedRevision = nil }, wantError: true},
		{name: "process command rejects mission scope", edit: func(c *OperatorCommand) { c.Kind = CommandGracefulShutdown }, wantError: true},
		{name: "valid process command", edit: func(c *OperatorCommand) {
			c.Kind = CommandGracefulShutdown
			c.Target = CommandTarget{}
			c.ExpectedRevision = nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := valid
			if test.edit != nil {
				test.edit(&command)
			}
			err := command.Validate()
			if (err != nil) != test.wantError {
				t.Fatalf("Validate() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestCommandReceiptDistinguishesAcceptanceFromEffect(t *testing.T) {
	now := time.Date(2026, 7, 16, 0, 20, 0, 0, time.UTC)
	base := CommandReceipt{SchemaVersion: SchemaVersionV1, ID: "receipt_1", CommandID: "cmd_1", RecordedAt: now}
	accepted := base
	accepted.State = CommandAccepted
	if err := accepted.Validate(); err != nil {
		t.Fatalf("accepted receipt rejected: %v", err)
	}
	accepted.ResultRef = "mission_1@13"
	if err := accepted.Validate(); err == nil {
		t.Fatal("accepted receipt claimed an applied result")
	}
	applied := base
	applied.State = CommandApplied
	if err := applied.Validate(); err == nil {
		t.Fatal("applied receipt without result was accepted")
	}
	applied.ResultRef = "mission_1@13"
	if err := applied.Validate(); err != nil {
		t.Fatalf("applied receipt rejected: %v", err)
	}
}

func TestExternalEventValidationKeepsContentBoundedAndCorrelated(t *testing.T) {
	now := time.Date(2026, 7, 16, 0, 20, 0, 0, time.UTC)
	valid := ExternalEvent{
		SchemaVersion: SchemaVersionV1, ID: "ext_1", DeduplicationKey: "telegram:update:1",
		Source: "telegram", SourceActorID: "user_1", Kind: ExternalUserAnswer,
		MissionID: "mission_1", CorrelationID: "ask_1", TransportMessageID: "message_1",
		Content:    ExternalContent{MediaType: "application/json", Structured: json.RawMessage(`{"option_id":"minimal"}`)},
		ReceivedAt: now,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid external event rejected: %v", err)
	}
	missingCorrelation := valid
	missingCorrelation.CorrelationID = ""
	if err := missingCorrelation.Validate(); err == nil {
		t.Fatal("user answer without question correlation was accepted")
	}
	ambiguous := valid
	ambiguous.Content.Text = "also text"
	if err := ambiguous.Validate(); err == nil {
		t.Fatal("content with two representations was accepted")
	}
	oversized := valid
	oversized.Content = ExternalContent{MediaType: "text/plain", Text: strings.Repeat("x", MaxControlPayloadBytes+1)}
	if err := oversized.Validate(); err == nil {
		t.Fatal("oversized external content was accepted")
	}
}
