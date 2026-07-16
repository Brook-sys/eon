package domain

import (
	"fmt"
	"sort"
	"strings"
)

// Dependency reference prefixes for KnowledgeArtifact.Dependencies.
// Format: "<kind>:<id>" or "claim:<id>@<version>".
const (
	DependencyKindClaim          = "claim"
	DependencyKindEvidenceLink   = "evidence_link"
	DependencyKindObservation    = "observation"
	DependencyKindSourceFragment = "source_fragment"
	DependencyKindSourceVersion  = "source_version"
	DependencyKindSource         = "source"
	DependencyKindCanonical      = "canonical"
)

// FormatClaimDependency encodes a versioned claim dependency.
func FormatClaimDependency(id ClaimID, version uint64) string {
	return fmt.Sprintf("%s:%s@%d", DependencyKindClaim, id, version)
}

// FormatDependency encodes a non-versioned dependency reference.
func FormatDependency(kind, id string) string {
	return kind + ":" + id
}

// FormatCanonicalDependency addresses a changeset target in the dependency graph.
func FormatCanonicalDependency(entityType, entityID string) string {
	return FormatDependency(DependencyKindCanonical, entityType+"/"+entityID)
}

// ParseDependencyRef splits "kind:id" or "claim:id@version".
// For claim refs, id is the claim id without the @version suffix; version is returned separately.
func ParseDependencyRef(ref string) (kind, id string, version uint64, ok bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", 0, false
	}
	kind, rest, found := strings.Cut(ref, ":")
	if !found || strings.TrimSpace(kind) == "" || strings.TrimSpace(rest) == "" {
		return "", "", 0, false
	}
	kind = strings.TrimSpace(kind)
	rest = strings.TrimSpace(rest)
	if kind == DependencyKindClaim {
		base, verText, hasVersion := strings.Cut(rest, "@")
		if !hasVersion || strings.TrimSpace(base) == "" {
			return kind, rest, 0, true
		}
		var parsed uint64
		for _, r := range verText {
			if r < '0' || r > '9' {
				return kind, rest, 0, true
			}
			parsed = parsed*10 + uint64(r-'0')
		}
		return kind, strings.TrimSpace(base), parsed, true
	}
	return kind, rest, 0, true
}

// DependencyKey is the unversioned match key used for invalidation cascade
// (claim:id matches claim:id@N for any N).
func DependencyKey(ref string) string {
	kind, id, _, ok := ParseDependencyRef(ref)
	if !ok {
		return strings.TrimSpace(ref)
	}
	return kind + ":" + id
}

// ArtifactDependsOn reports whether artifact.Dependencies reference any of the keys.
// Keys may be full refs or unversioned "kind:id" forms.
func ArtifactDependsOn(artifact KnowledgeArtifact, keys ...string) bool {
	if artifact.Stale || len(keys) == 0 || len(artifact.Dependencies) == 0 {
		return false
	}
	wanted := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		wanted[DependencyKey(key)] = struct{}{}
		// Also accept exact ref as provided.
		wanted[key] = struct{}{}
	}
	if len(wanted) == 0 {
		return false
	}
	for _, dep := range artifact.Dependencies {
		if _, ok := wanted[dep]; ok {
			return true
		}
		if _, ok := wanted[DependencyKey(dep)]; ok {
			return true
		}
	}
	return false
}

// PlanArtifactInvalidation selects non-stale artifacts whose dependencies intersect
// changedKeys. Results are sorted by artifact ID for deterministic application.
// Local audit/report kinds are skipped so continuity trails remain readable.
func PlanArtifactInvalidation(artifacts []KnowledgeArtifact, changedKeys []string, skipKind func(string) bool) []ArtifactID {
	if len(artifacts) == 0 || len(changedKeys) == 0 {
		return nil
	}
	var out []ArtifactID
	for _, artifact := range artifacts {
		if artifact.Stale {
			continue
		}
		if skipKind != nil && skipKind(artifact.Kind) {
			continue
		}
		if ArtifactDependsOn(artifact, changedKeys...) {
			out = append(out, artifact.ID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ChangeDependencyKeys maps official Change targets to dependency keys that
// derived artifacts may declare (entity type id and canonical composite).
func ChangeDependencyKeys(changes []Change) []string {
	if len(changes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(changes)*2)
	var keys []string
	add := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for _, change := range changes {
		entityType := strings.TrimSpace(change.EntityType)
		entityID := strings.TrimSpace(change.EntityID)
		if entityType == "" || entityID == "" {
			continue
		}
		// Entity-type keys match cited-view dependency prefixes (claim:, observation:, ...).
		add(FormatDependency(entityType, entityID))
		add(FormatCanonicalDependency(entityType, entityID))
	}
	sort.Strings(keys)
	return keys
}

// EvidenceDeltaDependencyKeys keys invalidated when evidence links are appended
// to a claim. Observations are immutable once stored, so only the claim identity
// and the new evidence_link ids are cascade keys (FR-KNOW-005).
func EvidenceDeltaDependencyKeys(claimID ClaimID, links []EvidenceLink) []string {
	if claimID == "" {
		return nil
	}
	seen := map[string]struct{}{
		FormatDependency(DependencyKindClaim, string(claimID)): {},
	}
	keys := []string{FormatDependency(DependencyKindClaim, string(claimID))}
	for _, link := range links {
		if link.ID == "" {
			continue
		}
		key := FormatDependency(DependencyKindEvidenceLink, string(link.ID))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// IsLocalAuditArtifactKind reports continuity/local audit report kinds that
// must not be cascade-staled by dependency invalidation (FR-KNOW-005 refresh
// still may mark non-audit derived views).
func IsLocalAuditArtifactKind(kind string) bool {
	switch kind {
	case "local_operation_audit", "integrity_audit_report", "frontier_manage_report",
		"gap_scan_report", "coverage_scan_report", "source_freshness_report",
		"artifact_refresh_report", "conflict_review_report", "harness_evaluation_report":
		return true
	default:
		return false
	}
}
