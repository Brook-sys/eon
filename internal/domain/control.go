package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const MaxControlPayloadBytes = 64 << 10

type ActorType string

const (
	ActorOperator ActorType = "OPERATOR"
	ActorKernel   ActorType = "KERNEL"
	ActorAdapter  ActorType = "ADAPTER"
)

func (a ActorType) valid() bool {
	switch a {
	case ActorOperator, ActorKernel, ActorAdapter:
		return true
	default:
		return false
	}
}

type OperatorCommandKind string

const (
	CommandPauseMission     OperatorCommandKind = "PAUSE_MISSION"
	CommandResumeMission    OperatorCommandKind = "RESUME_MISSION"
	CommandCancelMission    OperatorCommandKind = "CANCEL_MISSION"
	CommandGracefulShutdown OperatorCommandKind = "GRACEFUL_SHUTDOWN"
)

func (k OperatorCommandKind) valid() bool {
	switch k {
	case CommandPauseMission, CommandResumeMission, CommandCancelMission, CommandGracefulShutdown:
		return true
	default:
		return false
	}
}

// CommandTarget names the canonical object affected by a command. MissionID is
// required for mission-scoped commands and forbidden for process-scoped ones.
type CommandTarget struct {
	MissionID MissionID `json:"mission_id,omitempty"`
}

type OperatorCommand struct {
	SchemaVersion    int                 `json:"schema_version"`
	ID               CommandID           `json:"command_id"`
	IdempotencyKey   IdempotencyKey      `json:"idempotency_key"`
	ActorType        ActorType           `json:"actor_type"`
	ActorID          string              `json:"actor_id"`
	Kind             OperatorCommandKind `json:"kind"`
	Target           CommandTarget       `json:"target"`
	ExpectedRevision *uint64             `json:"expected_revision,omitempty"`
	Reason           string              `json:"reason"`
	SubmittedAt      time.Time           `json:"submitted_at"`
}

func (c OperatorCommand) Validate() error {
	if c.SchemaVersion != SchemaVersionV1 || c.ID == "" || c.IdempotencyKey == "" || c.ActorID == "" || c.Reason == "" || c.SubmittedAt.IsZero() {
		return errors.New("operator command is incomplete or has unsupported schema version")
	}
	if !c.ActorType.valid() {
		return fmt.Errorf("unknown command actor type %q", c.ActorType)
	}
	if !c.Kind.valid() {
		return fmt.Errorf("unknown operator command kind %q", c.Kind)
	}
	if c.ExpectedRevision != nil && *c.ExpectedRevision == 0 {
		return errors.New("expected revision must be positive when present")
	}
	switch c.Kind {
	case CommandPauseMission, CommandResumeMission, CommandCancelMission:
		if c.Target.MissionID == "" || c.ExpectedRevision == nil {
			return errors.New("mission command requires mission target and expected revision")
		}
	case CommandGracefulShutdown:
		if c.Target.MissionID != "" || c.ExpectedRevision != nil {
			return errors.New("graceful shutdown is process-scoped and must not carry mission revision")
		}
	}
	return nil
}

type CommandState string

const (
	CommandReceived    CommandState = "RECEIVED"
	CommandValidating  CommandState = "VALIDATING"
	CommandAccepted    CommandState = "ACCEPTED"
	CommandRejected    CommandState = "REJECTED"
	CommandApplying    CommandState = "APPLYING"
	CommandApplied     CommandState = "APPLIED"
	CommandReconciling CommandState = "RECONCILING"
	CommandFailed      CommandState = "FAILED"
)

func (s CommandState) valid() bool {
	switch s {
	case CommandReceived, CommandValidating, CommandAccepted, CommandRejected,
		CommandApplying, CommandApplied, CommandReconciling, CommandFailed:
		return true
	default:
		return false
	}
}

func (s CommandState) Terminal() bool {
	switch s {
	case CommandRejected, CommandApplied, CommandFailed:
		return true
	default:
		return false
	}
}

