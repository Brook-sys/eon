package gatecampaign

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"motor-autonomo/internal/modeltext"
)

// StructuralExpectation declares expected JSON fields and values for structural
// comparison. Unlike byte-for-byte equality, the structural oracle normalizes
// JSON objects and compares by: field presence, scalar value equality, array
// set-equality (order-independent), and recursive structural comparison of
// nested objects. This allows the oracle to distinguish reordering/whitespace
// from genuine semantic divergence without preserving redacted content.
//
// The oracle never transforms model output into authority. It reports
// structural facts that humans or downstream logic can use to classify
// whether a divergent response is semantically equivalent or not.
type StructuralExpectation struct {
	// Fields maps field names to expected values. Scalar values (strings,
	// numbers, bools) are compared by decoded Go value. Arrays are compared
	// as sets (order-independent). Objects are compared recursively.
	Fields map[string]any `json:"fields"`

	// OptionalFields are checked only if present in the response. If absent,
	// they are reported as "optional_absent" rather than "missing".
	OptionalFields []string `json:"optional_fields,omitempty"`

	// ArraySetEquality treats arrays as unordered sets when true (default).
	// When false, arrays must appear in the same order.
	ArraySetEquality bool `json:"array_set_equality,omitempty"`
}

// StructuralComparison is the result of comparing a model response against a
// StructuralExpectation. It records per-field outcomes without retaining
// response content, preserving only sanitized diagnoses.
type StructuralComparison struct {
	Configured       bool                     `json:"configured"`
	ArraySetEquality bool                     `json:"array_set_equality"`
	OverallMatch     bool                     `json:"overall_match"`
	FieldsChecked    int                      `json:"fields_checked"`
	FieldsMatched    int                      `json:"fields_matched"`
	FieldsMismatched int                      `json:"fields_mismatched"`
	FieldsAbsent     int                      `json:"fields_absent"`
	FieldOutcomes    []StructuralFieldOutcome `json:"field_outcomes"`
}

type StructuralFieldOutcome struct {
	Field  string `json:"field"`
	Status string `json:"status"` // matched, value_mismatch, type_mismatch, missing, optional_absent
	Reason string `json:"reason,omitempty"`
}

// compareJSONStructural parses the response text as JSON and compares it
// field-by-field against the expectation. It returns a StructuralComparison
// with sanitized per-field outcomes. If the response is not valid JSON, the
// comparison reports configured=true with overall_match=false and no field
// outcomes (the framing classification already captures invalidity).
func compareJSONStructural(responseText string, expectation StructuralExpectation) StructuralComparison {
	result := StructuralComparison{Configured: true, ArraySetEquality: true}
	if expectation.ArraySetEquality == false {
		result.ArraySetEquality = false
	}
	// Parse response
	var response map[string]json.RawMessage
	candidate := modeltext.BestJSONCandidate(responseText)
	if err := json.Unmarshal([]byte(candidate), &response); err != nil || response == nil {
		// Not valid JSON — framing classification handles this
		return result
	}
	optionalSet := make(map[string]bool, len(expectation.OptionalFields))
	for _, f := range expectation.OptionalFields {
		optionalSet[f] = true
	}
	result.FieldsChecked = len(expectation.Fields)
	allMatched := true
	for field, expected := range expectation.Fields {
		outcome := StructuralFieldOutcome{Field: field}
		rawVal, present := response[field]
		if !present {
			if optionalSet[field] {
				outcome.Status = "optional_absent"
				result.FieldsAbsent++
				// Optional absent does not break overall match
				result.FieldOutcomes = append(result.FieldOutcomes, outcome)
				continue
			}
			outcome.Status = "missing"
			result.FieldsAbsent++
			allMatched = false
			result.FieldOutcomes = append(result.FieldOutcomes, outcome)
			continue
		}
		match, reason := compareValues(rawVal, expected, result.ArraySetEquality)
		if match {
			outcome.Status = "matched"
			result.FieldsMatched++
		} else {
			outcome.Status = reason
			result.FieldsMismatched++
			allMatched = false
		}
		result.FieldOutcomes = append(result.FieldOutcomes, outcome)
	}
	result.OverallMatch = allMatched
	return result
}

