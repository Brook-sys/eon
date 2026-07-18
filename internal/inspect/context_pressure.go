package inspect

import (
	"context"
	"errors"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// ModelContextPressureView is a safe operator projection of durable, authority-free
// context-pressure state (FR-MODEL-007). It never includes prompts, secrets, or
// provider bodies. Zero-level bindings are omitted unless persisted.
type ModelContextPressureView struct {
	BindingID        string `json:"binding_id"`
	Level            int    `json:"level"`
	SuccessesAtLevel int    `json:"successes_at_level"`
	// ReductionActive is true when the level currently imposes a ceiling.
	ReductionActive bool `json:"reduction_active"`
	// ReductionFraction is the remaining share of the declared window at this
	// level (0.75 / 0.50 / 0.25). Absent when level is zero.
	ReductionFraction float64   `json:"reduction_fraction,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ModelContextPressuresProjection lists persisted pressure rows for inspect.
type ModelContextPressuresProjection struct {
	SchemaVersion int       `json:"schema_version"`
	ObservedAt    time.Time `json:"observed_at"`
	Count         int       `json:"count"`
	// Pressures is sorted by BindingID (store contract).
	Pressures []ModelContextPressureView `json:"pressures"`
	// Note explains empty state without inventing zero-pressure rows.
	Note string `json:"note,omitempty"`
}

// ListModelContextPressures projects durable context-pressure control signals.
// Missing bindings are not synthesized: only rows written by ModelExecutor appear.
func (p *Projector) ListModelContextPressures(ctx context.Context) (ModelContextPressuresProjection, error) {
	if p == nil {
		return ModelContextPressuresProjection{}, errors.New("projector is nil")
	}
	now := p.Clock().UTC()
	out := ModelContextPressuresProjection{
		SchemaVersion: domain.SchemaVersionV1,
		ObservedAt:    now,
		Pressures:     []ModelContextPressureView{},
	}
	err := p.Store.View(ctx, func(r port.Reader) error {
		rows, err := r.ModelContextPressures()
		if err != nil {
			return err
		}
		out.Pressures = make([]ModelContextPressureView, 0, len(rows))
		for _, row := range rows {
			out.Pressures = append(out.Pressures, projectModelContextPressure(row))
		}
		out.Count = len(out.Pressures)
		if out.Count == 0 {
			out.Note = "no model context pressure rows persisted yet; ModelExecutor writes rows only after observed context pressure or recovery"
		}
		return nil
	})
	if err != nil {
		return ModelContextPressuresProjection{}, err
	}
	return out, nil
}

// ModelContextPressure projects one binding row or ErrNotFound.
func (p *Projector) ModelContextPressure(ctx context.Context, bindingID string) (ModelContextPressureView, error) {
	if p == nil {
		return ModelContextPressureView{}, errors.New("projector is nil")
	}
	var view ModelContextPressureView
	err := p.Store.View(ctx, func(r port.Reader) error {
		row, err := r.ModelContextPressure(bindingID)
		if err != nil {
			return err
		}
		view = projectModelContextPressure(row)
		return nil
	})
	if err != nil {
		return ModelContextPressureView{}, err
	}
	return view, nil
}

func projectModelContextPressure(row domain.ModelContextPressure) ModelContextPressureView {
	view := ModelContextPressureView{
		BindingID:        row.BindingID,
		Level:            row.State.Level,
		SuccessesAtLevel: row.State.SuccessesAtLevel,
		UpdatedAt:        row.UpdatedAt.UTC(),
	}
	if row.State.Level > 0 {
		view.ReductionActive = true
		level := row.State.Level
		if level > domain.MaxContextPressureLevel {
			level = domain.MaxContextPressureLevel
		}
		// Matches ReductionForPressure: remaining share = (4-level)/4.
		view.ReductionFraction = float64(4-level) / 4
	}
	return view
}
