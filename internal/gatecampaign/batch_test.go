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
		{SchemaVersion: 1, MaxCalls: 1, ExternalCalls: 1, ProviderSucceeded: true, DurableReopen: true, ExpectedResponseMatch: true, ResponseJSONValid: true, ResponseFramingClass: "exact", SelectedBindingID: "groq", FinishReason: port.CompletionFinishStop, SecondAcquireReason: "resource_rate", ObservedInputTokens: 10, ObservedOutputTokens: 2, ProviderLatency: 200 * time.Millisecond},
		{SchemaVersion: 1, MaxCalls: 1, ExternalCalls: 1, ProviderSucceeded: true, DurableReopen: true, ExpectedResponseMatch: false, ResponseFramingClass: "markdown_fence", SelectedBindingID: "groq", FinishReason: port.CompletionFinishStop, SecondAcquireReason: "resource_rate", ObservedInputTokens: 11, ObservedOutputTokens: 4, ProviderLatency: 400 * time.Millisecond},
	}
	batch, err := BuildRuntimeGateBatchReport("repeatability", reports)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Trials != 2 || batch.ExternalCalls != 2 || batch.ProviderSuccesses != 2 || batch.DurableReopens != 2 || batch.ExpectedMatches != 1 || batch.JSONValid != 1 {
		t.Fatalf("batch counts=%+v", batch)
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