type CommandReceipt struct {
	SchemaVersion int          `json:"schema_version"`
	ID            ReceiptID    `json:"receipt_id"`
	CommandID     CommandID    `json:"command_id"`
	State         CommandState `json:"state"`
	ResultRef     string       `json:"result_ref,omitempty"`
	FailureCode   string       `json:"failure_code,omitempty"`
	RecordedAt    time.Time    `json:"recorded_at"`
}

func (r CommandReceipt) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 || r.ID == "" || r.CommandID == "" || r.RecordedAt.IsZero() {
		return errors.New("command receipt is incomplete or has unsupported schema version")
	}
	if !r.State.valid() {
		return fmt.Errorf("unknown command state %q", r.State)
	}
	if r.ResultRef != "" && r.FailureCode != "" {
		return errors.New("command receipt cannot contain both result and failure")
	}
	switch r.State {
	case CommandApplied:
		if r.ResultRef == "" {
			return errors.New("applied command receipt requires result reference")
		}
	case CommandRejected, CommandFailed:
		if r.FailureCode == "" {
			return errors.New("rejected or failed command receipt requires failure code")
		}
	default:
		if r.ResultRef != "" || r.FailureCode != "" {
			return errors.New("non-terminal command receipt must not claim a result or failure")
		}
	}
	return nil
}

type ExternalEventKind string

const (
	ExternalUserMessage        ExternalEventKind = "USER_MESSAGE"
	ExternalUserAnswer         ExternalEventKind = "USER_ANSWER"
	ExternalAuthorizedSource   ExternalEventKind = "AUTHORIZED_SOURCE"
	ExternalAvailabilitySignal ExternalEventKind = "AVAILABILITY_SIGNAL"
)

func (k ExternalEventKind) valid() bool {
	switch k {
	case ExternalUserMessage, ExternalUserAnswer, ExternalAuthorizedSource, ExternalAvailabilitySignal:
		return true
	default:
		return false
	}
}

// ExternalContent remains untrusted data. Structured is an optional bounded
// JSON object interpreted only by the handler selected from Kind; it never
// carries command authority.
type ExternalContent struct {
	MediaType  string          `json:"media_type"`
	Text       string          `json:"text,omitempty"`
	Reference  string          `json:"reference,omitempty"`
	Structured json.RawMessage `json:"structured,omitempty"`
}

func (c ExternalContent) Validate() error {
	if c.MediaType == "" {
		return errors.New("external content requires media type")
	}
	present := 0
	if c.Text != "" {
		present++
	}
	if c.Reference != "" {
		present++
	}
	if len(c.Structured) != 0 {
		present++
		if len(c.Structured) > MaxControlPayloadBytes {
			return errors.New("external structured content exceeds byte limit")
		}
		if !json.Valid(c.Structured) || firstNonSpace(c.Structured) != '{' {
			return errors.New("external structured content must be a JSON object")
		}
	}
	if present != 1 {
		return errors.New("external content requires exactly one of text, reference, or structured")
	}
	if len(c.Text) > MaxControlPayloadBytes || len(c.Reference) > MaxControlPayloadBytes {
		return errors.New("external content exceeds byte limit")
	}
	return nil
}

func firstNonSpace(value []byte) byte {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return 0
	}
	return trimmed[0]
}

type ExternalEvent struct {
	SchemaVersion      int               `json:"schema_version"`
	ID                 ExternalEventID   `json:"event_id"`
	DeduplicationKey   string            `json:"deduplication_key"`
	Source             string            `json:"source"`
	SourceActorID      string            `json:"source_actor_id"`
	Kind               ExternalEventKind `json:"kind"`
	MissionID          MissionID         `json:"mission_id,omitempty"`
	CorrelationID      string            `json:"correlation_id,omitempty"`
	TransportMessageID string            `json:"transport_message_id,omitempty"`
	Content            ExternalContent   `json:"content"`
	ReceivedAt         time.Time         `json:"received_at"`
}

