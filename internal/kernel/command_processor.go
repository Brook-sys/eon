package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

const (
	EventOperatorCommandReceived = "operator.command.received"
	EventOperatorCommandRejected = "operator.command.rejected"
	EventOperatorCommandApplied  = "operator.command.applied"
	EventProcessStopping         = "process.stopping"
	EventMissionPaused           = "mission.paused"
	EventMissionResumed          = "mission.resumed"
	EventMissionCancelled        = "mission.cancelled"
)

// CommandProcessor applies allowlisted operator commands through durable store
// transactions. Transports only submit; this kernel component owns effects.
type CommandProcessor struct {
	Store port.Store
	Clock source.Clock
	IDs   source.IDGenerator
}

func NewCommandProcessor(store port.Store, clock source.Clock, ids source.IDGenerator) (*CommandProcessor, error) {
	if store == nil || clock == nil || ids == nil {
		return nil, errors.New("command processor requires store, clock, and ID generator")
	}
	return &CommandProcessor{Store: store, Clock: clock, IDs: ids}, nil
}

// ProcessNext applies at most one pending command. Returning false means the
// inbox is empty at the observed snapshot.
func (p *CommandProcessor) ProcessNext(ctx context.Context) (domain.CommandReceipt, bool, error) {
	var commandID domain.CommandID
	err := p.Store.View(ctx, func(r port.Reader) error {
		pending, err := r.PendingOperatorCommands(1)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
		commandID = pending[0].ID
		return nil
	})
	if err != nil {
		return domain.CommandReceipt{}, false, err
	}
	if commandID == "" {
		return domain.CommandReceipt{}, false, nil
	}
	receipt, err := p.Process(ctx, commandID)
	return receipt, true, err
}

// Process applies one command idempotently. Terminal receipts are replayed.
func (p *CommandProcessor) Process(ctx context.Context, commandID domain.CommandID) (domain.CommandReceipt, error) {
	if commandID == "" {
		return domain.CommandReceipt{}, errors.New("command ID is required")
	}
	var final domain.CommandReceipt
	err := p.Store.Update(ctx, func(tx port.Transaction) error {
		command, err := tx.OperatorCommand(commandID)
		if err != nil {
			return err
		}
		receipt, err := tx.OperatorCommandReceipt(commandID)
		if err != nil {
			return err
		}
		if receipt.State.Terminal() {
			final = receipt
			return nil
		}
		now := p.Clock.Now().UTC()
		receipt, err = p.advance(tx, receipt, domain.CommandValidating, now, "", "")
		if err != nil {
			return err
		}

		control, err := ensureControlState(tx, now)
		if err != nil {
			return err
		}
		active, err := loadActiveMission(tx, command)
		if err != nil {
			if rejectErr := p.reject(tx, &receipt, now, failureCode(err), err); rejectErr != nil {
				return rejectErr
			}
			final = receipt
			return nil
		}
		next, resultRef, applyErr := domain.ApplyOperatorCommand(control, command, active, now)
		if applyErr != nil {
			if rejectErr := p.reject(tx, &receipt, now, failureCode(applyErr), applyErr); rejectErr != nil {
				return rejectErr
			}
			final = receipt
			return nil
		}
		receipt, err = p.advance(tx, receipt, domain.CommandAccepted, now.Add(time.Nanosecond), "", "")
		if err != nil {
			return err
		}
		receipt, err = p.advance(tx, receipt, domain.CommandApplying, now.Add(2*time.Nanosecond), "", "")
		if err != nil {
			return err
		}
		if next.Revision != control.Revision {
			if err := tx.SaveControlState(next, control.Revision); err != nil {
				return err
			}
		}
		if err := p.appendEffectEvents(tx, command, next, resultRef, now); err != nil {
			return err
		}
		receipt, err = p.advance(tx, receipt, domain.CommandApplied, now.Add(3*time.Nanosecond), resultRef, "")
		if err != nil {
			return err
		}
		final = receipt
		return nil
	})
	if err != nil {
		return domain.CommandReceipt{}, err
	}
	return final, nil
}

