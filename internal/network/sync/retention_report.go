package peersync

import (
	"context"
	"fmt"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// RetentionPressureReport summarizes the soft pressure state of the canonical
// event log after a sync pass. It does NOT authorize or trigger deletion —
// the append-only invariant is preserved by MVP retention policy.
type RetentionPressureReport struct {
	EventHeadSequence uint64
	EventHeadPressure string // "", "info", or "warn"
	PruneAuthorized   bool   // always false in MVP (append-only)
}

// String returns a human-readable summary suitable for logs and telemetry.
func (r RetentionPressureReport) String() string {
	return fmt.Sprintf("event_head_sequence=%d pressure=%s prune_authorized=%v",
		r.EventHeadSequence, r.EventHeadPressure, r.PruneAuthorized)
}

// RetentionReporter evaluates event log head pressure after sync passes.
// It is a read-only observer — it never mutates canonical state.
type RetentionReporter struct {
	policy domain.StoreRetentionPolicy
	store  port.Store
}

// NewRetentionReporter creates a reporter bound to a retention policy and store.
// The store is only used for read-only pressure assessment via EventReader.
func NewRetentionReporter(policy domain.StoreRetentionPolicy, store port.Store) (*RetentionReporter, error) {
	if store == nil {
		return nil, ErrInvalidFrame
	}
	policy = policy.Normalize()
	return &RetentionReporter{policy: policy, store: store}, nil
}

// Report computes the current retention pressure snapshot.
func (r *RetentionReporter) Report(ctx context.Context) (RetentionPressureReport, error) {
	var headSeq uint64
	err := r.store.View(ctx, func(reader port.Reader) error {
		headSeq = reader.LatestEventSequence()
		return nil
	})
	if err != nil {
		return RetentionPressureReport{}, fmt.Errorf("retention report: %w", err)
	}
	return RetentionPressureReport{
		EventHeadSequence: headSeq,
		EventHeadPressure: r.policy.EventHeadPressure(headSeq),
		PruneAuthorized:   r.policy.IsRetentionActionAuthorized(domain.RetentionActionEventLogPrune),
	}, nil
}
