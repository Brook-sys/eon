package domain

import (
	"errors"
	"fmt"
	"time"
)

// ProcessMode is process-scoped control authority. It is independent from any
// single mission revision and never grants a model capability.
type ProcessMode string

const (
	ProcessRunning  ProcessMode = "RUNNING"
	ProcessStopping ProcessMode = "STOPPING"
	ProcessStopped  ProcessMode = "STOPPED"
)

func (m ProcessMode) valid() bool {
	switch m {
	case ProcessRunning, ProcessStopping, ProcessStopped:
		return true
	default:
		return false
	}
}

// MissionDispatchMode controls admission of new dispatches for one mission.
// In-flight work is not killed by pause; cancellation remains cooperative.
type MissionDispatchMode string

const (
	MissionDispatchEnabled   MissionDispatchMode = "ENABLED"
	MissionDispatchPaused    MissionDispatchMode = "PAUSED"
	MissionDispatchCancelled MissionDispatchMode = "CANCELLED"
)

func (m MissionDispatchMode) valid() bool {
	switch m {
	case MissionDispatchEnabled, MissionDispatchPaused, MissionDispatchCancelled:
		return true
	default:
		return false
	}
}

// MissionControl is the durable control projection for one mission.
type MissionControl struct {
	MissionID        MissionID           `json:"mission_id"`
	Mode             MissionDispatchMode `json:"mode"`
	RevisionAtChange uint64              `json:"revision_at_change"`
	Reason           string              `json:"reason,omitempty"`
	LastCommandID    CommandID           `json:"last_command_id,omitempty"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

func (m MissionControl) Validate() error {
	if m.MissionID == "" || m.RevisionAtChange == 0 || m.UpdatedAt.IsZero() || !m.Mode.valid() {
		return errors.New("mission control is incomplete")
	}
	if len(m.Reason) > MaxControlPayloadBytes {
		return errors.New("mission control reason exceeds byte limit")
	}
	return nil
}

// ControlState is the authoritative process/mission control projection used by
// the scheduler and operator commands. It is mutated only by the kernel.
type ControlState struct {
	SchemaVersion     int                              `json:"schema_version"`
	Revision          uint64                           `json:"revision"`
	ProcessMode       ProcessMode                      `json:"process_mode"`
	ShutdownCommandID CommandID                        `json:"shutdown_command_id,omitempty"`
	Missions          map[MissionID]MissionControl     `json:"missions,omitempty"`
	UpdatedAt         time.Time                        `json:"updated_at"`
}

func DefaultControlState(now time.Time) ControlState {
	return ControlState{
		SchemaVersion: SchemaVersionV1,
		Revision:      0,
		ProcessMode:   ProcessRunning,
		Missions:      map[MissionID]MissionControl{},
		UpdatedAt:     now.UTC(),
	}
}

func (s ControlState) Validate() error {
	if s.SchemaVersion != SchemaVersionV1 || s.UpdatedAt.IsZero() || !s.ProcessMode.valid() {
		return errors.New("control state is incomplete or has unsupported schema version")
	}
	if s.ProcessMode == ProcessRunning && s.ShutdownCommandID != "" {
		return errors.New("running process must not retain a shutdown command")
	}
	if s.ProcessMode != ProcessRunning && s.ShutdownCommandID == "" {
		return errors.New("stopping or stopped process requires shutdown command reference")
	}
	for id, mission := range s.Missions {
		if id != mission.MissionID {
			return fmt.Errorf("mission control map key %s disagrees with payload %s", id, mission.MissionID)
		}
		if err := mission.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// AllowsDispatch reports whether the scheduler may select new READY work.
// Resume of local waits may still occur when process is running and the mission
// is not cancelled; pause only blocks new dispatch after selection checks.
func (s ControlState) AllowsDispatch(missionID MissionID) bool {
	if s.ProcessMode != ProcessRunning {
		return false
	}
	mission, ok := s.Missions[missionID]
	if !ok {
		return true
	}
	return mission.Mode == MissionDispatchEnabled
}

// ApplyOperatorCommand is a pure control transition. Callers must still enforce
// optimistic concurrency against the active mission revision number.
func ApplyOperatorCommand(current ControlState, command OperatorCommand, active MissionRevision, now time.Time) (ControlState, string, error) {
	if err := current.Validate(); err != nil {
		return ControlState{}, "", fmt.Errorf("validate control state: %w", err)
	}
	if err := command.Validate(); err != nil {
		return ControlState{}, "", fmt.Errorf("validate operator command: %w", err)
	}
	if now.IsZero() {
		return ControlState{}, "", errors.New("control transition requires occurrence time")
	}
	now = now.UTC()
	next := cloneControlState(current)
	if next.Missions == nil {
		next.Missions = map[MissionID]MissionControl{}
	}

	switch command.Kind {
	case CommandGracefulShutdown:
		if next.ProcessMode == ProcessStopping || next.ProcessMode == ProcessStopped {
			if next.ShutdownCommandID == command.ID {
				return next, "process:stopping", nil
			}
			return ControlState{}, "", fmt.Errorf("%w: process already shutting down", ErrConflict)
		}
		next.ProcessMode = ProcessStopping
		next.ShutdownCommandID = command.ID
		next.Revision++
		next.UpdatedAt = now
		return next, "process:stopping", nil

	case CommandPauseMission, CommandResumeMission, CommandCancelMission:
		if active.MissionID != command.Target.MissionID {
			return ControlState{}, "", fmt.Errorf("%w: command target is not the active mission", ErrConflict)
		}
		if command.ExpectedRevision == nil || active.Revision != *command.ExpectedRevision {
			return ControlState{}, "", fmt.Errorf("%w: stale mission revision", ErrConflict)
		}
		if next.ProcessMode != ProcessRunning {
			return ControlState{}, "", fmt.Errorf("%w: mission control rejected while process is not running", ErrConflict)
		}
		existing, ok := next.Missions[command.Target.MissionID]
		switch command.Kind {
		case CommandPauseMission:
			if ok && existing.Mode == MissionDispatchPaused && existing.RevisionAtChange == active.Revision {
				return next, resultRefMission(command.Target.MissionID, active.Revision, MissionDispatchPaused), nil
			}
			if ok && existing.Mode == MissionDispatchCancelled {
				return ControlState{}, "", fmt.Errorf("%w: cancelled mission cannot be paused", ErrConflict)
			}
			next.Missions[command.Target.MissionID] = MissionControl{
				MissionID: command.Target.MissionID, Mode: MissionDispatchPaused, RevisionAtChange: active.Revision,
				Reason: command.Reason, LastCommandID: command.ID, UpdatedAt: now,
			}
			next.Revision++
			next.UpdatedAt = now
			return next, resultRefMission(command.Target.MissionID, active.Revision, MissionDispatchPaused), nil
		case CommandResumeMission:
			if !ok || existing.Mode != MissionDispatchPaused {
				return ControlState{}, "", fmt.Errorf("%w: only a paused mission can be resumed", ErrConflict)
			}
			if existing.RevisionAtChange != active.Revision {
				return ControlState{}, "", fmt.Errorf("%w: stale paused mission control", ErrConflict)
			}
			next.Missions[command.Target.MissionID] = MissionControl{
				MissionID: command.Target.MissionID, Mode: MissionDispatchEnabled, RevisionAtChange: active.Revision,
				Reason: command.Reason, LastCommandID: command.ID, UpdatedAt: now,
			}
			next.Revision++
			next.UpdatedAt = now
			return next, resultRefMission(command.Target.MissionID, active.Revision, MissionDispatchEnabled), nil
		case CommandCancelMission:
			if ok && existing.Mode == MissionDispatchCancelled && existing.RevisionAtChange == active.Revision {
				return next, resultRefMission(command.Target.MissionID, active.Revision, MissionDispatchCancelled), nil
			}
			next.Missions[command.Target.MissionID] = MissionControl{
				MissionID: command.Target.MissionID, Mode: MissionDispatchCancelled, RevisionAtChange: active.Revision,
				Reason: command.Reason, LastCommandID: command.ID, UpdatedAt: now,
			}
			next.Revision++
			next.UpdatedAt = now
			return next, resultRefMission(command.Target.MissionID, active.Revision, MissionDispatchCancelled), nil
		}
	}
	return ControlState{}, "", fmt.Errorf("unsupported operator command kind %q", command.Kind)
}

func resultRefMission(missionID MissionID, revision uint64, mode MissionDispatchMode) string {
	return fmt.Sprintf("%s@%d:%s", missionID, revision, mode)
}

func cloneControlState(src ControlState) ControlState {
	dst := src
	if src.Missions != nil {
		dst.Missions = make(map[MissionID]MissionControl, len(src.Missions))
		for id, mission := range src.Missions {
			dst.Missions[id] = mission
		}
	}
	return dst
}

// AdvanceCommandReceipt enforces monotonic command lifecycle transitions.
func AdvanceCommandReceipt(current CommandReceipt, next CommandReceipt) error {
	if err := current.Validate(); err != nil {
		return fmt.Errorf("validate current receipt: %w", err)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate next receipt: %w", err)
	}
	if current.CommandID != next.CommandID || current.ID != next.ID {
		return errors.New("command receipt identity changed")
	}
	if next.RecordedAt.Before(current.RecordedAt) {
		return errors.New("command receipt time moved backwards")
	}
	if current.State == next.State {
		if current == next {
			return nil
		}
		return fmt.Errorf("%w: command receipt changed without state advance", ErrConflict)
	}
	if current.State.Terminal() {
		return fmt.Errorf("%w: terminal command receipt cannot advance", ErrConflict)
	}
	allowed := map[CommandState][]CommandState{
		CommandReceived:    {CommandValidating, CommandRejected},
		CommandValidating:  {CommandAccepted, CommandRejected},
		CommandAccepted:    {CommandApplying, CommandRejected},
		CommandApplying:    {CommandApplied, CommandReconciling, CommandFailed},
		CommandReconciling: {CommandApplied, CommandFailed},
	}
	for _, state := range allowed[current.State] {
		if state == next.State {
			return nil
		}
	}
	return fmt.Errorf("%w: illegal command receipt transition %s → %s", ErrConflict, current.State, next.State)
}

// ErrConflict is used by pure control helpers so call sites can map to storage.
var ErrConflict = errors.New("control conflict")
