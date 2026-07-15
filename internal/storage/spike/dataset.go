// Package spike implements the backend-neutral storage comparison workload.
package spike

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
)

const DatasetSchemaVersion = 1

type DatasetConfig struct {
	Seed          int64 `json:"seed"`
	Sources       int   `json:"sources"`
	Claims        int   `json:"claims"`
	EvidenceLinks int   `json:"evidence_links"`
	SnapshotMin   int   `json:"snapshot_min_bytes"`
	SnapshotMax   int   `json:"snapshot_max_bytes"`
}

func ReducedConfig() DatasetConfig {
	return DatasetConfig{Seed: 20260715, Sources: 10, Claims: 20, EvidenceLinks: 60, SnapshotMin: 128, SnapshotMax: 512}
}

func FullConfig() DatasetConfig {
	return DatasetConfig{Seed: 20260715, Sources: 1000, Claims: 10000, EvidenceLinks: 30000, SnapshotMin: 1024, SnapshotMax: 8192}
}

type SourceFixture struct {
	Source      domain.Source         `json:"source"`
	Version     domain.SourceVersion  `json:"version"`
	Snapshot    domain.SourceSnapshot `json:"snapshot"`
	Fragment    domain.SourceFragment `json:"fragment"`
	Observation domain.Observation    `json:"observation"`
}

type ClaimFixture struct {
	Claim domain.Claim          `json:"claim"`
	Links []domain.EvidenceLink `json:"links"`
}

type Dataset struct {
	SchemaVersion int             `json:"schema_version"`
	Config        DatasetConfig   `json:"config"`
	Sources       []SourceFixture `json:"sources"`
	Claims        []ClaimFixture  `json:"claims"`
}

type Manifest struct {
	SchemaVersion int           `json:"schema_version"`
	Config        DatasetConfig `json:"config"`
	Fragments     int           `json:"fragments"`
	Observations  int           `json:"observations"`
	SHA256        string        `json:"sha256"`
}

func Generate(config DatasetConfig) (Dataset, Manifest, error) {
	if config.Sources <= 0 || config.Claims <= 0 || config.EvidenceLinks < config.Claims || config.SnapshotMin <= 0 || config.SnapshotMax < config.SnapshotMin {
		return Dataset{}, Manifest{}, fmt.Errorf("invalid dataset config: %+v", config)
	}
	rng := rand.New(rand.NewSource(config.Seed))
	at := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	dataset := Dataset{SchemaVersion: DatasetSchemaVersion, Config: config}
	for i := 0; i < config.Sources; i++ {
		size := config.SnapshotMin
		if span := config.SnapshotMax - config.SnapshotMin + 1; span > 1 {
			size += rng.Intn(span)
		}
		prefix := fmt.Sprintf("source-%06d seed-%d ", i, config.Seed)
		content := []byte(strings.Repeat(prefix, (size/len(prefix))+1)[:size])
		hash := contentHash(content)
		sourceID := domain.SourceID(fmt.Sprintf("source_%06d", i))
		versionID := domain.SourceVersionID(fmt.Sprintf("source_version_%06d", i))
		fragmentID := domain.SourceFragmentID(fmt.Sprintf("fragment_%06d", i))
		dataset.Sources = append(dataset.Sources, SourceFixture{
			Source:      domain.Source{SchemaVersion: 1, ID: sourceID, Kind: "synthetic", Locator: fmt.Sprintf("fixture://source/%06d", i), ObservedAt: at.Add(time.Duration(i) * time.Second)},
			Version:     domain.SourceVersion{SchemaVersion: 1, ID: versionID, SourceID: sourceID, ContentHash: hash, ContentRef: hash, ExternalVersion: "v1", ObservedAt: at.Add(time.Duration(i) * time.Second)},
			Snapshot:    domain.SourceSnapshot{SchemaVersion: 1, SourceVersionID: versionID, MediaType: "text/plain; charset=utf-8", Content: content},
			Fragment:    domain.SourceFragment{SchemaVersion: 1, ID: fragmentID, SourceVersionID: versionID, Location: fmt.Sprintf("bytes:0-%d", len(content)), StartOffset: 0, EndOffset: uint64(len(content)), ContentHash: hash, ContentRef: hash},
			Observation: domain.Observation{SchemaVersion: 1, ID: domain.ObservationID(fmt.Sprintf("observation_%06d", i)), Statement: fmt.Sprintf("Synthetic observation %d", i), ExactQuote: string(content), Anchor: domain.ObservationAnchor{SourceFragmentID: fragmentID}, Provenance: "storage-spike-generator/v1"},
		})
	}
	base := config.EvidenceLinks / config.Claims
	extra := config.EvidenceLinks % config.Claims
	linkIndex := 0
	for i := 0; i < config.Claims; i++ {
		count := base
		if i < extra {
			count++
		}
		claimID := domain.ClaimID(fmt.Sprintf("claim_%06d", i))
		fixture := ClaimFixture{Claim: domain.Claim{SchemaVersion: 1, ID: claimID, Proposition: fmt.Sprintf("Synthetic proposition %d", i), Qualifiers: map[string]string{"scope": "synthetic", "seed": fmt.Sprint(config.Seed)}, Version: 1}}
		for j := 0; j < count; j++ {
			observation := dataset.Sources[(i+j)%len(dataset.Sources)].Observation.ID
			fixture.Links = append(fixture.Links, domain.EvidenceLink{SchemaVersion: 1, ID: domain.EvidenceLinkID(fmt.Sprintf("evidence_%08d", linkIndex)), ObservationID: observation, ClaimID: claimID, Relation: domain.EvidenceSupports, Rationale: "deterministic synthetic workload"})
			linkIndex++
		}
		dataset.Claims = append(dataset.Claims, fixture)
	}
	encoded, err := json.Marshal(dataset)
	if err != nil {
		return Dataset{}, Manifest{}, fmt.Errorf("encode dataset: %w", err)
	}
	digest := sha256.Sum256(encoded)
	manifest := Manifest{SchemaVersion: DatasetSchemaVersion, Config: config, Fragments: len(dataset.Sources), Observations: len(dataset.Sources), SHA256: hex.EncodeToString(digest[:])}
	return dataset, manifest, nil
}

func contentHash(content []byte) string {
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}
