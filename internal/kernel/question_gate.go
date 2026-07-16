package kernel

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"motor-autonomo/internal/domain"
)

type QuestionGateDecision string

const (
	QuestionAdmit    QuestionGateDecision = "ADMIT"
	QuestionSuppress QuestionGateDecision = "SUPPRESS"
	QuestionDefer    QuestionGateDecision = "DEFER"
)

type QuestionGateReason string

const (
	QuestionGateAllowed                  QuestionGateReason = "ALLOWED"
	QuestionGateDuplicatePending         QuestionGateReason = "DUPLICATE_PENDING"
	QuestionGateCooldown                 QuestionGateReason = "COOLDOWN"
	QuestionGatePendingLimit             QuestionGateReason = "PENDING_LIMIT"
	QuestionGateRateLimit                QuestionGateReason = "RATE_LIMIT"
	QuestionGateQuietHours               QuestionGateReason = "QUIET_HOURS"
	QuestionGatePriorityLow              QuestionGateReason = "PRIORITY_TOO_LOW"
	QuestionGateSafeDefault              QuestionGateReason = "SAFE_REVERSIBLE_DEFAULT"
	QuestionGateInsufficientAlternatives QuestionGateReason = "INSUFFICIENT_ALTERNATIVES"
)

type QuestionGatePolicy struct {
	MinPriority                   uint8
	MaxPending                    int
	MaxDeliveredPerWindow         int
	Window                        time.Duration
	Cooldown                      time.Duration
	QuietStartHour                int
	QuietEndHour                  int
	UrgentPriority                uint8
	MinAlternativesTried          int
	SuppressSafeReversibleDefault bool
}

func (p QuestionGatePolicy) Validate() error {
	if p.MinPriority == 0 || p.UrgentPriority == 0 || p.UrgentPriority < p.MinPriority {
		return errors.New("question gate priorities must be positive and urgent must not be lower than minimum")
	}
	if p.MaxPending < 0 || p.MaxDeliveredPerWindow < 0 || p.MinAlternativesTried < 0 || p.Window < 0 || p.Cooldown < 0 {
		return errors.New("question gate limits and durations must not be negative")
	}
	if p.MaxDeliveredPerWindow > 0 && p.Window == 0 {
		return errors.New("question gate rate limit requires a positive window")
	}
	if (p.QuietStartHour < 0 || p.QuietStartHour > 23) || (p.QuietEndHour < 0 || p.QuietEndHour > 23) {
		return errors.New("question gate quiet hours must be between 0 and 23")
	}
	return nil
}

type QuestionGateRecord struct {
	QuestionID     domain.OperatorQuestionID
	MissionID      domain.MissionID
	DedupSignature string
	Status         domain.OperatorQuestionStatus
	DeliveredAt    time.Time
	ClosedAt       time.Time
}

func (r QuestionGateRecord) Validate() error {
	if r.QuestionID == "" || r.MissionID == "" || r.DedupSignature == "" || !r.Status.Terminal() && r.Status != domain.OperatorQuestionPending {
		return errors.New("question gate record is incomplete")
	}
	if r.Status.Terminal() && r.ClosedAt.IsZero() {
		return errors.New("closed question gate record requires close time")
	}
	if !r.ClosedAt.IsZero() && r.ClosedAt.Before(r.DeliveredAt) {
		return errors.New("question gate record closes before delivery")
	}
	return nil
}

type QuestionGateResult struct {
	Decision   QuestionGateDecision
	Reason     QuestionGateReason
	RetryAfter time.Time
}

