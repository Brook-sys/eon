package spike

import (
	"encoding/json"
	"testing"
)

func TestGenerateIsDeterministicAndCounted(t *testing.T) {
	config := ReducedConfig()
	first, firstManifest, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	second, secondManifest, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("same seed and config produced different datasets")
	}
	if firstManifest != secondManifest {
		t.Fatalf("same dataset produced different manifests: %#v != %#v", firstManifest, secondManifest)
	}
	if len(first.Sources) != config.Sources || len(first.Claims) != config.Claims {
		t.Fatalf("dataset counts = sources %d claims %d", len(first.Sources), len(first.Claims))
	}
	links := 0
	for _, fixture := range first.Claims {
		links += len(fixture.Links)
	}
	if links != config.EvidenceLinks {
		t.Fatalf("evidence links = %d, want %d", links, config.EvidenceLinks)
	}
}

func TestGenerateChangesDigestWithSeed(t *testing.T) {
	config := ReducedConfig()
	_, first, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	config.Seed++
	_, second, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 == second.SHA256 {
		t.Fatal("different seeds produced the same dataset digest")
	}
}

func TestGenerateRejectsInvalidConfig(t *testing.T) {
	if _, _, err := Generate(DatasetConfig{}); err == nil {
		t.Fatal("invalid config was accepted")
	}
}
