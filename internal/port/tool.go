package port

import "encoding/json"

// ToolDefinition exposes a tool's capability and expected payload format.
// The schema must be valid JSON Schema describing an object.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}
