package observability_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"motor-autonomo/internal/domain"
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

type stubCommandProcessor struct {
	next    domain.CommandReceipt
	ok      bool
	err     error
	process domain.CommandReceipt
	pErr    error
	seenID  domain.CommandID
}

func (s *stubCommandProcessor) ProcessNext(context.Context) (domain.CommandReceipt, bool, error) {
	return s.next, s.ok, s.err
}
func (s *stubCommandProcessor) Process(_ context.Context, id domain.CommandID) (domain.CommandReceipt, error) {
	s.seenID = id
	return s.process, s.pErr
}

type stubEventProcessor struct {
	next    domain.ExternalEventDisposition
	ok      bool
	err     error
	process domain.ExternalEventDisposition
	pErr    error
}

func (s *stubEventProcessor) ProcessNext(context.Context) (domain.ExternalEventDisposition, bool, error) {
	return s.next, s.ok, s.err
}
func (s *stubEventProcessor) Process(context.Context, domain.ExternalEventID) (domain.ExternalEventDisposition, error) {
	return s.process, s.pErr
}

func TestInstrumentCommandEmitsSpanWithoutBodies(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	rt, err := observability.Setup(context.Background(), observability.Config{Enabled: true, SampleRatio: 1}, observability.WithSpanExporter(exporter))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	inner := &stubCommandProcessor{
		next: domain.CommandReceipt{
			CommandID:   "cmd_1",
			State:       domain.CommandApplied,
			ResultRef:   "mission@1",
			FailureCode: "",
		},
		ok: true,
	}
	proc := observability.InstrumentCommand(inner, rt)
	receipt, ok, err := proc.ProcessNext(context.Background())
	if err != nil || !ok || receipt.CommandID != "cmd_1" {
		t.Fatalf("receipt=%#v ok=%v err=%v", receipt, ok, err)
	}
	// Idle drain must not export a span.
	inner.ok = false
	if _, ok, err := proc.ProcessNext(context.Background()); ok || err != nil {
		t.Fatalf("idle peek should be empty, ok=%v err=%v", ok, err)
	}
	spans := exporter.GetSpans()
	_ = rt.Shutdown(context.Background())
	if len(spans) != 1 || spans[0].Name != "control.command.process" {
		t.Fatalf("spans=%#v", spans)
	}
	blob := attributesBlob(spans[0])
	if strings.Contains(blob, "Bearer ") || !strings.Contains(blob, "motor.telemetry.canonical=false") {
		t.Fatalf("attrs=%s", blob)
	}
	if !strings.Contains(blob, "motor.command.id=cmd_1") {
		t.Fatalf("missing command id: %s", blob)
	}
}

func TestInstrumentExternalEventProcessPath(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	rt, err := observability.Setup(context.Background(), observability.Config{Enabled: true, SampleRatio: 1}, observability.WithSpanExporter(exporter))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	inner := &stubEventProcessor{
		process: domain.ExternalEventDisposition{
			EventID: "evt_1", State: domain.ExternalEventRejected, FailureCode: "UNKNOWN_KIND",
		},
	}
	proc := observability.InstrumentExternalEvent(inner, rt)
	disp, err := proc.Process(context.Background(), "evt_1")
	if err != nil || disp.State != domain.ExternalEventRejected {
		t.Fatalf("disp=%#v err=%v", disp, err)
	}
	spans := exporter.GetSpans()
	_ = rt.Shutdown(context.Background())
	if len(spans) != 1 || spans[0].Name != "control.external_event.process" {
		t.Fatalf("spans=%#v", spans)
	}
	blob := attributesBlob(spans[0])
	if !strings.Contains(blob, "motor.control.failure_code=UNKNOWN_KIND") {
		t.Fatalf("attrs=%s", blob)
	}
}

func TestCycleInstrumentsDisabledNoop(t *testing.T) {
	rt, err := observability.Setup(context.Background(), observability.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	ci := observability.NewCycleInstruments(rt)
	ci.Record(context.Background(), observability.CycleSnapshot{Outcome: "worked", CommandsProcessed: 2, SchedulerRan: true, SchedulerKind: "DISPATCH"})
}

func TestDisabledProcessorPassthrough(t *testing.T) {
	rt, err := observability.Setup(context.Background(), observability.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	inner := &stubCommandProcessor{
		process: domain.CommandReceipt{CommandID: "cmd_x", State: domain.CommandApplied, ResultRef: "r"},
	}
	proc := observability.InstrumentCommand(inner, rt)
	got, err := proc.Process(context.Background(), "cmd_x")
	if err != nil || got.CommandID != "cmd_x" || inner.seenID != "cmd_x" {
		t.Fatalf("got=%#v err=%v seen=%q", got, err, inner.seenID)
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
