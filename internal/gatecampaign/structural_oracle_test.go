package gatecampaign

import "testing"

func TestCompareJSONStructuralIgnoresObjectAndConfiguredArrayOrder(t *testing.T) {
	expectation := StructuralExpectation{
		Fields: map[string]any{
			"status": "ready",
			"score":  float64(2),
			"tags":   []any{"safe", "bounded"},
			"meta":   map[string]any{"enabled": true},
		},
		ArraySetEquality: true,
	}
	report := compareJSONStructural(`{"meta":{"enabled":true},"tags":["bounded","safe"],"score":2,"status":"ready"}`, expectation)
	if !report.OverallMatch || report.FieldsMatched != 4 || report.FieldsMismatched != 0 || report.FieldsAbsent != 0 {
		t.Fatalf("unexpected comparison: %+v", report)
	}
}

func TestCompareJSONStructuralLocalizesDivergence(t *testing.T) {
	expectation := StructuralExpectation{
		Fields: map[string]any{
			"status": "ready",
			"score":  float64(2),
			"tags":   []any{"safe", "bounded"},
			"needed": true,
			"note":   "optional",
		},
		OptionalFields:   []string{"note"},
		ArraySetEquality: true,
	}
	report := compareJSONStructural(`{"status":"blocked","score":"2","tags":["safe"],"extra":1}`, expectation)
	if report.OverallMatch || report.FieldsMismatched != 3 || report.FieldsAbsent != 2 {
		t.Fatalf("unexpected comparison: %+v", report)
	}
	statuses := map[string]string{}
	for _, outcome := range report.FieldOutcomes {
		statuses[outcome.Field] = outcome.Status
	}
	want := map[string]string{
		"status": "value_mismatch", "score": "type_mismatch", "tags": "value_mismatch",
		"needed": "missing", "note": "optional_absent",
	}
	for field, status := range want {
		if statuses[field] != status {
			t.Errorf("%s status = %q, want %q", field, statuses[field], status)
		}
	}
}

func TestCompareJSONStructuralCanRequireArrayOrder(t *testing.T) {
	expectation := StructuralExpectation{Fields: map[string]any{"steps": []any{"first", "second"}}}
	report := compareJSONStructural(`{"steps":["second","first"]}`, expectation)
	if report.OverallMatch || report.ArraySetEquality {
		t.Fatalf("ordered comparison unexpectedly matched: %+v", report)
	}
}

func TestCompareJSONStructuralRejectsInvalidJSON(t *testing.T) {
	report := compareJSONStructural(`not-json`, StructuralExpectation{Fields: map[string]any{"status": "ready"}})
	if !report.Configured || report.OverallMatch || report.FieldsMatched != 0 {
		t.Fatalf("unexpected invalid JSON comparison: %+v", report)
	}
}
