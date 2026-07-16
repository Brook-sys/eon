package inspect

import (
	"context"
	"errors"
	"sort"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// CommitSummary is a compact row for commit browse lists.
type CommitSummary struct {
	ID                  domain.CommitID          `json:"id"`
	Version             uint64                   `json:"version"`
	MissionRevision     domain.MissionRevisionID `json:"mission_revision_id"`
	AcceptedChangeSetID domain.ChangeSetID       `json:"accepted_change_set_id"`
	BaseCommitID        domain.CommitID          `json:"base_commit_id"`
	CommittedAt         string                   `json:"committed_at"`
	ReceiptID           domain.ReceiptID         `json:"receipt_id"`
	IdempotencyKey      domain.IdempotencyKey    `json:"idempotency_key"`
	IsHead              bool                     `json:"is_head,omitempty"`
}

// CommitFilter constrains commit browse lists.
type CommitFilter struct {
	MissionRevision domain.MissionRevisionID `json:"mission_revision_id,omitempty"`
	// HeadOnly returns only current head commits per mission revision.
	HeadOnly bool `json:"head_only,omitempty"`
}

// CommitPage is a paginated commit browse response.
type CommitPage struct {
	SchemaVersion   int                      `json:"schema_version"`
	Total           int                      `json:"total"`
	Limit           int                      `json:"limit"`
	Offset          int                      `json:"offset"`
	MissionRevision domain.MissionRevisionID `json:"mission_revision_id,omitempty"`
	HeadOnly        bool                     `json:"head_only,omitempty"`
	Items           []CommitSummary          `json:"items"`
}

// ListCommits returns a stable, offset-limited commit browse page.
// Order is Version ascending then ID (same as KnowledgeReader.Commits).
func (p *Projector) ListCommits(ctx context.Context, limit, offset int, filter CommitFilter) (CommitPage, error) {
	limit, offset, err := normalizeKnowledgePage(limit, offset)
	if err != nil {
		return CommitPage{}, err
	}
	revisionFilter := domain.MissionRevisionID(strings.TrimSpace(string(filter.MissionRevision)))
	var page CommitPage
	err = p.Store.View(ctx, func(r port.Reader) error {
		commits, err := r.Commits()
		if err != nil {
			return err
		}
		// Build head set for optional flagging / HeadOnly filter.
		headIDs := map[domain.CommitID]struct{}{}
		// When filtering by revision, resolve that head; otherwise scan all
		// commits' revisions (bounded by commit set size).
		seenRevisions := map[domain.MissionRevisionID]struct{}{}
		for _, c := range commits {
			if _, ok := seenRevisions[c.MissionRevision]; ok {
				continue
			}
			seenRevisions[c.MissionRevision] = struct{}{}
			if head, herr := r.HeadCommit(c.MissionRevision); herr == nil {
				headIDs[head.ID] = struct{}{}
			} else if !isNotFound(herr) {
				return herr
			}
		}
		items := make([]CommitSummary, 0, len(commits))
		for _, commit := range commits {
			if revisionFilter != "" && commit.MissionRevision != revisionFilter {
				continue
			}
			_, isHead := headIDs[commit.ID]
			if filter.HeadOnly && !isHead {
				continue
			}
			items = append(items, CommitSummary{
				ID:                  commit.ID,
				Version:             commit.Version,
				MissionRevision:     commit.MissionRevision,
				AcceptedChangeSetID: commit.AcceptedChangeSetID,
				BaseCommitID:        commit.BaseCommitID,
				CommittedAt:         commit.CommittedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
				ReceiptID:           commit.ReceiptID,
				IdempotencyKey:      commit.IdempotencyKey,
				IsHead:              isHead,
			})
		}
		// Keep Version then ID order from store; re-sort defensively.
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Version != items[j].Version {
				return items[i].Version < items[j].Version
			}
			return items[i].ID < items[j].ID
		})
		page = CommitPage{
			SchemaVersion:   domain.SchemaVersionV1,
			Total:           len(items),
			Limit:           limit,
			Offset:          offset,
			MissionRevision: revisionFilter,
			HeadOnly:        filter.HeadOnly,
			Items:           slicePage(items, offset, limit),
		}
		return nil
	})
	if err != nil {
		return CommitPage{}, err
	}
	return page, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, port.ErrNotFound)
}
