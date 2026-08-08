package prompt

import (
	"errors"
	"strings"
	"testing"

	"motor-autonomo/internal/domain"
)

type wordEstimator struct{}

func (wordEstimator) Count(text string) (int, error) { return len(strings.Fields(text)), nil }

func TestCompileSelectsFactsUnderEffectiveBudget(t *testing.T) {
	spec := validSpec()
	spec.Budget.Tokens = 66
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
	if len(result.OmittedFactIDs) != 1 || result.OmittedFactIDs[0] != "low" || result.Request.MaxOutputTokens != 25 || result.Request.Temperature != 0 {
		t.Fatalf("unexpected compile result: %+v", result)
	}
}

func TestCompileUsesSmallerProviderBudget(t *testing.T) {
	spec := validSpec()
	_, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 32}).Compile(spec, validInput())
	if err == nil || !strings.Contains(err.Error(), "input limit is 4") {
		t.Fatalf("expected smaller provider budget failure, got %v", err)
	}
}

func TestCompileRejectsRequiredContentThatDoesNotFit(t *testing.T) {
	spec := validSpec()
	spec.Budget.Tokens = 35
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
		if providerLimit < 30 || providerLimit > 4096 {
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
	return domain.OperationSpec{SchemaVersion: 1, ID: "select@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "facts", OutputSchema: "closed choice", Budget: domain.Budget{ModelCalls: 1, Tokens: 100, Attempts: 1}, MaxOutputTokens: 25, SafetyMargin: 3, Validators: []string{"allowed-option"}, RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly}
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

func TestBudgetGuardRejectsInsufficientOutputTokens(t *testing.T) {
	spec := validSpec()
	spec.MaxOutputTokens = 3 // very low
	spec.Budget.Tokens = 200
	spec.SafetyMargin = 3
	input := Input{
		Task:           "Extract date and source.",
		AllowedOutputs: []string{"DATE: value", "SOURCE: value"},
		AnswerFormat:   "DATE: 2025-11-03\nSOURCE: S-17",
	}
	result, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 400}).Compile(spec, input)
	if !errors.Is(err, ErrOutputBudgetInsufficient) {
		t.Fatalf("expected ErrOutputBudgetInsufficient, got %v", err)
	}
	if result.MinOutputTokens <= 0 {
		t.Fatalf("MinOutputTokens should be positive, got %d", result.MinOutputTokens)
	}
	if result.MinOutputTokens <= spec.MaxOutputTokens {
		t.Fatalf("MinOutputTokens (%d) should exceed MaxOutputTokens (%d)", result.MinOutputTokens, spec.MaxOutputTokens)
	}
}

func TestBudgetGuardAcceptsSufficientOutputTokens(t *testing.T) {
	spec := validSpec()
	spec.MaxOutputTokens = 50
	spec.Budget.Tokens = 200
	spec.SafetyMargin = 3
	input := Input{
		Task:           "Extract date and source.",
		AllowedOutputs: []string{"DATE: value", "SOURCE: value"},
		AnswerFormat:   "DATE: 2025-11-03\nSOURCE: S-17",
	}
	result, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 400}).Compile(spec, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MinOutputTokens <= 0 {
		t.Fatalf("MinOutputTokens should be positive, got %d", result.MinOutputTokens)
	}
	if result.MinOutputTokens > spec.MaxOutputTokens {
		t.Fatalf("MinOutputTokens (%d) should not exceed MaxOutputTokens (%d)", result.MinOutputTokens, spec.MaxOutputTokens)
	}
}

func TestBudgetGuardRespectsExplicitMinOutputTokens(t *testing.T) {
	spec := validSpec()
	spec.MaxOutputTokens = 10
	spec.Budget.Tokens = 200
	spec.SafetyMargin = 3
	input := Input{
		Task:            "Extract date.",
		AllowedOutputs:  []string{"DATE: value"},
		AnswerFormat:    "DATE: 2025-11-03",
		MinOutputTokens: 15, // caller says it needs 15 but spec only allows 10
	}
	_, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 400}).Compile(spec, input)
	if !errors.Is(err, ErrOutputBudgetInsufficient) {
		t.Fatalf("expected ErrOutputBudgetInsufficient with explicit MinOutputTokens, got %v", err)
	}
}

func TestBudgetGuardIncludesThinkingOverhead(t *testing.T) {
	spec := validSpec()
	spec.MaxOutputTokens = 128 // normal format fits, but thinking overhead doesn't
	spec.Budget.Tokens = 1000
	spec.SafetyMargin = 3
	input := Input{
		Task:                   "Extract date and source.",
		AllowedOutputs:         []string{"DATE: value", "SOURCE: value"},
		AnswerFormat:           "DATE: 2025-11-03\nSOURCE: S-17",
		ThinkingOverheadTokens: 384, // Phase 388 floor for qwen3.6-27b
	}
	result, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 2000}).Compile(spec, input)
	if !errors.Is(err, ErrOutputBudgetInsufficient) {
		t.Fatalf("expected ErrOutputBudgetInsufficient when thinking overhead exceeds budget, got %v", err)
	}
	if result.MinOutputTokens < 390 {
		t.Fatalf("MinOutputTokens (%d) should include thinking overhead (384 + format floor)", result.MinOutputTokens)
	}
}

func TestEstimateMinOutputTokens(t *testing.T) {
	cases := []struct {
		format   string
		minFloor int
	}{
		{"", 8},
		{"A", 8},
		{"DATE: 2025-11-03\nSOURCE: S-17", 8},
		{strings.Repeat("x", 100), 40}, // 100 bytes -> 34 tokens + 20% = 40
	}
	for _, tc := range cases {
		got := estimateMinOutputTokens(tc.format)
		if got < tc.minFloor-1 { // allow ±1 for integer division
			t.Errorf("estimateMinOutputTokens(%q) = %d, expected >= %d", tc.format, got, tc.minFloor-1)
		}
		if got < 8 {
			t.Errorf("estimateMinOutputTokens(%q) = %d, minimum should be 8", tc.format, got)
		}
	}
}