// EvaluateQuestion deterministically decides whether a proposed interruption
// may reach an outbox. Records are observations only; callers persist the
// resulting admit/suppress/defer decision and any delivery separately.
func EvaluateQuestion(policy QuestionGatePolicy, now time.Time, proposal domain.OperatorQuestionProposal, history []QuestionGateRecord) (QuestionGateResult, error) {
	if err := policy.Validate(); err != nil {
		return QuestionGateResult{}, fmt.Errorf("validate question gate policy: %w", err)
	}
	if now.IsZero() {
		return QuestionGateResult{}, errors.New("question gate requires current time")
	}
	if err := proposal.Validate(); err != nil {
		return QuestionGateResult{}, fmt.Errorf("validate question proposal: %w", err)
	}
	for i, record := range history {
		if err := record.Validate(); err != nil {
			return QuestionGateResult{}, fmt.Errorf("validate question history %d: %w", i, err)
		}
	}
	question := proposal.Question
	if question.Priority < policy.MinPriority {
		return QuestionGateResult{Decision: QuestionSuppress, Reason: QuestionGatePriorityLow}, nil
	}
	if policy.SuppressSafeReversibleDefault && proposal.Justification.HasSafeDefault && proposal.Justification.DefaultReversible && question.Priority < policy.UrgentPriority {
		return QuestionGateResult{Decision: QuestionSuppress, Reason: QuestionGateSafeDefault}, nil
	}
	if len(proposal.Justification.AlternativesTried) < policy.MinAlternativesTried && question.Priority < policy.UrgentPriority {
		return QuestionGateResult{Decision: QuestionSuppress, Reason: QuestionGateInsufficientAlternatives}, nil
	}
	if inQuietHours(now.Hour(), policy.QuietStartHour, policy.QuietEndHour) && question.Priority < policy.UrgentPriority {
		return QuestionGateResult{Decision: QuestionDefer, Reason: QuestionGateQuietHours, RetryAfter: nextQuietEnd(now, policy.QuietEndHour)}, nil
	}

	pending := 0
	windowStart := now.Add(-policy.Window)
	windowDelivered := 0
	var cooldownUntil time.Time
	for _, record := range history {
		if record.MissionID != question.MissionID {
			continue
		}
		if record.Status == domain.OperatorQuestionPending {
			pending++
			if record.DedupSignature == question.DedupSignature {
				return QuestionGateResult{Decision: QuestionSuppress, Reason: QuestionGateDuplicatePending}, nil
			}
		}
		if policy.MaxDeliveredPerWindow > 0 && !record.DeliveredAt.IsZero() && !record.DeliveredAt.Before(windowStart) && !record.DeliveredAt.After(now) {
			windowDelivered++
		}
		if record.DedupSignature == question.DedupSignature {
			base := record.DeliveredAt
			if !record.ClosedAt.IsZero() {
				base = record.ClosedAt
			}
			if !base.IsZero() {
				candidate := base.Add(policy.Cooldown)
				if candidate.After(cooldownUntil) {
					cooldownUntil = candidate
				}
			}
		}
	}
	if policy.MaxPending > 0 && pending >= policy.MaxPending {
		return QuestionGateResult{Decision: QuestionDefer, Reason: QuestionGatePendingLimit}, nil
	}
	if policy.MaxDeliveredPerWindow > 0 && windowDelivered >= policy.MaxDeliveredPerWindow {
		return QuestionGateResult{Decision: QuestionDefer, Reason: QuestionGateRateLimit, RetryAfter: oldestWindowExpiry(now, policy.Window, question.MissionID, history)}, nil
	}
	if cooldownUntil.After(now) {
		return QuestionGateResult{Decision: QuestionDefer, Reason: QuestionGateCooldown, RetryAfter: cooldownUntil}, nil
	}
	return QuestionGateResult{Decision: QuestionAdmit, Reason: QuestionGateAllowed}, nil
}

func inQuietHours(hour, start, end int) bool {
	if start == end {
		return false
	}
	if start < end {
		return hour >= start && hour < end
	}
	return hour >= start || hour < end
}

func nextQuietEnd(now time.Time, endHour int) time.Time {
	end := time.Date(now.Year(), now.Month(), now.Day(), endHour, 0, 0, 0, now.Location())
	if !end.After(now) {
		end = end.AddDate(0, 0, 1)
	}
	return end
}

func oldestWindowExpiry(now time.Time, window time.Duration, missionID domain.MissionID, history []QuestionGateRecord) time.Time {
	deliveries := make([]time.Time, 0, len(history))
	start := now.Add(-window)
	for _, record := range history {
		if record.MissionID == missionID && !record.DeliveredAt.Before(start) && !record.DeliveredAt.After(now) {
			deliveries = append(deliveries, record.DeliveredAt)
		}
	}
	if len(deliveries) == 0 {
		return time.Time{}
	}
	sort.Slice(deliveries, func(i, j int) bool { return deliveries[i].Before(deliveries[j]) })
	return deliveries[0].Add(window)
}
