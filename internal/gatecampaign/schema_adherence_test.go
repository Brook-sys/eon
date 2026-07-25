package gatecampaign

import (
	"encoding/json"
	"testing"
)

func TestEvaluateProposedChangeSetAdherenceValid(t *testing.T) {
	valid := `{
		"schema_version": 1,
		"id": "cs_001",
		"mission_revision_id": "rev_1",
		"operation_id": "op_1",
		"base_commit_id": "commit_1",
		"read_set": ["manifest"],
		"preconditions": [],
		"changes": [
			{"kind": "upsert", "entity_type": "observation", "entity_id": "obs_1", "payload_ref": "artifact_1"}
		],
		"expected_delta": "one observation",
		"validator_ids": ["schema"],
		"provenance": "probe",
		"idempotency_key": "key_1"
	}`
	report := evaluateProposedChangeSetAdherence(valid)
	if !report.SchemaValid {
		t.Fatal("schema_valid must be true for valid JSON")
	}
	if report.FieldsChecked != 12 {
		t.Fatalf("fields_checked = %d, want 12", report.FieldsChecked)
	}
	if report.FieldsPresent != 12 {
		t.Fatalf("fields_present = %d, want 12", report.FieldsPresent)
	}
	if report.FieldsCorrectType != 12 {
		t.Fatalf("fields_correct_type = %d, want 12", report.FieldsCorrectType)
	}
	if !report.ChangesValid {
		t.Fatal("changes_valid must be true when all change sub-fields are present")
	}
	if report.ChangesChecked != 1 {
		t.Fatalf("changes_checked = %d, want 1", report.ChangesChecked)
	}
	if report.ChangesWithAllFields != 1 {
		t.Fatalf("changes_with_all_fields = %d, want 1", report.ChangesWithAllFields)
	}
	// NonEmpty is tracked for strings and arrays only, not numbers.
	// schema_version is number (not tracked) and preconditions is [] (empty array) = 10 non-empty.
	if report.FieldsNonEmpty != 10 {
		t.Fatalf("fields_non_empty = %d, want 10", report.FieldsNonEmpty)
	}
	for _, fr := range report.FieldResults {
		switch fr.Field {
		case "schema_version":
			// number type — NonEmpty not tracked
			if fr.NonEmpty {
				t.Fatalf("field %q (number) should not track non_empty", fr.Field)
			}
		case "preconditions":
			// empty array in test fixture
			if fr.NonEmpty {
				t.Fatalf("field %q (empty array) should not be non_empty", fr.Field)
			}
		default:
			if !fr.NonEmpty {
				t.Fatalf("field %q non_empty should be true", fr.Field)
			}
		}
	}
}

