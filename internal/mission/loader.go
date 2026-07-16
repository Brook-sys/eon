// Package mission owns the deterministic boundary that accepts a versioned
// MissionSpec and installs its immutable MissionRevision atomically.
package mission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const DefaultMaxSpecBytes int64 = 64 << 10

const EventMissionRevisionActivated = "mission.revision_activated"

type Clock interface{ Now() time.Time }
type IDGenerator interface {
	NewID(prefix string) (string, error)
}

// Spec is the external, versioned MissionSpec accepted by the runtime. It is
// deliberately separate from the persistence-facing MissionRevision.
type Spec struct {
	SchemaVersion        int                          `json:"schema_version"`
	ID                   domain.MissionID             `json:"id"`
	Revision             uint64                       `json:"revision"`
	OriginalText         string                       `json:"original_text"`
	Purpose              string                       `json:"purpose"`
	Domains              []string                     `json:"domains"`
	Policies             []string                     `json:"policies"`
	Budget               domain.Budget                `json:"budget"`
	Status               domain.MissionStatus         `json:"status"`
	StandingObjectives   []string                     `json:"standing_objectives,omitempty"`
	RecurringObligations []domain.RecurringObligation `json:"recurring_obligations,omitempty"`
}

func (s Spec) Validate() error {
	if s.SchemaVersion != domain.SchemaVersionV1 {
		return fmt.Errorf("unsupported mission spec schema version %d", s.SchemaVersion)
	}
	if s.ID == "" || s.Revision == 0 || strings.TrimSpace(s.OriginalText) == "" || strings.TrimSpace(s.Purpose) == "" {
		return errors.New("mission spec is missing id, revision, original_text, or purpose")
	}
	if err := validateStringSet("domains", s.Domains); err != nil {
		return err
	}
	if err := validateStringSet("policies", s.Policies); err != nil {
		return err
	}
	if s.Status != domain.MissionActive {
		return fmt.Errorf("mission spec status must be %s to become active, got %q", domain.MissionActive, s.Status)
	}
	if err := domain.ValidateStandingObjectives(s.StandingObjectives); err != nil {
		return err
	}
	if err := domain.ValidateRecurringObligations(s.RecurringObligations); err != nil {
		return err
	}
	return s.Budget.Validate()
}

func validateStringSet(name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must not be empty", name)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("%s contains an empty value", name)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%s contains duplicate %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

type Loader struct {
	Store        port.Store
	Clock        Clock
	IDs          IDGenerator
	MaxSpecBytes int64
}

// Load parses exactly one strict JSON object, validates it, then appends and
// activates the corresponding revision together with its audit event in one
// storage transaction (FR-AUTH-001, FR-OBS-001).
func (l Loader) Load(ctx context.Context, raw []byte, provenance string) (domain.MissionRevision, error) {
	if l.Store == nil || l.Clock == nil || l.IDs == nil {
		return domain.MissionRevision{}, errors.New("mission loader dependencies are incomplete")
	}
	if strings.TrimSpace(provenance) == "" {
		return domain.MissionRevision{}, errors.New("mission provenance must not be empty")
	}
	limit := l.MaxSpecBytes
	if limit == 0 {
		limit = DefaultMaxSpecBytes
	}
	if limit < 0 {
		return domain.MissionRevision{}, errors.New("mission spec byte limit must not be negative")
	}
	if int64(len(raw)) > limit {
		return domain.MissionRevision{}, fmt.Errorf("mission spec exceeds %d-byte limit", limit)
	}

	var spec Spec
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return domain.MissionRevision{}, fmt.Errorf("decode mission spec: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return domain.MissionRevision{}, errors.New("decode mission spec: trailing JSON value")
		}
		return domain.MissionRevision{}, fmt.Errorf("decode mission spec trailing data: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return domain.MissionRevision{}, fmt.Errorf("validate mission spec: %w", err)
	}

	revisionID, err := l.IDs.NewID("mission_revision")
	if err != nil {
		return domain.MissionRevision{}, fmt.Errorf("generate mission revision ID: %w", err)
	}
	eventID, err := l.IDs.NewID("event")
	if err != nil {
		return domain.MissionRevision{}, fmt.Errorf("generate mission event ID: %w", err)
	}
	now := l.Clock.Now().UTC()
	revision := domain.MissionRevision{
		SchemaVersion:        spec.SchemaVersion,
		ID:                   domain.MissionRevisionID(revisionID),
		MissionID:            spec.ID,
		Revision:             spec.Revision,
		OriginalText:         spec.OriginalText,
		Purpose:              spec.Purpose,
		Domains:              append([]string(nil), spec.Domains...),
		Policies:             append([]string(nil), spec.Policies...),
		Budget:               spec.Budget,
		Status:               spec.Status,
		StandingObjectives:   append([]string(nil), spec.StandingObjectives...),
		RecurringObligations: append([]domain.RecurringObligation(nil), spec.RecurringObligations...),
		Provenance:           provenance,
		AcceptedAt:           now,
	}
	if err := revision.Validate(); err != nil {
		return domain.MissionRevision{}, fmt.Errorf("build mission revision: %w", err)
	}

	err = l.Store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		if err := tx.ActivateMissionRevision(revision.MissionID, revision.ID); err != nil {
			return err
		}
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1, ID: domain.EventID(eventID), Kind: EventMissionRevisionActivated,
			OccurredAt: now, MissionRevision: revision.ID, PayloadRef: string(revision.ID),
		})
		return err
	})
	if err != nil {
		return domain.MissionRevision{}, fmt.Errorf("install mission revision: %w", err)
	}
	return revision, nil
}
