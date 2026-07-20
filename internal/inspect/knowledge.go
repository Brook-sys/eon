package inspect

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// DefaultKnowledgeListLimit caps knowledge browse lists for the Control API.
const DefaultKnowledgeListLimit = 50

// MaxKnowledgeListLimit is the hard ceiling for knowledge list endpoints.
const MaxKnowledgeListLimit = 200

// KnowledgeCatalogSummary is a compact inventory of durable knowledge entities.
// It is derived from store maps and never mutates canonical state.
type KnowledgeCatalogSummary struct {
	SchemaVersion   int `json:"schema_version"`
	Sources         int `json:"sources"`
	SourceVersions  int `json:"source_versions"`
	Observations    int `json:"observations"`
	Claims          int `json:"claims"`
	EvidenceLinks   int `json:"evidence_links"`
	Artifacts       int `json:"artifacts"`
	StaleArtifacts  int `json:"stale_artifacts"`
	ClaimsWithoutEv int `json:"claims_without_evidence"`
	ContradictingEv int `json:"contradicting_evidence_links"`
	SupportingEv    int `json:"supporting_evidence_links"`
}

// SourceSummary is a compact browse row for a source.
type SourceSummary struct {
	ID         domain.SourceID `json:"id"`
	Kind       string          `json:"kind"`
	Locator    string          `json:"locator"`
	ObservedAt string          `json:"observed_at"`
	Versions   int             `json:"versions"`
}

// KnowledgeSourceFilter constrains source browse lists.
// Empty fields are ignored. Kind is an exact match; Q is a case-insensitive
// substring match against locator or kind.
type KnowledgeSourceFilter struct {
	Kind string `json:"kind,omitempty"`
	Q    string `json:"q,omitempty"`
}

// SourcePage is a paginated source browse response.
type SourcePage struct {
	SchemaVersion int             `json:"schema_version"`
	Total         int             `json:"total"`
	Limit         int             `json:"limit"`
	Offset        int             `json:"offset"`
	KindFilter    string          `json:"kind_filter,omitempty"`
	QFilter       string          `json:"q_filter,omitempty"`
	Items         []SourceSummary `json:"items"`
}

// SourceDetail reconstructs a source with versions and fragment counts.
// Snapshot bytes are intentionally omitted; content_hash/content_ref remain.
type SourceDetail struct {
	SchemaVersion int                    `json:"schema_version"`
	Source        domain.Source          `json:"source"`
	Versions      []SourceVersionSummary `json:"versions"`
}

// SourceVersionSummary is a compact version row without snapshot bytes.
type SourceVersionSummary struct {
	ID              domain.SourceVersionID `json:"id"`
	ContentHash     string                 `json:"content_hash"`
	ContentRef      string                 `json:"content_ref"`
	ExternalVersion string                 `json:"external_version,omitempty"`
	ObservedAt      string                 `json:"observed_at"`
	FragmentCount   int                    `json:"fragment_count"`
	HasSnapshot     bool                   `json:"has_snapshot"`
	MediaType       string                 `json:"media_type,omitempty"`
	ContentBytes    int                    `json:"content_bytes,omitempty"`
}

// ObservationSummary is a compact observation browse row.
type ObservationSummary struct {
	ID               domain.ObservationID    `json:"id"`
	Statement        string                  `json:"statement"`
	ExactQuote       string                  `json:"exact_quote,omitempty"`
	Provenance       string                  `json:"provenance"`
	SourceFragmentID domain.SourceFragmentID `json:"source_fragment_id,omitempty"`
	ReceiptID        domain.ReceiptID        `json:"receipt_id,omitempty"`
}

// KnowledgeObservationFilter constrains observation browse lists.
// Provenance is an exact match; Q is a case-insensitive substring on statement.
// LinkedOnly keeps observations that participate in at least one evidence link.
type KnowledgeObservationFilter struct {
	Provenance string `json:"provenance,omitempty"`
	Q          string `json:"q,omitempty"`
	LinkedOnly bool   `json:"linked_only,omitempty"`
}

