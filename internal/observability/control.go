package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TraceControl marks a control-plane unit of work. It never decides outcomes.
func (r *Runtime) TraceControl(ctx context.Context, name, kind, missionID, correlation string) (context.Context, trace.Span) {
	return r.StartSpan(ctx, name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(ControlAttributes(kind, missionID, correlation)...),
	)
}

// EndControl finishes a control span with a coarse outcome label only.
func EndControl(span trace.Span, outcome, failureCode string) {
	if span == nil {
		return
	}
	if outcome != "" {
		span.SetAttributes(attribute.String("motor.control.outcome", sanitizeLabel(outcome, "outcome")))
	}
	if failureCode != "" {
		span.SetAttributes(attribute.String("motor.control.failure_code", sanitizeCode(failureCode)))
		span.SetStatus(codes.Error, sanitizeCode(failureCode))
		span.End()
		return
	}
	span.SetStatus(codes.Ok, "")
	span.End()
}