// compareValues compares a json.RawMessage from the response against an
// expected Go value (any). It returns (matched bool, reason string) where
// reason is "value_mismatch" or "type_mismatch" for non-matches.
func compareValues(raw json.RawMessage, expected any, arraySetEquality bool) (bool, string) {
	trimmed := strings.TrimSpace(string(raw))
	switch expectedVal := expected.(type) {
	case string:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			var num json.Number
			if json.Unmarshal([]byte(trimmed), &num) == nil {
				return false, "type_mismatch"
			}
			return false, "type_mismatch"
		}
		if s == expectedVal {
			return true, ""
		}
		return false, "value_mismatch"
	case float64: // JSON numbers decode to float64
		if len(trimmed) == 0 || trimmed[0] == '"' || trimmed == "true" || trimmed == "false" || trimmed == "null" || trimmed[0] == '[' || trimmed[0] == '{' {
			return false, "type_mismatch"
		}
		var num json.Number
		if err := json.Unmarshal(raw, &num); err != nil {
			return false, "type_mismatch"
		}
		var expectedNum json.Number
		expectedNum = json.Number(fmt.Sprintf("%v", expectedVal))
		if num == expectedNum {
			return true, ""
		}
		// Try integer comparison too
		var expectedInt int
		if v, ok := expected.(float64); ok {
			expectedInt = int(v)
			if v == float64(expectedInt) {
				var respInt int
				if json.Unmarshal(raw, &respInt) == nil && respInt == expectedInt {
					return true, ""
				}
			}
		}
		return false, "value_mismatch"
	case bool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return false, "type_mismatch"
		}
		if b == expectedVal {
			return true, ""
		}
		return false, "value_mismatch"
	case int:
		var n int
		if err := json.Unmarshal(raw, &n); err != nil {
			return false, "type_mismatch"
		}
		if n == expectedVal {
			return true, ""
		}
		return false, "value_mismatch"
	case []any:
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return false, "type_mismatch"
		}
		if len(arr) != len(expectedVal) {
			return false, "value_mismatch"
		}
		if arraySetEquality {
			// Set comparison: each expected element must match one response element
			used := make([]bool, len(arr))
			for _, exp := range expectedVal {
				found := false
				for i, resp := range arr {
					if used[i] {
						continue
					}
					if match, _ := compareValues(resp, exp, arraySetEquality); match {
						used[i] = true
						found = true
						break
					}
				}
				if !found {
					return false, "value_mismatch"
				}
			}
			return true, ""
		}
		// Ordered comparison
		for i, exp := range expectedVal {
			if i >= len(arr) {
				return false, "value_mismatch"
			}
			if match, _ := compareValues(arr[i], exp, arraySetEquality); !match {
				return false, "value_mismatch"
			}
		}
		return true, ""
	case map[string]any:
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return false, "type_mismatch"
		}
		if len(obj) != len(expectedVal) {
			return false, "value_mismatch"
		}
		for k, exp := range expectedVal {
			rawVal, ok := obj[k]
			if !ok {
				return false, "value_mismatch"
			}
			if match, _ := compareValues(rawVal, exp, arraySetEquality); !match {
				return false, "value_mismatch"
			}
		}
		return true, ""
	default:
		// Unknown expected type — compare via JSON re-encoding
		expectedJSON, err := json.Marshal(expected)
		if err != nil {
			return false, "type_mismatch"
		}
		var normalizedExpected json.RawMessage = expectedJSON
		// Normalize both via re-encode
		var normResp any
		if err := json.Unmarshal(raw, &normResp); err != nil {
			return false, "type_mismatch"
		}
		respNormalized, _ := json.Marshal(normResp)
		if string(respNormalized) == string(normalizedExpected) {
			return true, ""
		}
		return false, "value_mismatch"
	}
}

// sortStructuralOutcomes ensures deterministic ordering of field outcomes in
// the report. Fields are sorted alphabetically.
func sortStructuralOutcomes(outcomes []StructuralFieldOutcome) {
	sort.Slice(outcomes, func(i, j int) bool {
		return outcomes[i].Field < outcomes[j].Field
	})
}
