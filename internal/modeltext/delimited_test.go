package modeltext

import (
	"encoding/json"
	"strings"
	"testing"
)

const validDelimited = `CHANGESET_DELIMITED_V1
SCHEMA_VERSION: 1
ID: "cs-1"
MISSION_REVISION_ID: "rev-1"
OPERATION_ID: "op-1"
BASE_COMMIT_ID: "commit-1"
READ_SET: []
PRECONDITIONS: []
CHANGES: [{"kind":"UPSERT_CLAIM","target_id":"claim-1","payload":"x"}]
EXPECTED_DELTA: "one claim"
VALIDATOR_IDS: ["v1"]
PROVENANCE: "model:test"
IDEMPOTENCY_KEY: "idem-1"`

func TestDelimitedChangeSetJSONStrictConversion(t *testing.T) {
	got, err := DelimitedChangeSetJSON(validDelimited)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(got)) || !strings.Contains(got, `"operation_id":"op-1"`) {
		t.Fatalf("converted = %q", got)
	}
	for _, bad := range []string{
		strings.Replace(validDelimited, "ID: \"cs-1\"\n", "", 1),
		validDelimited + "\nID: \"again\"",
		validDelimited + "\nSURPRISE: true",
		strings.Replace(validDelimited, `READ_SET: []`, `READ_SET: nope`, 1),
	} {
		if _, err := DelimitedChangeSetJSON(bad); err == nil {
			t.Fatalf("accepted invalid delimited payload: %q", bad)
		}
	}
}
