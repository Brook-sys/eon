// Package observability exports optional OpenTelemetry signals.
//
// Telemetry is a disposable projection (FR-CTRL-007, FR-OBS-001/002):
//   - it MUST NOT be consulted by the kernel for decisions;
//   - it MUST NOT mutate canonical store state;
//   - secrets and prompt bodies are never recorded as attributes;
//   - when disabled, every call is a no-op using the global noop providers.
package observability

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	instrumentationName    = "motor-autonomo/observability"
	instrumentationVersion = "0.1.0"
	defaultServiceName     = "motor-autonomo"
)

// Config enables optional export. Zero value keeps telemetry disabled.
type Config struct {
	Enabled        bool
	ServiceName    string
	ServiceVersion string
	// Environment is an optional deployment label (e.g. "dev").
	Environment string
	// OTLPEndpoint is host:port or URL accepted by the OTLP HTTP exporter.
	// Empty means in-process only (useful with a custom SpanProcessor/exporter in tests).
	OTLPEndpoint string
	// Insecure disables TLS for OTLP HTTP (local collectors only).
	Insecure bool
	// SampleRatio is in [0,1]. Zero with Enabled keeps traces off; use 1 for always-on.
	SampleRatio float64
	// Retention bounds disposable OTLP export buffers (not store retention).
	// Zero fields apply package defaults when OTLP export is active.
	Retention ExportRetention
}

func (c Config) Validate() error {
	if !c.Enabled {
		// Still validate retention when partially filled so bad flags fail early.
		return c.Retention.Validate()
	}
	if c.SampleRatio < 0 || c.SampleRatio > 1 {
		return errors.New("observability sample ratio must be between 0 and 1")
	}
	if strings.ContainsAny(c.ServiceName, "\r\n") || strings.ContainsAny(c.ServiceVersion, "\r\n") {
		return errors.New("observability service labels must be single-line")
	}
	if err := c.Retention.Validate(); err != nil {
		return err
	}
	return nil
}

// Runtime owns optional SDK providers. Close flushes exporters.
type Runtime struct {
	mu             sync.Mutex
	enabled        bool
	hasOTLP        bool
	retention      ExportRetention
	tracer         trace.Tracer
	meter          metric.Meter
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	shutdowns      []func(context.Context) error
}

// Setup installs optional providers. When disabled, returns a Runtime that uses
// the global noop tracer/meter and never registers global OTel providers.
func Setup(ctx context.Context, cfg Config, opts ...Option) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	options := options{}
	for _, opt := range opts {
		opt(&options)
	}
	retention := cfg.Retention.Normalize()
	rt := &Runtime{
		retention: retention,
		tracer:    noop.NewTracerProvider().Tracer(instrumentationName),
		meter:     otel.Meter(instrumentationName),
	}
	if !cfg.Enabled {
		return rt, nil
	}

	service := strings.TrimSpace(cfg.ServiceName)
	if service == "" {
		service = defaultServiceName
	}
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(service),
			semconv.ServiceVersion(strings.TrimSpace(cfg.ServiceVersion)),
			attribute.String("deployment.environment", strings.TrimSpace(cfg.Environment)),
			attribute.String("motor.observability.role", "derived_export"),
			attribute.Bool("motor.observability.canonical", false),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("observability resource: %w", err)
	}

	var spanProcessors []sdktrace.SpanProcessor
	if options.spanExporter != nil {
		// TracerProvider.Shutdown already shuts down processors (and their exporters).
		// Do not also register exporter.Shutdown here: tracetest.InMemoryExporter.Shutdown
		// clears captured spans and would race test assertions.
		spanProcessors = append(spanProcessors, sdktrace.NewSimpleSpanProcessor(options.spanExporter))
	}
	if strings.TrimSpace(cfg.OTLPEndpoint) != "" {
		rt.hasOTLP = true
		traceOpts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.OTLPEndpoint)}
		if cfg.Insecure {
			traceOpts = append(traceOpts, otlptracehttp.WithInsecure())
		}
		otlpTrace, err := otlptracehttp.New(ctx, traceOpts...)
		if err != nil {
			return nil, fmt.Errorf("observability otlp trace exporter: %w", err)
		}
		spanProcessors = append(spanProcessors, sdktrace.NewBatchSpanProcessor(
			otlpTrace,
			sdktrace.WithMaxQueueSize(retention.TraceMaxQueueSize),
			sdktrace.WithMaxExportBatchSize(retention.TraceMaxExportBatchSize),
			sdktrace.WithBatchTimeout(retention.TraceBatchTimeout),
			sdktrace.WithExportTimeout(retention.TraceExportTimeout),
		))
		rt.shutdowns = append(rt.shutdowns, otlpTrace.Shutdown)
	}
	if len(spanProcessors) == 0 {
		// Enabled without exporter is still useful for local Tracer injection
		// and metric instruments; spans remain in-process until a processor is added.
		spanProcessors = append(spanProcessors, sdktrace.NewSimpleSpanProcessor(noopSpanExporter{}))
	}

	ratio := cfg.SampleRatio
	if ratio == 0 {
		ratio = 1
	}
	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	}
	for _, sp := range spanProcessors {
		tpOpts = append(tpOpts, sdktrace.WithSpanProcessor(sp))
	}
	tp := sdktrace.NewTracerProvider(tpOpts...)

	var metricReaders []sdkmetric.Option
	metricReaders = append(metricReaders, sdkmetric.WithResource(res))
	if options.metricReader != nil {
		metricReaders = append(metricReaders, sdkmetric.WithReader(options.metricReader))
	}
	if strings.TrimSpace(cfg.OTLPEndpoint) != "" {
		metricOpts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(cfg.OTLPEndpoint)}
		if cfg.Insecure {
			metricOpts = append(metricOpts, otlpmetrichttp.WithInsecure())
		}
		otlpMetric, err := otlpmetrichttp.New(ctx, metricOpts...)
		if err != nil {
			_ = tp.Shutdown(ctx)
			return nil, fmt.Errorf("observability otlp metric exporter: %w", err)
		}
		// PeriodicReader owns exporter shutdown via MeterProvider.Shutdown.
		metricReaders = append(metricReaders, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			otlpMetric,
			sdkmetric.WithInterval(retention.MetricInterval),
			sdkmetric.WithTimeout(retention.MetricExportTimeout),
		)))
	}
	mp := sdkmetric.NewMeterProvider(metricReaders...)

	rt.enabled = true
	rt.tracerProvider = tp
	rt.meterProvider = mp
	rt.tracer = tp.Tracer(instrumentationName, trace.WithInstrumentationVersion(instrumentationVersion))
	rt.meter = mp.Meter(instrumentationName, metric.WithInstrumentationVersion(instrumentationVersion))
	rt.shutdowns = append(rt.shutdowns, tp.Shutdown, mp.Shutdown)
	return rt, nil
}

