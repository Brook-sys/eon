package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// CycleInstruments holds derived counters for control-loop activity.
// Instruments are nil-safe: recording is a no-op when telemetry is disabled.
type CycleInstruments struct {
	cycles    metric.Int64Counter
	commands  metric.Int64Counter
	events    metric.Int64Counter
	opsDone   metric.Int64Counter
	opsSkip   metric.Int64Counter
	leases    metric.Int64Counter
	scheduler metric.Int64Counter
}

// NewCycleInstruments binds counters to the runtime meter. Safe with nil/disabled.
func NewCycleInstruments(rt *Runtime) *CycleInstruments {
	ci := &CycleInstruments{}
	if rt == nil || !rt.Enabled() {
		return ci
	}
	meter := rt.Meter()
	if c, err := meter.Int64Counter("motor.control.cycles"); err == nil {
		ci.cycles = c
	}
	if c, err := meter.Int64Counter("motor.control.commands"); err == nil {
		ci.commands = c
	}
	if c, err := meter.Int64Counter("motor.control.events"); err == nil {
		ci.events = c
	}
	if c, err := meter.Int64Counter("motor.control.operations.completed"); err == nil {
		ci.opsDone = c
	}
	if c, err := meter.Int64Counter("motor.control.operations.skipped"); err == nil {
		ci.opsSkip = c
	}
	if c, err := meter.Int64Counter("motor.control.leases.reconciled"); err == nil {
		ci.leases = c
	}
	if c, err := meter.Int64Counter("motor.control.scheduler.steps"); err == nil {
		ci.scheduler = c
	}
	return ci
}

// CycleSnapshot is a telemetry-only summary of one control cycle.
// It must never be used as authority for kernel decisions.
type CycleSnapshot struct {
	Outcome            string
	CommandsProcessed  int
	EventsProcessed    int
	OperationsExecuted int
	OperationsSkipped  int
	LeasesReconciled   int
	SchedulerRan       bool
	SchedulerKind      string
	Worked             bool
	Stopping           bool
}

// Record emits derived cycle metrics. Never panics; ignores nil receivers.
func (c *CycleInstruments) Record(ctx context.Context, snap CycleSnapshot) {
	if c == nil {
		return
	}
	outcome := sanitizeLabel(snap.Outcome, "idle")
	attrs := []attribute.KeyValue{
		attribute.String("motor.control.outcome", outcome),
		attribute.Bool("motor.telemetry.canonical", false),
	}
	if c.cycles != nil {
		c.cycles.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if snap.CommandsProcessed > 0 && c.commands != nil {
		c.commands.Add(ctx, int64(snap.CommandsProcessed), metric.WithAttributes(attrs...))
	}
	if snap.EventsProcessed > 0 && c.events != nil {
		c.events.Add(ctx, int64(snap.EventsProcessed), metric.WithAttributes(attrs...))
	}
	if snap.OperationsExecuted > 0 && c.opsDone != nil {
		c.opsDone.Add(ctx, int64(snap.OperationsExecuted), metric.WithAttributes(attrs...))
	}
	if snap.OperationsSkipped > 0 && c.opsSkip != nil {
		c.opsSkip.Add(ctx, int64(snap.OperationsSkipped), metric.WithAttributes(attrs...))
	}
	if snap.LeasesReconciled > 0 && c.leases != nil {
		c.leases.Add(ctx, int64(snap.LeasesReconciled), metric.WithAttributes(attrs...))
	}
	if snap.SchedulerRan && c.scheduler != nil {
		kindAttrs := append(attrs, attribute.String("motor.scheduler.kind", sanitizeLabel(snap.SchedulerKind, "unknown")))
		c.scheduler.Add(ctx, 1, metric.WithAttributes(kindAttrs...))
	}
}
