package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
)

// CommandProcessor applies operator commands. Implemented by kernel.CommandProcessor.
type CommandProcessor interface {
	ProcessNext(ctx context.Context) (domain.CommandReceipt, bool, error)
	Process(ctx context.Context, commandID domain.CommandID) (domain.CommandReceipt, error)
}

// ExternalEventProcessor applies untrusted external stimuli.
type ExternalEventProcessor interface {
	ProcessNext(ctx context.Context) (domain.ExternalEventDisposition, bool, error)
	Process(ctx context.Context, eventID domain.ExternalEventID) (domain.ExternalEventDisposition, error)
}

// TracedCommandProcessor decorates a command processor with derived spans/metrics.
// Telemetry never changes apply outcomes and never consults canonical state.
type TracedCommandProcessor struct {
	Inner   CommandProcessor
	Runtime *Runtime
	calls   metric.Int64Counter
}

// InstrumentCommand wraps a command processor. Nil runtime is safe.
func InstrumentCommand(inner CommandProcessor, runtime *Runtime) CommandProcessor {
	if inner == nil {
		return nil
	}
	p := &TracedCommandProcessor{Inner: inner, Runtime: runtime}
	if runtime != nil && runtime.Enabled() {
		if c, err := runtime.Meter().Int64Counter("motor.control.command_process"); err == nil {
			p.calls = c
		}
	}
	return p
}

func (p *TracedCommandProcessor) ProcessNext(ctx context.Context) (domain.CommandReceipt, bool, error) {
	if p == nil || p.Inner == nil {
		return domain.CommandReceipt{}, false, errString("observability command processor is missing")
	}
	// Kernel ProcessNext peeks then Process on the concrete type, so the drain
	// path never enters the decorated Process. Instrument the drain unit here.
	// Span start is stamped from wall clock before the call so approximate
	// duration is preserved without exporting empty peeks.
	started := time.Now()
	receipt, ok, err := p.Inner.ProcessNext(ctx)
	if !ok && err == nil {
		return receipt, false, nil
	}
	if p.Runtime == nil || !p.Runtime.Enabled() {
		return receipt, ok, err
	}
	_, span := p.Runtime.StartSpan(ctx, "control.command.process",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithTimestamp(started),
		trace.WithAttributes(ControlAttributes("operator_command", "", string(receipt.CommandID))...),
	)
	if err != nil {
		RecordError(span, err, "COMMAND_PROCESS")
		span.End(trace.WithTimestamp(time.Now()))
		p.recordCall(ctx, "error", "")
		return domain.CommandReceipt{}, true, err
	}
	if receipt.CommandID != "" {
		span.SetAttributes(attribute.String("motor.command.id", sanitizeLabel(string(receipt.CommandID), "command")))
	}
	p.finishCommand(ctx, span, receipt)
	return receipt, true, nil
}

func (p *TracedCommandProcessor) Process(ctx context.Context, commandID domain.CommandID) (domain.CommandReceipt, error) {
	if p == nil || p.Inner == nil {
		return domain.CommandReceipt{}, errString("observability command processor is missing")
	}
	if p.Runtime == nil || !p.Runtime.Enabled() {
		return p.Inner.Process(ctx, commandID)
	}
	started := time.Now()
	ctx, span := p.Runtime.StartSpan(ctx, "control.command.process",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithTimestamp(started),
		trace.WithAttributes(ControlAttributes("operator_command", "", string(commandID))...),
	)
	span.SetAttributes(attribute.String("motor.command.id", sanitizeLabel(string(commandID), "command")))

	receipt, err := p.Inner.Process(ctx, commandID)
	if err != nil {
		RecordError(span, err, "COMMAND_PROCESS")
		span.End(trace.WithTimestamp(time.Now()))
		p.recordCall(ctx, "error", "")
		return domain.CommandReceipt{}, err
	}
	p.finishCommand(ctx, span, receipt)
	return receipt, nil
}

func (p *TracedCommandProcessor) finishCommand(ctx context.Context, span trace.Span, receipt domain.CommandReceipt) {
	outcome := string(receipt.State)
	if outcome == "" {
		outcome = "unknown"
	}
	if receipt.FailureCode != "" {
		span.SetAttributes(attribute.String("motor.control.failure_code", sanitizeCode(receipt.FailureCode)))
	}
	// EndControl ends the span exactly once.
	EndControl(span, outcome, receipt.FailureCode)
	p.recordCall(ctx, outcome, receipt.FailureCode)
}

