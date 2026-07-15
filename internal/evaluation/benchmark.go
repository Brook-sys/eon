// Package evaluation runs reproducible, provider-neutral cognitive benchmarks.
package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
)

const FixtureSchemaVersion = 1

type Operation string

const (
	OperationExtract    Operation = "EXTRACT"
	OperationSynthesize Operation = "SYNTHESIZE"
	OperationConflict   Operation = "CONFLICT"
	OperationRepair     Operation = "REPAIR"
)

type Format string

const (
	FormatChoice    Format = "CHOICE"
	FormatDelimited Format = "DELIMITED"
	FormatJSON      Format = "JSON"
)

type FixtureSet struct {
	SchemaVersion int    `json:"schema_version"`
	Name          string `json:"name"`
	Cases         []Case `json:"cases"`
}

type Case struct {
	ID            string            `json:"id"`
	Operation     Operation         `json:"operation"`
	Task          string            `json:"task"`
	RequiredFacts []FixtureFact     `json:"required_facts"`
	OptionalFacts []FixtureFact     `json:"optional_facts"`
	Constraints   []string          `json:"constraints"`
	Formats       []Format          `json:"formats"`
	Expected      map[string]string `json:"expected"`
}

type FixtureFact struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Priority int    `json:"priority"`
}

func DecodeFixtures(r io.Reader, maxBytes int64) (FixtureSet, error) {
	if r == nil || maxBytes <= 0 {
		return FixtureSet{}, errors.New("fixture reader and positive byte limit are required")
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return FixtureSet{}, fmt.Errorf("read fixtures: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return FixtureSet{}, errors.New("fixture set exceeds byte limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var set FixtureSet
	if err := decoder.Decode(&set); err != nil {
		return FixtureSet{}, fmt.Errorf("decode fixtures: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return FixtureSet{}, err
	}
	if err := set.Validate(); err != nil {
		return FixtureSet{}, err
	}
	return set, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("fixture set contains trailing JSON value")
		}
		return fmt.Errorf("decode trailing fixture data: %w", err)
	}
	return nil
}

func (s FixtureSet) Validate() error {
	if s.SchemaVersion != FixtureSchemaVersion || strings.TrimSpace(s.Name) == "" || len(s.Cases) == 0 {
		return errors.New("fixture set is incomplete or has unsupported schema version")
	}
	seen := map[string]bool{}
	for _, c := range s.Cases {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("case %q: %w", c.ID, err)
		}
		if seen[c.ID] {
			return fmt.Errorf("duplicate case ID %q", c.ID)
		}
		seen[c.ID] = true
	}
	return nil
}

func (c Case) Validate() error {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Task) == "" || len(c.RequiredFacts) == 0 || len(c.Formats) == 0 || len(c.Expected) == 0 {
		return errors.New("case identity, task, required facts, formats, and expected values are required")
	}
	switch c.Operation {
	case OperationExtract, OperationSynthesize, OperationConflict, OperationRepair:
	default:
		return fmt.Errorf("unsupported operation %q", c.Operation)
	}
	ids := map[string]bool{}
	for _, f := range append(append([]FixtureFact(nil), c.RequiredFacts...), c.OptionalFacts...) {
		if strings.TrimSpace(f.ID) == "" || strings.TrimSpace(f.Text) == "" || ids[f.ID] {
			return errors.New("facts require unique non-empty IDs and text")
		}
		ids[f.ID] = true
	}
	formats := map[Format]bool{}
	for _, f := range c.Formats {
		switch f {
		case FormatChoice, FormatDelimited, FormatJSON:
		default:
			return fmt.Errorf("unsupported format %q", f)
		}
		if formats[f] {
			return fmt.Errorf("duplicate format %q", f)
		}
		formats[f] = true
	}
	for key, value := range c.Expected {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return errors.New("expected keys and values must be non-empty")
		}
	}
	return nil
}

type Matrix struct {
	ContextTokens []int `json:"context_tokens"`
}

func (m Matrix) Validate() error {
	if len(m.ContextTokens) == 0 {
		return errors.New("at least one context limit is required")
	}
	seen := map[int]bool{}
	for _, limit := range m.ContextTokens {
		if limit <= 0 || seen[limit] {
			return errors.New("context limits must be positive and unique")
		}
		seen[limit] = true
	}
	return nil
}

