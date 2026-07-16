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
	QuestionGateDuplicateTopic           QuestionGateReason = "DUPLICATE_TOPIC"
	QuestionGateBudgetExhausted          QuestionGateReason = "BUDGET_EXHAUSTED"
	QuestionGateDigestFull               QuestionGateReason = "DIGEST_FULL"
)

// QuestionGatePolicy is the versioned interruption policy evaluated before any
// outbox write. Zero Max* values disable the corresponding limit. Digest and
// Reminder are optional post-MVP controls that remain pure and deterministic.
type QuestionGatePolicy struct {
	MinPriority                   uint8
	MaxPending                    int
	MaxDeliveredPerWindow         int
	MaxAdmittedPerWindow          int
	Window                        time.Duration
	Cooldown                      time.Duration
	TopicCooldown                 time.Duration
	QuietStartHour                int
	QuietEndHour                  int
	UrgentPriority                uint8
	MinAlternativesTried          int
	SuppressSafeReversibleDefault bool
	Digest                        domain.DigestPolicy
	Reminder                      domain.ReminderPolicy
}

func (p QuestionGatePolicy) Validate() error {
	if p.MinPriority == 0 || p.UrgentPriority == 0 || p.UrgentPriority < p.MinPriority {
		return errors.New("question gate priorities must be positive and urgent must not be lower than minimum")
	}
	if p.MaxPending < 0 || p.MaxDeliveredPerWindow < 0 || p.MaxAdmittedPerWindow < 0 || p.MinAlternativesTried < 0 || p.Window < 0 || p.Cooldown < 0 || p.TopicCooldown < 0 {
		return errors.New("question gate limits and durations must not be negative")
	}
	if (p.MaxDeliveredPerWindow > 0 || p.MaxAdmittedPerWindow > 0) && p.Window == 0 {
		return errors.New("question gate rate/admission limits require a positive window")
	}
	if (p.QuietStartHour < 0 || p.QuietStartHour > 23) || (p.QuietEndHour < 0 || p.QuietEndHour > 23) {
		return errors.New("question gate quiet hours must be between 0 and 23")
	}
	if err := p.Digest.Validate(); err != nil {
		return err
	}
	if err := p.Reminder.Validate(); err != nil {
		return err
	}
	return nil
}

// InterruptionBudget projects the versioned budget controls of this policy for
// audit surfaces without exposing kernel-only fields.
func (p QuestionGatePolicy) InterruptionBudget(policyVersion string) domain.InterruptionBudgetPolicy {
	return domain.InterruptionBudgetPolicy{
		PolicyVersion:         policyVersion,
		MaxPending:            p.MaxPending,
		MaxAdmittedPerWindow:  p.MaxAdmittedPerWindow,
		MaxDeliveredPerWindow: p.MaxDeliveredPerWindow,
		Window:                p.Window,
	}
}

