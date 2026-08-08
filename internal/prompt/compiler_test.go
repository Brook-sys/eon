package prompt

import (
	"strings"
	"testing"

	"motor-autonomo/internal/domain"
)

type wordEstimator struct{}

func (wordEstimator) Count(text string) (int, error) { return len(strings.Fields(text)), nil }

func TestCompileSelectsFactsUnderEffectiveBudget(t *testing.T) {
	spec := validSpec()
	spec.Budget.Tokens = 46
	result, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 100}).Compile(spec, Input{
		Task: "Choose the supported claim.",
		Facts: []Fact{
			{ID: "required", Text: "The source is immutable.", Required: true},
			{ID: "low", Text: "This optional detail is deliberately very long and should not fit inside the remaining context budget.", Priority: 1},
			{ID: "high", Text: "Evidence supports A.", Priority: 10},
		},
		Constraints: []string{"Use only listed facts."}, AllowedOutputs: []string{"claim A", "claim B"}, AnswerFormat: "Only A or B.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EstimatedInputTokens > result.InputTokenLimit || result.InputTokenLimit != 38 {
		t.Fatalf("invalid budget result: %+v", result)
	}
	if !strings.Contains(result.Request.Prompt, "[required]") || !strings.Contains(result.Request.Prompt, "[high]") || strings.Contains(result.Request.Prompt, "[low]") {
		t.Fatalf("unexpected selected facts:\n%s", result.Request.Prompt)
	}
	if len(result.OmittedFactIDs) != 1 || result.OmittedFactIDs[0] != "low" || result.Request.MaxOutputTokens != 5 || result.Request.Temperature != 0 {
		t.Fatalf("unexpected compile result: %+v", result)
	}
}

func TestCompileUsesSmallerProviderBudget(t *testing.T) {
	spec := validSpec()
	_, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 12}).Compile(spec, validInput())
	if err == nil || !strings.Contains(err.Error(), "input limit is 4") {
		t.Fatalf("expected smaller provider budget failure, got %v", err)
	}
}

func TestCompileRejectsRequiredContentThatDoesNotFit(t *testing.T) {
	spec := validSpec()
	spec.Budget.Tokens = 20
	input := validInput()
	input.Facts = []Fact{{ID: "required", Text: strings.Repeat("required ", 30), Required: true}}
	_, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 100}).Compile(spec, input)
	if err == nil || !strings.Contains(err.Error(), "required prompt needs") {
		t.Fatalf("expected bounded failure, got %v", err)
	}
}

func FuzzCompileNeverExceedsBudget(f *testing.F) {
	f.Add("short fact", 64)
	f.Add(strings.Repeat("x", 1000), 32)
	f.Fuzz(func(t *testing.T, text string, providerLimit int) {
		if providerLimit < 10 || providerLimit > 4096 {
			t.Skip()
		}
		spec := validSpec()
		spec.Budget.Tokens = 4096
		input := validInput()
		input.Facts = []Fact{{ID: "optional", Text: text, Priority: 1}}
		result, err := (Compiler{Estimator: ConservativeEstimator{}, ProviderContextTokens: providerLimit}).Compile(spec, input)
		if err != nil {
			return
		}
		if result.EstimatedInputTokens > result.InputTokenLimit || result.EstimatedInputTokens+spec.MaxOutputTokens+spec.SafetyMargin > providerLimit {
			t.Fatalf("compiler exceeded provider limit: %+v", result)
		}
	})
}

func validSpec() domain.OperationSpec {
	return domain.OperationSpec{SchemaVersion: 1, ID: "select@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "facts", OutputSchema: "closed choice", Budget: domain.Budget{ModelCalls: 1, Tokens: 100, Attempts: 1}, MaxOutputTokens: 5, SafetyMargin: 3, Validators: []string{"allowed-option"}, RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly}
}

func validInput() Input {
	return Input{Task: "Choose one.", AllowedOutputs: []string{"first", "second"}, AnswerFormat: "Only A or B."}
}

func TestCompileRenderIncludesFormatExample(t *testing.T) {
	spec := validSpec()
	spec.Budget.Tokens = 200
	result, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 400}).Compile(spec, Input{
		Task:           "Extract the publication date and source.",
		AllowedOutputs: []string{"DATE: value", "SOURCE: value"},
		AnswerFormat:   "DATE: <value>\\nSOURCE: <value>",
		FormatExample:  "DATE: 2025-11-03\\nSOURCE: S-17",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Request.Prompt, "EXAMPLE") {
		t.Fatalf("prompt missing EXAMPLE block:\n%s", result.Request.Prompt)
	}
	if !strings.Contains(result.Request.Prompt, "DATE: 2025-11-03") {
		t.Fatalf("prompt missing format example content:\n%s", result.Request.Prompt)
	}
	if !strings.Contains(result.Request.Prompt, "SOURCE: S-17") {
		t.Fatalf("prompt missing format example source content:\n%s", result.Request.Prompt)
	}
}

func TestCompileOmitsFormatExampleWhenEmpty(t *testing.T) {
	spec := validSpec()
	spec.Budget.Tokens = 200
	result, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 400}).Compile(spec, validInput())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Request.Prompt, "EXAMPLE") {
		t.Fatalf("prompt should not contain EXAMPLE block when FormatExample is empty:\n%s", result.Request.Prompt)
	}
}