// ObservationPage is a paginated observation browse response.
type ObservationPage struct {
	SchemaVersion    int                  `json:"schema_version"`
	Total            int                  `json:"total"`
	Limit            int                  `json:"limit"`
	Offset           int                  `json:"offset"`
	ProvenanceFilter string               `json:"provenance_filter,omitempty"`
	QFilter          string               `json:"q_filter,omitempty"`
	LinkedOnly       bool                 `json:"linked_only,omitempty"`
	Items            []ObservationSummary `json:"items"`
}

// ObservationDetail correlates an observation with its anchor and outbound links.
type ObservationDetail struct {
	SchemaVersion int                    `json:"schema_version"`
	Observation   domain.Observation     `json:"observation"`
	Fragment      *domain.SourceFragment `json:"source_fragment,omitempty"`
	SourceVersion *domain.SourceVersion  `json:"source_version,omitempty"`
	Source        *domain.Source         `json:"source,omitempty"`
	EvidenceLinks []domain.EvidenceLink  `json:"evidence_links"`
	LinkedClaims  []domain.ClaimID       `json:"linked_claim_ids"`
}

// ClaimSummary is a compact claim browse row with evidence tallies.
type ClaimSummary struct {
	ID              domain.ClaimID `json:"id"`
	Proposition     string         `json:"proposition"`
	Version         uint64         `json:"version"`
	QualifierCount  int            `json:"qualifier_count"`
	EvidenceCount   int            `json:"evidence_count"`
	Supports        int            `json:"supports"`
	Contradicts     int            `json:"contradicts"`
	OtherRelations  int            `json:"other_relations"`
	WithoutEvidence bool           `json:"without_evidence"`
	Provenance      string         `json:"provenance,omitempty"`
	Quorum          int            `json:"quorum,omitempty"`
}

// KnowledgeClaimFilter constrains claim browse lists.
// WithoutEvidenceOnly keeps claims with zero evidence links.
// HasContradiction keeps claims with at least one CONTRADICTS relation.
// Q is a case-insensitive substring match against the proposition.
type KnowledgeClaimFilter struct {
	WithoutEvidenceOnly bool   `json:"without_evidence,omitempty"`
	HasContradiction    bool   `json:"has_contradiction,omitempty"`
	Q                   string `json:"q,omitempty"`
}

// ClaimPage is a paginated claim browse response.
type ClaimPage struct {
	SchemaVersion        int            `json:"schema_version"`
	Total                int            `json:"total"`
	Limit                int            `json:"limit"`
	Offset               int            `json:"offset"`
	WithoutEvidenceOnly  bool           `json:"without_evidence,omitempty"`
	HasContradictionOnly bool           `json:"has_contradiction,omitempty"`
	QFilter              string         `json:"q_filter,omitempty"`
	Items                []ClaimSummary `json:"items"`
}

// ClaimDetail reconstructs a claim with evidence chain and optional citations.
type ClaimDetail struct {
	SchemaVersion   int                     `json:"schema_version"`
	Claim           domain.Claim            `json:"claim"`
	Evidence        []EvidenceLinkDetail    `json:"evidence"`
	WithoutEvidence bool                    `json:"without_evidence"`
	Provenance      string                  `json:"provenance,omitempty"`
	Quorum          int                     `json:"quorum,omitempty"`
	Canonical       *domain.CanonicalEntity `json:"canonical_entity,omitempty"`
}

// EvidenceLinkDetail expands one evidence link with the anchored observation.
type EvidenceLinkDetail struct {
	Link        domain.EvidenceLink    `json:"link"`
	Observation *domain.Observation    `json:"observation,omitempty"`
	Fragment    *domain.SourceFragment `json:"source_fragment,omitempty"`
}

// ArtifactSummary is a compact knowledge-artifact browse row.
type ArtifactSummary struct {
	ID           domain.ArtifactID `json:"id"`
	Kind         string            `json:"kind"`
	BaseCommitID domain.CommitID   `json:"base_commit_id"`
	Stale        bool              `json:"stale"`
	ContentRef   string            `json:"content_ref"`
	Dependencies int               `json:"dependency_count"`
	ContentBytes int               `json:"content_bytes"`
}