type Run struct {
	CaseID               string            `json:"case_id"`
	Operation            Operation         `json:"operation"`
	Format               Format            `json:"format"`
	ContextTokens        int               `json:"context_tokens"`
	Compiled             bool              `json:"compiled"`
	EstimatedInputTokens int               `json:"estimated_input_tokens,omitempty"`
	ActualInputTokens    int               `json:"actual_input_tokens,omitempty"`
	OutputTokens         int               `json:"output_tokens,omitempty"`
	DurationMillis       int64             `json:"duration_millis,omitempty"`
	OmittedFactIDs       []string          `json:"omitted_fact_ids,omitempty"`
	SyntaxValid          bool              `json:"syntax_valid"`
	SemanticallyCorrect  bool              `json:"semantically_correct"`
	ErrorKind            string            `json:"error_kind,omitempty"`
	Output               string            `json:"output,omitempty"`
	Values               map[string]string `json:"values,omitempty"`
}

type Report struct {
	SchemaVersion int     `json:"schema_version"`
	FixtureName   string  `json:"fixture_name"`
	Model         string  `json:"model"`
	Runs          []Run   `json:"runs"`
	Summary       Summary `json:"summary"`
}

type Summary struct {
	Total             int `json:"total"`
	Compiled          int `json:"compiled"`
	SyntaxValid       int `json:"syntax_valid"`
	SemanticallyRight int `json:"semantically_correct"`
	InputTokens       int `json:"input_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

type Runner struct {
	Provider  port.ModelProvider
	Estimator prompt.TokenEstimator
	Spec      domain.OperationSpec
}

func (r Runner) Run(ctx context.Context, fixtures FixtureSet, matrix Matrix) (Report, error) {
	if r.Provider == nil || r.Estimator == nil {
		return Report{}, errors.New("provider and token estimator are required")
	}
	if err := r.Spec.Validate(); err != nil {
		return Report{}, fmt.Errorf("validate benchmark operation spec: %w", err)
	}
	if err := fixtures.Validate(); err != nil {
		return Report{}, err
	}
	if err := matrix.Validate(); err != nil {
		return Report{}, err
	}
	var runs []Run
	model := ""
	for _, c := range fixtures.Cases {
		for _, format := range c.Formats {
			for _, contextTokens := range matrix.ContextTokens {
				run := Run{CaseID: c.ID, Operation: c.Operation, Format: format, ContextTokens: contextTokens}
				input := benchmarkInput(c, format)
				compiled, err := (prompt.Compiler{Estimator: r.Estimator, ProviderContextTokens: contextTokens}).Compile(r.Spec, input)
				if err != nil {
					run.ErrorKind = "COMPILE"
					runs = append(runs, run)
					continue
				}
				run.Compiled = true
				run.EstimatedInputTokens = compiled.EstimatedInputTokens
				run.OmittedFactIDs = compiled.OmittedFactIDs
				started := time.Now()
				result, err := r.Provider.Complete(ctx, compiled.Request)
				run.DurationMillis = time.Since(started).Milliseconds()
				if err != nil {
					run.ErrorKind = "PROVIDER"
					runs = append(runs, run)
					continue
				}
				if model == "" {
					model = result.Model
				}
				run.ActualInputTokens, run.OutputTokens, run.Output = result.InputTokens, result.OutputTokens, result.Text
				values, err := Parse(format, result.Text, sortedKeys(c.Expected))
				if err != nil {
					run.ErrorKind = "VALIDATION"
					runs = append(runs, run)
					continue
				}
				run.SyntaxValid, run.Values = true, values
				run.SemanticallyCorrect = equalValues(values, c.Expected)
				runs = append(runs, run)
			}
		}
	}
	report := Report{SchemaVersion: 1, FixtureName: fixtures.Name, Model: model, Runs: runs}
	for _, run := range runs {
		report.Summary.Total++
		if run.Compiled {
			report.Summary.Compiled++
		}
		if run.SyntaxValid {
			report.Summary.SyntaxValid++
		}
		if run.SemanticallyCorrect {
			report.Summary.SemanticallyRight++
		}
		report.Summary.InputTokens += run.ActualInputTokens
		report.Summary.OutputTokens += run.OutputTokens
	}
	return report, nil
}

func benchmarkInput(c Case, format Format) prompt.Input {
	facts := make([]prompt.Fact, 0, len(c.RequiredFacts)+len(c.OptionalFacts))
	for _, f := range c.RequiredFacts {
		facts = append(facts, prompt.Fact{ID: f.ID, Text: f.Text, Required: true, Priority: f.Priority})
	}
	for _, f := range c.OptionalFacts {
		facts = append(facts, prompt.Fact{ID: f.ID, Text: f.Text, Priority: f.Priority})
	}
	keys := sortedKeys(c.Expected)
	allowed := make([]string, len(keys))
	for i, key := range keys {
		allowed[i] = key + "=" + c.Expected[key]
	}
	answer := ""
	switch format {
	case FormatChoice:
		answer = "Return exactly one line per key as KEY=VALUE. Use only the allowed values."
	case FormatDelimited:
		answer = "Return exactly one line per key as KEY: VALUE, with no prose."
	case FormatJSON:
		answer = "Return one JSON object with exactly these string keys: " + strings.Join(keys, ", ")
	}
	return prompt.Input{Task: c.Task, Facts: facts, Constraints: append([]string{"Use only supplied facts; do not invent missing evidence."}, c.Constraints...), AllowedOutputs: allowed, AnswerFormat: answer}
}

func Parse(format Format, text string, expectedKeys []string) (map[string]string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("empty answer")
	}
	values := map[string]string{}
	switch format {
	case FormatChoice:
		for _, line := range strings.Split(text, "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
			if len(parts) != 2 {
				return nil, errors.New("choice output must use KEY=VALUE")
			}
			if err := addValue(values, parts[0], parts[1]); err != nil {
				return nil, err
			}
		}
	case FormatDelimited:
		for _, line := range strings.Split(text, "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
			if len(parts) != 2 {
				return nil, errors.New("delimited output must use KEY: VALUE")
			}
			if err := addValue(values, parts[0], parts[1]); err != nil {
				return nil, err
			}
		}
	case FormatJSON:
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&values); err != nil {
			return nil, err
		}
		if err := ensureEOF(decoder); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
	if len(values) != len(expectedKeys) {
		return nil, errors.New("answer has wrong number of fields")
	}
	for _, key := range expectedKeys {
		if _, ok := values[key]; !ok {
			return nil, fmt.Errorf("answer is missing key %q", key)
		}
	}
	return values, nil
}

func addValue(values map[string]string, key, value string) error {
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)
	if key == "" || value == "" {
		return errors.New("answer contains empty key or value")
	}
	if _, exists := values[key]; exists {
		return fmt.Errorf("duplicate answer key %q", key)
	}
	values[key] = value
	return nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func equalValues(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

// DefaultOperationSpec reserves a small bounded answer and leaves the context
// limit itself to the benchmark matrix.
func DefaultOperationSpec() domain.OperationSpec {
	return domain.OperationSpec{SchemaVersion: 1, ID: "cognitive-benchmark@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "benchmark-case-v1", OutputSchema: "choice-delimited-json-v1", Budget: domain.Budget{Tokens: 8192, ModelCalls: 1, Attempts: 1, Duration: time.Minute}, MaxOutputTokens: 256, SafetyMargin: 128, Validators: []string{"strict-format", "exact-reference"}, RetryPolicy: "none", FallbackPolicy: "record-failure", MaximumAuthority: domain.AuthorityReadOnly}
}

// WriteArtifacts atomically writes the machine-readable report and a compact
// human-readable summary. The destination directory must not contain secrets.
func WriteArtifacts(directory string, report Report) error {
	if strings.TrimSpace(directory) == "" || report.SchemaVersion != 1 || report.FixtureName == "" || len(report.Runs) == 0 {
		return errors.New("artifact directory and complete report are required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	body = append(body, '\n')
	if err := atomicWrite(filepath.Join(directory, "report.json"), body); err != nil {
		return err
	}
	var markdown strings.Builder
	fmt.Fprintf(&markdown, "# Cognitive benchmark\n\n- Fixture: `%s`\n- Model: `%s`\n- Runs: %d\n- Compiled: %d\n- Syntax valid: %d\n- Semantically correct: %d\n- Input tokens: %d\n- Output tokens: %d\n\n", report.FixtureName, report.Model, report.Summary.Total, report.Summary.Compiled, report.Summary.SyntaxValid, report.Summary.SemanticallyRight, report.Summary.InputTokens, report.Summary.OutputTokens)
	markdown.WriteString("| Case | Operation | Format | Context | Result | Input/output tokens | Latency |\n| --- | --- | --- | ---: | --- | ---: | ---: |\n")
	for _, run := range report.Runs {
		result := run.ErrorKind
		if result == "" {
			if run.SemanticallyCorrect {
				result = "CORRECT"
			} else {
				result = "INCORRECT"
			}
		}
		fmt.Fprintf(&markdown, "| %s | %s | %s | %d | %s | %d/%d | %d ms |\n", run.CaseID, run.Operation, run.Format, run.ContextTokens, result, run.ActualInputTokens, run.OutputTokens, run.DurationMillis)
	}
	return atomicWrite(filepath.Join(directory, "report.md"), []byte(markdown.String()))
}

func atomicWrite(path string, body []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".benchmark-*")
	if err != nil {
		return fmt.Errorf("create temporary artifact: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return fmt.Errorf("write artifact: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close artifact: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("publish artifact: %w", err)
	}
	return nil
}
