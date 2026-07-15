package spike

import (
	"context"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type PhaseMetric struct {
	Name       string        `json:"name"`
	Operations int           `json:"operations"`
	Duration   time.Duration `json:"duration_ns"`
	Throughput float64       `json:"operations_per_second"`
}

type Metrics struct {
	SchemaVersion int           `json:"schema_version"`
	Backend       string        `json:"backend"`
	DatasetSHA256 string        `json:"dataset_sha256"`
	StartedAt     time.Time     `json:"started_at"`
	FinishedAt    time.Time     `json:"finished_at"`
	Phases        []PhaseMetric `json:"phases"`
}

type Clock func() time.Time

type Runner struct{ Now Clock }

func (r Runner) Run(ctx context.Context, backend string, store port.Store, dataset Dataset, manifest Manifest) (Metrics, error) {
	if backend == "" || store == nil {
		return Metrics{}, fmt.Errorf("backend and store are required")
	}
	now := r.Now
	if now == nil {
		now = time.Now
	}
	metrics := Metrics{SchemaVersion: 1, Backend: backend, DatasetSHA256: manifest.SHA256, StartedAt: now()}
	measure := func(name string, operations int, fn func() error) error {
		started := now()
		if err := fn(); err != nil {
			return fmt.Errorf("phase %s: %w", name, err)
		}
		duration := now().Sub(started)
		throughput := 0.0
		if duration > 0 {
			throughput = float64(operations) / duration.Seconds()
		}
		metrics.Phases = append(metrics.Phases, PhaseMetric{Name: name, Operations: operations, Duration: duration, Throughput: throughput})
		return nil
	}
	if err := measure("load_sources", len(dataset.Sources), func() error {
		for _, fixture := range dataset.Sources {
			fixture := fixture
			if err := store.Update(ctx, func(tx port.Transaction) error {
				if err := tx.AppendSource(fixture.Source, fixture.Version, fixture.Snapshot); err != nil {
					return err
				}
				if err := tx.AppendSourceFragments(fixture.Version.ID, []domain.SourceFragment{fixture.Fragment}); err != nil {
					return err
				}
				return tx.AppendObservation(fixture.Observation)
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Metrics{}, err
	}
	if err := measure("load_claims", len(dataset.Claims), func() error {
		for _, fixture := range dataset.Claims {
			fixture := fixture
			if err := store.Update(ctx, func(tx port.Transaction) error { return tx.AppendClaimWithEvidence(fixture.Claim, fixture.Links) }); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Metrics{}, err
	}
	if err := measure("query_claims", len(dataset.Claims), func() error {
		return store.View(ctx, func(reader port.Reader) error {
			for _, fixture := range dataset.Claims {
				if _, err := reader.Claim(fixture.Claim.ID); err != nil {
					return err
				}
				if _, err := reader.EvidenceLinksForClaim(fixture.Claim.ID); err != nil {
					return err
				}
			}
			return nil
		})
	}); err != nil {
		return Metrics{}, err
	}
	metrics.FinishedAt = now()
	return metrics, nil
}
