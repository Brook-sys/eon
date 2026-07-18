package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const ModelPresetCatalogSchema = "model-presets.v1"

// ModelPreset is evidence-backed operator input for a ModelsConfig draft.
// It is deliberately separate from routing authority: applying a preset still
// requires the normal config draft, diff, validation, and apply flow.
type ModelPreset struct {
	ID                  string              `json:"id"`
	Provider            ModelProviderConfig `json:"provider"`
	Binding             ModelBindingConfig  `json:"binding"`
	ObservedAt          time.Time           `json:"observed_at"`
	Qualification       string              `json:"qualification"`
	EvidenceReport      string              `json:"evidence_report"`
	EvidenceSHA256      string              `json:"evidence_sha256"`
	RecommendedPriority int                 `json:"recommended_priority"`
}

type ModelPresetCatalog struct {
	Schema  string        `json:"schema"`
	Presets []ModelPreset `json:"presets"`
}

// ModelPresetEnablementPreview makes the authority change required to enable
// an installed preset explicit. Candidate is populated only when the active
// MODELS revision still contains the exact evidence-backed provider and
// disabled binding from the preset.
type ModelPresetEnablementPreview struct {
	PresetID        string   `json:"preset_id"`
	EvidenceReport  string   `json:"evidence_report"`
	EvidenceSHA256  string   `json:"evidence_sha256"`
	Blocked         bool     `json:"blocked"`
	BlockReasons    []string `json:"block_reasons,omitempty"`
	Risks           []string `json:"risks"`
	EnabledBefore   int      `json:"enabled_before"`
	EnabledAfter    int      `json:"enabled_after"`
	RoutingBefore   []string `json:"routing_before"`
	RoutingAfter    []string `json:"routing_after"`
	PrimaryBefore   string   `json:"primary_before,omitempty"`
	PrimaryAfter    string   `json:"primary_after,omitempty"`
	PrimaryChanged  bool     `json:"primary_changed"`
	IntroducesFirst bool     `json:"introduces_first_enabled_model"`
	// QuotaSummary and ContextSummary are non-authoritative operator aids derived
	// from the preset's declared limits. They never invent live usage.
	QuotaSummary   *ModelPresetQuotaSummary   `json:"quota_summary,omitempty"`
	ContextSummary *ModelPresetContextSummary `json:"context_summary,omitempty"`
	Candidate      *ModelsConfig              `json:"candidate,omitempty"`
}

// ModelPresetQuotaSummary surfaces configured ResourceGate ceilings only.
type ModelPresetQuotaSummary struct {
	ProviderResource        ResourceID    `json:"provider_resource"`
	BindingResource         ResourceID    `json:"binding_resource"`
	ProviderMaxConcurrent   int           `json:"provider_max_concurrent,omitempty"`
	ProviderMaxPerMinute    int           `json:"provider_max_per_minute,omitempty"`
	ProviderMaxPerDay       int           `json:"provider_max_per_day,omitempty"`
	ProviderMaxTokensPerMin int           `json:"provider_max_tokens_per_minute,omitempty"`
	ProviderCooldownBase    time.Duration `json:"provider_cooldown_base,omitempty"`
	ProviderCooldownMax     time.Duration `json:"provider_cooldown_max,omitempty"`
	BindingMaxConcurrent    int           `json:"binding_max_concurrent,omitempty"`
	BindingMaxPerMinute     int           `json:"binding_max_per_minute,omitempty"`
	BindingMaxPerDay        int           `json:"binding_max_per_day,omitempty"`
	BindingMaxTokensPerMin  int           `json:"binding_max_tokens_per_minute,omitempty"`
	BindingCooldownBase     time.Duration `json:"binding_cooldown_base,omitempty"`
	BindingCooldownMax      time.Duration `json:"binding_cooldown_max,omitempty"`
	Note                    string        `json:"note"`
}

// ModelPresetContextSummary surfaces declared prompt windows only.
type ModelPresetContextSummary struct {
	DeclaredContextTokens  int    `json:"declared_context_tokens"`
	MaxOutputTokens        int    `json:"max_output_tokens"`
	MaxOutputDialect       string `json:"max_output_dialect"`
	ConservativeWindowHint int    `json:"conservative_window_hint"`
	Note                   string `json:"note"`
}

