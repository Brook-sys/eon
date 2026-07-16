package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// APIStyle identifies the wire dialect the adapter will use. The MVP path is
// chat_completions; other styles are profile metadata only until adapters exist.
type APIStyle string

const (
	APIStyleChatCompletions APIStyle = "chat_completions"
	APIStyleResponses       APIStyle = "responses"
	APIStyleAuto            APIStyle = "auto"
)

// CapabilitySource records how a profile field was established (FR-MODEL-005).
// Unknown capabilities must remain unknown — never silently presumed available.
type CapabilitySource string

const (
	CapabilityDeclared CapabilitySource = "declared"
	CapabilityProbed   CapabilitySource = "probed"
	CapabilityInferred CapabilitySource = "inferred"
	CapabilityOverride CapabilitySource = "operator_override"
	CapabilityUnknown  CapabilitySource = "unknown"
)

// MaxOutputDialect is the Chat Completions field used to bound model output.
// Selection is configuration, never an automatic dual-field fallback.
type MaxOutputDialect string

const (
	MaxOutputDialectLegacy     MaxOutputDialect = "max_tokens"
	MaxOutputDialectCompletion MaxOutputDialect = "max_completion_tokens"
)

// ProviderProfile is a versioned, non-secret description of a model provider's
// known capabilities and limits. Secrets and free-form prompts never appear here.
type ProviderProfile struct {
	SchemaVersion int `json:"schema_version"`

	// Name is a stable operator-facing label (for example "ollama-chat").
	Name string `json:"name"`
	// Model is the configured model id; may be empty before binding.
	Model string `json:"model,omitempty"`
	// APIStyle is the configured wire dialect.
	APIStyle APIStyle `json:"api_style"`
	// MaxOutputDialect is the request field used to bound completions.
	MaxOutputDialect MaxOutputDialect `json:"max_output_dialect"`

	// Capability flags. False means unsupported or unconfirmed — not "try it".
	SupportsSystemRole bool `json:"supports_system_role"`
	SupportsStreaming  bool `json:"supports_streaming"`
	SupportsJSONMode   bool `json:"supports_json_mode"`
	SupportsJSONSchema bool `json:"supports_json_schema"`
	SupportsTools      bool `json:"supports_tools"`
	SupportsSeed       bool `json:"supports_seed"`
	// TextToTextConfirmed is true only after a successful probe or equivalent
	// contract evidence that plain text completion works.
	TextToTextConfirmed bool `json:"text_to_text_confirmed"`

	MaxContextTokens int `json:"max_context_tokens,omitempty"`
	MaxOutputTokens  int `json:"max_output_tokens,omitempty"`

	// Source distinguishes declared vs probe vs operator override provenance.
	Source CapabilitySource `json:"source"`
	// ObservedAt is when this profile snapshot was produced (declared or probe).
	ObservedAt time.Time `json:"observed_at,omitempty"`
	// ProbeBudgetRemaining is the remaining allowed live probes for this binding
	// (0 means further Probe calls must return the cached/declared snapshot only).
	ProbeBudgetRemaining int `json:"probe_budget_remaining,omitempty"`
	// Quirks are non-secret adapter notes (for example dialect ids).
	Quirks []string `json:"quirks,omitempty"`
	// SafeDetail is bounded operator-facing diagnostics without secrets/bodies.
	SafeDetail string `json:"safe_detail,omitempty"`
	// PolicyVersion stamps the capability policy that produced this snapshot.
	PolicyVersion string `json:"policy_version,omitempty"`
}

// Validate enforces the minimum profile contract for persistence/export.
func (p ProviderProfile) Validate() error {
	if p.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported provider profile schema version %d", p.SchemaVersion)
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("provider profile name is required")
	}
	switch p.APIStyle {
	case APIStyleChatCompletions, APIStyleResponses, APIStyleAuto:
	default:
		return fmt.Errorf("unsupported api style %q", p.APIStyle)
	}
	switch p.MaxOutputDialect {
	case MaxOutputDialectLegacy, MaxOutputDialectCompletion:
	default:
		return fmt.Errorf("unsupported max output dialect %q", p.MaxOutputDialect)
	}
	switch p.Source {
	case CapabilityDeclared, CapabilityProbed, CapabilityInferred, CapabilityOverride, CapabilityUnknown:
	default:
		return fmt.Errorf("unsupported capability source %q", p.Source)
	}
	if p.MaxContextTokens < 0 || p.MaxOutputTokens < 0 || p.ProbeBudgetRemaining < 0 {
		return errors.New("provider profile token/probe budgets must not be negative")
	}
	return nil
}

// BaselineDeclaredProfile returns a conservative text→text Chat Completions
// profile: only the MVP contract is assumed; richer features stay false/unknown.
func BaselineDeclaredProfile(name, model string, dialect MaxOutputDialect, contextTokens int, now time.Time) ProviderProfile {
	if dialect == "" {
		dialect = MaxOutputDialectLegacy
	}
	if strings.TrimSpace(name) == "" {
		name = "openai-compatible"
	}
	return ProviderProfile{
		SchemaVersion:        SchemaVersionV1,
		Name:                 name,
		Model:                model,
		APIStyle:             APIStyleChatCompletions,
		MaxOutputDialect:     dialect,
		SupportsSystemRole:   false,
		SupportsStreaming:    false,
		SupportsJSONMode:     false,
		SupportsJSONSchema:   false,
		SupportsTools:        false,
		SupportsSeed:         false,
		TextToTextConfirmed:  false,
		MaxContextTokens:     contextTokens,
		MaxOutputTokens:      0,
		Source:               CapabilityDeclared,
		ObservedAt:           now.UTC(),
		ProbeBudgetRemaining: 1,
		Quirks:               []string{string(dialect)},
		SafeDetail:           "declared baseline; richer capabilities unknown until probe or operator override",
		PolicyVersion:        "provider-profile@1",
	}
}