func (e ExternalEvent) Validate() error {
	if e.SchemaVersion != SchemaVersionV1 || e.ID == "" || e.DeduplicationKey == "" || e.Source == "" || e.SourceActorID == "" || e.ReceivedAt.IsZero() {
		return errors.New("external event is incomplete or has unsupported schema version")
	}
	if !e.Kind.valid() {
		return fmt.Errorf("unknown external event kind %q", e.Kind)
	}
	if e.MissionID == "" && e.Kind != ExternalAvailabilitySignal {
		return errors.New("external event kind requires mission scope")
	}
	if e.Kind == ExternalUserAnswer && e.CorrelationID == "" {
		return errors.New("user answer requires canonical correlation ID")
	}
	return e.Content.Validate()
}

// ExternalEventDisposition records kernel handling of an untrusted stimulus.
// Content never becomes policy; only disposition + audit events do.
type ExternalEventDispositionState string

const (
	ExternalEventReceived ExternalEventDispositionState = "RECEIVED"
	ExternalEventApplied  ExternalEventDispositionState = "APPLIED"
	ExternalEventRejected ExternalEventDispositionState = "REJECTED"
	ExternalEventIgnored  ExternalEventDispositionState = "IGNORED"
)

func (s ExternalEventDispositionState) valid() bool {
	switch s {
	case ExternalEventReceived, ExternalEventApplied, ExternalEventRejected, ExternalEventIgnored:
		return true
	default:
		return false
	}
}

func (s ExternalEventDispositionState) Terminal() bool {
	switch s {
	case ExternalEventApplied, ExternalEventRejected, ExternalEventIgnored:
		return true
	default:
		return false
	}
}

type ExternalEventDisposition struct {
	SchemaVersion int                           `json:"schema_version"`
	EventID       ExternalEventID               `json:"event_id"`
	State         ExternalEventDispositionState `json:"state"`
	ResultRef     string                        `json:"result_ref,omitempty"`
	FailureCode   string                        `json:"failure_code,omitempty"`
	RecordedAt    time.Time                     `json:"recorded_at"`
}

func (d ExternalEventDisposition) Validate() error {
	if d.SchemaVersion != SchemaVersionV1 || d.EventID == "" || d.RecordedAt.IsZero() {
		return errors.New("external event disposition is incomplete or has unsupported schema version")
	}
	if !d.State.valid() {
		return fmt.Errorf("unknown external event disposition state %q", d.State)
	}
	if d.ResultRef != "" && d.FailureCode != "" {
		return errors.New("external event disposition cannot contain both result and failure")
	}
	switch d.State {
	case ExternalEventApplied:
		if d.ResultRef == "" {
			return errors.New("applied external event disposition requires result reference")
		}
	case ExternalEventRejected:
		if d.FailureCode == "" {
			return errors.New("rejected external event disposition requires failure code")
		}
	case ExternalEventIgnored:
		if d.FailureCode == "" && d.ResultRef == "" {
			return errors.New("ignored external event disposition requires reason ref or failure code")
		}
	default:
		if d.ResultRef != "" || d.FailureCode != "" {
			return errors.New("received external event disposition must not claim result or failure")
		}
	}
	return nil
}

// AdvanceExternalEventDisposition enforces monotonic handling of stimuli.
func AdvanceExternalEventDisposition(current, next ExternalEventDisposition) error {
	if err := current.Validate(); err != nil {
		return fmt.Errorf("validate current external disposition: %w", err)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate next external disposition: %w", err)
	}
	if current.EventID != next.EventID {
		return errors.New("external event disposition identity changed")
	}
	if next.RecordedAt.Before(current.RecordedAt) {
		return errors.New("external event disposition time moved backwards")
	}
	if current.State == next.State {
		if current == next {
			return nil
		}
		return fmt.Errorf("%w: external event disposition changed without state advance", ErrConflict)
	}
	if current.State.Terminal() {
		return fmt.Errorf("%w: terminal external event disposition cannot advance", ErrConflict)
	}
	if current.State != ExternalEventReceived {
		return fmt.Errorf("%w: illegal external event disposition transition %s → %s", ErrConflict, current.State, next.State)
	}
	switch next.State {
	case ExternalEventApplied, ExternalEventRejected, ExternalEventIgnored:
		return nil
	default:
		return fmt.Errorf("%w: illegal external event disposition transition %s → %s", ErrConflict, current.State, next.State)
	}
}
