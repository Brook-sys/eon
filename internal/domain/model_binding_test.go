package domain

import (
	"strings"
	"testing"
	"time"
)

func TestModelBindingConfigValidateReasoningEffort(t *testing.T) {
	base := ModelBindingConfig{
		ID: "groq-qwen36", ProviderRef: "groq", ModelID: "qwen/qwen3.6-27b",
		Enabled: true, Priority: 10, ContextTokens: 32768, MaxOutputTokens: 1024,
		MaxOutputDialect: MaxOutputDialectCompletion,
		Limit:            ResourceLimit{Resource: ModelBindingResource("groq-qwen36")},
	}
	cases := []struct {
		name   string
		effort string
		ok     bool
	}{
		{"empty_-inherit", "", true},
		{"none", "none", true},
		{"default", "default", true},
		{"low", "low", true},
		{"medium", "medium", true},
		{"high", "high", true},
		{"invalid", "ultra", false},
		{"invalid_case", "Low", false},
		{"invalid_empty_space", " ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := base
			b.ReasoningEffort = strings.TrimSpace(tc.effort)
			if tc.effort == " " {
				b.ReasoningEffort = " "
			}
			err := b.Validate()
			if tc.ok && err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected invalid reasoning_effort %q, got valid", tc.effort)
			}
		})
	}
}

func TestModelBindingConfigValidateReasoningFormat(t *testing.T) {
	base := ModelBindingConfig{
		ID: "groq-gptoss", ProviderRef: "groq", ModelID: "openai/gpt-oss-20b",
		Enabled: true, Priority: 10, ContextTokens: 32768, MaxOutputTokens: 1024,
		MaxOutputDialect: MaxOutputDialectCompletion,
		Limit:            ResourceLimit{Resource: ModelBindingResource("groq-gptoss")},
	}
	cases := []struct {
		name   string
		format string
		ok     bool
	}{
		{"empty_inherit", "", true},
		{"parsed", "parsed", true},
		{"raw", "raw", true},
		{"hidden", "hidden", true},
		{"invalid", "xml", false},
		{"invalid_case", "Hidden", false},
		{"invalid_empty_space", " ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := base
			b.ReasoningFormat = strings.TrimSpace(tc.format)
			if tc.format == " " {
				b.ReasoningFormat = " "
			}
			err := b.Validate()
			if tc.ok && err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected invalid reasoning_format %q, got valid", tc.format)
			}
		})
	}
}

func TestModelBindingConfigValidateTimeout(t *testing.T) {
	base := ModelBindingConfig{
		ID: "groq-qwen36", ProviderRef: "groq", ModelID: "qwen/qwen3.6-27b",
		Enabled: true, Priority: 10, ContextTokens: 32768, MaxOutputTokens: 1024,
		MaxOutputDialect: MaxOutputDialectCompletion,
		Limit:            ResourceLimit{Resource: ModelBindingResource("groq-qwen36")},
	}
	cases := []struct {
		name    string
		timeout time.Duration
		ok      bool
	}{
		{"zero", 0, true},
		{"positive", 10 * time.Second, true},
		{"max", 10 * time.Minute, true},
		{"negative", -1 * time.Second, false},
		{"too_large", 11 * time.Minute, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := base
			b.Timeout = tc.timeout
			err := b.Validate()
			if tc.ok && err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected invalid timeout %v, got valid", tc.timeout)
			}
		})
	}
}
