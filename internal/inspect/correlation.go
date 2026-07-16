package inspect

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// OperationDetail reconstructs an operation and the official records that
// explain its outcome without relying on model chain-of-thought.
type OperationDetail struct {
	SchemaVersion  int                        `json:"schema_version"`
	Operation      domain.Operation           `json:"operation"`
	Spec           *domain.OperationSpec      `json:"spec,omitempty"`
	Inquiry        *domain.Inquiry            `json:"inquiry,omitempty"`
	Question       *domain.Question           `json:"question,omitempty"`
	RawOutputs     []domain.RawModelOutput    `json:"raw_model_outputs"`
	Proposed       []domain.ProposedChangeSet `json:"proposed_change_sets"`
	Accepted       []domain.AcceptedChangeSet `json:"accepted_change_sets"`
	Commits        []domain.Commit            `json:"commits"`
	CommitReceipts []domain.CommitReceipt     `json:"commit_receipts"`
	Validations    []domain.ValidationReceipt `json:"validation_receipts"`
	Events         []domain.Event             `json:"events"`
	Idempotency    *domain.IdempotencyRecord  `json:"idempotency,omitempty"`
	HeadCommit     *domain.Commit             `json:"head_commit,omitempty"`
	// ModelRecovery is a derived, read-only summary of FR-MODEL-004 ladder
	// decisions for this operation (parsed from official event PayloadRefs).
	// Empty when no recovery/model events are present.
	ModelRecovery *ModelRecoverySummary `json:"model_recovery,omitempty"`
}

// ModelRecoverySummary is a presentation projection of model recovery events.
// It never invents state: every field is derived from durable event kinds and
// PayloadRef tags already written by the kernel.
type ModelRecoverySummary struct {
	// Decisions are operation.model_recovery_decision events in occurred order.
	Decisions []ModelRecoveryDecisionView `json:"decisions"`
	// Invocations count operation.model_invoked events (including recovery calls).
	Invocations int `json:"invocations"`
	// FallbackInvocations counts invocations tagged fallback=1.
	FallbackInvocations int `json:"fallback_invocations"`
	// RecoveryInvocations counts invocations tagged recovery=1.
	RecoveryInvocations int `json:"recovery_invocations"`
	// Exhausted is true when an operation.model_exhausted event exists.
	Exhausted bool `json:"exhausted"`
	// LastDisposition is the disposition of the latest recovery decision, if any.
	LastDisposition string `json:"last_disposition,omitempty"`
	// LastStage is the stage of the latest recovery decision, if any.
	LastStage string `json:"last_stage,omitempty"`
	// StagesTried lists unique recovery stages seen (stable first-seen order).
	StagesTried []string `json:"stages_tried,omitempty"`
}

