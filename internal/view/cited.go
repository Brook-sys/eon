// Package view materializes deterministic, regenerable views over canonical
// knowledge records. Views contain citations but never replace their inputs.
package view

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
)

type Generator struct {
	Store port.Store
	IDs   runtimesource.IDGenerator
}

func (g Generator) Generate(ctx context.Context, claimID domain.ClaimID, base domain.CommitID) (domain.KnowledgeArtifact, error) {
	if g.Store == nil || g.IDs == nil {
		return domain.KnowledgeArtifact{}, errors.New("cited view generator requires store and ID generator")
	}
	id, err := g.IDs.NewID("artifact")
	if err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("generate artifact ID: %w", err)
	}
	var artifact domain.KnowledgeArtifact
	err = g.Store.Update(ctx, func(tx port.Transaction) error {
		var err error
		artifact, err = render(tx, domain.ArtifactID(id), claimID, base)
		if err != nil {
			return err
		}
		return tx.AppendKnowledgeArtifact(artifact)
	})
	if err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("generate cited view: %w", err)
	}
	return artifact, nil
}

type EvidencePatch struct {
	ObservationID domain.ObservationID
	Relation      domain.EvidenceRelation
	Rationale     string
}

type Patcher struct {
	Store port.Store
	IDs   runtimesource.IDGenerator
}

// Apply atomically appends an evidence delta, marks the previous materialized
// view stale, and persists a fresh cited successor.
func (p Patcher) Apply(ctx context.Context, priorID domain.ArtifactID, patch EvidencePatch, base domain.CommitID) (domain.KnowledgeArtifact, error) {
	if p.Store == nil || p.IDs == nil {
		return domain.KnowledgeArtifact{}, errors.New("cited view patcher requires store and ID generator")
	}
	linkID, err := p.IDs.NewID("evidence_link")
	if err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("generate evidence link ID: %w", err)
	}
	artifactID, err := p.IDs.NewID("artifact")
	if err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("generate artifact ID: %w", err)
	}
	var successor domain.KnowledgeArtifact
	err = p.Store.Update(ctx, func(tx port.Transaction) error {
		prior, err := tx.KnowledgeArtifact(priorID)
		if err != nil {
			return err
		}
		if prior.Stale {
			return fmt.Errorf("%w: cannot patch stale knowledge artifact", port.ErrConflict)
		}
		claimID, err := claimDependency(prior.Dependencies)
		if err != nil {
			return err
		}
		link := domain.EvidenceLink{SchemaVersion: domain.SchemaVersionV1, ID: domain.EvidenceLinkID(linkID), ObservationID: patch.ObservationID, ClaimID: claimID, Relation: patch.Relation, Rationale: patch.Rationale}
		if err := tx.AppendEvidenceLinks(claimID, []domain.EvidenceLink{link}); err != nil {
			return err
		}
		successor, err = render(tx, domain.ArtifactID(artifactID), claimID, base)
		if err != nil {
			return err
		}
		prior.Stale = true
		if err := tx.SaveKnowledgeArtifact(prior); err != nil {
			return err
		}
		return tx.AppendKnowledgeArtifact(successor)
	})
	if err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("apply cited view patch: %w", err)
	}
	return successor, nil
}

func render(r port.Reader, artifactID domain.ArtifactID, claimID domain.ClaimID, base domain.CommitID) (domain.KnowledgeArtifact, error) {
	if base == "" {
		return domain.KnowledgeArtifact{}, errors.New("cited view requires base commit")
	}
	if base != domain.GenesisCommitID {
		if _, err := r.Commit(base); err != nil {
			return domain.KnowledgeArtifact{}, err
		}
	}
	claim, err := r.Claim(claimID)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	links, err := r.EvidenceLinksForClaim(claimID)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	if len(links) == 0 {
		return domain.KnowledgeArtifact{}, errors.New("cited view requires at least one evidence link")
	}
	sort.Slice(links, func(i, j int) bool { return links[i].ID < links[j].ID })
	dependencies := []string{fmt.Sprintf("claim:%s@%d", claim.ID, claim.Version)}
	var b strings.Builder
	fmt.Fprintf(&b, "# Claim\n\n%s\n\n## Evidence\n", claim.Proposition)
	for _, link := range links {
		observation, err := r.Observation(link.ObservationID)
		if err != nil {
			return domain.KnowledgeArtifact{}, err
		}
		if observation.Anchor.SourceFragmentID == "" {
			return domain.KnowledgeArtifact{}, errors.New("first cited view supports only source-fragment observations")
		}
		fragment, err := r.SourceFragment(observation.Anchor.SourceFragmentID)
		if err != nil {
			return domain.KnowledgeArtifact{}, err
		}
		version, err := r.SourceVersion(fragment.SourceVersionID)
		if err != nil {
			return domain.KnowledgeArtifact{}, err
		}
		source, err := r.Source(version.SourceID)
		if err != nil {
			return domain.KnowledgeArtifact{}, err
		}
		fmt.Fprintf(&b, "\n- **%s** — %s — “%s” ([%s, %s])", link.Relation, observation.Statement, observation.ExactQuote, source.Locator, fragment.Location)
		if link.Rationale != "" {
			fmt.Fprintf(&b, " — %s", link.Rationale)
		}
		dependencies = append(dependencies, "evidence_link:"+string(link.ID), "observation:"+string(observation.ID), "source_fragment:"+string(fragment.ID), "source_version:"+string(version.ID))
	}
	content := b.String() + "\n"
	hash := sha256.Sum256([]byte(content))
	return domain.KnowledgeArtifact{SchemaVersion: domain.SchemaVersionV1, ID: artifactID, Kind: "cited_claim_view", BaseCommitID: base, Dependencies: dependencies, ContentRef: "sha256:" + hex.EncodeToString(hash[:]), Content: content}, nil
}

func claimDependency(dependencies []string) (domain.ClaimID, error) {
	for _, dependency := range dependencies {
		if strings.HasPrefix(dependency, "claim:") {
			value := strings.TrimPrefix(dependency, "claim:")
			id, _, ok := strings.Cut(value, "@")
			if ok && id != "" {
				return domain.ClaimID(id), nil
			}
		}
	}
	return "", errors.New("knowledge artifact has no claim dependency")
}
