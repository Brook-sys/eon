// Package tool defines kernel-owned tool catalogs. Model output may request a
// tool by name, but only the kernel may resolve and execute that request.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode"

	"motor-autonomo/internal/port"
)

const (
	maxTools       = 32
	maxNameBytes   = 64
	maxDescription = 1024
	maxSchemaBytes = 16 << 10
)

// Tool is an operational capability owned by the deterministic kernel.
// Implementations must not be handed directly to a model adapter.
type Tool interface {
	Definition() port.ToolDefinition
	Execute(context.Context, json.RawMessage) (string, error)
}

// Provider exposes a bounded, immutable catalog. Lookup does not grant
// authority: callers must still authorize and audit execution separately.
type Provider interface {
	Definitions() []port.ToolDefinition
	Find(name string) (Tool, bool)
}

// Catalog is an immutable Provider assembled at bootstrap.
type Catalog struct {
	ordered []Tool
	byName  map[string]Tool
}

func NewCatalog(tools ...Tool) (*Catalog, error) {
	if len(tools) > maxTools {
		return nil, errors.New("tool catalog exceeds limit")
	}
	byName := make(map[string]Tool, len(tools))
	for _, candidate := range tools {
		if candidate == nil {
			return nil, errors.New("nil tool")
		}
		definition := candidate.Definition()
		if err := ValidateDefinition(definition); err != nil {
			return nil, err
		}
		if _, exists := byName[definition.Name]; exists {
			return nil, errors.New("duplicate tool name")
		}
		byName[definition.Name] = candidate
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	ordered := make([]Tool, 0, len(names))
	for _, name := range names {
		ordered = append(ordered, byName[name])
	}
	return &Catalog{ordered: ordered, byName: byName}, nil
}

func (c *Catalog) Definitions() []port.ToolDefinition {
	if c == nil {
		return nil
	}
	definitions := make([]port.ToolDefinition, 0, len(c.ordered))
	for _, candidate := range c.ordered {
		definition := candidate.Definition()
		definition.Parameters = append(json.RawMessage(nil), definition.Parameters...)
		definitions = append(definitions, definition)
	}
	return definitions
}

func (c *Catalog) Find(name string) (Tool, bool) {
	if c == nil {
		return nil, false
	}
	candidate, ok := c.byName[name]
	return candidate, ok
}

func ValidateDefinition(definition port.ToolDefinition) error {
	if !validName(definition.Name) {
		return errors.New("invalid tool name")
	}
	if strings.TrimSpace(definition.Description) == "" || len(definition.Description) > maxDescription {
		return errors.New("invalid tool description")
	}
	if len(definition.Parameters) == 0 || len(definition.Parameters) > maxSchemaBytes || !json.Valid(definition.Parameters) {
		return errors.New("invalid tool parameter schema")
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(definition.Parameters, &schema); err != nil {
		return errors.New("tool parameter schema must be an object")
	}
	var schemaType string
	if raw, ok := schema["type"]; !ok || json.Unmarshal(raw, &schemaType) != nil || schemaType != "object" {
		return errors.New("tool parameter schema type must be object")
	}
	return nil
}

func validName(name string) bool {
	if name == "" || len(name) > maxNameBytes {
		return false
	}
	for i, r := range name {
		if unicode.IsLetter(r) || r == '_' || (i > 0 && (unicode.IsDigit(r) || r == '-')) {
			continue
		}
		return false
	}
	return true
}
