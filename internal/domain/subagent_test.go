package domain_test

import (
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func TestSubagentRecordValidation(t *testing.T) {
	now := time.Now().UTC()
	valid := domain.SubagentRecord{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "session-1",
		TaskID:        "task-1",
		MissionID:     "mission-1",
		State:         domain.SubagentStatePending,
		StartedAt:     now,
		UpdatedAt:     now,
		Task:          "do work",
		ContextMode:   "isolated",
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid record, got: %v", err)
	}

	invalid := valid
	invalid.ID = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected error on empty ID")
	}

	invalid = valid
	invalid.State = "INVALID"
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected error on invalid state")
	}

	invalid = valid
	invalid.UpdatedAt = valid.StartedAt.Add(-time.Second)
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected error when updated_at precedes started_at")
	}
}
