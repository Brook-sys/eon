package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	if err := p.Validate(); err != nil {
		return ModelsConfig{}, err
	}
	if strings.TrimSpace(version) == "" {
		return ModelsConfig{}, errors.New("models config version is required")
	}
	binding := p.Binding
	binding.Enabled = false
	binding.Priority = p.RecommendedPriority
	config := ModelsConfig{Version: version, Providers: []ModelProviderConfig{p.Provider}, Bindings: []ModelBindingConfig{binding}}
	return config, config.Validate()
}
