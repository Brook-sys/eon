package prompt

import (
	"testing"
)

func TestParseResponse_BareKeyNextLineValue(t *testing.T) {
	// Models sometimes emit bare key names on one line and values on the next:
	//   DATE
	//   2026-08-09
	//   SOURCE
	//   Audit Log Beta
	//   STATUS
	//   PENDING
	// The parser should recognize bare keys and recover values from subsequent lines.
	text := "DATE\n2026-08-09\nSOURCE\nAudit Log Beta\nSTATUS\nPENDING"

	r := ParseResponse(text, []string{"DATE", "SOURCE", "STATUS"})

	if r.Values["DATE"] != "2026-08-09" {
		t.Errorf("DATE=%q, want '2026-08-09'", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "Audit Log Beta" {
		t.Errorf("SOURCE=%q, want 'Audit Log Beta'", r.Values["SOURCE"])
	}
	if r.Values["STATUS"] != "PENDING" {
		t.Errorf("STATUS=%q, want 'PENDING'", r.Values["STATUS"])
	}
	if r.UsedFallback {
		t.Fatal("should not use positional fallback when bare keys are detected")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Errorf("Strategy=%v, want %v", r.Strategy, ParseStrategyPrimary)
	}
}

func TestParseResponse_BareKeyNextLineValueWithBlankLines(t *testing.T) {
	// Bare keys with blank lines between key and value.
	text := "DATE\n\n2026-08-09\nSOURCE\n\nAudit Log Beta\nSTATUS\n\nPENDING"

	r := ParseResponse(text, []string{"DATE", "SOURCE", "STATUS"})

	if r.Values["DATE"] != "2026-08-09" {
		t.Errorf("DATE=%q, want '2026-08-09'", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "Audit Log Beta" {
		t.Errorf("SOURCE=%q, want 'Audit Log Beta'", r.Values["SOURCE"])
	}
	if r.Values["STATUS"] != "PENDING" {
		t.Errorf("STATUS=%q, want 'PENDING'", r.Values["STATUS"])
	}
}

func TestParseResponse_BareKeyStopsAtNextBareKey(t *testing.T) {
	// If no value line between two bare keys, the first key should not
	// accidentally grab the next key name as its value.
	text := "DATE\nSOURCE\nAudit Log Beta\nSTATUS\nPENDING"

	r := ParseResponse(text, []string{"DATE", "SOURCE", "STATUS"})

	// DATE has no value (next line is another bare key).
	if r.Values["DATE"] != "" {
		t.Errorf("DATE=%q, want empty (no value before next bare key)", r.Values["DATE"])
	}
	// SOURCE should get "Audit Log Beta"
	if r.Values["SOURCE"] != "Audit Log Beta" {
		t.Errorf("SOURCE=%q, want 'Audit Log Beta'", r.Values["SOURCE"])
	}
	// STATUS should get "PENDING"
	if r.Values["STATUS"] != "PENDING" {
		t.Errorf("STATUS=%q, want 'PENDING'", r.Values["STATUS"])
	}
}

func TestParseResponse_BareKeyMixedWithColonKeys(t *testing.T) {
	// Mix of bare key (next-line value) and normal colon key on same response.
	text := "DATE\n2026-08-09\nSOURCE: Audit Log Beta\nSTATUS: PENDING"

	r := ParseResponse(text, []string{"DATE", "SOURCE", "STATUS"})

	if r.Values["DATE"] != "2026-08-09" {
		t.Errorf("DATE=%q, want '2026-08-09'", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "Audit Log Beta" {
		t.Errorf("SOURCE=%q, want 'Audit Log Beta'", r.Values["SOURCE"])
	}
	if r.Values["STATUS"] != "PENDING" {
		t.Errorf("STATUS=%q, want 'PENDING'", r.Values["STATUS"])
	}
}
