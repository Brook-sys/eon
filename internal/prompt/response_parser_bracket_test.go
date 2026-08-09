package prompt_test

import (
	"testing"

	"motor-autonomo/internal/prompt"
)

func TestParseResponse_TruncatedBracketRecovery(t *testing.T) {
	// A JSON-like or custom-bracketed array interrupted by max_tokens limit
	// should not fail extraction completely. It should return the cleanly parsed
	// partial values.

	text := "TOOLS: [Wrench, Pliers, Socket"

	r := prompt.ParseResponse(text, []string{"TOOLS"})

	if r.Values["TOOLS"] != "Wrench, Pliers, Socket" {
		t.Errorf("TOOLS = %q, want %q", r.Values["TOOLS"], "Wrench, Pliers, Socket")
	}

	// With trailing comma
	text2 := "TOOLS: [Torque Wrench, Ratchet, "
	r2 := prompt.ParseResponse(text2, []string{"TOOLS"})

	if r2.Values["TOOLS"] != "Torque Wrench, Ratchet" {
		t.Errorf("TOOLS = %q, want %q", r2.Values["TOOLS"], "Torque Wrench, Ratchet")
	}

	// Hybrid format with array markers
	text3 := "- TOOLS:\n  - [\"Screwdriver\", \"Hammer\""
	r3 := prompt.ParseResponse(text3, []string{"TOOLS"})
	if r3.Values["TOOLS"] != "Screwdriver, Hammer" && r3.Values["TOOLS"] != "\"Screwdriver\", \"Hammer\"" {
		t.Errorf("Hybrid TOOLS = %q, want %q", r3.Values["TOOLS"], "\"Screwdriver\", \"Hammer\"")
	}
}
