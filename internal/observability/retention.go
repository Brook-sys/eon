package observability

import (
	"errors"
	"fmt"
	"time"
)

// Default export-buffer bounds. These cap disposable in-process queues only
// (FR-CTRL-007): they never govern canonical store retention or kernel GC.
const (
	DefaultTraceMaxQueueSize       = 2048
	DefaultTraceMaxExportBatchSize = 512
	DefaultTraceBatchTimeout       = 5 * time.Second
	DefaultTraceExportTimeout      = 30 * time.Second
	DefaultMetricInterval          = 60 * time.Second
	DefaultMetricExportTimeout     = 30 * time.Second

	// Hard ceilings prevent accidental multi-GB process buffers.
	MaxTraceMaxQueueSize       = 100_000
	MaxTraceMaxExportBatchSize = 10_000
	MaxTraceBatchTimeout       = 5 * time.Minute
	MaxTraceExportTimeout      = 5 * time.Minute
	MaxMetricInterval          = 15 * time.Minute
	MaxMetricExportTimeout     = 5 * time.Minute

	// RetentionPolicyVersion is stamped on inspect projections of export policy.
	RetentionPolicyVersion = "otel-export-retention.v1"
)

// ExportRetention bounds disposable OTLP export buffers and flush cadence.
// Zero fields mean "apply package defaults when OTLP export is active".
// This is NOT store/event retention and MUST NOT be read by the kernel.
type ExportRetention struct {
	// TraceMaxQueueSize caps BatchSpanProcessor queue length.
	TraceMaxQueueSize int
	// TraceMaxExportBatchSize caps spans per export batch.
	TraceMaxExportBatchSize int
	// TraceBatchTimeout is the maximum delay before a partial batch flushes.
	TraceBatchTimeout time.Duration
	// TraceExportTimeout bounds a single span export attempt.
	TraceExportTimeout time.Duration
	// MetricInterval is the PeriodicReader collect/export interval.
	MetricInterval time.Duration
	// MetricExportTimeout bounds a single metric export attempt.
	MetricExportTimeout time.Duration
}

// Normalize fills zero fields with defaults. Call after Validate when export
// is enabled; disabled runtimes may leave zeros untouched.
func (r ExportRetention) Normalize() ExportRetention {
	out := r
	if out.TraceMaxQueueSize <= 0 {
		out.TraceMaxQueueSize = DefaultTraceMaxQueueSize
	}
	if out.TraceMaxExportBatchSize <= 0 {
		out.TraceMaxExportBatchSize = DefaultTraceMaxExportBatchSize
	}
	if out.TraceBatchTimeout <= 0 {
		out.TraceBatchTimeout = DefaultTraceBatchTimeout
	}
	if out.TraceExportTimeout <= 0 {
		out.TraceExportTimeout = DefaultTraceExportTimeout
	}
	if out.MetricInterval <= 0 {
		out.MetricInterval = DefaultMetricInterval
	}
	if out.MetricExportTimeout <= 0 {
		out.MetricExportTimeout = DefaultMetricExportTimeout
	}
	// Batch size must never exceed queue size.
	if out.TraceMaxExportBatchSize > out.TraceMaxQueueSize {
		out.TraceMaxExportBatchSize = out.TraceMaxQueueSize
	}
	return out
}

// Validate rejects negative values and values above hard ceilings.
// Zero is allowed and means "default" until Normalize.
func (r ExportRetention) Validate() error {
	if r.TraceMaxQueueSize < 0 {
		return errors.New("observability trace max queue size must not be negative")
	}
	if r.TraceMaxQueueSize > MaxTraceMaxQueueSize {
		return fmt.Errorf("observability trace max queue size is capped at %d", MaxTraceMaxQueueSize)
	}
	if r.TraceMaxExportBatchSize < 0 {
		return errors.New("observability trace max export batch size must not be negative")
	}
	if r.TraceMaxExportBatchSize > MaxTraceMaxExportBatchSize {
		return fmt.Errorf("observability trace max export batch size is capped at %d", MaxTraceMaxExportBatchSize)
	}
	if r.TraceBatchTimeout < 0 {
		return errors.New("observability trace batch timeout must not be negative")
	}
	if r.TraceBatchTimeout > MaxTraceBatchTimeout {
		return fmt.Errorf("observability trace batch timeout is capped at %s", MaxTraceBatchTimeout)
	}
	if r.TraceExportTimeout < 0 {
		return errors.New("observability trace export timeout must not be negative")
	}
	if r.TraceExportTimeout > MaxTraceExportTimeout {
		return fmt.Errorf("observability trace export timeout is capped at %s", MaxTraceExportTimeout)
	}
	if r.MetricInterval < 0 {
		return errors.New("observability metric interval must not be negative")
	}
	if r.MetricInterval > MaxMetricInterval {
		return fmt.Errorf("observability metric interval is capped at %s", MaxMetricInterval)
	}
	if r.MetricExportTimeout < 0 {
		return errors.New("observability metric export timeout must not be negative")
	}
	if r.MetricExportTimeout > MaxMetricExportTimeout {
		return fmt.Errorf("observability metric export timeout is capped at %s", MaxMetricExportTimeout)
	}
	if r.TraceMaxQueueSize > 0 && r.TraceMaxExportBatchSize > r.TraceMaxQueueSize {
		return errors.New("observability trace max export batch size must not exceed max queue size")
	}
	return nil
}

// View is a presentation-safe snapshot of effective export retention.
type RetentionView struct {
	PolicyVersion           string `json:"policy_version"`
	Canonical               bool   `json:"canonical"` // always false
	TraceMaxQueueSize       int    `json:"trace_max_queue_size"`
	TraceMaxExportBatchSize int    `json:"trace_max_export_batch_size"`
	TraceBatchTimeoutMS     int64  `json:"trace_batch_timeout_ms"`
	TraceExportTimeoutMS    int64  `json:"trace_export_timeout_ms"`
	MetricIntervalMS        int64  `json:"metric_interval_ms"`
	MetricExportTimeoutMS   int64  `json:"metric_export_timeout_ms"`
}

// View returns JSON-friendly effective retention (normalized).
func (r ExportRetention) View() RetentionView {
	n := r.Normalize()
	return RetentionView{
		PolicyVersion:           RetentionPolicyVersion,
		Canonical:               false,
		TraceMaxQueueSize:       n.TraceMaxQueueSize,
		TraceMaxExportBatchSize: n.TraceMaxExportBatchSize,
		TraceBatchTimeoutMS:     n.TraceBatchTimeout.Milliseconds(),
		TraceExportTimeoutMS:    n.TraceExportTimeout.Milliseconds(),
		MetricIntervalMS:        n.MetricInterval.Milliseconds(),
		MetricExportTimeoutMS:   n.MetricExportTimeout.Milliseconds(),
	}
}
