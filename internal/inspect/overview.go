// Package inspect builds read-only control-plane projections from the store.
// Projections never mutate canonical state and never execute capabilities.
package inspect

import (
	"context"
	"errors"
	"sort"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// RuntimeIdentity is static process metadata published by the control API.
type RuntimeIdentity struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AgendaCounts summarizes operational states for one mission revision.
type AgendaCounts struct {
	Total    int                              `json:"total"`
	ByState  map[domain.OperationalState]int  `json:"by_state"`
	Ready    int                              `json:"ready"`
	Running  int                              `json:"running"`
	Waiting  int                              `json:"waiting"`
	Terminal int                              `json:"terminal"`
}

// OperationSummary is a compact agenda row for the overview and mission views.
type OperationSummary struct {
	ID              domain.OperationID           `json:"operation_id"`
	InquiryID       domain.InquiryID             `json:"inquiry_id"`
	SpecID          domain.OperationSpecID       `json:"spec_id"`
	State           domain.OperationalState      `json:"state"`
	Attempt         uint32                       `json:"attempt"`
	IdempotencyKey  domain.IdempotencyKey        `json:"idempotency_key"`
	Reevaluation    domain.ReevaluationCondition `json:"reevaluation"`
	MissionRevision domain.MissionRevisionID     `json:"mission_revision_id"`
}

// MissionOverview is the operator-facing mission projection.
type MissionOverview struct {
	MissionID         domain.MissionID         `json:"mission_id"`
	ActiveRevisionID  domain.MissionRevisionID `json:"active_revision_id"`
	ActiveRevision    uint64                   `json:"active_revision"`
	Status            domain.MissionStatus     `json:"status"`
	Purpose           string                   `json:"purpose"`
	DispatchMode      domain.MissionDispatchMode `json:"dispatch_mode"`
	DispatchAllowsNew bool                     `json:"dispatch_allows_new"`
	ControlReason     string                   `json:"control_reason,omitempty"`
	ControlUpdatedAt  time.Time                `json:"control_updated_at,omitempty"`
	Agenda            AgendaCounts             `json:"agenda"`
	Operations        []OperationSummary       `json:"operations"`
}

// Overview is Slice A of the control plane: health, control, mission, agenda.
type Overview struct {
	SchemaVersion      int                     `json:"schema_version"`
	GeneratedAt        time.Time               `json:"generated_at"`
	Runtime            RuntimeIdentity         `json:"runtime"`
	ProcessMode        domain.ProcessMode      `json:"process_mode"`
	ControlRevision    uint64                  `json:"control_revision"`
	ControlUpdatedAt   time.Time               `json:"control_updated_at,omitempty"`
	ShutdownCommandID  domain.CommandID        `json:"shutdown_command_id,omitempty"`
	EventHeadSequence  uint64                  `json:"event_head_sequence"`
	PendingCommands    int                     `json:"pending_commands"`
	PendingQuestions   int                     `json:"pending_operator_questions"`
	Mission            *MissionOverview        `json:"mission,omitempty"`
}

// Projector materializes inspectable views from a store reader.
type Projector struct {
	Store   port.Store
	Runtime RuntimeIdentity
	Clock   func() time.Time
}

func NewProjector(store port.Store, runtime RuntimeIdentity) (*Projector, error) {
	if store == nil {
		return nil, errors.New("inspect projector requires store")
	}
	if runtime.Name == "" {
		runtime.Name = "motor-autonomo"
	}
	if runtime.Version == "" {
		runtime.Version = "dev"
	}
	return &Projector{
		Store:   store,
		Runtime: runtime,
		Clock:   func() time.Time { return time.Now().UTC() },
	}, nil
}

// BuildOverview projects process control and optional mission detail.
// When missionID is empty and exactly one active mission exists, it is selected.
func (p *Projector) BuildOverview(ctx context.Context, missionID domain.MissionID) (Overview, error) {
	var out Overview
	err := p.Store.View(ctx, func(r port.Reader) error {
		now := p.Clock().UTC()
		out = Overview{
			SchemaVersion: domain.SchemaVersionV1,
			GeneratedAt:   now,
			Runtime:       p.Runtime,
			ProcessMode:   domain.ProcessRunning,
		}
		control, err := r.ControlState()
		if err == nil {
			out.ProcessMode = control.ProcessMode
			out.ControlRevision = control.Revision
			out.ControlUpdatedAt = control.UpdatedAt
			out.ShutdownCommandID = control.ShutdownCommandID
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}

		head, err := eventHead(r)
		if err != nil {
			return err
		}
		out.EventHeadSequence = head

		pending, err := r.PendingOperatorCommands(1000)
		if err != nil {
			return err
		}
		out.PendingCommands = len(pending)

		if missionID == "" {
			// Best-effort selection is only for local single-mission runs.
			// Multi-mission discovery is intentionally not invented here.
			return nil
		}
		mission, err := buildMissionOverview(r, control, missionID)
		if err != nil {
			return err
		}
		out.Mission = &mission
		questions, err := r.OperatorQuestions(missionID, domain.OperatorQuestionPending)
		if err != nil && !errors.Is(err, port.ErrNotFound) {
			// Older stores may not implement pending filters for empty maps.
			return err
		}
		out.PendingQuestions = len(questions)
		return nil
	})
	if err != nil {
		return Overview{}, err
	}
	return out, nil
}

// ListOperations returns stable, sorted operation summaries for a mission revision.
func (p *Projector) ListOperations(ctx context.Context, missionRevision domain.MissionRevisionID) ([]OperationSummary, AgendaCounts, error) {
	var (
		ops    []OperationSummary
		counts AgendaCounts
	)
	err := p.Store.View(ctx, func(r port.Reader) error {
		operations, err := r.Operations(missionRevision)
		if err != nil {
			return err
		}
		ops, counts = summarizeOperations(operations)
		return nil
	})
	if err != nil {
		return nil, AgendaCounts{}, err
	}
	return ops, counts, nil
}

// MissionDetail projects one mission by ID using its active revision.
func (p *Projector) MissionDetail(ctx context.Context, missionID domain.MissionID) (MissionOverview, error) {
	var out MissionOverview
	err := p.Store.View(ctx, func(r port.Reader) error {
		control, err := r.ControlState()
		if err != nil && !errors.Is(err, port.ErrNotFound) {
			return err
		}
		if errors.Is(err, port.ErrNotFound) {
			control = domain.DefaultControlState(p.Clock())
		}
		mission, err := buildMissionOverview(r, control, missionID)
		if err != nil {
			return err
		}
		out = mission
		return nil
	})
	return out, err
}

func buildMissionOverview(r port.Reader, control domain.ControlState, missionID domain.MissionID) (MissionOverview, error) {
	if missionID == "" {
		return MissionOverview{}, errors.New("mission ID is required")
	}
	active, err := r.ActiveMissionRevision(missionID)
	if err != nil {
		return MissionOverview{}, err
	}
	ops, err := r.Operations(active.ID)
	if err != nil {
		return MissionOverview{}, err
	}
	summaries, counts := summarizeOperations(ops)
	out := MissionOverview{
		MissionID:         missionID,
		ActiveRevisionID:  active.ID,
		ActiveRevision:    active.Revision,
		Status:            active.Status,
		Purpose:           active.Purpose,
		DispatchMode:      domain.MissionDispatchEnabled,
		DispatchAllowsNew: control.AllowsDispatch(missionID),
		Agenda:            counts,
		Operations:        summaries,
	}
	if mission, ok := control.Missions[missionID]; ok {
		out.DispatchMode = mission.Mode
		out.ControlReason = mission.Reason
		out.ControlUpdatedAt = mission.UpdatedAt
	}
	return out, nil
}

func summarizeOperations(operations []domain.Operation) ([]OperationSummary, AgendaCounts) {
	counts := AgendaCounts{ByState: map[domain.OperationalState]int{}}
	out := make([]OperationSummary, 0, len(operations))
	for _, op := range operations {
		counts.Total++
		counts.ByState[op.State]++
		switch {
		case op.State == domain.StateReady:
			counts.Ready++
		case op.State == domain.StateRunning || op.State == domain.StateVerifying:
			counts.Running++
		case op.State == domain.StateWaitingTime || op.State == domain.StateWaitingEvent ||
			op.State == domain.StateWaitingApproval || op.State == domain.StateThrottled ||
			op.State == domain.StateBlockedDependency || op.State == domain.StateReplanning:
			counts.Waiting++
		case op.State.Terminal():
			counts.Terminal++
		}
		out = append(out, OperationSummary{
			ID:              op.ID,
			InquiryID:       op.InquiryID,
			SpecID:          op.SpecID,
			State:           op.State,
			Attempt:         op.Attempt,
			IdempotencyKey:  op.IdempotencyKey,
			Reevaluation:    op.Reevaluation,
			MissionRevision: op.MissionRevision,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].State == out[j].State {
			return out[i].ID < out[j].ID
		}
		return out[i].State < out[j].State
	})
	return out, counts
}

func eventHead(r port.Reader) (uint64, error) {
	// Walk forward in pages to discover the latest sequence without requiring a
	// specialized reverse cursor on the store port.
	var after uint64
	for {
		page, err := r.Events(after, 200)
		if err != nil {
			return 0, err
		}
		if len(page) == 0 {
			return after, nil
		}
		after = page[len(page)-1].Sequence
		if len(page) < 200 {
			return after, nil
		}
	}
}