type QuestionGateRecord struct {
	QuestionID     domain.OperatorQuestionID
	MissionID      domain.MissionID
	DedupSignature string
	Status         domain.OperatorQuestionStatus
	DeliveredAt    time.Time
	ClosedAt       time.Time
	// AdmittedAt is the first durable observation of an admitted question.
	// When zero, Created-equivalent delivery time is used for admission budget.
	AdmittedAt time.Time
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
	Decision          QuestionGateDecision
	Reason            QuestionGateReason
	RetryAfter        time.Time
	DeliveryAvailable time.Time
	DigestHeld        bool
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
	signature := domain.NormalizeDedupSignature(question.DedupSignature)
	topic := domain.SemanticTopicKey(question.DedupSignature)
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
	windowAdmitted := 0
	heldInDigest := 0
	var cooldownUntil time.Time
	var topicCooldownUntil time.Time
	for _, record := range history {
		if record.MissionID != question.MissionID {
			continue
		}
		recordSignature := domain.NormalizeDedupSignature(record.DedupSignature)
		recordTopic := domain.SemanticTopicKey(record.DedupSignature)
		if record.Status == domain.OperatorQuestionPending {
			pending++
			if recordSignature == signature {
				return QuestionGateResult{Decision: QuestionSuppress, Reason: QuestionGateDuplicatePending}, nil
			}
			if policy.TopicCooldown > 0 && recordTopic != "" && recordTopic == topic {
				return QuestionGateResult{Decision: QuestionSuppress, Reason: QuestionGateDuplicateTopic}, nil
			}
			if policy.Digest.Enabled() && question.Priority < policy.Digest.MinPriorityImmediate {
				// Count pending items that have not yet reached their first
				// delivery; they occupy the current digest hold window.
				if record.DeliveredAt.IsZero() {
					heldInDigest++
				}
			}
		}
		if policy.MaxDeliveredPerWindow > 0 && !record.DeliveredAt.IsZero() && !record.DeliveredAt.Before(windowStart) && !record.DeliveredAt.After(now) {
			windowDelivered++
		}
		admittedAt := record.AdmittedAt
		if admittedAt.IsZero() {
			// Fall back to delivery or close observation for older records.
			admittedAt = record.DeliveredAt
			if admittedAt.IsZero() {
				admittedAt = record.ClosedAt
			}
		}
		if policy.MaxAdmittedPerWindow > 0 && !admittedAt.IsZero() && !admittedAt.Before(windowStart) && !admittedAt.After(now) {
			windowAdmitted++
		}
		if recordSignature == signature {
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
		if policy.TopicCooldown > 0 && recordTopic != "" && recordTopic == topic {
			base := record.DeliveredAt
			if !record.ClosedAt.IsZero() {
				base = record.ClosedAt
			}
			if !base.IsZero() {
				candidate := base.Add(policy.TopicCooldown)
				if candidate.After(topicCooldownUntil) {
					topicCooldownUntil = candidate
				}
			}
		}
	}
	if policy.MaxPending > 0 && pending >= policy.MaxPending {
		return QuestionGateResult{Decision: QuestionDefer, Reason: QuestionGatePendingLimit}, nil
	}
	if policy.MaxAdmittedPerWindow > 0 && windowAdmitted >= policy.MaxAdmittedPerWindow {
		return QuestionGateResult{
			Decision:   QuestionDefer,
			Reason:     QuestionGateBudgetExhausted,
			RetryAfter: oldestAdmissionExpiry(now, policy.Window, question.MissionID, history),
		}, nil
	}
	if policy.MaxDeliveredPerWindow > 0 && windowDelivered >= policy.MaxDeliveredPerWindow {
		return QuestionGateResult{Decision: QuestionDefer, Reason: QuestionGateRateLimit, RetryAfter: oldestWindowExpiry(now, policy.Window, question.MissionID, history)}, nil
	}
	if cooldownUntil.After(now) {
		return QuestionGateResult{Decision: QuestionDefer, Reason: QuestionGateCooldown, RetryAfter: cooldownUntil}, nil
	}
	if topicCooldownUntil.After(now) {
		return QuestionGateResult{Decision: QuestionDefer, Reason: QuestionGateDuplicateTopic, RetryAfter: topicCooldownUntil}, nil
	}
	if policy.Digest.Enabled() && question.Priority < policy.Digest.MinPriorityImmediate {
		if policy.Digest.MaxItems > 0 && heldInDigest >= policy.Digest.MaxItems {
			return QuestionGateResult{
				Decision:   QuestionDefer,
				Reason:     QuestionGateDigestFull,
				RetryAfter: policy.Digest.NextDigestAvailable(now),
			}, nil
		}
		available := policy.Digest.NextDigestAvailable(now)
		return QuestionGateResult{
			Decision:          QuestionAdmit,
			Reason:            QuestionGateAllowed,
			DeliveryAvailable: available,
			DigestHeld:        true,
		}, nil
	}
	return QuestionGateResult{Decision: QuestionAdmit, Reason: QuestionGateAllowed, DeliveryAvailable: now}, nil
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

func oldestAdmissionExpiry(now time.Time, window time.Duration, missionID domain.MissionID, history []QuestionGateRecord) time.Time {
	admissions := make([]time.Time, 0, len(history))
	start := now.Add(-window)
	for _, record := range history {
		if record.MissionID != missionID {
			continue
		}
		admittedAt := record.AdmittedAt
		if admittedAt.IsZero() {
			admittedAt = record.DeliveredAt
			if admittedAt.IsZero() {
				admittedAt = record.ClosedAt
			}
		}
		if !admittedAt.IsZero() && !admittedAt.Before(start) && !admittedAt.After(now) {
			admissions = append(admissions, admittedAt)
		}
	}
	if len(admissions) == 0 {
		return time.Time{}
	}
	sort.Slice(admissions, func(i, j int) bool { return admissions[i].Before(admissions[j]) })
	return admissions[0].Add(window)
}
