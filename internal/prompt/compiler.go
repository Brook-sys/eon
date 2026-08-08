// Package prompt compiles bounded, provider-neutral text prompts.
package prompt

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// TokenEstimator is injected because OpenAI-compatible providers do not share
// one tokenizer. Implementations must be deterministic for a given string.
type TokenEstimator interface {
	Count(string) (int, error)
}

// ConservativeEstimator is an offline fallback, not a claim about an exact
// provider tokenizer. Counting at most three UTF-8 bytes per estimated token
// intentionally spends part of the safety budget on multilingual text.
type ConservativeEstimator struct{}

func (ConservativeEstimator) Count(text string) (int, error) {
	if !utf8.ValidString(text) {
		return 0, errors.New("prompt text is not valid UTF-8")
	}
	if text == "" {
		return 0, nil
	}
	return (len([]byte(text)) + 2) / 3, nil
}

type Fact struct {
	ID       string
	Text     string
	Required bool
	Priority int
}

type Input struct {
	Task           string
	Facts          []Fact
	Constraints    []string
	AllowedOutputs []string
	AnswerFormat   string
	// FormatExample is an optional explicit example of the expected response
	// format. When non-empty, it is rendered as an EXAMPLE block between
	// ANSWER and the closing delimiter. Adversarial fire sweeps (300+ live
	// calls, 2026-08-06) found that an explicit format example is the single
	// most effective prompt intervention: it lifted 70B model format
	// compliance from 0% to 100% under PT-BR language pressure.
	FormatExample string
}

type Result struct {
	Request              port.CompletionRequest
	TemplateVersion      uint64
	EstimatedInputTokens int
	InputTokenLimit      int
	OmittedFactIDs       []string
}

type Compiler struct {
	Estimator             TokenEstimator
	ProviderContextTokens int
}

// Compile enforces FR-MODEL-003. Required material either fits in the
// effective budget or compilation fails; optional facts are admitted by
// priority and stable input order without truncating their content.
func (c Compiler) Compile(spec domain.OperationSpec, input Input) (Result, error) {
	if c.Estimator == nil || c.ProviderContextTokens <= 0 {
		return Result{}, errors.New("prompt compiler dependencies are incomplete")
	}
	if err := spec.Validate(); err != nil {
		return Result{}, fmt.Errorf("validate operation spec: %w", err)
	}
	if strings.TrimSpace(input.Task) == "" || strings.TrimSpace(input.AnswerFormat) == "" || len(input.AllowedOutputs) == 0 {
		return Result{}, errors.New("task, allowed outputs, and answer format are required")
	}
	effective := min(spec.Budget.Tokens, c.ProviderContextTokens)
	inputLimit := effective - spec.MaxOutputTokens - spec.SafetyMargin
	if inputLimit <= 0 {
		return Result{}, errors.New("effective context budget leaves no input capacity")
	}

	selected := make([]Fact, 0, len(input.Facts))
	optional := make([]indexedFact, 0, len(input.Facts))
	for index, fact := range input.Facts {
		if strings.TrimSpace(fact.ID) == "" || strings.TrimSpace(fact.Text) == "" {
			return Result{}, errors.New("facts require non-empty IDs and text")
		}
		if fact.Required {
			selected = append(selected, fact)
		} else {
			optional = append(optional, indexedFact{Fact: fact, index: index})
		}
	}
	sort.SliceStable(optional, func(i, j int) bool { return optional[i].Priority > optional[j].Priority })

	promptText := render(spec.TemplateVersion, input, selected)
	count, err := c.Estimator.Count(promptText)
	if err != nil {
		return Result{}, fmt.Errorf("estimate required prompt: %w", err)
	}
	if count > inputLimit {
		return Result{}, fmt.Errorf("required prompt needs %d tokens, input limit is %d", count, inputLimit)
	}
	omitted := make([]indexedFact, 0)
	for _, candidate := range optional {
		trial := append(append([]Fact(nil), selected...), candidate.Fact)
		trialPrompt := render(spec.TemplateVersion, input, trial)
		trialCount, err := c.Estimator.Count(trialPrompt)
		if err != nil {
			return Result{}, fmt.Errorf("estimate optional fact %q: %w", candidate.ID, err)
		}
		if trialCount <= inputLimit {
			selected, promptText, count = trial, trialPrompt, trialCount
		} else {
			omitted = append(omitted, candidate)
		}
	}
	sort.Slice(omitted, func(i, j int) bool { return omitted[i].index < omitted[j].index })
	omittedIDs := make([]string, len(omitted))
	for i := range omitted {
		omittedIDs[i] = omitted[i].ID
	}
	return Result{
		Request:         port.CompletionRequest{Prompt: promptText, MaxOutputTokens: spec.MaxOutputTokens, Temperature: 0},
		TemplateVersion: spec.TemplateVersion, EstimatedInputTokens: count,
		InputTokenLimit: inputLimit, OmittedFactIDs: omittedIDs,
	}, nil
}

type indexedFact struct {
	Fact
	index int
}

func render(version uint64, input Input, facts []Fact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "TEMPLATE v%d\n\nTASK\n%s\n", version, strings.TrimSpace(input.Task))
	if len(facts) > 0 {
		b.WriteString("\nFACTS\n")
		for index, fact := range facts {
			fmt.Fprintf(&b, "F%d [%s]: %s\n", index+1, fact.ID, strings.TrimSpace(fact.Text))
		}
	}
	if len(input.Constraints) > 0 {
		b.WriteString("\nCONSTRAINTS\n")
		for _, constraint := range input.Constraints {
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(constraint))
		}
	}
	b.WriteString("\nALLOWED OUTPUTS\n")
	for index, output := range input.AllowedOutputs {
		fmt.Fprintf(&b, "%c: %s\n", 'A'+index, strings.TrimSpace(output))
	}
	fmt.Fprintf(&b, "\nANSWER\n%s\n", strings.TrimSpace(input.AnswerFormat))
	if strings.TrimSpace(input.FormatExample) != "" {
		fmt.Fprintf(&b, "\nEXAMPLE\n%s\n", strings.TrimSpace(input.FormatExample))
	}
	return b.String()
}
