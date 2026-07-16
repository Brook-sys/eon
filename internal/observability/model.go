package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// ModelProvider decorates a text→text provider with optional spans/metrics.
// Failures of telemetry never change the underlying completion result.
type ModelProvider struct {
	Inner     port.ModelProvider
	Runtime   *Runtime
	Name      string
	calls     metric.Int64Counter
	tokensIn  metric.Int64Counter
	tokensOut metric.Int64Counter
}

// InstrumentModel wraps provider. A nil runtime or disabled runtime is safe.
func InstrumentModel(inner port.ModelProvider, runtime *Runtime, name string) port.ModelProvider {
	if inner == nil {
		return nil
	}
	if name == "" {
		name = "model"
	}
	mp := &ModelProvider{Inner: inner, Runtime: runtime, Name: name}
	if runtime != nil && runtime.Enabled() {
		meter := runtime.Meter()
		if c, err := meter.Int64Counter("motor.model.calls"); err == nil {
			mp.calls = c
		}
		if c, err := meter.Int64Counter("motor.model.tokens.input"); err == nil {
			mp.tokensIn = c
		}
		if c, err := meter.Int64Counter("motor.model.tokens.output"); err == nil {
			mp.tokensOut = c
		}
	}
	return mp
}

func (p *ModelProvider) Complete(ctx context.Context, request port.CompletionRequest) (port.CompletionResult, error) {
	if p == nil || p.Inner == nil {
		return port.CompletionResult{}, errMissingProvider
	}
	ctx, span := p.Runtime.StartSpan(ctx, "model.complete",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(ModelCallAttributes(p.Name, "", request.MaxOutputTokens)...),
	)
	defer span.End()

	// Never put the prompt or temperature-sensitive free text on the span.
	span.SetAttributes(attribute.Int("motor.prompt.chars", len(request.Prompt)))

	result, err := p.Inner.Complete(ctx, request)
	if err != nil {
		RecordError(span, err, errorKind(err))
		if p.calls != nil {
			p.calls.Add(ctx, 1, metric.WithAttributes(
				attribute.String("motor.provider.adapter", sanitizeLabel(p.Name, "provider")),
				attribute.String("motor.outcome", "error"),
			))
		}
		return port.CompletionResult{}, err
	}
	span.SetStatus(codes.Ok, "")
	span.SetAttributes(
		attribute.String("gen_ai.response.model", sanitizeLabel(result.Model, "model")),
		attribute.Int("gen_ai.usage.input_tokens", result.InputTokens),
		attribute.Int("gen_ai.usage.output_tokens", result.OutputTokens),
		attribute.Int("motor.completion.chars", len(result.Text)),
	)
	if p.calls != nil {
		p.calls.Add(ctx, 1, metric.WithAttributes(
			attribute.String("motor.provider.adapter", sanitizeLabel(p.Name, "provider")),
			attribute.String("motor.outcome", "ok"),
		))
	}
	if p.tokensIn != nil {
		p.tokensIn.Add(ctx, int64(result.InputTokens), metric.WithAttributes(
			attribute.String("motor.provider.adapter", sanitizeLabel(p.Name, "provider")),
		))
	}
	if p.tokensOut != nil {
		p.tokensOut.Add(ctx, int64(result.OutputTokens), metric.WithAttributes(
			attribute.String("motor.provider.adapter", sanitizeLabel(p.Name, "provider")),
		))
	}
	return result, nil
}

// DeclaredProfile forwards capability snapshots when the inner provider reports them.
// When the inner adapter does not implement ModelCapabilityReporter, a valid
// unknown profile is returned so inspect never invents support for features.
func (p *ModelProvider) DeclaredProfile() domain.ProviderProfile {
	if p == nil || p.Inner == nil {
		return unknownProviderProfile("missing", "observability decorator has no inner provider")
	}
	if reporter, ok := p.Inner.(port.ModelCapabilityReporter); ok {
		return reporter.DeclaredProfile()
	}
	name := p.Name
	if name == "" {
		name = "model"
	}
	return unknownProviderProfile(name, "inner model provider does not report capabilities")
}

// Probe forwards budgeted probes when the inner provider reports capabilities.
func (p *ModelProvider) Probe(ctx context.Context) (domain.ProviderProfile, error) {
	if p == nil || p.Inner == nil {
		return domain.ProviderProfile{}, errMissingProvider
	}
	if reporter, ok := p.Inner.(port.ModelCapabilityReporter); ok {
		return reporter.Probe(ctx)
	}
	return domain.ProviderProfile{}, errString("inner model provider does not report capabilities")
}

func unknownProviderProfile(name, detail string) domain.ProviderProfile {
	return domain.ProviderProfile{
		SchemaVersion:    domain.SchemaVersionV1,
		Name:             name,
		APIStyle:         domain.APIStyleChatCompletions,
		MaxOutputDialect: domain.MaxOutputDialectLegacy,
		Source:           domain.CapabilityUnknown,
		SafeDetail:       detail,
		PolicyVersion:    "provider-profile@1",
	}
}

// Compile-time: decorator exposes the optional capability surface.
var _ port.ModelCapabilityReporter = (*ModelProvider)(nil)

var errMissingProvider = errString("observability model provider is missing")

type errString string

func (e errString) Error() string { return string(e) }

func errorKind(err error) string {
	if err == nil {
		return ""
	}
	// Prefer a short kind when adapters expose one without importing them.
	type withKind interface{ Kind() string }
	if k, ok := err.(withKind); ok {
		return sanitizeCode(k.Kind())
	}
	// openai.Error formats as "openai-compatible provider: KIND ..."; keep it bounded.
	msg := err.Error()
	if len(msg) > 64 {
		msg = msg[:64]
	}
	return msg
}