func DecodeModelPresetCatalog(r io.Reader, maxBytes int64) (ModelPresetCatalog, error) {
	if r == nil || maxBytes <= 0 {
		return ModelPresetCatalog{}, errors.New("preset catalog reader and positive byte limit are required")
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return ModelPresetCatalog{}, fmt.Errorf("read model preset catalog: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ModelPresetCatalog{}, errors.New("model preset catalog exceeds byte limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var catalog ModelPresetCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return ModelPresetCatalog{}, fmt.Errorf("decode model preset catalog: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return ModelPresetCatalog{}, errors.New("model preset catalog contains trailing JSON value")
		}
		return ModelPresetCatalog{}, fmt.Errorf("decode trailing model preset data: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return ModelPresetCatalog{}, err
	}
	return catalog, nil
}

func (p ModelPreset) Validate() error {
	if err := validateStableID(p.ID, "preset"); err != nil {
		return err
	}
	if err := p.Provider.Validate(); err != nil {
		return fmt.Errorf("preset provider: %w", err)
	}
	if err := p.Binding.Validate(); err != nil {
		return fmt.Errorf("preset binding: %w", err)
	}
	if p.Binding.ProviderRef != p.Provider.ID {
		return errors.New("preset binding must reference its provider")
	}
	if p.Binding.Enabled {
		return errors.New("preset binding must be disabled until explicitly enabled by the operator")
	}
	if p.ObservedAt.IsZero() || p.Qualification != "QUALIFIED" {
		return errors.New("preset requires dated QUALIFIED live evidence")
	}
	if strings.TrimSpace(p.EvidenceReport) == "" || strings.Contains(p.EvidenceReport, "..") || strings.HasPrefix(p.EvidenceReport, "/") {
		return errors.New("preset evidence_report must be a safe workspace-relative path")
	}
	if len(p.EvidenceSHA256) != 64 {
		return errors.New("preset evidence_sha256 must be a lowercase SHA-256 digest")
	}
	for _, r := range p.EvidenceSHA256 {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return errors.New("preset evidence_sha256 must be a lowercase SHA-256 digest")
		}
	}
	if p.RecommendedPriority < 0 || p.RecommendedPriority > 1_000_000 {
		return errors.New("preset recommended_priority is out of range")
	}
	return nil
}

func (c ModelPresetCatalog) Validate() error {
	if c.Schema != ModelPresetCatalogSchema || len(c.Presets) == 0 || len(c.Presets) > 128 {
		return errors.New("model preset catalog requires supported schema and bounded presets")
	}
	seen := map[string]bool{}
	for i := range c.Presets {
		if err := c.Presets[i].Validate(); err != nil {
			return fmt.Errorf("preset %d: %w", i, err)
		}
		if seen[c.Presets[i].ID] {
			return errors.New("model preset catalog has duplicate preset id")
		}
		seen[c.Presets[i].ID] = true
	}
	return nil
}

// ModelsConfigDraft materializes a disabled, non-authoritative config payload.
func (p ModelPreset) ModelsConfigDraft(version string) (ModelsConfig, error) {
	return p.ModelsConfigDraftFromActive(nil, version)
}

// ModelsConfigDraftFromActive installs a disabled preset without discarding an
// existing MODELS catalog. Identifier collisions fail closed: an operator must
// resolve provider/binding drift through the ordinary explicit config editor.
func (p ModelPreset) ModelsConfigDraftFromActive(active *ModelsConfig, version string) (ModelsConfig, error) {
	if err := p.Validate(); err != nil {
		return ModelsConfig{}, err
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return ModelsConfig{}, errors.New("models config version is required")
	}
	config := ModelsConfig{Version: version}
	if active != nil {
		if err := active.Validate(); err != nil {
			return ModelsConfig{}, fmt.Errorf("active models config: %w", err)
		}
		config = *cloneModelsConfig(active)
		config.Version = version
	}
	for _, provider := range config.Providers {
		if provider.ID != p.Provider.ID {
			continue
		}
		if provider != p.Provider {
			return ModelsConfig{}, errors.New("active provider identifier collides with a different preset configuration")
		}
		goto providerReady
	}
	config.Providers = append(config.Providers, p.Provider)

providerReady:
	for _, binding := range config.Bindings {
		if binding.ID == p.Binding.ID {
			return ModelsConfig{}, errors.New("preset binding is already installed in the active MODELS revision")
		}
	}
	binding := p.Binding
	binding.Enabled = false
	binding.Priority = p.RecommendedPriority
	config.Bindings = append(config.Bindings, binding)
	return config, config.Validate()
}

// PreviewEnablement produces a candidate MODELS payload but never persists or
// applies it. Enablement is allowed only after the disabled preset has passed
// through the ordinary draft/validate/apply lifecycle unchanged.
func (p ModelPreset) PreviewEnablement(active *ModelsConfig, version string) (ModelPresetEnablementPreview, error) {
	if err := p.Validate(); err != nil {
		return ModelPresetEnablementPreview{}, err
	}
	if strings.TrimSpace(version) == "" {
		return ModelPresetEnablementPreview{}, errors.New("models config version is required")
	}
	preview := ModelPresetEnablementPreview{
		PresetID: p.ID, EvidenceReport: p.EvidenceReport, EvidenceSHA256: p.EvidenceSHA256,
		Risks: []string{
			"enables external model calls after the applied MODELS revision is activated by coordinated restart",
			"sends bounded operation prompts to the configured provider",
			"consumes provider quota and may trigger cooldown or fallback",
			"requires the configured API-key environment reference at runtime",
		},
		// Summaries are authority-free operator aids derived only from the preset.
		QuotaSummary:   modelPresetQuotaSummary(p),
		ContextSummary: modelPresetContextSummary(p),
	}
	if active == nil {
		preview.Blocked = true
		preview.BlockReasons = []string{"preset must first be installed disabled and applied as the active MODELS revision"}
		return preview, nil
	}
	if err := active.Validate(); err != nil {
		return ModelPresetEnablementPreview{}, fmt.Errorf("active models config: %w", err)
	}
	providerFound := false
	for _, provider := range active.Providers {
		if provider.ID == p.Provider.ID {
			providerFound = true
			if provider != p.Provider {
				preview.BlockReasons = append(preview.BlockReasons, "active provider differs from the evidence-backed preset")
			}
			break
		}
	}
	bindingIndex := -1
	for i, binding := range active.Bindings {
		if binding.ID != p.Binding.ID {
			continue
		}
		bindingIndex = i
		expected := p.Binding
		expected.Priority = p.RecommendedPriority
		if binding.Enabled {
			preview.BlockReasons = append(preview.BlockReasons, "preset binding is already enabled")
		} else if binding != expected {
			preview.BlockReasons = append(preview.BlockReasons, "active binding differs from the evidence-backed preset")
		}
		break
	}
	if !providerFound {
		preview.BlockReasons = append(preview.BlockReasons, "evidence-backed provider is not installed in the active MODELS revision")
	}
	if bindingIndex < 0 {
		preview.BlockReasons = append(preview.BlockReasons, "evidence-backed disabled binding is not installed in the active MODELS revision")
	}
	if len(preview.BlockReasons) > 0 {
		preview.Blocked = true
		return preview, nil
	}
	candidate := cloneModelsConfig(active)
	candidate.Version = strings.TrimSpace(version)
	candidate.Bindings[bindingIndex].Enabled = true
	if err := candidate.Validate(); err != nil {
		return ModelPresetEnablementPreview{}, err
	}
	preview.Candidate = candidate
	preview.RoutingBefore = enabledModelRouting(active.Bindings)
	preview.RoutingAfter = enabledModelRouting(candidate.Bindings)
	preview.EnabledBefore = len(preview.RoutingBefore)
	preview.EnabledAfter = len(preview.RoutingAfter)
	if preview.EnabledBefore > 0 {
		preview.PrimaryBefore = preview.RoutingBefore[0]
	}
	if preview.EnabledAfter > 0 {
		preview.PrimaryAfter = preview.RoutingAfter[0]
	}
	preview.PrimaryChanged = preview.PrimaryBefore != preview.PrimaryAfter
	preview.IntroducesFirst = preview.EnabledBefore == 0 && preview.EnabledAfter == 1
	return preview, nil
}

func modelPresetQuotaSummary(p ModelPreset) *ModelPresetQuotaSummary {
	return &ModelPresetQuotaSummary{
		ProviderResource:        p.Provider.GlobalLimit.Resource,
		BindingResource:         p.Binding.Limit.Resource,
		ProviderMaxConcurrent:   p.Provider.GlobalLimit.MaxConcurrent,
		ProviderMaxPerMinute:    p.Provider.GlobalLimit.MaxPerMinute,
		ProviderMaxPerDay:       p.Provider.GlobalLimit.MaxPerDay,
		ProviderMaxTokensPerMin: p.Provider.GlobalLimit.MaxTokensPerMinute,
		ProviderCooldownBase:    p.Provider.GlobalLimit.CooldownBase,
		ProviderCooldownMax:     p.Provider.GlobalLimit.CooldownMax,
		BindingMaxConcurrent:    p.Binding.Limit.MaxConcurrent,
		BindingMaxPerMinute:     p.Binding.Limit.MaxPerMinute,
		BindingMaxPerDay:        p.Binding.Limit.MaxPerDay,
		BindingMaxTokensPerMin:  p.Binding.Limit.MaxTokensPerMinute,
		BindingCooldownBase:     p.Binding.Limit.CooldownBase,
		BindingCooldownMax:      p.Binding.Limit.CooldownMax,
		Note:                    "configured ceilings only; live usage is projected by GET /resources and is not invented here",
	}
}

func modelPresetContextSummary(p ModelPreset) *ModelPresetContextSummary {
	// Reuse the same conservative default policy the kernel applies offline.
	hint := DefaultContextBudgetPolicy(p.Binding.ContextTokens).EffectiveContextTokens()
	return &ModelPresetContextSummary{
		DeclaredContextTokens:  p.Binding.ContextTokens,
		MaxOutputTokens:        p.Binding.MaxOutputTokens,
		MaxOutputDialect:       string(p.Binding.MaxOutputDialect),
		ConservativeWindowHint: hint,
		Note:                   "declared window and default conservative compile hint; durable context pressure remains binding-local and reversible",
	}
}

func enabledModelRouting(bindings []ModelBindingConfig) []string {
	enabled := make([]ModelBindingConfig, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Enabled {
			enabled = append(enabled, binding)
		}
	}
	sort.SliceStable(enabled, func(i, j int) bool {
		if enabled[i].Priority == enabled[j].Priority {
			return enabled[i].ID < enabled[j].ID
		}
		return enabled[i].Priority < enabled[j].Priority
	})
	result := make([]string, len(enabled))
	for i := range enabled {
		result[i] = enabled[i].ID
	}
	return result
}