func TestEvaluateProposedChangeSetAdherenceArrayAsString(t *testing.T) {
	// read_set as a string instead of array — the exact regression from Phase 210
	invalid := `{
		"schema_version": 1,
		"id": "cs_001",
		"mission_revision_id": "rev_1",
		"operation_id": "op_1",
		"base_commit_id": "commit_1",
		"read_set": "manifest",
		"preconditions": [],
		"changes": [
			{"kind": "upsert", "entity_type": "observation", "entity_id": "obs_1", "payload_ref": "artifact_1"}
		],
		"expected_delta": "one observation",
		"validator_ids": ["schema"],
		"provenance": "probe",
		"idempotency_key": "key_1"
	}`
	report := evaluateProposedChangeSetAdherence(invalid)
	if !report.SchemaValid {
		t.Fatal("schema_valid must be true (JSON is valid)")
	}
	if report.FieldsCorrectType != 11 {
		t.Fatalf("fields_correct_type = %d, want 11 (read_set is wrong type)", report.FieldsCorrectType)
	}
	found := false
	for _, fr := range report.FieldResults {
		if fr.Field == "read_set" {
			if !fr.Present {
				t.Fatal("read_set should be present (key exists with wrong type)")
			}
			if fr.CorrectType {
				t.Fatal("read_set should not have correct type (string instead of array)")
			}
			if fr.ObservedType != "string" {
				t.Fatalf("observed_type = %q, want \"string\"", fr.ObservedType)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("read_set field result not found")
	}
}

func TestEvaluateProposedChangeSetAdherenceExpectedDeltaAsArray(t *testing.T) {
	// expected_delta as an array — the exact regression from Phase 207
	invalid := `{
		"schema_version": 1,
		"id": "cs_001",
		"mission_revision_id": "rev_1",
		"operation_id": "op_1",
		"base_commit_id": "commit_1",
		"read_set": ["manifest"],
		"preconditions": [],
		"changes": [
			{"kind": "upsert", "entity_type": "observation", "entity_id": "obs_1", "payload_ref": "artifact_1"}
		],
		"expected_delta": ["one", "observation"],
		"validator_ids": ["schema"],
		"provenance": "probe",
		"idempotency_key": "key_1"
	}`
	report := evaluateProposedChangeSetAdherence(invalid)
	if !report.SchemaValid {
		t.Fatal("schema_valid must be true (JSON is valid)")
	}
	if report.FieldsCorrectType != 11 {
		t.Fatalf("fields_correct_type = %d, want 11 (expected_delta is wrong type)", report.FieldsCorrectType)
	}
	for _, fr := range report.FieldResults {
		if fr.Field == "expected_delta" {
			if fr.CorrectType {
				t.Fatal("expected_delta should not have correct type (array instead of string)")
			}
			if fr.ObservedType != "other" {
				t.Fatalf("observed_type = %q, want \"other\" for array", fr.ObservedType)
			}
			break
		}
	}
}

func TestEvaluateProposedChangeSetAdherenceMissingFields(t *testing.T) {
	partial := `{
		"schema_version": 1,
		"id": "cs_001",
		"changes": []
	}`
	report := evaluateProposedChangeSetAdherence(partial)
	if !report.SchemaValid {
		t.Fatal("schema_valid must be true (JSON parses)")
	}
	if report.FieldsPresent != 3 {
		t.Fatalf("fields_present = %d, want 3 (schema_version, id, changes)", report.FieldsPresent)
	}
	if report.FieldsCorrectType != 3 {
		t.Fatalf("fields_correct_type = %d, want 3", report.FieldsCorrectType)
	}
	// changes is empty array, so changes_valid should be false (no entries to check)
	if report.ChangesValid {
		t.Fatal("changes_valid should be false for empty changes array")
	}
	// id is non-empty string, schema_version is number (NonEmpty not tracked for numbers), changes is empty array
	// Only "id" should be non-empty (string); schema_version is number; changes is empty array
	for _, fr := range report.FieldResults {
		switch fr.Field {
		case "id":
			if !fr.NonEmpty {
				t.Fatal("id should be non_empty")
			}
		case "changes":
			if fr.NonEmpty {
				t.Fatal("empty changes array should not be non_empty")
			}
		}
	}
}

func TestEvaluateProposedChangeSetAdherenceNonEmptyTracking(t *testing.T) {
	// Adversarial: all typed fields present and correct, but strings are empty
	// and arrays are empty — NonEmpty should be false for those.
	emptyStrings := `{
		"schema_version": 1,
		"id": "",
		"mission_revision_id": "",
		"operation_id": "",
		"base_commit_id": "",
		"read_set": [],
		"preconditions": [],
		"changes": [],
		"expected_delta": "",
		"validator_ids": [],
		"provenance": "",
		"idempotency_key": ""
	}`
	report := evaluateProposedChangeSetAdherence(emptyStrings)
	if !report.SchemaValid {
		t.Fatal("schema_valid must be true (valid JSON)")
	}
	if report.FieldsPresent != 12 {
		t.Fatalf("fields_present = %d, want 12", report.FieldsPresent)
	}
	if report.FieldsCorrectType != 12 {
		t.Fatalf("fields_correct_type = %d, want 12 (types match even if empty)", report.FieldsCorrectType)
	}
	// schema_version is number (NonEmpty not tracked), all strings empty, all arrays empty
	// preconditions is allowed empty, but NonEmpty tracks content regardless of semantics
	if report.FieldsNonEmpty != 0 {
		t.Fatalf("fields_non_empty = %d, want 0 (all strings/arrays empty)", report.FieldsNonEmpty)
	}
	for _, fr := range report.FieldResults {
		if fr.NonEmpty {
			t.Fatalf("field %q should not be non_empty with empty content", fr.Field)
		}
	}

	// Mix: some non-empty, some empty
	mixed := `{
		"schema_version": 1,
		"id": "cs_001",
		"mission_revision_id": "",
		"operation_id": "op_1",
		"base_commit_id": "",
		"read_set": ["manifest"],
		"preconditions": [],
		"changes": [
			{"kind": "upsert", "entity_type": "observation", "entity_id": "obs_1", "payload_ref": "artifact_1"}
		],
		"expected_delta": "one observation",
		"validator_ids": [],
		"provenance": "   ",
		"idempotency_key": "key_1"
	}`
	report2 := evaluateProposedChangeSetAdherence(mixed)
	if !report2.SchemaValid {
		t.Fatal("schema_valid must be true")
	}
	// Non-empty: id, operation_id, read_set, changes, expected_delta, idempotency_key = 6
	// Empty: mission_revision_id, base_commit_id, preconditions, validator_ids, provenance (whitespace only) = 5
	// schema_version is number, not tracked = 0
	wantNonEmpty := 6
	if report2.FieldsNonEmpty != wantNonEmpty {
		t.Fatalf("fields_non_empty = %d, want %d", report2.FieldsNonEmpty, wantNonEmpty)
	}
	// Verify specific fields
	expected := map[string]bool{
		"schema_version":      false, // number, not tracked
		"id":                  true,
		"mission_revision_id": false,
		"operation_id":        true,
		"base_commit_id":      false,
		"read_set":            true,
		"preconditions":       false,
		"changes":             true,
		"expected_delta":      true,
		"validator_ids":       false,
		"provenance":          false, // whitespace-only
		"idempotency_key":     true,
	}
	for _, fr := range report2.FieldResults {
		want, ok := expected[fr.Field]
		if !ok {
			continue
		}
		if fr.NonEmpty != want {
			t.Errorf("field %q non_empty = %v, want %v (observed_type=%s)", fr.Field, fr.NonEmpty, want, fr.ObservedType)
		}
	}
}

func TestEvaluateProposedChangeSetAdherenceInvalidJSON(t *testing.T) {
	report := evaluateProposedChangeSetAdherence("not json at all")
	if report.SchemaValid {
		t.Fatal("schema_valid must be false for non-JSON")
	}
	if report.FieldsChecked != 0 {
		t.Fatalf("fields_checked = %d, want 0", report.FieldsChecked)
	}
}

func TestEvaluateProposedChangeSetAdherenceFencedJSON(t *testing.T) {
	// Model wraps JSON in markdown fence — BestJSONCandidate should extract it
	fenced := "```json\n" + `{
		"schema_version": 1,
		"id": "cs_001",
		"mission_revision_id": "rev_1",
		"operation_id": "op_1",
		"base_commit_id": "commit_1",
		"read_set": ["manifest"],
		"preconditions": [],
		"changes": [
			{"kind": "upsert", "entity_type": "observation", "entity_id": "obs_1", "payload_ref": "artifact_1"}
		],
		"expected_delta": "one observation",
		"validator_ids": ["schema"],
		"provenance": "probe",
		"idempotency_key": "key_1"
	}` + "\n```"
	report := evaluateProposedChangeSetAdherence(fenced)
	if !report.SchemaValid {
		t.Fatal("schema_valid must be true (fence should be extracted)")
	}
	if report.FieldsPresent != 12 {
		t.Fatalf("fields_present = %d, want 12", report.FieldsPresent)
	}
}

func TestEvaluateProposedChangeSetAdherenceChangesMissingSubField(t *testing.T) {
	missingSub := `{
		"schema_version": 1,
		"id": "cs_001",
		"mission_revision_id": "rev_1",
		"operation_id": "op_1",
		"base_commit_id": "commit_1",
		"read_set": ["manifest"],
		"preconditions": [],
		"changes": [
			{"kind": "upsert", "entity_type": "observation", "entity_id": "obs_1"}
		],
		"expected_delta": "one observation",
		"validator_ids": ["schema"],
		"provenance": "probe",
		"idempotency_key": "key_1"
	}`
	report := evaluateProposedChangeSetAdherence(missingSub)
	if !report.SchemaValid {
		t.Fatal("schema_valid must be true")
	}
	if report.FieldsCorrectType != 12 {
		t.Fatalf("fields_correct_type = %d, want 12", report.FieldsCorrectType)
	}
	if report.ChangesValid {
		t.Fatal("changes_valid must be false when payload_ref is missing from a change")
	}
	if report.ChangesChecked != 1 {
		t.Fatalf("changes_checked = %d, want 1", report.ChangesChecked)
	}
	if report.ChangesWithAllFields != 0 {
		t.Fatalf("changes_with_all_fields = %d, want 0", report.ChangesWithAllFields)
	}
}

func TestSchemaAdherenceReportJSONRoundTrip(t *testing.T) {
	report := SchemaAdherenceReport{
		SchemaValid:       true,
		FieldsChecked:     12,
		FieldsPresent:     12,
		FieldsCorrectType: 11,
		FieldsNonEmpty:    9,
		FieldResults: []SchemaFieldResult{
			{Field: "read_set", Present: true, CorrectType: false, NonEmpty: true, ObservedType: "string", ExpectedType: "array"},
			{Field: "id", Present: true, CorrectType: true, NonEmpty: false, ObservedType: "string", ExpectedType: "string"},
		},
		ChangesValid:         true,
		ChangesChecked:       1,
		ChangesWithAllFields: 1,
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded SchemaAdherenceReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.FieldsChecked != 12 || decoded.FieldsPresent != 12 || decoded.FieldsCorrectType != 11 {
		t.Fatalf("round-trip mismatch: %+v", decoded)
	}
	if decoded.FieldsNonEmpty != 9 {
		t.Fatalf("fields_non_empty round-trip = %d, want 9", decoded.FieldsNonEmpty)
	}
	if !decoded.ChangesValid || decoded.ChangesChecked != 1 {
		t.Fatalf("changes round-trip mismatch: %+v", decoded)
	}
	// Verify NonEmpty round-trips per field
	if !decoded.FieldResults[0].NonEmpty {
		t.Fatal("read_set NonEmpty should survive round-trip")
	}
	if decoded.FieldResults[1].NonEmpty {
		t.Fatal("id NonEmpty=false should survive round-trip")
	}
}
