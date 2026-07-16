package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// CommandInbox persists operator commands through the store without granting
// transport adapters authority to mutate control state or mission records.
type CommandInbox struct {
	Store   port.Store
	Receipt ReceiptFactory
}

// ReceiptFactory builds the initial RECEIVED receipt for a command. Kernel
// processors later advance that receipt through its lifecycle.
type ReceiptFactory func(command domain.OperatorCommand) (domain.CommandReceipt, error)

func NewCommandInbox(store port.Store, receipts ReceiptFactory) (*CommandInbox, error) {
	if store == nil || receipts == nil {
		return nil, fmt.Errorf("command inbox requires store and receipt factory")
	}
	return &CommandInbox{Store: store, Receipt: receipts}, nil
}

func (i *CommandInbox) SubmitCommand(command domain.OperatorCommand) (domain.CommandReceipt, error) {
	if err := command.Validate(); err != nil {
		return domain.CommandReceipt{}, fmt.Errorf("validate operator command: %w", err)
	}
	receipt, err := i.Receipt(command)
	if err != nil {
		return domain.CommandReceipt{}, fmt.Errorf("build command receipt: %w", err)
	}
	if err := receipt.Validate(); err != nil {
		return domain.CommandReceipt{}, fmt.Errorf("validate command receipt: %w", err)
	}
	if receipt.CommandID != command.ID || receipt.State != domain.CommandReceived {
		return domain.CommandReceipt{}, fmt.Errorf("receipt factory must emit RECEIVED state for the command")
	}
	var stored domain.CommandReceipt
	err = i.Store.Update(context.Background(), func(tx port.Transaction) error {
		if existing, lookupErr := tx.OperatorCommand(command.ID); lookupErr == nil {
			if !equalOperatorCommands(existing, command) {
				return fmt.Errorf("%w: operator command ID reused with different content", port.ErrConflict)
			}
			current, err := tx.OperatorCommandReceipt(command.ID)
			if err != nil {
				return err
			}
			stored = current
			return nil
		} else if !errors.Is(lookupErr, port.ErrNotFound) {
			return lookupErr
		}
		if existing, lookupErr := tx.OperatorCommandByIdempotency(command.IdempotencyKey); lookupErr == nil {
			if !equalOperatorCommands(existing, command) {
				return fmt.Errorf("%w: operator command idempotency key reused with different content", port.ErrConflict)
			}
			current, err := tx.OperatorCommandReceipt(existing.ID)
			if err != nil {
				return err
			}
			stored = current
			return nil
		} else if !errors.Is(lookupErr, port.ErrNotFound) {
			return lookupErr
		}
		if err := tx.CreateOperatorCommand(command, receipt); err != nil {
			return err
		}
		stored = receipt
		return nil
	})
	if err != nil {
		return domain.CommandReceipt{}, err
	}
	return stored, nil
}

func (i *CommandInbox) Command(id domain.CommandID) (domain.OperatorCommand, error) {
	var command domain.OperatorCommand
	err := i.Store.View(context.Background(), func(r port.Reader) error {
		got, err := r.OperatorCommand(id)
		if err != nil {
			return err
		}
		command = got
		return nil
	})
	return command, err
}

func (i *CommandInbox) CommandReceipt(id domain.CommandID) (domain.CommandReceipt, error) {
	var receipt domain.CommandReceipt
	err := i.Store.View(context.Background(), func(r port.Reader) error {
		got, err := r.OperatorCommandReceipt(id)
		if err != nil {
			return err
		}
		receipt = got
		return nil
	})
	return receipt, err
}

// FixedReceiptFactory is useful in tests that need deterministic receipt IDs.
func FixedReceiptFactory(receiptID domain.ReceiptID, recordedAt time.Time) ReceiptFactory {
	return func(command domain.OperatorCommand) (domain.CommandReceipt, error) {
		return domain.CommandReceipt{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            receiptID,
			CommandID:     command.ID,
			State:         domain.CommandReceived,
			RecordedAt:    recordedAt.UTC(),
		}, nil
	}
}

func equalOperatorCommands(a, b domain.OperatorCommand) bool {
	if a.SchemaVersion != b.SchemaVersion || a.ID != b.ID || a.IdempotencyKey != b.IdempotencyKey || a.ActorType != b.ActorType || a.ActorID != b.ActorID || a.Kind != b.Kind || a.Target != b.Target || a.Reason != b.Reason || !a.SubmittedAt.Equal(b.SubmittedAt) {
		return false
	}
	switch {
	case a.ExpectedRevision == nil && b.ExpectedRevision == nil:
		return true
	case a.ExpectedRevision == nil || b.ExpectedRevision == nil:
		return false
	default:
		return *a.ExpectedRevision == *b.ExpectedRevision
	}
}

var _ port.CommandInbox = (*CommandInbox)(nil)
