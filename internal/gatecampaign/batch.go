package gatecampaign

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const MaxRuntimeGateBatchTrials = 5

// RuntimeGateBatchReport aggregates isolated one-call trials. Each trial keeps
// the existing fail-closed ResourceGate and durable-reopen proof; the batch
// adds distribution and repeatability evidence without sharing quota state.
type RuntimeGateBatchReport struct {
	SchemaVersion         int            `json:"schema_version"`
	Name                  string         `json:"name"`
	Trials                int            `json:"trials"`
	ExternalCalls         int            `json:"external_calls"`
	ProviderSuccesses     int            `json:"provider_successes"`
	ExecutionFailures     int            `json:"execution_failures"`
	DurableReopens        int            `json:"durable_reopens"`
	ExpectedMatches       int            `json:"expected_matches"`
	JSONValid             int            `json:"json_valid"`
	SchemaEvaluated       int            `json:"schema_evaluated"`
	SchemaAdherent        int            `json:"schema_adherent"`
	SchemaContentComplete int            `json:"schema_content_complete"`
	ChangesValid          int            `json:"changes_valid"`
	InputTokens           int            `json:"input_tokens"`
	OutputTokens          int            `json:"output_tokens"`
	LatencyP50            time.Duration  `json:"latency_p50"`
	LatencyP95            time.Duration  `json:"latency_p95"`
	LatencyMax            time.Duration  `json:"latency_max"`
	SelectedBindings      map[string]int `json:"selected_bindings"`
	FinishReasons         map[string]int `json:"finish_reasons"`
	FramingClasses        map[string]int `json:"framing_classes,omitempty"`
	ProviderHTTPStatuses  map[string]int `json:"provider_http_statuses,omitempty"`
	SecondAcquireReasons  map[string]int `json:"second_acquire_reasons"`
}

func BuildRuntimeGateBatchReport(name string, reports []RuntimeGateCampaignReport) (RuntimeGateBatchReport, error) {
	if strings.TrimSpace(name) == "" || len(reports) < 2 || len(reports) > MaxRuntimeGateBatchTrials {
		return RuntimeGateBatchReport{}, fmt.Errorf("runtime gate batch requires a name and 2..%d trials", MaxRuntimeGateBatchTrials)
	}
	batch := RuntimeGateBatchReport{
		SchemaVersion:    RuntimeGateCampaignSchemaVersion,
		Name:             name,
		Trials:           len(reports),
		SelectedBindings: map[string]int{}, FinishReasons: map[string]int{},
		FramingClasses: map[string]int{}, ProviderHTTPStatuses: map[string]int{},
		SecondAcquireReasons: map[string]int{},
	}
	latencies := make([]time.Duration, 0, len(reports))
	for index, report := range reports {
		if report.SchemaVersion != RuntimeGateCampaignSchemaVersion || report.MaxCalls != 1 || report.ExternalCalls != 1 {
			return RuntimeGateBatchReport{}, fmt.Errorf("runtime gate batch trial %d is incomplete or over budget", index+1)
		}
		batch.ExternalCalls += report.ExternalCalls
		if report.ProviderSucceeded {
			batch.ProviderSuccesses++
		}
		if report.ExecutionError != "" {
			batch.ExecutionFailures++
		}
		if report.DurableReopen {
			batch.DurableReopens++
		}
		if report.ExpectedResponseMatch {
			batch.ExpectedMatches++
		}
		if report.ResponseJSONValid {
			batch.JSONValid++
		}
		if report.SchemaAdherence != nil {
			batch.SchemaEvaluated++
			if report.SchemaAdherence.FieldsPresent == report.SchemaAdherence.FieldsChecked &&
				report.SchemaAdherence.FieldsCorrectType == report.SchemaAdherence.FieldsChecked {
				batch.SchemaAdherent++
			}
			// ProposedChangeSet has ten fields whose content must be non-empty.
			// schema_version is numeric and preconditions may legitimately be [].
			if report.SchemaAdherence.FieldsNonEmpty == 10 {
				batch.SchemaContentComplete++
			}
			if report.SchemaAdherence.ChangesValid {
				batch.ChangesValid++
			}
		}
		batch.InputTokens += report.ObservedInputTokens
		batch.OutputTokens += report.ObservedOutputTokens
		batch.SelectedBindings[report.SelectedBindingID]++
		batch.FinishReasons[string(report.FinishReason)]++
		batch.SecondAcquireReasons[report.SecondAcquireReason]++
		if report.ResponseFramingClass != "" {
			batch.FramingClasses[report.ResponseFramingClass]++
		}
		if report.ProviderHTTPStatus != 0 {
			batch.ProviderHTTPStatuses[fmt.Sprint(report.ProviderHTTPStatus)]++
		}
		latencies = append(latencies, report.ProviderLatency)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	batch.LatencyP50 = batchPercentile(latencies, 50)
	batch.LatencyP95 = batchPercentile(latencies, 95)
	batch.LatencyMax = latencies[len(latencies)-1]
	return batch, nil
}

func WriteRuntimeGateBatchArtifacts(directory string, report RuntimeGateBatchReport) error {
	if strings.TrimSpace(directory) == "" || report.SchemaVersion != RuntimeGateCampaignSchemaVersion || report.Trials < 2 || report.ExternalCalls != report.Trials {
		return errors.New("artifact directory and complete bounded runtime gate batch report are required")
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(directory+"/runtime-gate-batch.json", append(body, '\n')); err != nil {
		return err
	}
	markdown := fmt.Sprintf("# Runtime provider gate batch\n\n- Name: `%s`\n- Trials/calls: %d/%d\n- Provider successes: %d\n- Execution failures: %d\n- Durable reopens: %d\n- Expected matches: %d\n- JSON valid: %d\n- Schema evaluated/adherent/content-complete: %d/%d/%d\n- Changes valid: %d\n- Tokens input/output: %d/%d\n- Provider latency p50/p95/max: `%s` / `%s` / `%s`\n- Selected bindings: `%v`\n- Finish reasons: `%v`\n- Framing classes: `%v`\n- Second acquire reasons: `%v`\n\nEach trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.\n", report.Name, report.Trials, report.ExternalCalls, report.ProviderSuccesses, report.ExecutionFailures, report.DurableReopens, report.ExpectedMatches, report.JSONValid, report.SchemaEvaluated, report.SchemaAdherent, report.SchemaContentComplete, report.ChangesValid, report.InputTokens, report.OutputTokens, report.LatencyP50, report.LatencyP95, report.LatencyMax, report.SelectedBindings, report.FinishReasons, report.FramingClasses, report.SecondAcquireReasons)
	return atomicWrite(directory+"/runtime-gate-batch.md", []byte(markdown))
}

func batchPercentile(values []time.Duration, percent int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := (len(values)*percent+99)/100 - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}
