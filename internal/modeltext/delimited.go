package modeltext

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const delimitedChangeSetHeader = "CHANGESET_DELIMITED_V1"

var delimitedChangeSetKeys = []string{
	"schema_version", "id", "mission_revision_id", "operation_id",
	"base_commit_id", "read_set", "preconditions", "changes",
	"expected_delta", "validator_ids", "provenance", "idempotency_key",
}

// DelimitedChangeSetJSON converts the explicitly versioned line format into a
// canonical JSON object. It is intentionally not a generic YAML/KV parser:
// unknown, duplicate, missing, multiline, or non-JSON values fail closed.
func DelimitedChangeSetJSON(text string) (string, error) {
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	// Standardize markdown stripping
	text = strings.TrimPrefix(text, "```markdown")
	text = strings.TrimPrefix(text, "```text")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != delimitedChangeSetHeader {
		return "", errors.New("delimited changeset header is required")
	}
	allowed := make(map[string]struct{}, len(delimitedChangeSetKeys))
	for _, key := range delimitedChangeSetKeys {
		allowed[key] = struct{}{}
	}
	values := make(map[string]json.RawMessage, len(delimitedChangeSetKeys))
	for _, rawLine := range lines[1:] {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return "", errors.New("delimited changeset line must use KEY: JSON_VALUE")
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		if _, ok := allowed[key]; !ok {
			return "", fmt.Errorf("unknown delimited changeset key %q", key)
		}
		if _, exists := values[key]; exists {
			return "", fmt.Errorf("duplicate delimited changeset key %q", key)
		}
		value := bytes.TrimSpace([]byte(parts[1]))
		if len(value) == 0 || !json.Valid(value) {
			return "", fmt.Errorf("invalid JSON value for delimited changeset key %q", key)
		}
		values[key] = append(json.RawMessage(nil), value...)
	}
	var out bytes.Buffer
	out.WriteByte('{')
	for i, key := range delimitedChangeSetKeys {
		value, ok := values[key]
		if !ok {
			return "", fmt.Errorf("missing delimited changeset key %q", key)
		}
		if i > 0 {
			out.WriteByte(',')
		}
		encodedKey, _ := json.Marshal(key)
		out.Write(encodedKey)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return out.String(), nil
}
