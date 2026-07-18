package evaluation

import (
	"fmt"
	"strings"
	"testing"
)

func TestDecodeCampaignManifestAndCallBound(t *testing.T) {
	body := `{
		"schema_version":1,"name":"nightly","fixture_path":"testdata/cognitive-v1.json",
		"context_tokens":[2048],"max_calls":22,"max_output_tokens":256,"max_total_output_tokens":5632,"timeout_seconds":600,
		"models":[
			{"provider":"groq","binding_id":"groq-8b","base_url":"https://api.groq.com/openai","model":"model-a","api_key_env":"GROQ_API_KEY","max_output_field":"max_tokens"},
			{"provider":"nvidia_nim","binding_id":"nim-8b","base_url":"https://integrate.api.nvidia.com","model":"model-b","api_key_env":"NVIDIA_API_KEY","max_output_field":"max_tokens"}
		]
	}`
	manifest, err := DecodeCampaignManifest(strings.NewReader(body), 16<<10)
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := LoadEmbeddedCognitiveV1()
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.PlannedCalls(fixtures); got != 22 {
		t.Fatalf("planned calls=%d want 22", got)
	}
}

func TestCampaignManifestRejectsUnsafeOrUnboundedValues(t *testing.T) {
	base := `{"schema_version":1,"name":"x","fixture_path":"f","context_tokens":[2048],"max_calls":1,"max_output_tokens":1,"max_total_output_tokens":1,"timeout_seconds":1,"models":[{"provider":"groq","binding_id":"%s","base_url":"https://example.test","model":"m","api_key_env":"KEY","max_output_field":"max_tokens"}]}`
	for _, binding := range []string{"../escape", "has/slash", ""} {
		if _, err := DecodeCampaignManifest(strings.NewReader(fmt.Sprintf(base, binding)), 16<<10); err == nil {
			t.Fatalf("expected binding %q to fail", binding)
		}
	}
	if _, err := DecodeCampaignManifest(strings.NewReader(strings.Replace(fmt.Sprintf(base, "ok"), `"max_calls":1`, `"max_calls":0`, 1)), 16<<10); err == nil {
		t.Fatal("expected zero max_calls to fail")
	}
}

func TestCompareReportsFindsPerDimensionRegression(t *testing.T) {
	baselineRun := Run{CaseID: "c", Operation: OperationConflict, Format: FormatJSON, ContextTokens: 2048, Compiled: true, SyntaxValid: true, SemanticallyCorrect: true}
	currentRun := baselineRun
	currentRun.SyntaxValid = false
	currentRun.SemanticallyCorrect = false
	currentRun.ErrorKind = "VALIDATION"
	baseline := Report{SchemaVersion: 1, FixtureName: "f", Runs: []Run{baselineRun}, Breakdown: summarizeRuns([]Run{baselineRun})}
	current := Report{SchemaVersion: 1, FixtureName: "f", Runs: []Run{currentRun}, Breakdown: summarizeRuns([]Run{currentRun})}
	got, err := CompareReports(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("regressions=%+v", got)
	}
	if got[0].Delta != -1 {
		t.Fatalf("first regression=%+v", got[0])
	}
}

func TestCompareReportsUsesRatesAcrossExpandedFixture(t *testing.T) {
	baselineRun := Run{CaseID: "old", Operation: OperationExtract, Format: FormatJSON, ContextTokens: 2048, Compiled: true, SyntaxValid: true, SemanticallyCorrect: true}
	currentRuns := []Run{
		{CaseID: "old", Operation: OperationExtract, Format: FormatJSON, ContextTokens: 2048, Compiled: true, SyntaxValid: true, SemanticallyCorrect: true},
		{CaseID: "new", Operation: OperationExtract, Format: FormatJSON, ContextTokens: 2048, Compiled: true, SyntaxValid: false, SemanticallyCorrect: false, ErrorKind: "VALIDATION"},
	}
	baseline := Report{SchemaVersion: 1, FixtureName: "cognitive-v1", Runs: []Run{baselineRun}, Breakdown: summarizeRuns([]Run{baselineRun})}
	current := Report{SchemaVersion: 1, FixtureName: "cognitive-v2", Runs: currentRuns, Breakdown: summarizeRuns(currentRuns)}
	got, err := CompareReports(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("regressions=%+v", got)
	}
	if got[0].BaselineTotal != 1 || got[0].CurrentTotal != 2 {
		t.Fatalf("regression totals=%+v", got[0])
	}
}

func TestQualifyReportUsesConservativeThresholds(t *testing.T) {
	tests := []struct {
		name    string
		summary Summary
		want    QualificationVerdict
	}{
		{name: "qualified", summary: Summary{Total: 33, SyntaxValid: 30, SemanticallyRight: 24}, want: QualificationQualified},
		{name: "degraded", summary: Summary{Total: 33, SyntaxValid: 25, SemanticallyRight: 19, ProviderErrors: 4}, want: QualificationDegraded},
		{name: "provider incompatible", summary: Summary{Total: 33, ProviderErrors: 17}, want: QualificationIncompatible},
		{name: "syntax incompatible", summary: Summary{Total: 33, SyntaxValid: 16, SemanticallyRight: 10}, want: QualificationIncompatible},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QualifyReport(Report{SchemaVersion: 1, Summary: tt.summary})
			if got.Verdict != tt.want || got.Reason == "" {
				t.Fatalf("qualification=%+v want=%s", got, tt.want)
			}
		})
	}
}
