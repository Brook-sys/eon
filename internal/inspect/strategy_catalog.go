package inspect

import (
	"sort"
	"strings"

	"motor-autonomo/internal/domain"
)

// ContinuityStrategyDescriptor is a presentation-safe view of one continuity
// family entry. It never grants authority and does not embed implementations.
type ContinuityStrategyDescriptor struct {
	Name            string            `json:"name"`
	Family          domain.WorkFamily `json:"family"`
	Version         string            `json:"version"`
	Priority        uint8             `json:"priority"`
	RequiresModel   bool              `json:"requires_model"`
	RequiresNetwork bool              `json:"requires_network"`
	LocalOnly       bool              `json:"local_only"`
	// Ref is the stable name@version token used in diagnosis trails.
	Ref string `json:"ref"`
}

// ContinuityStrategyCatalog is the process-local, versioned portfolio projection.
// Catalogue evolution is a code change; this view only reports what the process
// registered at assembly time (or later via SetContinuityCatalog).
type ContinuityStrategyCatalog struct {
	SchemaVersion  int                            `json:"schema_version"`
	CatalogVersion string                         `json:"catalog_version"`
	StrategyCount  int                            `json:"strategy_count"`
	Strategies     []ContinuityStrategyDescriptor `json:"strategies"`
	StrategyRefs   []string                       `json:"strategy_refs"`
}

// BuildContinuityStrategyCatalog clones and normalizes descriptors into a
// stable projection. Order is preserved; empty catalog versions are allowed
// (tests may leave the portfolio unversioned).
func BuildContinuityStrategyCatalog(catalogVersion string, strategies []ContinuityStrategyDescriptor) ContinuityStrategyCatalog {
	out := ContinuityStrategyCatalog{
		SchemaVersion:  domain.SchemaVersionV1,
		CatalogVersion: strings.TrimSpace(catalogVersion),
		Strategies:     make([]ContinuityStrategyDescriptor, 0, len(strategies)),
		StrategyRefs:   make([]string, 0, len(strategies)),
	}
	for _, item := range strategies {
		name := strings.TrimSpace(item.Name)
		version := strings.TrimSpace(item.Version)
		ref := strings.TrimSpace(item.Ref)
		if ref == "" {
			if name != "" && version != "" {
				ref = name + "@" + version
			} else {
				ref = name
			}
		}
		out.Strategies = append(out.Strategies, ContinuityStrategyDescriptor{
			Name:            name,
			Family:          item.Family,
			Version:         version,
			Priority:        item.Priority,
			RequiresModel:   item.RequiresModel,
			RequiresNetwork: item.RequiresNetwork,
			LocalOnly:       item.LocalOnly,
			Ref:             ref,
		})
		if ref != "" {
			out.StrategyRefs = append(out.StrategyRefs, ref)
		}
	}
	out.StrategyCount = len(out.Strategies)
	return out
}

// Clone returns a deep copy safe for concurrent presentation.
func (c ContinuityStrategyCatalog) Clone() ContinuityStrategyCatalog {
	out := ContinuityStrategyCatalog{
		SchemaVersion:  c.SchemaVersion,
		CatalogVersion: c.CatalogVersion,
		StrategyCount:  c.StrategyCount,
		Strategies:     append([]ContinuityStrategyDescriptor(nil), c.Strategies...),
		StrategyRefs:   append([]string(nil), c.StrategyRefs...),
	}
	if out.SchemaVersion == 0 {
		out.SchemaVersion = domain.SchemaVersionV1
	}
	return out
}

// SetContinuityCatalog installs a defensive copy of the process portfolio on
// the projector. Empty catalogues clear the projection.
func (p *Projector) SetContinuityCatalog(catalog ContinuityStrategyCatalog) {
	if p == nil {
		return
	}
	if len(catalog.Strategies) == 0 && strings.TrimSpace(catalog.CatalogVersion) == "" {
		p.continuityCatalog = nil
		return
	}
	cloned := catalog.Clone()
	p.continuityCatalog = &cloned
}

// ContinuityCatalog returns a clone of the process portfolio, if configured.
func (p *Projector) ContinuityCatalog() (ContinuityStrategyCatalog, bool) {
	if p == nil || p.continuityCatalog == nil {
		return ContinuityStrategyCatalog{}, false
	}
	return p.continuityCatalog.Clone(), true
}

// catalogVersionFromSafeDetail extracts the trailing catalog= token that the
// scheduler embeds in ContinuityDiagnosis.SafeDetail (when registry versioned).
func catalogVersionFromSafeDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	const marker = "catalog="
	idx := strings.LastIndex(detail, marker)
	if idx < 0 {
		return ""
	}
	raw := strings.TrimSpace(detail[idx+len(marker):])
	if raw == "" {
		return ""
	}
	// Stop at common separators if more fields are appended later.
	if cut := strings.IndexAny(raw, ";, \t"); cut >= 0 {
		raw = strings.TrimSpace(raw[:cut])
	}
	return raw
}

// sortedStrategyNames is a test/helper utility for stable assertion text.
func sortedStrategyNames(strategies []ContinuityStrategyDescriptor) []string {
	names := make([]string, 0, len(strategies))
	for _, s := range strategies {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return names
}
