// Package segment creates deterministic, lossless source fragments.
package segment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
)

const DefaultMaxFragmentBytes = 4096

type Result struct {
	Fragments []domain.SourceFragment
	Event     domain.Event
}

type TextSegmenter struct {
	Store    port.Store
	Clock    runtimesource.Clock
	IDs      runtimesource.IDGenerator
	MaxBytes int
}

func (s TextSegmenter) Segment(ctx context.Context, missionRevision domain.MissionRevisionID, versionID domain.SourceVersionID) (Result, error) {
	if s.Store == nil || s.Clock == nil || s.IDs == nil {
		return Result{}, errors.New("text segmenter requires store, clock and ID generator")
	}
	limit := s.MaxBytes
	if limit == 0 {
		limit = DefaultMaxFragmentBytes
	}
	if limit < utf8.UTFMax {
		return Result{}, fmt.Errorf("fragment byte limit must be at least %d", utf8.UTFMax)
	}

	var snapshot domain.SourceSnapshot
	if err := s.Store.View(ctx, func(r port.Reader) error {
		var err error
		snapshot, err = r.SourceSnapshot(versionID)
		return err
	}); err != nil {
		return Result{}, fmt.Errorf("read source snapshot: %w", err)
	}
	if !strings.HasPrefix(strings.ToLower(snapshot.MediaType), "text/") || !utf8.Valid(snapshot.Content) {
		return Result{}, errors.New("source snapshot must be valid UTF-8 text")
	}

	result := Result{}
	for start := 0; start < len(snapshot.Content); {
		end := min(start+limit, len(snapshot.Content))
		for end < len(snapshot.Content) && !utf8.RuneStart(snapshot.Content[end]) {
			end--
		}
		if end == start {
			return Result{}, errors.New("fragment limit cannot contain next UTF-8 rune")
		}
		id, err := s.IDs.NewID("source_fragment")
		if err != nil {
			return Result{}, fmt.Errorf("generate source fragment ID: %w", err)
		}
		digest := sha256.Sum256(snapshot.Content[start:end])
		hash := "sha256:" + hex.EncodeToString(digest[:])
		result.Fragments = append(result.Fragments, domain.SourceFragment{
			SchemaVersion: 1, ID: domain.SourceFragmentID(id), SourceVersionID: versionID,
			Location: fmt.Sprintf("bytes:%d-%d", start, end), StartOffset: uint64(start), EndOffset: uint64(end),
			ContentHash: hash, ContentRef: hash,
		})
		start = end
	}
	eventID, err := s.IDs.NewID("event")
	if err != nil {
		return Result{}, fmt.Errorf("generate event ID: %w", err)
	}
	result.Event = domain.Event{SchemaVersion: 1, ID: domain.EventID(eventID), Kind: "source.segmented", OccurredAt: s.Clock.Now(), MissionRevision: missionRevision, PayloadRef: string(versionID)}
	err = s.Store.Update(ctx, func(tx port.Transaction) error {
		if missionRevision != "" {
			if _, err := tx.MissionRevision(missionRevision); err != nil {
				return err
			}
		}
		if err := tx.AppendSourceFragments(versionID, result.Fragments); err != nil {
			return err
		}
		persisted, err := tx.AppendEvent(result.Event)
		if err != nil {
			return err
		}
		result.Event = persisted
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("persist source segmentation: %w", err)
	}
	return result, nil
}
