package spike

import (
	"context"
	"fmt"
	"sort"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type PhaseMetric struct {
	Name       string        `json:"name"`
	Operations int           `json:"operations"`
	Batches    int           `json:"batches"`
	Duration   time.Duration `json:"duration_ns"`
	P50        time.Duration `json:"p50_ns"`
	P95        time.Duration `json:"p95_ns"`
	P99        time.Duration `json:"p99_ns"`
	Throughput float64       `json:"operations_per_second"`
}

type Metrics struct {
	SchemaVersion int              `json:"schema_version"`
	Backend       string           `json:"backend"`
	DatasetSHA256 string           `json:"dataset_sha256"`
	StartedAt     time.Time        `json:"started_at"`
	FinishedAt    time.Time        `json:"finished_at"`
	Footprint     *FootprintMetric `json:"footprint,omitempty"`
	Phases        []PhaseMetric    `json:"phases"`
}

type FootprintMetric struct {
	BeforeBytes int64 `json:"before_bytes"`
	AfterBytes  int64 `json:"after_bytes"`
	DeltaBytes  int64 `json:"delta_bytes"`
}

type Clock func() time.Time

type Runner struct {
	Now           Clock
	BatchSize     int
	FootprintRoot string
}

func (r Runner) Run(ctx context.Context, backend string, store port.Store, dataset Dataset, manifest Manifest) (Metrics, error) {
	if backend == "" || store == nil {
		return Metrics{}, fmt.Errorf("backend and store are required")
	}
	now := r.Now
	if now == nil {
		now = time.Now
	}
	batchSize := r.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}
	metrics := Metrics{SchemaVersion: 2, Backend: backend, DatasetSHA256: manifest.SHA256, StartedAt: now()}
	if r.FootprintRoot != "" {
		before, err := DiskFootprint(r.FootprintRoot)
		if err != nil {
			return Metrics{}, fmt.Errorf("measure initial footprint: %w", err)
		}
		metrics.Footprint = &FootprintMetric{BeforeBytes: before}
	}
	measure := func(name string, operations, batches int, fn func(record func(time.Duration)) error) error {
		var samples []time.Duration
		started := now()
		if err := fn(func(duration time.Duration) { samples = append(samples, duration) }); err != nil {
			return fmt.Errorf("phase %s: %w", name, err)
		}
		duration := now().Sub(started)
		throughput := 0.0
		if duration > 0 {
			throughput = float64(operations) / duration.Seconds()
		}
		metrics.Phases = append(metrics.Phases, PhaseMetric{
			Name: name, Operations: operations, Batches: batches, Duration: duration,
			P50: percentile(samples, 50), P95: percentile(samples, 95), P99: percentile(samples, 99),
			Throughput: throughput,
		})
		return nil
	}
	if err := measure("load_sources", len(dataset.Sources), batchCount(len(dataset.Sources), batchSize), func(record func(time.Duration)) error {
		for start := 0; start < len(dataset.Sources); start += batchSize {
			end := min(start+batchSize, len(dataset.Sources))
			batchStarted := now()
			if err := store.Update(ctx, func(tx port.Transaction) error {
				for _, fixture := range dataset.Sources[start:end] {
					if err := tx.AppendSource(fixture.Source, fixture.Version, fixture.Snapshot); err != nil {
						return err
					}
					if err := tx.AppendSourceFragments(fixture.Version.ID, []domain.SourceFragment{fixture.Fragment}); err != nil {
						return err
					}
					if err := tx.AppendObservation(fixture.Observation); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
			record(now().Sub(batchStarted))
		}
		return nil
	}); err != nil {
		return Metrics{}, err
	}
	if err := measure("load_claims", len(dataset.Claims), batchCount(len(dataset.Claims), batchSize), func(record func(time.Duration)) error {
		for start := 0; start < len(dataset.Claims); start += batchSize {
			end := min(start+batchSize, len(dataset.Claims))
			batchStarted := now()
			if err := store.Update(ctx, func(tx port.Transaction) error {
				for _, fixture := range dataset.Claims[start:end] {
					if err := tx.AppendClaimWithEvidence(fixture.Claim, fixture.Links); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
			record(now().Sub(batchStarted))
		}
		return nil
	}); err != nil {
		return Metrics{}, err
	}
	if err := measure("query_claims", len(dataset.Claims), len(dataset.Claims), func(record func(time.Duration)) error {
		return store.View(ctx, func(reader port.Reader) error {
			for _, fixture := range dataset.Claims {
				queryStarted := now()
				if _, err := reader.Claim(fixture.Claim.ID); err != nil {
					return err
				}
				if _, err := reader.EvidenceLinksForClaim(fixture.Claim.ID); err != nil {
					return err
				}
				record(now().Sub(queryStarted))
			}
			return nil
		})
	}); err != nil {
		return Metrics{}, err
	}
	metrics.FinishedAt = now()
	if metrics.Footprint != nil {
		after, err := DiskFootprint(r.FootprintRoot)
		if err != nil {
			return Metrics{}, fmt.Errorf("measure final footprint: %w", err)
		}
		metrics.Footprint.AfterBytes = after
		metrics.Footprint.DeltaBytes = after - metrics.Footprint.BeforeBytes
	}
	return metrics, nil
}

func batchCount(total, size int) int {
	if total == 0 {
		return 0
	}
	return (total + size - 1) / size
}

func percentile(samples []time.Duration, percent int) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (percent*len(sorted) + 99) / 100 // nearest-rank, converted to zero-based below
	if index < 1 {
		index = 1
	}
	return sorted[index-1]
}
