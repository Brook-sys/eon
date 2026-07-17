package inspect

import (
	"context"
	"errors"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// ResourceUsageView is a safe operator-facing ResourceGate snapshot (FR-RES-001).
// It never includes secrets, raw provider bodies, or authorization tokens.
type ResourceUsageView struct {
	Resource            domain.ResourceID `json:"resource"`
	InFlight            int               `json:"in_flight"`
	MinuteCount         int               `json:"minute_count"`
	DayCount            int               `json:"day_count"`
	TokenMinuteCount    int               `json:"token_minute_count"`
	ConsecutiveFailures int               `json:"consecutive_failures"`
	// CircuitOpen is true when the gate is currently open at ObservedAt.
	CircuitOpen bool `json:"circuit_open"`
	// CircuitOpenUntil is set when a concrete open-until instant is known.
	CircuitOpenUntil  *time.Time `json:"circuit_open_until,omitempty"`
	LastFailureAt     *time.Time `json:"last_failure_at,omitempty"`
	MinuteWindowStart *time.Time `json:"minute_window_start,omitempty"`
	DayWindowStart    *time.Time `json:"day_window_start,omitempty"`
}

// ResourcesProjection lists persisted ResourceUsage rows for inspect.
type ResourcesProjection struct {
	SchemaVersion int       `json:"schema_version"`
	ObservedAt    time.Time `json:"observed_at"`
	Count         int       `json:"count"`
	// Resources is sorted by ResourceID (store contract).
	Resources []ResourceUsageView `json:"resources"`
	// Note explains empty state without implying unlimited capacity.
	Note string `json:"note,omitempty"`
}

// ListResourceUsages projects durable ResourceGate usage. Missing usage is not
// synthesized: only rows persisted by authorization wiring appear.
func (p *Projector) ListResourceUsages(ctx context.Context) (ResourcesProjection, error) {
	if p == nil {
		return ResourcesProjection{}, errors.New("projector is nil")
	}
	now := p.Clock().UTC()
	out := ResourcesProjection{
		SchemaVersion: domain.SchemaVersionV1,
		ObservedAt:    now,
		Resources:     []ResourceUsageView{},
	}
	err := p.Store.View(ctx, func(r port.Reader) error {
		rows, err := r.ResourceUsages()
		if err != nil {
			return err
		}
		out.Resources = make([]ResourceUsageView, 0, len(rows))
		for _, u := range rows {
			out.Resources = append(out.Resources, projectResourceUsage(u, now))
		}
		out.Count = len(out.Resources)
		if out.Count == 0 {
			out.Note = "no resource usage rows persisted yet; gates create rows on first acquire"
		}
		return nil
	})
	if err != nil {
		return ResourcesProjection{}, err
	}
	return out, nil
}

// ResourceUsage projects one resource row or ErrNotFound.
func (p *Projector) ResourceUsage(ctx context.Context, id domain.ResourceID) (ResourceUsageView, error) {
	if p == nil {
		return ResourceUsageView{}, errors.New("projector is nil")
	}
	var view ResourceUsageView
	now := p.Clock().UTC()
	err := p.Store.View(ctx, func(r port.Reader) error {
		u, err := r.ResourceUsage(id)
		if err != nil {
			return err
		}
		view = projectResourceUsage(u, now)
		return nil
	})
	if err != nil {
		return ResourceUsageView{}, err
	}
	return view, nil
}

func projectResourceUsage(u domain.ResourceUsage, now time.Time) ResourceUsageView {
	view := ResourceUsageView{
		Resource:            u.Resource,
		InFlight:            u.InFlight,
		MinuteCount:         u.MinuteCount,
		DayCount:            u.DayCount,
		TokenMinuteCount:    u.TokenMinuteCount,
		ConsecutiveFailures: u.ConsecutiveFailures,
	}
	if !u.MinuteWindowStart.IsZero() {
		t := u.MinuteWindowStart.UTC()
		view.MinuteWindowStart = &t
	}
	if !u.DayWindowStart.IsZero() {
		t := u.DayWindowStart.UTC()
		view.DayWindowStart = &t
	}
	if u.LastFailureAt != nil {
		t := u.LastFailureAt.UTC()
		view.LastFailureAt = &t
	}
	if u.CircuitOpenUntil != nil {
		t := u.CircuitOpenUntil.UTC()
		view.CircuitOpenUntil = &t
		view.CircuitOpen = now.Before(t)
	}
	return view
}
