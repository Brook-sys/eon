package control

import (
	"fmt"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/runtime/source"
)

// ReceiptFactoryFrom mints RECEIVED receipts with injectable identity and time.
func ReceiptFactoryFrom(clock source.Clock, ids source.IDGenerator) ReceiptFactory {
	return func(command domain.OperatorCommand) (domain.CommandReceipt, error) {
		if clock == nil || ids == nil {
			return domain.CommandReceipt{}, fmt.Errorf("receipt factory requires clock and ID generator")
		}
		receiptID, err := ids.NewID("receipt")
		if err != nil {
			return domain.CommandReceipt{}, err
		}
		return domain.CommandReceipt{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            domain.ReceiptID(receiptID),
			CommandID:     command.ID,
			State:         domain.CommandReceived,
			RecordedAt:    clock.Now().UTC(),
		}, nil
	}
}

// DispositionFactoryFrom mints RECEIVED dispositions with injectable time.
func DispositionFactoryFrom(clock source.Clock) DispositionFactory {
	return func(event domain.ExternalEvent) (domain.ExternalEventDisposition, error) {
		if clock == nil {
			return domain.ExternalEventDisposition{}, fmt.Errorf("disposition factory requires clock")
		}
		return domain.ExternalEventDisposition{
			SchemaVersion: domain.SchemaVersionV1,
			EventID:       event.ID,
			State:         domain.ExternalEventReceived,
			RecordedAt:    clock.Now().UTC(),
		}, nil
	}
}

// Ensure command IDs exist before durable submit. Empty IDs are filled; supplied
// IDs are preserved so clients can correlate retries.
func ensureCommandIdentity(command domain.OperatorCommand, clock source.Clock, ids source.IDGenerator) (domain.OperatorCommand, error) {
	if clock == nil || ids == nil {
		return domain.OperatorCommand{}, fmt.Errorf("command identity requires clock and ID generator")
	}
	if command.SchemaVersion == 0 {
		command.SchemaVersion = domain.SchemaVersionV1
	}
	if command.ActorType == "" {
		command.ActorType = domain.ActorOperator
	}
	if command.SubmittedAt.IsZero() {
		command.SubmittedAt = clock.Now().UTC()
	} else {
		command.SubmittedAt = command.SubmittedAt.UTC()
	}
	if command.ID == "" {
		id, err := ids.NewID("cmd")
		if err != nil {
			return domain.OperatorCommand{}, err
		}
		command.ID = domain.CommandID(id)
	}
	if command.IdempotencyKey == "" {
		key, err := ids.NewID("idem")
		if err != nil {
			return domain.OperatorCommand{}, err
		}
		command.IdempotencyKey = domain.IdempotencyKey(key)
	}
	return command, nil
}

func ensureExternalEventIdentity(event domain.ExternalEvent, clock source.Clock, ids source.IDGenerator) (domain.ExternalEvent, error) {
	if clock == nil || ids == nil {
		return domain.ExternalEvent{}, fmt.Errorf("external event identity requires clock and ID generator")
	}
	if event.SchemaVersion == 0 {
		event.SchemaVersion = domain.SchemaVersionV1
	}
	if event.ReceivedAt.IsZero() {
		event.ReceivedAt = clock.Now().UTC()
	} else {
		event.ReceivedAt = event.ReceivedAt.UTC()
	}
	if event.ID == "" {
		id, err := ids.NewID("ext")
		if err != nil {
			return domain.ExternalEvent{}, err
		}
		event.ID = domain.ExternalEventID(id)
	}
	if event.DeduplicationKey == "" {
		key, err := ids.NewID("dedupe")
		if err != nil {
			return domain.ExternalEvent{}, err
		}
		event.DeduplicationKey = key
	}
	return event, nil
}
