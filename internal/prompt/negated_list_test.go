package prompt

import (
	"strings"
	"testing"
)

func TestNegatedListConstraintRendersRestatedRule(t *testing.T) {
	constraint, ok := NegatedListConstraint(NegatedSetSelection{
		LineKey:    "FACTORS",
		Candidates: []string{"F-3", "F-4", "F-5"},
		EmptyToken: "NONE",
	})
	if !ok {
		t.Fatal("expected coherent selection")
	}
	for _, want := range []string{
		"FACTORS", "ONLY", "FAILED", "MUST NOT appear",
		"F-3, F-4, F-5", "FACTORS: NONE",
	} {
		if !strings.Contains(constraint, want) {
			t.Fatalf("constraint missing %q:\n%s", want, constraint)
		}
	}
	if strings.Contains(constraint, "F-4 is failed") {
		t.Fatal("constraint must not carry semantic answers")
	}
}

func TestNegatedListConstraintSortsCandidatesDeterministically(t *testing.T) {
	first, ok1 := NegatedListConstraint(NegatedSetSelection{LineKey: "K", Candidates: []string{"C", "A", "B"}})
	second, ok2 := NegatedListConstraint(NegatedSetSelection{LineKey: "K", Candidates: []string{"B", "C", "A"}})
	if !ok1 || !ok2 || first != second {
		t.Fatalf("sorting not deterministic: %q vs %q", first, second)
	}
	if !strings.Contains(first, "{A, B, C}") {
		t.Fatalf("expected sorted universe, got %q", first)
	}
}

func TestNegatedListConstraintDefaultsEmptyToken(t *testing.T) {
	constraint, ok := NegatedListConstraint(NegatedSetSelection{LineKey: "FACTORS", Candidates: []string{"F-1"}})
	if !ok || !strings.Contains(constraint, "FACTORS: NONE") {
		t.Fatalf("expected default empty token NONE, got %q (ok=%v)", constraint, ok)
	}
}

func TestNegatedListConstraintRejectsIncoherentInput(t *testing.T) {
	cases := []NegatedSetSelection{
		{LineKey: "", Candidates: []string{"F-1"}},
		{LineKey: "BAD:KEY", Candidates: []string{"F-1"}},
		{LineKey: "K", Candidates: nil},
		{LineKey: "K", Candidates: []string{""}},
		{LineKey: "K", Candidates: []string{"F-1", "F-1"}},
		{LineKey: "K", Candidates: []string{"F-1", "x:y"}},
		{LineKey: "K", Candidates: []string{"F-1"}, EmptyToken: "bad:token"},
	}
	for i, sel := range cases {
		if constraint, ok := NegatedListConstraint(sel); ok || constraint != "" {
			t.Fatalf("case %d must fail closed, got %q ok=%v", i, constraint, ok)
		}
	}
	changed := cases[5]
	changed.EmptyToken = "NONE"
	if _, ok := NegatedListConstraint(changed); ok {
		t.Fatal("candidate containing colon must still be rejected")
	}
}

func TestNegatedListConstraintIntegratesWithCompiler(t *testing.T) {
	constraint, ok := NegatedListConstraint(NegatedSetSelection{
		LineKey:    "FACTORS",
		Candidates: []string{"F-3", "F-4", "F-5"},
	})
	if !ok {
		t.Fatal("incoherent selection")
	}
	input := validInput()
	input.Task = "Judge each factor and list exactly the factors NOT satisfied."
	input.Facts = []Fact{{ID: "F-3-status", Text: "F-3: supported by source S-1.", Required: true}}
	input.Constraints = append(input.Constraints, constraint)
	input.AnswerFormat = "VERDICT: PASS|FAIL\nFACTORS: <failed ids only or NONE>"
	spec := validSpec()
	spec.Budget.Tokens = 200
	result, err := (Compiler{Estimator: wordEstimator{}, ProviderContextTokens: 400}).Compile(spec, input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Request.Prompt, "satisfied MUST NOT appear") {
		t.Fatalf("restated rule not embedded in compiled prompt:\n%s", result.Request.Prompt)
	}
	if result.EstimatedInputTokens > result.InputTokenLimit {
		t.Fatalf("compiled prompt exceeds budget: %+v", result)
	}
}