// KnowledgeArtifactFilter constrains knowledge-artifact browse lists.
// Kind is an exact match; Q is a case-insensitive substring on kind or content_ref.
type KnowledgeArtifactFilter struct {
	StaleOnly bool   `json:"stale,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Q         string `json:"q,omitempty"`
}

// ArtifactPage is a paginated artifact browse response.
type ArtifactPage struct {
	SchemaVersion int               `json:"schema_version"`
	Total         int               `json:"total"`
	Limit         int               `json:"limit"`
	Offset        int               `json:"offset"`
	StaleOnly     bool              `json:"stale_only,omitempty"`
	KindFilter    string            `json:"kind_filter,omitempty"`
	QFilter       string            `json:"q_filter,omitempty"`
	Items         []ArtifactSummary `json:"items"`
}

// ArtifactDetail is the operator-facing knowledge artifact inspector payload.
// Free-text content is presentation-redacted separately at the HTTP boundary.
type ArtifactDetail struct {
	SchemaVersion int                      `json:"schema_version"`
	Artifact      domain.KnowledgeArtifact `json:"artifact"`
	BaseCommit    *domain.Commit           `json:"base_commit,omitempty"`
}

// KnowledgeCatalog builds inventory counters over durable knowledge entities.
func (p *Projector) KnowledgeCatalog(ctx context.Context) (KnowledgeCatalogSummary, error) {
	var summary KnowledgeCatalogSummary
	err := p.Store.View(ctx, func(r port.Reader) error {
		sources, err := r.Sources()
		if err != nil {
			return err
		}
		versions, err := r.SourceVersions("")
		if err != nil {
			return err
		}
		observations, err := r.Observations()
		if err != nil {
			return err
		}
		claims, err := r.Claims()
		if err != nil {
			return err
		}
		links, err := r.EvidenceLinks()
		if err != nil {
			return err
		}
		artifacts, err := r.KnowledgeArtifacts()
		if err != nil {
			return err
		}
		linksByClaim := map[domain.ClaimID]int{}
		supports, contradicts := 0, 0
		for _, link := range links {
			linksByClaim[link.ClaimID]++
			switch link.Relation {
			case domain.EvidenceSupports:
				supports++
			case domain.EvidenceContradicts:
				contradicts++
			}
		}
		without := 0
		for _, claim := range claims {
			if linksByClaim[claim.ID] == 0 {
				without++
			}
		}
		stale := 0
		for _, artifact := range artifacts {
			if artifact.Stale {
				stale++
			}
		}
		summary = KnowledgeCatalogSummary{
			SchemaVersion:   domain.SchemaVersionV1,
			Sources:         len(sources),
			SourceVersions:  len(versions),
			Observations:    len(observations),
			Claims:          len(claims),
			EvidenceLinks:   len(links),
			Artifacts:       len(artifacts),
			StaleArtifacts:  stale,
			ClaimsWithoutEv: without,
			ContradictingEv: contradicts,
			SupportingEv:    supports,
		}
		return nil
	})
	if err != nil {
		return KnowledgeCatalogSummary{}, err
	}
	return summary, nil
}

// ListSources returns a stable, offset-limited source browse page.
func (p *Projector) ListSources(ctx context.Context, limit, offset int, filter KnowledgeSourceFilter) (SourcePage, error) {
	limit, offset, err := normalizeKnowledgePage(limit, offset)
	if err != nil {
		return SourcePage{}, err
	}
	kindFilter := strings.TrimSpace(filter.Kind)
	qFilter := strings.TrimSpace(filter.Q)
	var page SourcePage
	err = p.Store.View(ctx, func(r port.Reader) error {
		sources, err := r.Sources()
		if err != nil {
			return err
		}
		versions, err := r.SourceVersions("")
		if err != nil {
			return err
		}
		versionCount := map[domain.SourceID]int{}
		for _, version := range versions {
			versionCount[version.SourceID]++
		}
		items := make([]SourceSummary, 0, len(sources))
		for _, source := range sources {
			if kindFilter != "" && source.Kind != kindFilter {
				continue
			}
			if qFilter != "" && !knowledgeTextMatches(qFilter, source.Locator, source.Kind, string(source.ID)) {
				continue
			}
			items = append(items, SourceSummary{
				ID:         source.ID,
				Kind:       source.Kind,
				Locator:    source.Locator,
				ObservedAt: source.ObservedAt.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
				Versions:   versionCount[source.ID],
			})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		page = SourcePage{
			SchemaVersion: domain.SchemaVersionV1,
			Total:         len(items),
			Limit:         limit,
			Offset:        offset,
			KindFilter:    kindFilter,
			QFilter:       qFilter,
			Items:         slicePage(items, offset, limit),
		}
		return nil
	})
	if err != nil {
		return SourcePage{}, err
	}
	return page, nil
}

// SourceInspector loads one source and its versions without snapshot bytes.
func (p *Projector) SourceInspector(ctx context.Context, sourceID domain.SourceID) (SourceDetail, error) {
	if sourceID == "" {
		return SourceDetail{}, errors.New("source ID is required")
	}
	var detail SourceDetail
	err := p.Store.View(ctx, func(r port.Reader) error {
		source, err := r.Source(sourceID)
		if err != nil {
			return err
		}
		versions, err := r.SourceVersions(sourceID)
		if err != nil {
			return err
		}
		summaries := make([]SourceVersionSummary, 0, len(versions))
		for _, version := range versions {
			summary := SourceVersionSummary{
				ID:              version.ID,
				ContentHash:     version.ContentHash,
				ContentRef:      version.ContentRef,
				ExternalVersion: version.ExternalVersion,
				ObservedAt:      version.ObservedAt.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
			}
			if fragments, ferr := r.SourceFragments(version.ID); ferr == nil {
				summary.FragmentCount = len(fragments)
			} else if !errors.Is(ferr, port.ErrNotFound) {
				return ferr
			}
			if snapshot, serr := r.SourceSnapshot(version.ID); serr == nil {
				summary.HasSnapshot = true
				summary.MediaType = snapshot.MediaType
				summary.ContentBytes = len(snapshot.Content)
			} else if !errors.Is(serr, port.ErrNotFound) {
				return serr
			}
			summaries = append(summaries, summary)
		}
		detail = SourceDetail{
			SchemaVersion: domain.SchemaVersionV1,
			Source:        source,
			Versions:      summaries,
		}
		return nil
	})
	if err != nil {
		return SourceDetail{}, err
	}
	return detail, nil
}

// ListObservations returns a stable, offset-limited observation browse page.
func (p *Projector) ListObservations(ctx context.Context, limit, offset int, filter KnowledgeObservationFilter) (ObservationPage, error) {
	limit, offset, err := normalizeKnowledgePage(limit, offset)
	if err != nil {
		return ObservationPage{}, err
	}
	provenanceFilter := strings.TrimSpace(filter.Provenance)
	qFilter := strings.TrimSpace(filter.Q)
	var page ObservationPage
	err = p.Store.View(ctx, func(r port.Reader) error {
		observations, err := r.Observations()
		if err != nil {
			return err
		}
		linked := map[domain.ObservationID]struct{}{}
		if filter.LinkedOnly {
			links, lerr := r.EvidenceLinks()
			if lerr != nil {
				return lerr
			}
			for _, link := range links {
				linked[link.ObservationID] = struct{}{}
			}
		}
		items := make([]ObservationSummary, 0, len(observations))
		for _, observation := range observations {
			if provenanceFilter != "" && observation.Provenance != provenanceFilter {
				continue
			}
			if qFilter != "" && !knowledgeTextMatches(qFilter, observation.Statement, string(observation.ID)) {
				continue
			}
			if filter.LinkedOnly {
				if _, ok := linked[observation.ID]; !ok {
					continue
				}
			}
			items = append(items, ObservationSummary{
				ID:               observation.ID,
				Statement:        observation.Statement,
				ExactQuote:       observation.ExactQuote,
				Provenance:       observation.Provenance,
				SourceFragmentID: observation.Anchor.SourceFragmentID,
				ReceiptID:        observation.Anchor.ReceiptID,
			})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		page = ObservationPage{
			SchemaVersion:    domain.SchemaVersionV1,
			Total:            len(items),
			Limit:            limit,
			Offset:           offset,
			ProvenanceFilter: provenanceFilter,
			QFilter:          qFilter,
			LinkedOnly:       filter.LinkedOnly,
			Items:            slicePage(items, offset, limit),
		}
		return nil
	})
	if err != nil {
		return ObservationPage{}, err
	}
	return page, nil
}

// ObservationInspector correlates one observation with anchor and evidence.
func (p *Projector) ObservationInspector(ctx context.Context, observationID domain.ObservationID) (ObservationDetail, error) {
	if observationID == "" {
		return ObservationDetail{}, errors.New("observation ID is required")
	}
	var detail ObservationDetail
	err := p.Store.View(ctx, func(r port.Reader) error {
		observation, err := r.Observation(observationID)
		if err != nil {
			return err
		}
		detail = ObservationDetail{
			SchemaVersion: domain.SchemaVersionV1,
			Observation:   observation,
			EvidenceLinks: []domain.EvidenceLink{},
			LinkedClaims:  []domain.ClaimID{},
		}
		if observation.Anchor.SourceFragmentID != "" {
			fragment, ferr := r.SourceFragment(observation.Anchor.SourceFragmentID)
			if ferr != nil {
				if !errors.Is(ferr, port.ErrNotFound) {
					return ferr
				}
			} else {
				detail.Fragment = &fragment
				if version, verr := r.SourceVersion(fragment.SourceVersionID); verr == nil {
					detail.SourceVersion = &version
					if source, serr := r.Source(version.SourceID); serr == nil {
						detail.Source = &source
					} else if !errors.Is(serr, port.ErrNotFound) {
						return serr
					}
				} else if !errors.Is(verr, port.ErrNotFound) {
					return verr
				}
			}
		}
		links, err := r.EvidenceLinks()
		if err != nil {
			return err
		}
		seenClaims := map[domain.ClaimID]struct{}{}
		for _, link := range links {
			if link.ObservationID != observationID {
				continue
			}
			detail.EvidenceLinks = append(detail.EvidenceLinks, link)
			if _, ok := seenClaims[link.ClaimID]; !ok {
				seenClaims[link.ClaimID] = struct{}{}
				detail.LinkedClaims = append(detail.LinkedClaims, link.ClaimID)
			}
		}
		sort.Slice(detail.LinkedClaims, func(i, j int) bool {
			return detail.LinkedClaims[i] < detail.LinkedClaims[j]
		})
		return nil
	})
	if err != nil {
		return ObservationDetail{}, err
	}
	return detail, nil
}

// ListClaims returns a stable, offset-limited claim browse page with evidence tallies.
func (p *Projector) ListClaims(ctx context.Context, limit, offset int, filter KnowledgeClaimFilter) (ClaimPage, error) {
	limit, offset, err := normalizeKnowledgePage(limit, offset)
	if err != nil {
		return ClaimPage{}, err
	}
	qFilter := strings.TrimSpace(filter.Q)
	var page ClaimPage
	err = p.Store.View(ctx, func(r port.Reader) error {
		claims, err := r.Claims()
		if err != nil {
			return err
		}
		links, err := r.EvidenceLinks()
		if err != nil {
			return err
		}
		type tally struct {
			total, supports, contradicts, other int
		}
		byClaim := map[domain.ClaimID]*tally{}
		for _, link := range links {
			t := byClaim[link.ClaimID]
			if t == nil {
				t = &tally{}
				byClaim[link.ClaimID] = t
			}
			t.total++
			switch link.Relation {
			case domain.EvidenceSupports:
				t.supports++
			case domain.EvidenceContradicts:
				t.contradicts++
			default:
				t.other++
			}
		}
		items := make([]ClaimSummary, 0, len(claims))
		for _, claim := range claims {
			t := byClaim[claim.ID]
			if t == nil {
				t = &tally{}
			}
			without := t.total == 0
			if filter.WithoutEvidenceOnly && !without {
				continue
			}
			if filter.HasContradiction && t.contradicts == 0 {
				continue
			}
			if qFilter != "" && !knowledgeTextMatches(qFilter, claim.Proposition, string(claim.ID)) {
				continue
			}
			items = append(items, ClaimSummary{
				ID:              claim.ID,
				Proposition:     claim.Proposition,
				Version:         claim.Version,
				QualifierCount:  len(claim.Qualifiers),
				EvidenceCount:   t.total,
				Supports:        t.supports,
				Contradicts:     t.contradicts,
				OtherRelations:  t.other,
				WithoutEvidence: without,
			})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		page = ClaimPage{
			SchemaVersion:        domain.SchemaVersionV1,
			Total:                len(items),
			Limit:                limit,
			Offset:               offset,
			WithoutEvidenceOnly:  filter.WithoutEvidenceOnly,
			HasContradictionOnly: filter.HasContradiction,
			QFilter:              qFilter,
			Items:                slicePage(items, offset, limit),
		}
		return nil
	})
	if err != nil {
		return ClaimPage{}, err
	}
	return page, nil
}

// ClaimInspector expands one claim with evidence, observations and optional anchors.
func (p *Projector) ClaimInspector(ctx context.Context, claimID domain.ClaimID) (ClaimDetail, error) {
	if claimID == "" {
		return ClaimDetail{}, errors.New("claim ID is required")
	}
	var detail ClaimDetail
	err := p.Store.View(ctx, func(r port.Reader) error {
		claim, err := r.Claim(claimID)
		if err != nil {
			return err
		}
		links, err := r.EvidenceLinksForClaim(claimID)
		if err != nil {
			return err
		}
		evidence := make([]EvidenceLinkDetail, 0, len(links))
		for _, link := range links {
			item := EvidenceLinkDetail{Link: link}
			if observation, oerr := r.Observation(link.ObservationID); oerr == nil {
				item.Observation = &observation
				if observation.Anchor.SourceFragmentID != "" {
					if fragment, ferr := r.SourceFragment(observation.Anchor.SourceFragmentID); ferr == nil {
						item.Fragment = &fragment
					} else if !errors.Is(ferr, port.ErrNotFound) {
						return ferr
					}
				}
			} else if !errors.Is(oerr, port.ErrNotFound) {
				return oerr
			}
			evidence = append(evidence, item)
		}
		detail = ClaimDetail{
			SchemaVersion:   domain.SchemaVersionV1,
			Claim:           claim,
			Evidence:        evidence,
			WithoutEvidence: len(evidence) == 0,
		}
		if canonical, cerr := r.CanonicalEntity("claim", string(claimID)); cerr == nil {
			detail.Canonical = &canonical
		} else if !errors.Is(cerr, port.ErrNotFound) {
			return cerr
		}
		return nil
	})
	if err != nil {
		return ClaimDetail{}, err
	}
	return detail, nil
}

// ListArtifacts returns a stable, offset-limited knowledge-artifact browse page.
func (p *Projector) ListArtifacts(ctx context.Context, limit, offset int, filter KnowledgeArtifactFilter) (ArtifactPage, error) {
	limit, offset, err := normalizeKnowledgePage(limit, offset)
	if err != nil {
		return ArtifactPage{}, err
	}
	kindFilter := strings.TrimSpace(filter.Kind)
	qFilter := strings.TrimSpace(filter.Q)
	var page ArtifactPage
	err = p.Store.View(ctx, func(r port.Reader) error {
		artifacts, err := r.KnowledgeArtifacts()
		if err != nil {
			return err
		}
		items := make([]ArtifactSummary, 0, len(artifacts))
		for _, artifact := range artifacts {
			if filter.StaleOnly && !artifact.Stale {
				continue
			}
			if kindFilter != "" && artifact.Kind != kindFilter {
				continue
			}
			if qFilter != "" && !knowledgeTextMatches(qFilter, artifact.Kind, artifact.ContentRef, string(artifact.ID)) {
				continue
			}
			items = append(items, ArtifactSummary{
				ID:           artifact.ID,
				Kind:         artifact.Kind,
				BaseCommitID: artifact.BaseCommitID,
				Stale:        artifact.Stale,
				ContentRef:   artifact.ContentRef,
				Dependencies: len(artifact.Dependencies),
				ContentBytes: len(artifact.Content),
			})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		page = ArtifactPage{
			SchemaVersion: domain.SchemaVersionV1,
			Total:         len(items),
			Limit:         limit,
			Offset:        offset,
			StaleOnly:     filter.StaleOnly,
			KindFilter:    kindFilter,
			QFilter:       qFilter,
			Items:         slicePage(items, offset, limit),
		}
		return nil
	})
	if err != nil {
		return ArtifactPage{}, err
	}
	return page, nil
}

// ArtifactInspector loads one knowledge artifact and optional base commit.
func (p *Projector) ArtifactInspector(ctx context.Context, artifactID domain.ArtifactID) (ArtifactDetail, error) {
	if artifactID == "" {
		return ArtifactDetail{}, errors.New("artifact ID is required")
	}
	var detail ArtifactDetail
	err := p.Store.View(ctx, func(r port.Reader) error {
		artifact, err := r.KnowledgeArtifact(artifactID)
		if err != nil {
			return err
		}
		detail = ArtifactDetail{
			SchemaVersion: domain.SchemaVersionV1,
			Artifact:      artifact,
		}
		if artifact.BaseCommitID != "" && artifact.BaseCommitID != domain.GenesisCommitID {
			if commit, cerr := r.Commit(artifact.BaseCommitID); cerr == nil {
				detail.BaseCommit = &commit
			} else if !errors.Is(cerr, port.ErrNotFound) {
				return cerr
			}
		}
		return nil
	})
	if err != nil {
		return ArtifactDetail{}, err
	}
	return detail, nil
}

// knowledgeTextMatches reports whether needle appears in any haystack field
// using case-insensitive substring comparison.
func knowledgeTextMatches(needle string, fields ...string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return true
	}
	n := strings.ToLower(needle)
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), n) {
			return true
		}
	}
	return false
}
func normalizeKnowledgePage(limit, offset int) (int, int, error) {
	if limit <= 0 {
		limit = DefaultKnowledgeListLimit
	}
	if limit > MaxKnowledgeListLimit {
		return 0, 0, fmt.Errorf("limit must be between 1 and %d", MaxKnowledgeListLimit)
	}
	if offset < 0 {
		return 0, 0, errors.New("offset must be non-negative")
	}
	return limit, offset, nil
}

func slicePage[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return []T{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	out := make([]T, end-offset)
	copy(out, items[offset:end])
	return out
}

// truncateKnowledgeText is used by presentation redaction for free-text fields.
func truncateKnowledgeText(text string, maxBytes int) (string, int) {
	bounded, removed := BoundUTF8(text, maxBytes)
	return bounded, removed
}

// knowledgeTextMax is the presentation bound for observation statements/quotes
// and artifact content shown through inspect endpoints.
const knowledgeTextMax = 8 * 1024

// RedactObservationDetail sanitizes free-text fields for operator export.
func RedactObservationDetail(detail ObservationDetail) (ObservationDetail, RedactionReport) {
	out := detail
	report := RedactionReport{}
	if out.Observation.Statement != "" {
		text, n := RedactSensitiveText(out.Observation.Statement)
		if n > 0 {
			report.Applied = true
			report.SecretMatches += n
			report.Notes = append(report.Notes, "secret-shaped substrings replaced in observation statement")
		}
		bounded, removed := truncateKnowledgeText(text, knowledgeTextMax)
		if removed > 0 {
			report.Applied = true
			report.TruncatedBytes += removed
			report.Notes = append(report.Notes, "observation statement truncated for presentation")
		}
		out.Observation.Statement = bounded
	}
	if out.Observation.ExactQuote != "" {
		text, n := RedactSensitiveText(out.Observation.ExactQuote)
		if n > 0 {
			report.Applied = true
			report.SecretMatches += n
			report.Notes = append(report.Notes, "secret-shaped substrings replaced in exact quote")
		}
		bounded, removed := truncateKnowledgeText(text, knowledgeTextMax)
		if removed > 0 {
			report.Applied = true
			report.TruncatedBytes += removed
			report.Notes = append(report.Notes, "exact quote truncated for presentation")
		}
		out.Observation.ExactQuote = bounded
	}
	return out, report
}

// RedactClaimDetail sanitizes nested observation free-text on evidence rows.
func RedactClaimDetail(detail ClaimDetail) (ClaimDetail, RedactionReport) {
	out := detail
	report := RedactionReport{}
	if out.Claim.Proposition != "" {
		text, n := RedactSensitiveText(out.Claim.Proposition)
		if n > 0 {
			report.Applied = true
			report.SecretMatches += n
			report.Notes = append(report.Notes, "secret-shaped substrings replaced in claim proposition")
		}
		bounded, removed := truncateKnowledgeText(text, knowledgeTextMax)
		if removed > 0 {
			report.Applied = true
			report.TruncatedBytes += removed
			report.Notes = append(report.Notes, "claim proposition truncated for presentation")
		}
		out.Claim.Proposition = bounded
	}
	if len(out.Evidence) == 0 {
		return out, report
	}
	redacted := make([]EvidenceLinkDetail, 0, len(out.Evidence))
	for _, item := range out.Evidence {
		if item.Observation != nil {
			obs := *item.Observation
			if obs.Statement != "" {
				text, n := RedactSensitiveText(obs.Statement)
				if n > 0 {
					report.Applied = true
					report.SecretMatches += n
					if !containsString(report.Notes, "secret-shaped substrings replaced in observation statement") {
						report.Notes = append(report.Notes, "secret-shaped substrings replaced in observation statement")
					}
				}
				bounded, removed := truncateKnowledgeText(text, knowledgeTextMax)
				if removed > 0 {
					report.Applied = true
					report.TruncatedBytes += removed
					if !containsString(report.Notes, "observation statement truncated for presentation") {
						report.Notes = append(report.Notes, "observation statement truncated for presentation")
					}
				}
				obs.Statement = bounded
			}
			if obs.ExactQuote != "" {
				text, n := RedactSensitiveText(obs.ExactQuote)
				if n > 0 {
					report.Applied = true
					report.SecretMatches += n
					if !containsString(report.Notes, "secret-shaped substrings replaced in exact quote") {
						report.Notes = append(report.Notes, "secret-shaped substrings replaced in exact quote")
					}
				}
				bounded, removed := truncateKnowledgeText(text, knowledgeTextMax)
				if removed > 0 {
					report.Applied = true
					report.TruncatedBytes += removed
					if !containsString(report.Notes, "exact quote truncated for presentation") {
						report.Notes = append(report.Notes, "exact quote truncated for presentation")
					}
				}
				obs.ExactQuote = bounded
			}
			item.Observation = &obs
		}
		if item.Link.Rationale != "" {
			text, n := RedactSensitiveText(item.Link.Rationale)
			if n > 0 {
				report.Applied = true
				report.SecretMatches += n
				if !containsString(report.Notes, "secret-shaped substrings replaced in evidence rationale") {
					report.Notes = append(report.Notes, "secret-shaped substrings replaced in evidence rationale")
				}
			}
			item.Link.Rationale = text
		}
		redacted = append(redacted, item)
	}
	out.Evidence = redacted
	return out, report
}

// RedactArtifactDetail sanitizes artifact content for operator export.
func RedactArtifactDetail(detail ArtifactDetail) (ArtifactDetail, RedactionReport) {
	out := detail
	report := RedactionReport{}
	if out.Artifact.Content == "" {
		return out, report
	}
	text, n := RedactSensitiveText(out.Artifact.Content)
	if n > 0 {
		report.Applied = true
		report.SecretMatches += n
		report.Notes = append(report.Notes, "secret-shaped substrings replaced in artifact content")
	}
	// Artifact views can be longer; reuse raw model bound.
	bounded, removed := BoundUTF8(text, DefaultMaxRawContentBytes)
	if removed > 0 {
		report.Applied = true
		report.TruncatedBytes += removed
		report.Notes = append(report.Notes, "artifact content truncated for presentation")
	}
	out.Artifact.Content = bounded
	return out, report
}

// ObservationDetailResponse is the operator-facing observation inspector payload.
type ObservationDetailResponse struct {
	ObservationDetail
	Redaction RedactionReport `json:"redaction"`
}

// ClaimDetailResponse is the operator-facing claim inspector payload.
type ClaimDetailResponse struct {
	ClaimDetail
	Redaction RedactionReport `json:"redaction"`
}

// ArtifactDetailResponse is the operator-facing artifact inspector payload.
type ArtifactDetailResponse struct {
	ArtifactDetail
	Redaction RedactionReport `json:"redaction"`
}
