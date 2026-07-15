// Package ingest turns bounded external bytes into immutable source records.
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
)

const DefaultMaxFixtureBytes = 1 << 20

type Fixture struct {
	Kind            string
	Locator         string
	ExternalVersion string
	MediaType       string
	Content         []byte
}

type Result struct {
	Source   domain.Source
	Version  domain.SourceVersion
	Snapshot domain.SourceSnapshot
	Event    domain.Event
}

type Ingester struct {
	Store    port.Store
	Clock    runtimesource.Clock
	IDs      runtimesource.IDGenerator
	MaxBytes int
}

func (i Ingester) IngestFixture(ctx context.Context, missionRevision domain.MissionRevisionID, fixture Fixture) (Result, error) {
	if i.Store == nil || i.Clock == nil || i.IDs == nil {
		return Result{}, errors.New("fixture ingester requires store, clock and ID generator")
	}
	limit := i.MaxBytes
	if limit == 0 {
		limit = DefaultMaxFixtureBytes
	}
	if limit < 1 || len(fixture.Content) == 0 || len(fixture.Content) > limit {
		return Result{}, fmt.Errorf("fixture content size %d is outside limit 1..%d", len(fixture.Content), limit)
	}
	if strings.TrimSpace(fixture.Kind) == "" || strings.TrimSpace(fixture.Locator) == "" || strings.TrimSpace(fixture.MediaType) == "" {
		return Result{}, errors.New("fixture kind, locator and media type are required")
	}

	sourceID, err := i.IDs.NewID("source")
	if err != nil {
		return Result{}, fmt.Errorf("generate source ID: %w", err)
	}
	versionID, err := i.IDs.NewID("source_version")
	if err != nil {
		return Result{}, fmt.Errorf("generate source version ID: %w", err)
	}
	eventID, err := i.IDs.NewID("event")
	if err != nil {
		return Result{}, fmt.Errorf("generate event ID: %w", err)
	}
	now := i.Clock.Now()
	hashBytes := sha256.Sum256(fixture.Content)
	hash := "sha256:" + hex.EncodeToString(hashBytes[:])
	content := append([]byte(nil), fixture.Content...)
	result := Result{
		Source:   domain.Source{SchemaVersion: 1, ID: domain.SourceID(sourceID), Kind: fixture.Kind, Locator: fixture.Locator, ObservedAt: now},
		Version:  domain.SourceVersion{SchemaVersion: 1, ID: domain.SourceVersionID(versionID), SourceID: domain.SourceID(sourceID), ContentHash: hash, ContentRef: hash, ExternalVersion: fixture.ExternalVersion, ObservedAt: now},
		Snapshot: domain.SourceSnapshot{SchemaVersion: 1, SourceVersionID: domain.SourceVersionID(versionID), MediaType: fixture.MediaType, Content: content},
		Event:    domain.Event{SchemaVersion: 1, ID: domain.EventID(eventID), Kind: "source.ingested", OccurredAt: now, MissionRevision: missionRevision, PayloadRef: string(versionID)},
	}
	err = i.Store.Update(ctx, func(tx port.Transaction) error {
		if missionRevision != "" {
			if _, err := tx.MissionRevision(missionRevision); err != nil {
				return err
			}
		}
		if err := tx.AppendSource(result.Source, result.Version, result.Snapshot); err != nil {
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
		return Result{}, fmt.Errorf("persist fixture ingestion: %w", err)
	}
	return result, nil
}
