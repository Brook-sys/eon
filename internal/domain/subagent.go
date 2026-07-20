package domain

import (
	"errors"
	"time"
)

type SubagentState string

const (
	SubagentStatePending  SubagentState = "PENDING"
	SubagentStateRunning  SubagentState = "RUNNING"
	SubagentStateComplete SubagentState = "COMPLETE"
	SubagentStateError    SubagentState = "ERROR"
)

// SubagentRecord represents a persisted subagent session dispatched by the continuity strategy.
type SubagentRecord struct {
	SchemaVersion int           `json:"schema_version"`
	ID            string        `json:"id"`
	TaskID        string        `json:"task_id"`
	MissionID     string        `json:"mission_id"`
	State         SubagentState `json:"state"`
	StartedAt     time.Time     `json:"started_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Task          string        `json:"task"`
	ContextMode   string        `json:"context_mode"`
	Result        string        `json:"result,omitempty"`
	ErrorCode     string        `json:"error_code,omitempty"`
}

func (r SubagentRecord) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 {
		return errors.New("invalid subagent schema version")
	}
	if r.ID == "" || r.TaskID == "" || r.MissionID == "" {
		return errors.New("subagent identity fields cannot be empty")
	}
	if r.Task == "" {
		return errors.New("subagent task cannot be empty")
	}
	if r.State != SubagentStatePending && r.State != SubagentStateRunning && r.State != SubagentStateComplete && r.State != SubagentStateError {
		return errors.New("invalid subagent state")
	}
	if r.StartedAt.IsZero() || r.UpdatedAt.IsZero() {
		return errors.New("subagent timestamps cannot be zero")
	}
	if r.UpdatedAt.Before(r.StartedAt) {
		return errors.New("subagent updated_at cannot precede started_at")
	}
	return nil
}
