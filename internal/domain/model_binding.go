package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ProviderKind identifies operational policy, not a separate domain authority.
// All kinds still execute through an explicit adapter such as OpenAI-compatible.
type ProviderKind string

const (
	ProviderKindOpenAICompatible ProviderKind = "openai_compatible"
	ProviderKindGroq             ProviderKind = "groq"
	ProviderKindNVIDIANIM        ProviderKind = "nvidia_nim"
)

// ModelProviderConfig contains transport-level settings shared by bindings.
// APIKeyEnv names a secret source; the key value must never be persisted here.
type ModelProviderConfig struct {
	ID               string        `json:"id"`
	Kind             ProviderKind  `json:"kind"`
	BaseURL          string        `json:"base_url"`
	APIKeyEnv        string        `json:"api_key_env"`
	Timeout          time.Duration `json:"timeout"`
	MaxResponseBytes int64         `json:"max_response_bytes"`
	GlobalLimit      ResourceLimit `json:"global_limit"`
}

func (c ModelProviderConfig) Validate() error {
	if err := validateStableID(c.ID, "provider"); err != nil {
		return err
	}
	switch c.Kind {
	case ProviderKindOpenAICompatible, ProviderKindGroq, ProviderKindNVIDIANIM:
	default:
		return fmt.Errorf("unsupported provider kind %q", c.Kind)
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("provider base_url must be an absolute HTTP(S) URL")
	}
	if strings.TrimSpace(c.APIKeyEnv) == "" || strings.ContainsAny(c.APIKeyEnv, "=\x00\r\n") {
		return errors.New("provider api_key_env must name a secret source")
	}
	if c.Timeout <= 0 || c.Timeout > 10*time.Minute {
		return errors.New("provider timeout must be between zero and ten minutes")
	}
	if c.MaxResponseBytes <= 0 || c.MaxResponseBytes > 64<<20 {
		return errors.New("provider max_response_bytes must be between 1 and 64 MiB")
	}
	return c.GlobalLimit.Validate()
}

// ModelBindingConfig is an operator-controlled model instance with its own
// preference, context budget, wire dialect, and quota bucket.
type ModelBindingConfig struct {
	ID               string           `json:"id"`
	ProviderRef      string           `json:"provider_ref"`
	ModelID          string           `json:"model_id"`
	Enabled          bool             `json:"enabled"`
	Priority         int              `json:"priority"`
	ContextTokens    int              `json:"context_tokens"`
	MaxOutputTokens  int              `json:"max_output_tokens"`
	MaxOutputDialect MaxOutputDialect `json:"max_output_dialect"`
	Limit            ResourceLimit    `json:"limit"`
}

func (c ModelBindingConfig) Validate() error {
	if err := validateStableID(c.ID, "binding"); err != nil {
		return err
	}
	if err := validateStableID(c.ProviderRef, "provider_ref"); err != nil {
		return err
	}
	if strings.TrimSpace(c.ModelID) == "" || len(c.ModelID) > 256 || strings.ContainsAny(c.ModelID, "\x00\r\n") {
		return errors.New("binding model_id is required and must be bounded")
	}
	if c.Priority < 0 || c.Priority > 1_000_000 {
		return errors.New("binding priority must be between 0 and 1000000")
	}
	if c.ContextTokens <= 0 || c.MaxOutputTokens <= 0 || c.MaxOutputTokens >= c.ContextTokens {
		return errors.New("binding token limits require 0 < max_output_tokens < context_tokens")
	}
	switch c.MaxOutputDialect {
	case MaxOutputDialectLegacy, MaxOutputDialectCompletion:
	default:
		return fmt.Errorf("unsupported max output dialect %q", c.MaxOutputDialect)
	}
	return c.Limit.Validate()
}

func validateStableID(value, field string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return fmt.Errorf("%s id is required and must be bounded", field)
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return fmt.Errorf("%s id contains unsupported characters", field)
		}
	}
	return nil
}

func ModelProviderResource(providerID string) string { return "model-provider:" + providerID }
func ModelBindingResource(bindingID string) string   { return "model-binding:" + bindingID }
