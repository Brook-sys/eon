package port

import "motor-autonomo/internal/domain"

// CommandInbox is deliberately separate from Store: transports may persist
// requests through this narrow boundary without receiving canonical write
// authority. Implementations must deduplicate by command id and idempotency key.
type CommandInbox interface {
	SubmitCommand(domain.OperatorCommand) (domain.CommandReceipt, error)
	Command(domain.CommandID) (domain.OperatorCommand, error)
	CommandReceipt(domain.CommandID) (domain.CommandReceipt, error)
}

// CommandReceiptWriter is held by the kernel command processor, never by UI or
// transport adapters. State changes must be monotonic and auditable.
type CommandReceiptWriter interface {
	AdvanceCommand(domain.CommandReceipt) error
}

// ExternalEventInbox accepts bounded, typed and untrusted stimuli. Duplicate
// delivery keys replay the original record; divergent reuse is a conflict.
// Submit returns the durable disposition so transports can acknowledge receipt
// without obtaining write authority over domain effects.
type ExternalEventInbox interface {
	SubmitExternalEvent(domain.ExternalEvent) (domain.ExternalEventDisposition, error)
	ExternalEvent(domain.ExternalEventID) (domain.ExternalEvent, error)
	ExternalEventByDeduplicationKey(string) (domain.ExternalEvent, error)
	ExternalEventDisposition(domain.ExternalEventID) (domain.ExternalEventDisposition, error)
}