// ModelRecoveryDecisionView is one parsed recovery decision event.
type ModelRecoveryDecisionView struct {
	EventID     domain.EventID `json:"event_id"`
	OccurredAt  string         `json:"occurred_at"`
	Disposition string         `json:"disposition,omitempty"`
	Stage       string         `json:"stage,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	// Calls is the model_calls counter embedded in the payload (when present).
	Calls string `json:"calls,omitempty"`
	// PayloadRef is the original compact audit string (no free-text model body).
	PayloadRef string `json:"payload_ref,omitempty"`
}

// CommitDetail correlates a commit with its proposal and validation receipts.
type CommitDetail struct {
	SchemaVersion int                        `json:"schema_version"`
	Commit        domain.Commit              `json:"commit"`
	Receipt       *domain.CommitReceipt      `json:"commit_receipt,omitempty"`
	Accepted      *domain.AcceptedChangeSet  `json:"accepted_change_set,omitempty"`
	Proposed      *domain.ProposedChangeSet  `json:"proposed_change_set,omitempty"`
	Validations   []domain.ValidationReceipt `json:"validation_receipts"`
	Events        []domain.Event             `json:"events"`
}

// CommandDetail shows an operator command and its receipt without write access.
type CommandDetail struct {
	SchemaVersion int                    `json:"schema_version"`
	Command       domain.OperatorCommand `json:"command"`
	Receipt       domain.CommandReceipt  `json:"receipt"`
	Events        []domain.Event         `json:"events"`
}

// OperationInspector loads one operation and related official records.
func (p *Projector) OperationInspector(ctx context.Context, operationID domain.OperationID) (OperationDetail, error) {
	if operationID == "" {
		return OperationDetail{}, errors.New("operation ID is required")
	}
	var detail OperationDetail
	err := p.Store.View(ctx, func(r port.Reader) error {
		operation, err := r.Operation(operationID)
		if err != nil {
			return err
		}
		detail = OperationDetail{
			SchemaVersion:  domain.SchemaVersionV1,
			Operation:      operation,
			RawOutputs:     []domain.RawModelOutput{},
			Proposed:       []domain.ProposedChangeSet{},
			Accepted:       []domain.AcceptedChangeSet{},
			Commits:        []domain.Commit{},
			CommitReceipts: []domain.CommitReceipt{},
			Validations:    []domain.ValidationReceipt{},
			Events:         []domain.Event{},
		}
		if spec, err := r.OperationSpec(operation.SpecID); err == nil {
			detail.Spec = &spec
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		if inquiry, err := r.Inquiry(operation.InquiryID); err == nil {
			detail.Inquiry = &inquiry
			if question, qerr := r.Question(inquiry.QuestionID); qerr == nil {
				detail.Question = &question
			} else if !errors.Is(qerr, port.ErrNotFound) {
				return qerr
			}
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		if record, err := r.IdempotencyRecord(operation.IdempotencyKey); err == nil {
			detail.Idempotency = &record
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		if head, err := r.HeadCommit(operation.MissionRevision); err == nil {
			detail.HeadCommit = &head
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}

		// Correlate by scanning the event log for this operation.
		events, err := collectMatchingEvents(r, EventFilter{OperationID: operationID, Limit: MaxEventPageLimit})
		if err != nil {
			return err
		}
		detail.Events = events

		// Commit linkage is carried on events and on the operation idempotency key.
		seenCommits := map[domain.CommitID]struct{}{}
		for _, event := range events {
			if event.CommitID == "" {
				continue
			}
			if _, ok := seenCommits[event.CommitID]; ok {
				continue
			}
			seenCommits[event.CommitID] = struct{}{}
			commit, err := r.Commit(event.CommitID)
			if err != nil {
				if errors.Is(err, port.ErrNotFound) {
					continue
				}
				return err
			}
			detail.Commits = append(detail.Commits, commit)
			if receipt, err := r.CommitReceipt(commit.ReceiptID); err == nil {
				detail.CommitReceipts = append(detail.CommitReceipts, receipt)
			} else if !errors.Is(err, port.ErrNotFound) {
				return err
			}
			if accepted, err := r.AcceptedChangeSet(commit.AcceptedChangeSetID); err == nil {
				detail.Accepted = appendUniqueAccepted(detail.Accepted, accepted)
				for _, receiptID := range accepted.ValidationReceiptIDs {
					if receipt, err := r.ValidationReceipt(receiptID); err == nil {
						detail.Validations = appendUniqueValidation(detail.Validations, receipt)
					} else if !errors.Is(err, port.ErrNotFound) {
						return err
					}
				}
				if proposed, err := r.ProposedChangeSet(accepted.ProposedChangeSetID); err == nil {
					detail.Proposed = appendUniqueProposed(detail.Proposed, proposed)
				} else if !errors.Is(err, port.ErrNotFound) {
					return err
				}
			} else if !errors.Is(err, port.ErrNotFound) {
				return err
			}
		}

		// Also resolve commit by the operation's durable intent key when present.
		if commit, err := r.CommitByIdempotencyKey(operation.IdempotencyKey); err == nil {
			if _, ok := seenCommits[commit.ID]; !ok {
				detail.Commits = append(detail.Commits, commit)
				if receipt, err := r.CommitReceipt(commit.ReceiptID); err == nil {
					detail.CommitReceipts = append(detail.CommitReceipts, receipt)
				} else if !errors.Is(err, port.ErrNotFound) {
					return err
				}
				if accepted, err := r.AcceptedChangeSet(commit.AcceptedChangeSetID); err == nil {
					detail.Accepted = appendUniqueAccepted(detail.Accepted, accepted)
					for _, receiptID := range accepted.ValidationReceiptIDs {
						if receipt, err := r.ValidationReceipt(receiptID); err == nil {
							detail.Validations = appendUniqueValidation(detail.Validations, receipt)
						} else if !errors.Is(err, port.ErrNotFound) {
							return err
						}
					}
					if proposed, err := r.ProposedChangeSet(accepted.ProposedChangeSetID); err == nil {
						detail.Proposed = appendUniqueProposed(detail.Proposed, proposed)
					} else if !errors.Is(err, port.ErrNotFound) {
						return err
					}
				} else if !errors.Is(err, port.ErrNotFound) {
					return err
				}
			}
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}

		// Raw model outputs are evidence addressed by validation artifact refs.
		// There is no list-by-operation port; correlation stays receipt-driven.
		seenRaw := map[domain.ArtifactID]struct{}{}
		for _, receipt := range detail.Validations {
			if receipt.ArtifactRef == "" {
				continue
			}
			if _, ok := seenRaw[receipt.ArtifactRef]; ok {
				continue
			}
			raw, err := r.RawModelOutput(receipt.ArtifactRef)
			if err != nil {
				if errors.Is(err, port.ErrNotFound) {
					continue
				}
				return err
			}
			seenRaw[receipt.ArtifactRef] = struct{}{}
			detail.RawOutputs = append(detail.RawOutputs, raw)
		}
		sort.Slice(detail.RawOutputs, func(i, j int) bool {
			return detail.RawOutputs[i].CreatedAt.Before(detail.RawOutputs[j].CreatedAt)
		})

		sort.Slice(detail.Commits, func(i, j int) bool {
			return detail.Commits[i].Version < detail.Commits[j].Version
		})
		detail.ModelRecovery = deriveModelRecoverySummary(detail.Events)
		return nil
	})
	if err != nil {
		return OperationDetail{}, err
	}
	return detail, nil
}

// deriveModelRecoverySummary projects FR-MODEL-004 audit events into a compact
// operator view. Returns nil when the operation never contacted the model path
// for recovery (no recovery decision, exhaust, or recovery-tagged invoke).
func deriveModelRecoverySummary(events []domain.Event) *ModelRecoverySummary {
	if len(events) == 0 {
		return nil
	}
	var (
		sum        ModelRecoverySummary
		hasSignal  bool
		seenStages = map[string]struct{}{}
		stageOrder []string
	)
	// Events from collectMatchingEvents are already chronological for a page;
	// still sort by OccurredAt for multi-source safety.
	sorted := append([]domain.Event(nil), events...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].OccurredAt.Equal(sorted[j].OccurredAt) {
			return string(sorted[i].ID) < string(sorted[j].ID)
		}
		return sorted[i].OccurredAt.Before(sorted[j].OccurredAt)
	})
	for _, event := range sorted {
		switch event.Kind {
		case "operation.model_invoked":
			sum.Invocations++
			tags := parsePayloadTags(event.PayloadRef)
			if tags["fallback"] == "1" {
				sum.FallbackInvocations++
				hasSignal = true
			}
			if tags["recovery"] == "1" {
				sum.RecoveryInvocations++
				hasSignal = true
			}
		case "operation.model_recovery_decision":
			hasSignal = true
			tags := parsePayloadTags(event.PayloadRef)
			view := ModelRecoveryDecisionView{
				EventID:     event.ID,
				OccurredAt:  event.OccurredAt.UTC().Format(time.RFC3339Nano),
				Disposition: tags["disposition"],
				Stage:       tags["stage"],
				Reason:      tags["reason"],
				Calls:       tags["calls"],
				PayloadRef:  event.PayloadRef,
			}
			sum.Decisions = append(sum.Decisions, view)
			if view.Stage != "" {
				if _, ok := seenStages[view.Stage]; !ok {
					seenStages[view.Stage] = struct{}{}
					stageOrder = append(stageOrder, view.Stage)
				}
			}
			sum.LastDisposition = view.Disposition
			sum.LastStage = view.Stage
		case "operation.model_exhausted":
			hasSignal = true
			sum.Exhausted = true
			tags := parsePayloadTags(event.PayloadRef)
			if d := tags["disposition"]; d != "" {
				sum.LastDisposition = d
			}
		}
	}
	if !hasSignal {
		return nil
	}
	sum.StagesTried = stageOrder
	if sum.Decisions == nil {
		sum.Decisions = []ModelRecoveryDecisionView{}
	}
	return &sum
}

// parsePayloadTags splits compact audit refs of the form
// "lease...;key=value;flag=1" into a map. Values never contain free-text bodies.
func parsePayloadTags(payload string) map[string]string {
	out := map[string]string{}
	if payload == "" {
		return out
	}
	for _, part := range strings.Split(payload, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

// CommitInspector loads a commit and its official supporting records.
func (p *Projector) CommitInspector(ctx context.Context, commitID domain.CommitID) (CommitDetail, error) {
	if commitID == "" {
		return CommitDetail{}, errors.New("commit ID is required")
	}
	var detail CommitDetail
	err := p.Store.View(ctx, func(r port.Reader) error {
		commit, err := r.Commit(commitID)
		if err != nil {
			return err
		}
		detail = CommitDetail{
			SchemaVersion: domain.SchemaVersionV1,
			Commit:        commit,
			Validations:   []domain.ValidationReceipt{},
			Events:        []domain.Event{},
		}
		if receipt, err := r.CommitReceipt(commit.ReceiptID); err == nil {
			detail.Receipt = &receipt
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		if accepted, err := r.AcceptedChangeSet(commit.AcceptedChangeSetID); err == nil {
			detail.Accepted = &accepted
			for _, receiptID := range accepted.ValidationReceiptIDs {
				if receipt, err := r.ValidationReceipt(receiptID); err == nil {
					detail.Validations = append(detail.Validations, receipt)
				} else if !errors.Is(err, port.ErrNotFound) {
					return err
				}
			}
			if proposed, err := r.ProposedChangeSet(accepted.ProposedChangeSetID); err == nil {
				detail.Proposed = &proposed
			} else if !errors.Is(err, port.ErrNotFound) {
				return err
			}
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		events, err := collectMatchingEvents(r, EventFilter{CommitID: commitID, Limit: MaxEventPageLimit})
		if err != nil {
			return err
		}
		detail.Events = events
		return nil
	})
	return detail, err
}

// CommandInspector loads one operator command and receipt for audit.
func (p *Projector) CommandInspector(ctx context.Context, commandID domain.CommandID) (CommandDetail, error) {
	if commandID == "" {
		return CommandDetail{}, errors.New("command ID is required")
	}
	var detail CommandDetail
	err := p.Store.View(ctx, func(r port.Reader) error {
		command, err := r.OperatorCommand(commandID)
		if err != nil {
			return err
		}
		receipt, err := r.OperatorCommandReceipt(commandID)
		if err != nil {
			return err
		}
		// Correlate control events whose payload references the command.
		events, err := collectMatchingEvents(r, EventFilter{Limit: MaxEventPageLimit})
		if err != nil {
			return err
		}
		filtered := make([]domain.Event, 0)
		for _, event := range events {
			if event.PayloadRef == string(commandID) ||
				event.PayloadRef == string(commandID)+":"+receipt.ResultRef ||
				(receipt.FailureCode != "" && event.PayloadRef == string(commandID)+":"+receipt.FailureCode) ||
				event.PayloadRef == receipt.ResultRef {
				// Narrow to command-related kinds when payload is only resultRef.
				switch event.Kind {
				case "operator.command.received", "operator.command.rejected", "operator.command.applied",
					"process.stopping", "mission.paused", "mission.resumed", "mission.cancelled":
					filtered = append(filtered, event)
				default:
					if event.PayloadRef == string(commandID) ||
						(len(event.PayloadRef) >= len(commandID) && event.PayloadRef[:len(commandID)] == string(commandID)) {
						filtered = append(filtered, event)
					}
				}
			}
		}
		detail = CommandDetail{
			SchemaVersion: domain.SchemaVersionV1,
			Command:       command,
			Receipt:       receipt,
			Events:        filtered,
		}
		return nil
	})
	return detail, err
}

func appendUniqueProposed(items []domain.ProposedChangeSet, item domain.ProposedChangeSet) []domain.ProposedChangeSet {
	for _, existing := range items {
		if existing.ID == item.ID {
			return items
		}
	}
	return append(items, item)
}

func appendUniqueAccepted(items []domain.AcceptedChangeSet, item domain.AcceptedChangeSet) []domain.AcceptedChangeSet {
	for _, existing := range items {
		if existing.ID == item.ID {
			return items
		}
	}
	return append(items, item)
}

func appendUniqueValidation(items []domain.ValidationReceipt, item domain.ValidationReceipt) []domain.ValidationReceipt {
	for _, existing := range items {
		if existing.ID == item.ID {
			return items
		}
	}
	return append(items, item)
}

func collectMatchingEvents(r port.Reader, filter EventFilter) ([]domain.Event, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultEventPageLimit
	}
	if limit > MaxEventPageLimit {
		limit = MaxEventPageLimit
	}
	matched := make([]domain.Event, 0, limit)
	after := filter.AfterSequence
	for {
		batch, err := r.Events(after, MaxEventPageLimit)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, event := range batch {
			if !eventMatches(event, filter) {
				continue
			}
			matched = append(matched, event)
			if len(matched) >= limit {
				return matched, nil
			}
		}
		after = batch[len(batch)-1].Sequence
		if len(batch) < MaxEventPageLimit {
			break
		}
	}
	return matched, nil
}

// Health is a process liveness projection independent of mission detail.
type Health struct {
	Status            string             `json:"status"`
	Runtime           RuntimeIdentity    `json:"runtime"`
	ProcessMode       domain.ProcessMode `json:"process_mode"`
	ControlRevision   uint64             `json:"control_revision"`
	EventHeadSequence uint64             `json:"event_head_sequence"`
	StoreReachable    bool               `json:"store_reachable"`
}

// HealthProbe performs a read-only store touch for liveness.
func (p *Projector) HealthProbe(ctx context.Context) (Health, error) {
	health := Health{
		Status:  "ok",
		Runtime: p.Runtime,
	}
	err := p.Store.View(ctx, func(r port.Reader) error {
		health.StoreReachable = true
		health.ProcessMode = domain.ProcessRunning
		if control, err := r.ControlState(); err == nil {
			health.ProcessMode = control.ProcessMode
			health.ControlRevision = control.Revision
			if control.ProcessMode == domain.ProcessStopping {
				health.Status = "stopping"
			}
			if control.ProcessMode == domain.ProcessStopped {
				health.Status = "stopped"
			}
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		head, err := eventHead(r)
		if err != nil {
			return err
		}
		health.EventHeadSequence = head
		return nil
	})
	if err != nil {
		return Health{
			Status:         "degraded",
			Runtime:        p.Runtime,
			StoreReachable: false,
		}, fmt.Errorf("health probe: %w", err)
	}
	return health, nil
}
