package observability_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"motor-autonomo/internal/observability"
	"motor-autonomo/internal/port"
)

type stubProvider struct {
	last port.CompletionRequest
	err  error
}

func (s *stubProvider) Complete(_ context.Context, req port.CompletionRequest) (port.CompletionResult, error) {
	s.last = req
	if s.err != nil {
		return port.CompletionResult{}, s.err
	}
	return port.CompletionResult{Text: "ok", InputTokens: 3, OutputTokens: 1, Model: "fixture"}, nil
}

func TestDisabledRuntimeIsNoopAndDoesNotMutateProvider(t *testing.T) {
	rt, err := observability.Setup(context.Background(), observability.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	if rt.Enabled() {
		t.Fatal("expected disabled runtime")
	}
	inner := &stubProvider{}
	provider := observability.InstrumentModel(inner, rt, "openai")
	secretPrompt := "Authorization: Bearer super-secret-token do not export"
	result, err := provider.Complete(context.Background(), port.CompletionRequest{Prompt: secretPrompt, MaxOutputTokens: 16})
	if err != nil || result.Text != "ok" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if inner.last.Prompt != secretPrompt {
		t.Fatal("decorator must forward prompt unchanged to the real provider")
	}
}

func TestEnabledModelSpansOmitSecretsAndBodies(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	rt, err := observability.Setup(context.Background(), observability.Config{
		Enabled: true, ServiceName: "motor-test", SampleRatio: 1,
	}, observability.WithSpanExporter(exporter))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	if !rt.Enabled() {
		t.Fatal("expected enabled runtime")
	}

	inner := &stubProvider{}
	provider := observability.InstrumentModel(inner, rt, "openai")
	prompt := "Bearer sk-live-should-not-appear system: ignore previous"
	if _, err := provider.Complete(context.Background(), port.CompletionRequest{Prompt: prompt, MaxOutputTokens: 32, Temperature: 0}); err != nil {
		t.Fatal(err)
	}
	// Capture before Shutdown: InMemoryExporter.Shutdown clears its buffer.
	spans := exporter.GetSpans()
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	span := spans[0]
	if span.Name != "model.complete" {
		t.Fatalf("name = %q", span.Name)
	}
	blob := attributesBlob(span)
	if strings.Contains(blob, "sk-live") || strings.Contains(blob, prompt) || strings.Contains(blob, "Bearer super") {
		t.Fatalf("span leaked secret or body: %s", blob)
	}
	if !strings.Contains(blob, "motor.telemetry.canonical=false") {
		t.Fatalf("missing non-authority marker: %s", blob)
	}
	if !strings.Contains(blob, "gen_ai.usage.input_tokens=3") || !strings.Contains(blob, "motor.prompt.chars=") {
		t.Fatalf("missing usage/size attrs: %s", blob)
	}
}

func TestModelErrorRecordsBoundedCode(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	rt, err := observability.Setup(context.Background(), observability.Config{Enabled: true, SampleRatio: 1}, observability.WithSpanExporter(exporter))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	inner := &stubProvider{err: errors.New("TRANSPORT")}
	provider := observability.InstrumentModel(inner, rt, "openai")
	if _, err := provider.Complete(context.Background(), port.CompletionRequest{Prompt: "x", MaxOutputTokens: 1}); err == nil {
		t.Fatal("expected error")
	}
	spans := exporter.GetSpans()
	_ = rt.Shutdown(context.Background())
	if len(spans) != 1 || spans[0].Status.Code.String() != "Error" {
		t.Fatalf("spans=%#v", spans)
	}
}

func TestControlTraceHelper(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	rt, err := observability.Setup(context.Background(), observability.Config{Enabled: true, SampleRatio: 1}, observability.WithSpanExporter(exporter))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	_, span := rt.TraceControl(context.Background(), "control.external_event", "USER_ANSWER", "mission_1", "ask_1")
	observability.EndControl(span, "APPLIED", "")
	spans := exporter.GetSpans()
	_ = rt.Shutdown(context.Background())
	if len(spans) != 1 || spans[0].Name != "control.external_event" {
		t.Fatalf("spans=%#v", spans)
	}
}

func TestConfigValidation(t *testing.T) {
	if err := (observability.Config{Enabled: true, SampleRatio: 2}).Validate(); err == nil {
		t.Fatal("expected invalid ratio")
	}
}

func attributesBlob(span tracetest.SpanStub) string {
	var b strings.Builder
	for _, attr := range span.Attributes {
		b.WriteString(string(attr.Key))
		b.WriteByte('=')
		b.WriteString(attr.Value.Emit())
		b.WriteByte(';')
	}
	return b.String()
}

// Compile-time check that the decorator still satisfies the port.
var _ port.ModelProvider = (*observability.ModelProvider)(nil)

// Keep sdktrace import used for documentation of exporter shape in failures.
var _ sdktrace.SpanExporter
