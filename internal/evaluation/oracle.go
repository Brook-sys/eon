package evaluation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
)

// OracleModelLabel marks reports produced by the offline scripted oracle so they
// are never confused with a live provider baseline.
const OracleModelLabel = "offline-oracle"

// EncodeAnswer formats an expected key→value map exactly as Parse accepts for
// the given format. Keys are sorted for stable output.
func EncodeAnswer(format Format, expected map[string]string) (string, error) {
	if len(expected) == 0 {
		return "", errors.New("expected values are required")
	}
	keys := sortedKeys(expected)
	switch format {
	case FormatChoice:
		var b strings.Builder
		for i, key := range keys {
			if i > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "%s=%s", key, expected[key])
		}
		return b.String(), nil
	case FormatDelimited:
		var b strings.Builder
		for i, key := range keys {
			if i > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "%s=%s", key, expected[key])
		}
		return b.String(), nil
	case FormatJSON:
		// Manual encode keeps field order stable without depending on map walk.
		var b strings.Builder
		b.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			encKey, err := jsonString(key)
			if err != nil {
				return "", err
			}
			encVal, err := jsonString(expected[key])
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "%s:%s", encKey, encVal)
		}
		b.WriteByte('}')
		return b.String(), nil
	default:
		return "", fmt.Errorf("unsupported format %q", format)
	}
}

func jsonString(s string) (string, error) {
	// Minimal JSON string encoder for fixture-controlled text values.
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				return "", fmt.Errorf("unsupported control rune in expected value")
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

// OracleAnswers builds CompletionResults in the same order Runner.Run walks the
// matrix (case → format → context). Each answer is the perfect expected encoding.
func OracleAnswers(fixtures FixtureSet, matrix Matrix) ([]port.CompletionResult, error) {
	if err := fixtures.Validate(); err != nil {
		return nil, err
	}
	if err := matrix.Validate(); err != nil {
		return nil, err
	}
	var answers []port.CompletionResult
	for _, c := range fixtures.Cases {
		for _, format := range c.Formats {
			text, err := EncodeAnswer(format, c.Expected)
			if err != nil {
				return nil, fmt.Errorf("case %q format %s: %w", c.ID, format, err)
			}
			for range matrix.ContextTokens {
				// Token estimates stay small and deterministic for offline reports.
				answers = append(answers, port.CompletionResult{
					Text:         text,
					InputTokens:  32,
					OutputTokens: len(strings.Fields(text)) + 1,
					Model:        OracleModelLabel,
				})
			}
		}
	}
	return answers, nil
}

// QueueProvider is a ModelProvider that returns prebuilt answers in order.
// Exhaustion returns a PROVIDER-class error without inventing content.
type QueueProvider struct {
	Answers []port.CompletionResult
	index   int
	// Model overrides CompletionResult.Model when non-empty.
	Model string
}

// Complete implements port.ModelProvider.
func (p *QueueProvider) Complete(_ context.Context, _ port.CompletionRequest) (port.CompletionResult, error) {
	if p == nil || p.index >= len(p.Answers) {
		return port.CompletionResult{}, errors.New("oracle answer queue exhausted")
	}
	answer := p.Answers[p.index]
	p.index++
	if strings.TrimSpace(p.Model) != "" {
		answer.Model = p.Model
	}
	return answer, nil
}

// Calls reports how many Complete invocations were served.
func (p *QueueProvider) Calls() int {
	if p == nil {
		return 0
	}
	return p.index
}

// BuildOracleRunner constructs a Runner wired to a perfect answer queue for the
// fixture × matrix product. Spec and estimator match the live benchmark path.
func BuildOracleRunner(fixtures FixtureSet, matrix Matrix, estimator prompt.TokenEstimator, spec domain.OperationSpec) (Runner, *QueueProvider, error) {
	if estimator == nil {
		return Runner{}, nil, errors.New("token estimator is required")
	}
	answers, err := OracleAnswers(fixtures, matrix)
	if err != nil {
		return Runner{}, nil, err
	}
	provider := &QueueProvider{Answers: answers, Model: OracleModelLabel}
	return Runner{Provider: provider, Estimator: estimator, Spec: spec}, provider, nil
}

// RunOracle executes the cognitive matrix against perfect scripted answers.
// It never opens a network connection (offline residual of the cognitive eval).
func RunOracle(ctx context.Context, fixtures FixtureSet, matrix Matrix, estimator prompt.TokenEstimator, spec domain.OperationSpec) (Report, error) {
	runner, _, err := BuildOracleRunner(fixtures, matrix, estimator, spec)
	if err != nil {
		return Report{}, err
	}
	return runner.Run(ctx, fixtures, matrix)
}
