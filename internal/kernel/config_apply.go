package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

const (
	EventConfigDraftValidated = "config.draft.validated"
	EventConfigRevisionApplied = "config.revision.applied"
	EventConfigDraftRejected  = "config.draft.rejected"
)

// ConfigApplier validates and applies configuration drafts through durable
// transactions. Transports only create drafts; this component owns apply.
type ConfigApplier struct {
	Store port.Store
	Clock source.Clock
	IDs   source.IDGenerator
}

func NewConfigApplier(store port.Store, clock source.Clock, ids source.IDGenerator) (*ConfigApplier, error) {
	if store == nil || clock == nil || ids == nil {
		return nil, errors.New("config applier requires store, clock, and ID generator")
	}
	return &ConfigApplier{Store: store, Clock: clock, IDs: ids}, nil
}

// ValidateDraft runs pure validation, impact preview, and marks the draft
// VALIDATED with a RECEIVED→VALIDATING→ACCEPTED receipt trail when successful.
func (a *ConfigApplier) ValidateDraft(ctx context.Context, draftID domain.ConfigDraftID) (domain.ConfigImpactPreview, domain.ConfigDiff, error) {
	if draftID == "" {
		return domain.ConfigImpactPreview{}, domain.ConfigDiff{}, errors.New("draft ID is required")
	}
	var preview domain.ConfigImpactPreview
	var diff domain.ConfigDiff
	err := a.Store.Update(ctx, func(tx port.Transaction) error {
		draft, err := tx.ConfigDraft(draftID)
		if err != nil {
			return err
		}
		if draft.Status != domain.ConfigDraftOpen {
			return fmt.Errorf("%w: draft is not OPEN", port.ErrConflict)
		}
		var active *domain.ConfigRevision
		if current, err := tx.ActiveConfigRevision(draft.Scope); err == nil {
			active = &current
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		diff, err = domain.DiffConfig(active, draft)
		if err != nil {
			return err
		}
		preview, err = domain.PreviewConfigImpact(draft, diff)
		if err != nil {
			return err
		}
		now := a.Clock.Now().UTC()
		if err := a.ensureReceipt(tx, draftID, domain.ConfigApplyReceived, now, "", "", ""); err != nil {
			return err
		}
		if err := a.ensureReceipt(tx, draftID, domain.ConfigApplyValidating, now.Add(time.Nanosecond), "", "", ""); err != nil {
			return err
		}
		if preview.Blocked {
			if err := a.ensureReceipt(tx, draftID, domain.ConfigApplyRejected, now.Add(2*time.Nanosecond), "", "IMPACT_BLOCKED", ""); err != nil {
				return err
			}
			rejected := draft
			rejected.Status = domain.ConfigDraftRejected
			if err := tx.SaveConfigDraft(rejected); err != nil {
				return err
			}
			return a.appendConfigEvent(tx, EventConfigDraftRejected, string(draftID)+":IMPACT_BLOCKED", now)
		}
		validated, err := domain.MarkConfigDraftValidated(draft, now.Add(2*time.Nanosecond))
		if err != nil {
			return err
		}
		if err := tx.SaveConfigDraft(validated); err != nil {
			return err
		}
		if err := a.ensureReceipt(tx, draftID, domain.ConfigApplyAccepted, now.Add(3*time.Nanosecond), "", "", ""); err != nil {
			return err
		}
		return a.appendConfigEvent(tx, EventConfigDraftValidated, string(draftID)+":"+string(draft.Scope), now)
	})
	return preview, diff, err
}

// ApplyDraft promotes a validated draft to an immutable revision and active
// pointer. Receipt ends APPLIED or FAILED/REJECTED.
func (a *ConfigApplier) ApplyDraft(ctx context.Context, draftID domain.ConfigDraftID) (domain.ConfigRevision, domain.ConfigApplyReceipt, error) {
	if draftID == "" {
		return domain.ConfigRevision{}, domain.ConfigApplyReceipt{}, errors.New("draft ID is required")
	}
	var final domain.ConfigRevision
	var receipt domain.ConfigApplyReceipt
	err := a.Store.Update(ctx, func(tx port.Transaction) error {
		draft, err := tx.ConfigDraft(draftID)
		if err != nil {
			return err
		}
		existing, err := tx.ConfigApplyReceipt(draftID)
		if err == nil && existing.State.Terminal() {
			if existing.State == domain.ConfigApplyApplied {
				rev, revErr := tx.ConfigRevision(existing.RevisionID)
				if revErr != nil {
					return revErr
				}
				final = rev
				receipt = existing
				return nil
			}
			receipt = existing
			return fmt.Errorf("%w: draft apply already terminal with state %s", port.ErrConflict, existing.State)
		}
		if draft.Status != domain.ConfigDraftValidated {
			return fmt.Errorf("%w: draft must be VALIDATED before apply", port.ErrConflict)
		}
		var active *domain.ConfigRevision
		if current, err := tx.ActiveConfigRevision(draft.Scope); err == nil {
			active = &current
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		now := a.Clock.Now().UTC()
		if err := a.ensureReceipt(tx, draftID, domain.ConfigApplyApplying, now, "", "", ""); err != nil {
			return err
		}
		revisionID, err := a.IDs.NewID("cfgrev")
		if err != nil {
			return err
		}
		receiptID, err := a.IDs.NewID("receipt")
		if err != nil {
			return err
		}
		// Receipt identity is stable per draft; reuse existing receipt ID.
		if existing.ID != "" {
			receiptID = string(existing.ID)
		}
		revision, appliedDraft, appliedReceipt, err := domain.ApplyConfigDraft(active, draft, domain.ConfigRevisionID(revisionID), domain.ReceiptID(receiptID), now.Add(time.Nanosecond))
		if err != nil {
			code := "APPLY_FAILED"
			if errors.Is(err, domain.ErrConflict) {
				code = "STALE_BASE"
			}
			if advErr := a.ensureReceipt(tx, draftID, domain.ConfigApplyFailed, now.Add(2*time.Nanosecond), "", code, ""); advErr != nil {
				return advErr
			}
			return err
		}
		if err := tx.AppendConfigRevision(revision); err != nil {
			return err
		}
		if err := tx.ActivateConfigRevision(revision.Scope, revision.ID); err != nil {
			return err
		}
		if err := tx.SaveConfigDraft(appliedDraft); err != nil {
			return err
		}
		// Domain helper returns APPLIED receipt; advance from APPLYING.
		if err := tx.SaveConfigApplyReceipt(appliedReceipt); err != nil {
			return err
		}
		if err := a.appendConfigEvent(tx, EventConfigRevisionApplied, appliedReceipt.ResultRef, now); err != nil {
			return err
		}
		final = revision
		receipt = appliedReceipt
		return nil
	})
	return final, receipt, err
}

func (a *ConfigApplier) ensureReceipt(tx port.Transaction, draftID domain.ConfigDraftID, state domain.ConfigApplyState, at time.Time, resultRef, failureCode, revisionID string) error {
	current, err := tx.ConfigApplyReceipt(draftID)
	if errors.Is(err, port.ErrNotFound) {
		if state != domain.ConfigApplyReceived {
			return fmt.Errorf("%w: missing config apply receipt", port.ErrConflict)
		}
		id, idErr := a.IDs.NewID("receipt")
		if idErr != nil {
			return idErr
		}
		return tx.SaveConfigApplyReceipt(domain.ConfigApplyReceipt{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            domain.ReceiptID(id),
			DraftID:       draftID,
			State:         domain.ConfigApplyReceived,
			RecordedAt:    at.UTC(),
		})
	}
	if err != nil {
		return err
	}
	if current.State == state {
		return nil
	}
	next := current
	next.State = state
	next.RecordedAt = at.UTC()
	next.ResultRef = resultRef
	next.FailureCode = failureCode
	if revisionID != "" {
		next.RevisionID = domain.ConfigRevisionID(revisionID)
	}
	return tx.SaveConfigApplyReceipt(next)
}

func (a *ConfigApplier) appendConfigEvent(tx port.Transaction, kind, payload string, now time.Time) error {
	eventID, err := a.IDs.NewID("event")
	if err != nil {
		return err
	}
	_, err = tx.AppendEvent(domain.Event{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            domain.EventID(eventID),
		Kind:          kind,
		OccurredAt:    now.UTC(),
		PayloadRef:    payload,
	})
	return err
}
