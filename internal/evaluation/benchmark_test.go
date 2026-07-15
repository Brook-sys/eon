package evaluation

import (
	"context"
	"os"
	"strings"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
)

type scriptedProvider struct {
	answers []port.CompletionResult
	index   int
}

func (p *scriptedProvider) Complete(_ context.Context, _ port.CompletionRequest) (port.CompletionResult, error) {
	answer := p.answers[p.index]
	p.index++
	return answer, nil
}

func TestDecodeFixtures(t *testing.T) {
	file, err := os.Open("testdata/cognitive-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	set, err := DecodeFixtures(file, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Cases) != 4 || set.Cases[3].Operation != OperationRepair {
		t.Fatalf("unexpected fixtures: %+v", set)
	}
}

func TestDecodeFixturesRejectsUnknownAndDuplicate(t *testing.T) {
	for _, input := range []string{
		`{"schema_version":1,"name":"x","cases":[],"unknown":true}`,
		`{"schema_version":1,"name":"x","cases":[{"id":"a","operation":"EXTRACT","task":"t","required_facts":[{"id":"f","text":"x","priority":1}],"optional_facts":[],"constraints":[],"formats":["JSON"],"expected":{"x":"y"}},{"id":"a","operation":"EXTRACT","task":"t","required_facts":[{"id":"f","text":"x","priority":1}],"optional_facts":[],"constraints":[],"formats":["JSON"],"expected":{"x":"y"}}]}`,
	} {
		if _, err := DecodeFixtures(strings.NewReader(input), 16<<10); err == nil {
			t.Fatalf("expected rejection for %s", input)
		}
	}
}

func TestParseFormatsStrictly(t *testing.T) {
	keys := []string{"a", "b"}
	cases := []struct {
		format Format
		text   string
	}{
		{FormatChoice, "a=1\nb=2"},
		{FormatDelimited, "a: 1\nb: 2"},
		{FormatJSON, `{"a":"1","b":"2"}`},
	}
	for _, tc := range cases {
		got, err := Parse(tc.format, tc.text, keys)
		if err != nil || got["a"] != "1" || got["b"] != "2" {
			t.Fatalf("%s: got=%v err=%v", tc.format, got, err)
		}
	}
	for _, tc := range []struct {
		format Format
		text   string
	}{
		{FormatChoice, "a=1\na=2"},
		{FormatDelimited, "a: 1"},
		{FormatJSON, `{"a":"1","b":"2","c":"3"}`},
	} {
		if _, err := Parse(tc.format, tc.text, keys); err == nil {
			t.Fatalf("expected invalid %s output", tc.format)
		}
	}
}

func TestRunnerExecutesContextFormatMatrix(t *testing.T) {
	set := FixtureSet{SchemaVersion: 1, Name: "tiny", Cases: []Case{{
		ID: "extract", Operation: OperationExtract, Task: "Extract.",
		RequiredFacts: []FixtureFact{{ID: "f", Text: "Value is 7."}},
		Formats:       []Format{FormatChoice, FormatJSON}, Expected: map[string]string{"value": "7"},
	}}}
	provider := &scriptedProvider{answers: []port.CompletionResult{
		{Text: "value=7", InputTokens: 10, OutputTokens: 2, Model: "fake"},
		{Text: "value=8", InputTokens: 10, OutputTokens: 2, Model: "fake"},
		{Text: `{"value":"7"}`, InputTokens: 11, OutputTokens: 3, Model: "fake"},
		{Text: `{broken`, InputTokens: 11, OutputTokens: 3, Model: "fake"},
	}}
	report, err := (Runner{Provider: provider, Estimator: prompt.ConservativeEstimator{}, Spec: DefaultOperationSpec()}).Run(context.Background(), set, Matrix{ContextTokens: []int{2048, 4096}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Total != 4 || report.Summary.SyntaxValid != 3 || report.Summary.SemanticallyRight != 2 || report.Model != "fake" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestRunnerRecordsCompileFailureWithoutCallingProvider(t *testing.T) {
	set := FixtureSet{SchemaVersion: 1, Name: "large", Cases: []Case{{
		ID: "large", Operation: OperationExtract, Task: strings.Repeat("task ", 500),
		RequiredFacts: []FixtureFact{{ID: "f", Text: strings.Repeat("fact ", 500)}},
		Formats:       []Format{FormatJSON}, Expected: map[string]string{"value": "7"},
	}}}
	provider := &scriptedProvider{}
	report, err := (Runner{Provider: provider, Estimator: prompt.ConservativeEstimator{}, Spec: DefaultOperationSpec()}).Run(context.Background(), set, Matrix{ContextTokens: []int{512}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 1 || report.Runs[0].ErrorKind != "COMPILE" || provider.index != 0 {
		t.Fatalf("unexpected compile result: %+v", report)
	}
}

func TestWriteArtifacts(t *testing.T) {
	directory := t.TempDir()
	report := Report{SchemaVersion: 1, FixtureName: "fixture", Model: "model", Runs: []Run{{CaseID: "case", Operation: OperationExtract, Format: FormatJSON, ContextTokens: 2048, Compiled: true, SyntaxValid: true, SemanticallyCorrect: true}}, Summary: Summary{Total: 1, Compiled: 1, SyntaxValid: 1, SemanticallyRight: 1}}
	if err := WriteArtifacts(directory, report); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.json", "report.md"} {
		body, err := os.ReadFile(directory + "/" + name)
		if err != nil || len(body) == 0 {
			t.Fatalf("%s: body=%q err=%v", name, body, err)
		}
	}
}
