package gatecampaign

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/port"
)

func TestBuildRuntimeGateBatchReportAggregatesIsolatedTrials(t *testing.T) {
	reports := []RuntimeGateCampaignReport{
		{SchemaVersion: 1, MaxCalls: 1, ExternalCalls: 1, ProviderSucceeded: true, DurableReopen: true, ExpectedResponseMatch: true, ResponseJSONValid: true, SchemaAdherence: &SchemaAdherenceReport{SchemaValid: true, FieldsChecked: 12, FieldsPresent: 12, FieldsCorrectType: 12, ChangesValid: true, ChangesChecked: 1, ChangesWithAllFields: 1}, ResponseFramingClass: "exact", SelectedBindingID: "groq", FinishReason: port.CompletionFinishStop, SecondAcquireReason: "resource_rate", ObservedInputTokens: 10, ObservedOutputTokens: 2, ProviderLatency: 200 * time.Millisecond},
		{SchemaVersion: 1, MaxCalls: 1, ExternalCalls: 1, ProviderSucceeded: true, DurableReopen: true, ExpectedResponseMatch: false, ResponseFramingClass: "markdown_fence", SelectedBindingID: "groq", FinishReason: port.CompletionFinishStop, SecondAcquireReason: "resource_rate", ObservedInputTokens: 11, ObservedOutputTokens: 4, ProviderLatency: 400 * time.Millisecond},
	}
	batch, err := BuildRuntimeGateBatchReport("repeatability", reports)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Trials != 2 || batch.ExternalCalls != 2 || batch.ProviderSuccesses != 2 || batch.ExecutionFailures != 0 || batch.DurableReopens != 2 || batch.ExpectedMatches != 1 || batch.JSONValid != 1 {
		t.Fatalf("batch counts=%+v", batch)
	}
	if batch.SchemaEvaluated != 1 || batch.SchemaAdherent != 1 || batch.ChangesValid != 1 {
		t.Fatalf("batch schema counts=%+v", batch)
	}
	if batch.InputTokens != 21 || batch.OutputTokens != 6 || batch.LatencyP50 != 200*time.Millisecond || batch.LatencyP95 != 400*time.Millisecond || batch.LatencyMax != 400*time.Millisecond {
		t.Fatalf("batch metrics=%+v", batch)
	}
	directory := t.TempDir()
	if err := WriteRuntimeGateBatchArtifacts(directory, batch); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(directory, "runtime-gate-batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"external_calls": 2`) {
		t.Fatalf("artifact=%s", body)
	}
	if !strings.Contains(string(body), `"execution_failures": 0`) {
		t.Fatalf("batch artifact missing execution_failures=%s", body)
	}
	if !strings.Contains(string(body), `"schema_adherent": 1`) {
		t.Fatalf("batch artifact missing schema adherence=%s", body)
	}
}

func TestBuildRuntimeGateBatchReportAggregatesMixedSuccessAndFailure(t *testing.T) {
	reports := []RuntimeGateCampaignReport{
		{SchemaVersion: 1, MaxCalls: 1, ExternalCalls: 1, ProviderSucceeded: true, DurableReopen: true, ResponseJSONValid: true, SchemaAdherence: &SchemaAdherenceReport{SchemaValid: false, FieldsChecked: 12, FieldsPresent: 12, FieldsCorrectType: 11, ChangesValid: true, ChangesChecked: 1, ChangesWithAllFields: 1}, ResponseFramingClass: "valid_json_mismatch", SelectedBindingID: "groq", FinishReason: port.CompletionFinishStop, SecondAcquireReason: "resource_rate", ObservedInputTokens: 100, ObservedOutputTokens: 20, ProviderLatency: 500 * time.Millisecond},
		{SchemaVersion: 1, MaxCalls: 1, ExternalCalls: 1, ProviderSucceeded: true, DurableReopen: false, ResponseJSONValid: false, ResponseFramingClass: "invalid_json", SelectedBindingID: "groq", FinishReason: port.CompletionFinishLength, SecondAcquireReason: "", ExecutionError: "model recovery exhausted: invalid character '<'", ObservedInputTokens: 200, ObservedOutputTokens: 384, ProviderLatency: 1000 * time.Millisecond},
	}
	batch, err := BuildRuntimeGateBatchReport("mixed", reports)
	if err != nil {
		t.Fatal(err)
	}
	if batch.ProviderSuccesses != 2 || batch.ExecutionFailures != 1 || batch.DurableReopens != 1 || batch.JSONValid != 1 {
		t.Fatalf("batch counts=%+v", batch)
	}
	if batch.SchemaEvaluated != 1 || batch.SchemaAdherent != 0 || batch.ChangesValid != 1 {
		t.Fatalf("batch schema counts=%+v", batch)
	}
}

func TestBuildRuntimeGateBatchReportRejectsIncompleteOrUnboundedInput(t *testing.T) {
	valid := RuntimeGateCampaignReport{SchemaVersion: 1, MaxCalls: 1, ExternalCalls: 1}
	if _, err := BuildRuntimeGateBatchReport("one", []RuntimeGateCampaignReport{valid}); err == nil {
		t.Fatal("single trial must not masquerade as batch")
	}
	invalid := valid
	invalid.ExternalCalls = 2
	if _, err := BuildRuntimeGateBatchReport("bad", []RuntimeGateCampaignReport{valid, invalid}); err == nil {
		t.Fatal("over-budget trial must fail")
	}
}
