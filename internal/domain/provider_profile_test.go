package domain_test

import (
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func TestBaselineDeclaredProfileIsConservative(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	profile := domain.BaselineDeclaredProfile("ollama-chat", "tiny", domain.MaxOutputDialectLegacy, 2048, now)
	if err := profile.Validate(); err != nil {
		t.Fatal(err)
	}
	if profile.Source != domain.CapabilityDeclared {
		t.Fatalf("source = %s", profile.Source)
	}
	if profile.TextToTextConfirmed || profile.SupportsTools || profile.SupportsJSONSchema || profile.SupportsStreaming {
		t.Fatalf("baseline must not presume rich capabilities: %+v", profile)
	}
	if profile.APIStyle != domain.APIStyleChatCompletions || profile.MaxContextTokens != 2048 {
		t.Fatalf("unexpected baseline: %+v", profile)
	}
	if profile.ProbeBudgetRemaining != 1 {
		t.Fatalf("probe budget = %d", profile.ProbeBudgetRemaining)
	}
}

func TestProviderProfileRejectsUnknownSourceAndDialect(t *testing.T) {
	base := domain.BaselineDeclaredProfile("x", "m", domain.MaxOutputDialectLegacy, 0, time.Unix(0, 0).UTC())
	base.Source = "guessed"
	if err := base.Validate(); err == nil {
		t.Fatal("expected invalid source")
	}
	base = domain.BaselineDeclaredProfile("x", "m", domain.MaxOutputDialectLegacy, 0, time.Unix(0, 0).UTC())
	base.MaxOutputDialect = "both"
	if err := base.Validate(); err == nil {
		t.Fatal("expected invalid dialect")
	}
}
