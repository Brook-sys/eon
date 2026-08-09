package prompt

import (
	"testing"
)

func TestParseResponse_JSONFallback(t *testing.T) {
	keys := []string{"NAME", "AGE", "STATUS"}
	
	fencedJSON := "Here is your response:\n```json\n{\n  \"NAME\": \"Alice\",\n  \"AGE\": \"30\",\n  \"STATUS\": \"Active\"\n}\n```"
	res := ParseResponse(fencedJSON, keys)
	if res.Strategy != ParseStrategyJSONFallback {
		t.Errorf("Expected JSONFallback strategy for fenced JSON, got %v", res.Strategy)
	}
	if res.Values["NAME"] != "Alice" || res.Values["AGE"] != "30" || res.Values["STATUS"] != "Active" {
		t.Errorf("Failed to parse fenced JSON values: %v", res.Values)
	}

	bareJSON := "{\n  \"NAME\": \"Bob\",\n  \"AGE\": \"45\",\n  \"STATUS\": \"Inactive\"\n}"
	res2 := ParseResponse(bareJSON, keys)
	if res2.Strategy != ParseStrategyJSONFallback {
		t.Errorf("Expected JSONFallback strategy for bare JSON, got %v", res2.Strategy)
	}
	if res2.Values["NAME"] != "Bob" || res2.Values["AGE"] != "45" || res2.Values["STATUS"] != "Inactive" {
		t.Errorf("Failed to parse bare JSON values: %v", res2.Values)
	}
}