func (p *CommandProcessor) reject(tx port.Transaction, receipt *domain.CommandReceipt, now time.Time, code string, cause error) error {
	next, err := p.advance(tx, *receipt, domain.CommandRejected, now.Add(time.Nanosecond), "", code)
	if err != nil {
		return err
	}
	*receipt = next
	eventID, err := p.IDs.NewID("event")
	if err != nil {
		return err
	}
	if _, err := tx.AppendEvent(domain.Event{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            domain.EventID(eventID),
		Kind:          EventOperatorCommandRejected,
		OccurredAt:    now,
		PayloadRef:    string(receipt.CommandID) + ":" + code,
	}); err != nil {
		return err
	}
	// Rejection is a successful processing outcome for the inbox item.
	return nil
}

func (p *CommandProcessor) advance(tx port.Transaction, current domain.CommandReceipt, state domain.CommandState, at time.Time, resultRef, failureCode string) (domain.CommandReceipt, error) {
	next := current
	next.State = state
	next.RecordedAt = at.UTC()
	next.ResultRef = resultRef
	next.FailureCode = failureCode
	if err := tx.SaveOperatorCommandReceipt(next); err != nil {
		return domain.CommandReceipt{}, err
	}
	return next, nil
}

func (p *CommandProcessor) appendEffectEvents(tx port.Transaction, command domain.OperatorCommand, control domain.ControlState, resultRef string, now time.Time) error {
	kind := EventOperatorCommandApplied
	switch command.Kind {
	case domain.CommandGracefulShutdown:
		kind = EventProcessStopping
	case domain.CommandPauseMission:
		kind = EventMissionPaused
	case domain.CommandResumeMission:
		kind = EventMissionResumed
	case domain.CommandCancelMission:
		kind = EventMissionCancelled
	}
	eventID, err := p.IDs.NewID("event")
	if err != nil {
		return err
	}
	event := domain.Event{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            domain.EventID(eventID),
		Kind:          kind,
		OccurredAt:    now,
		PayloadRef:    resultRef,
	}
	if command.Target.MissionID != "" {
		if active, err := tx.ActiveMissionRevision(command.Target.MissionID); err == nil {
			event.MissionRevision = active.ID
		}
	}
	if _, err := tx.AppendEvent(event); err != nil {
		return err
	}
	if kind != EventOperatorCommandApplied {
		// Keep a generic audit event when a specialized kind is used.
		auditID, err := p.IDs.NewID("event")
		if err != nil {
			return err
		}
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(auditID),
			Kind:            EventOperatorCommandApplied,
			OccurredAt:      now,
			MissionRevision: event.MissionRevision,
			PayloadRef:      string(command.ID) + ":" + resultRef,
		}); err != nil {
			return err
		}
	}
	_ = control
	return nil
}

func ensureControlState(tx port.Transaction, now time.Time) (domain.ControlState, error) {
	state, err := tx.ControlState()
	if err == nil {
		return state, nil
	}
	if !errors.Is(err, port.ErrNotFound) {
		return domain.ControlState{}, err
	}
	initial := domain.DefaultControlState(now)
	if err := tx.SaveControlState(initial, 0); err != nil {
		return domain.ControlState{}, err
	}
	return initial, nil
}

func loadActiveMission(tx port.Transaction, command domain.OperatorCommand) (domain.MissionRevision, error) {
	switch command.Kind {
	case domain.CommandGracefulShutdown:
		return domain.MissionRevision{}, nil
	case domain.CommandPauseMission, domain.CommandResumeMission, domain.CommandCancelMission:
		return tx.ActiveMissionRevision(command.Target.MissionID)
	default:
		return domain.MissionRevision{}, fmt.Errorf("unsupported command kind %q", command.Kind)
	}
}

func failureCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrConflict), errors.Is(err, port.ErrConflict):
		return "CONTROL_CONFLICT"
	case errors.Is(err, port.ErrNotFound):
		return "TARGET_NOT_FOUND"
	default:
		return "COMMAND_INVALID"
	}
}
