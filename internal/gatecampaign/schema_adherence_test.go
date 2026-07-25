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
		FieldResults: []SchemaFieldResult{
			{Field: "read_set", Present: true, CorrectType: false, ObservedType: "string", ExpectedType: "array"},
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
	if !decoded.ChangesValid || decoded.ChangesChecked != 1 {
		t.Fatalf("changes round-trip mismatch: %+v", decoded)
	}
}