// Option customizes Setup for tests (e.g. in-memory exporters).
type Option func(*options)

type options struct {
	spanExporter sdktrace.SpanExporter
	metricReader sdkmetric.Reader
}

// WithSpanExporter installs a process-local span exporter (tests).
func WithSpanExporter(exporter sdktrace.SpanExporter) Option {
	return func(o *options) {
		o.spanExporter = exporter
	}
}

// WithMetricReader installs a process-local metric reader (tests).
func WithMetricReader(reader sdkmetric.Reader) Option {
	return func(o *options) {
		o.metricReader = reader
	}
}

// Enabled reports whether derived export is active.
func (r *Runtime) Enabled() bool {
	if r == nil {
		return false
	}
	return r.enabled
}

// HasOTLP reports whether an OTLP HTTP exporter was configured (remote path).
func (r *Runtime) HasOTLP() bool {
	if r == nil {
		return false
	}
	return r.hasOTLP
}

// Retention returns the effective export-buffer policy (normalized defaults).
// Disposable only: never consult from kernel decision paths.
func (r *Runtime) Retention() ExportRetention {
	if r == nil {
		return ExportRetention{}.Normalize()
	}
	return r.retention.Normalize()
}

// RetentionView is the presentation-safe export retention snapshot.
func (r *Runtime) RetentionView() RetentionView {
	return r.Retention().View()
}

// Tracer returns the package tracer (noop when disabled).
func (r *Runtime) Tracer() trace.Tracer {
	if r == nil || r.tracer == nil {
		return noop.NewTracerProvider().Tracer(instrumentationName)
	}
	return r.tracer
}

// Meter returns the package meter.
func (r *Runtime) Meter() metric.Meter {
	if r == nil || r.meter == nil {
		return otel.Meter(instrumentationName)
	}
	return r.meter
}

// Shutdown flushes exporters. Safe on nil/disabled runtimes.
func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var first error
	for i := len(r.shutdowns) - 1; i >= 0; i-- {
		if err := r.shutdowns[i](ctx); err != nil && first == nil {
			first = err
		}
	}
	r.shutdowns = nil
	r.enabled = false
	r.tracer = noop.NewTracerProvider().Tracer(instrumentationName)
	return first
}

// StartSpan is a convenience wrapper that never panics on a nil runtime.
func (r *Runtime) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return r.Tracer().Start(ctx, name, opts...)
}

// RecordError marks the span failed without attaching free-form bodies.
func RecordError(span trace.Span, err error, code string) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err, trace.WithAttributes(attribute.String("motor.error.kind", sanitizeCode(code))))
	span.SetStatus(codes.Error, sanitizeCode(code))
}

func sanitizeCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return "ERROR"
	}
	if len(code) > 64 {
		return code[:64]
	}
	return code
}

// Common attributes used by instrumented adapters. Never include secrets,
// Authorization headers, raw prompts, or full model completions.
func ModelCallAttributes(provider, model string, maxOutputTokens int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("gen_ai.system", "openai_compatible"),
		attribute.String("gen_ai.operation.name", "chat"),
		attribute.String("motor.provider.adapter", sanitizeLabel(provider, "provider")),
		attribute.Bool("motor.telemetry.canonical", false),
	}
	if model != "" {
		attrs = append(attrs, attribute.String("gen_ai.request.model", sanitizeLabel(model, "model")))
	}
	if maxOutputTokens > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.request.max_tokens", maxOutputTokens))
	}
	return attrs
}

func ControlAttributes(kind, missionID, correlation string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("motor.control.kind", sanitizeLabel(kind, "kind")),
		attribute.Bool("motor.telemetry.canonical", false),
	}
	if missionID != "" {
		attrs = append(attrs, attribute.String("motor.mission.id", sanitizeLabel(missionID, "mission")))
	}
	if correlation != "" {
		attrs = append(attrs, attribute.String("motor.correlation.id", sanitizeLabel(correlation, "correlation")))
	}
	return attrs
}

func sanitizeLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	// Hard cap prevents accidental prompt-sized attributes.
	if len(value) > 128 {
		return value[:128]
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "bearer ") || strings.Contains(lower, "api_key") || strings.Contains(lower, "authorization") {
		return "[redacted]"
	}
	return value
}

// noopSpanExporter satisfies sdktrace.SpanExporter when no remote export is configured.
type noopSpanExporter struct{}

func (noopSpanExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error { return nil }
func (noopSpanExporter) Shutdown(context.Context) error                             { return nil }

// Ensure time import stays useful for future metric views without churn.
var _ = time.Second