func (p *TracedCommandProcessor) recordCall(ctx context.Context, outcome, failure string) {
	if p.calls == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("motor.control.outcome", sanitizeLabel(outcome, "outcome")),
		attribute.Bool("motor.telemetry.canonical", false),
	}
	if failure != "" {
		attrs = append(attrs, attribute.String("motor.control.failure_code", sanitizeCode(failure)))
	}
	p.calls.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// TracedExternalEventProcessor decorates external-event apply with derived telemetry.
type TracedExternalEventProcessor struct {
	Inner   ExternalEventProcessor
	Runtime *Runtime
	calls   metric.Int64Counter
}

// InstrumentExternalEvent wraps an external-event processor. Nil runtime is safe.
func InstrumentExternalEvent(inner ExternalEventProcessor, runtime *Runtime) ExternalEventProcessor {
	if inner == nil {
		return nil
	}
	p := &TracedExternalEventProcessor{Inner: inner, Runtime: runtime}
	if runtime != nil && runtime.Enabled() {
		if c, err := runtime.Meter().Int64Counter("motor.control.external_event_process"); err == nil {
			p.calls = c
		}
	}
	return p
}

func (p *TracedExternalEventProcessor) ProcessNext(ctx context.Context) (domain.ExternalEventDisposition, bool, error) {
	if p == nil || p.Inner == nil {
		return domain.ExternalEventDisposition{}, false, errString("observability external event processor is missing")
	}
	started := time.Now()
	disposition, ok, err := p.Inner.ProcessNext(ctx)
	if !ok && err == nil {
		return disposition, false, nil
	}
	if p.Runtime == nil || !p.Runtime.Enabled() {
		return disposition, ok, err
	}
	_, span := p.Runtime.StartSpan(ctx, "control.external_event.process",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithTimestamp(started),
		trace.WithAttributes(ControlAttributes("external_event", "", string(disposition.EventID))...),
	)
	if err != nil {
		RecordError(span, err, "EXTERNAL_EVENT_PROCESS")
		span.End(trace.WithTimestamp(time.Now()))
		p.recordCall(ctx, "error", "")
		return domain.ExternalEventDisposition{}, true, err
	}
	if disposition.EventID != "" {
		span.SetAttributes(attribute.String("motor.external_event.id", sanitizeLabel(string(disposition.EventID), "event")))
	}
	p.finishEvent(ctx, span, disposition)
	return disposition, true, nil
}

func (p *TracedExternalEventProcessor) Process(ctx context.Context, eventID domain.ExternalEventID) (domain.ExternalEventDisposition, error) {
	if p == nil || p.Inner == nil {
		return domain.ExternalEventDisposition{}, errString("observability external event processor is missing")
	}
	if p.Runtime == nil || !p.Runtime.Enabled() {
		return p.Inner.Process(ctx, eventID)
	}
	started := time.Now()
	ctx, span := p.Runtime.StartSpan(ctx, "control.external_event.process",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithTimestamp(started),
		trace.WithAttributes(ControlAttributes("external_event", "", string(eventID))...),
	)
	span.SetAttributes(attribute.String("motor.external_event.id", sanitizeLabel(string(eventID), "event")))

	disposition, err := p.Inner.Process(ctx, eventID)
	if err != nil {
		RecordError(span, err, "EXTERNAL_EVENT_PROCESS")
		span.End(trace.WithTimestamp(time.Now()))
		p.recordCall(ctx, "error", "")
		return domain.ExternalEventDisposition{}, err
	}
	p.finishEvent(ctx, span, disposition)
	return disposition, nil
}

func (p *TracedExternalEventProcessor) finishEvent(ctx context.Context, span trace.Span, disposition domain.ExternalEventDisposition) {
	outcome := string(disposition.State)
	if outcome == "" {
		outcome = "unknown"
	}
	if disposition.FailureCode != "" {
		span.SetAttributes(attribute.String("motor.control.failure_code", sanitizeCode(disposition.FailureCode)))
	}
	EndControl(span, outcome, disposition.FailureCode)
	p.recordCall(ctx, outcome, disposition.FailureCode)
}

func (p *TracedExternalEventProcessor) recordCall(ctx context.Context, outcome, failure string) {
	if p.calls == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("motor.control.outcome", sanitizeLabel(outcome, "outcome")),
		attribute.Bool("motor.telemetry.canonical", false),
	}
	if failure != "" {
		attrs = append(attrs, attribute.String("motor.control.failure_code", sanitizeCode(failure)))
	}
	p.calls.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// Ensure kernel types continue to satisfy the local interfaces.
var (
	_ CommandProcessor       = (*kernel.CommandProcessor)(nil)
	_ ExternalEventProcessor = (*kernel.ExternalEventProcessor)(nil)
)
