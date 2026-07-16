package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// NormalizeDedupSignature collapses insignificant whitespace and casing so that
// paraphrases that share a declared signature form are comparable without
// invoking a model. Callers still own the signature vocabulary. Spaces around
// ':' separators are removed so "Topic: Detail" and "topic:detail" match.
func NormalizeDedupSignature(signature string) string {
	fields := strings.FieldsFunc(strings.TrimSpace(signature), func(r rune) bool {
		return unicode.IsSpace(r)
	})
	if len(fields) == 0 {
		return ""
	}
	joined := strings.ToLower(strings.Join(fields, " "))
	// Collapse " : " / ": " / " :" into a single colon without spaces.
	for strings.Contains(joined, " :") || strings.Contains(joined, ": ") {
		joined = strings.ReplaceAll(joined, " :", ":")
		joined = strings.ReplaceAll(joined, ": ", ":")
	}
	return joined
}

// SemanticTopicKey extracts the stable topic segment of a dedup signature.
// The vocabulary is "topic:detail"; when no separator is present the whole
// normalized signature is the topic. Empty input yields an empty topic.
func SemanticTopicKey(signature string) string {
	normalized := NormalizeDedupSignature(signature)
	if normalized == "" {
		return ""
	}
	if i := strings.IndexByte(normalized, ':'); i >= 0 {
		topic := strings.TrimSpace(normalized[:i])
		if topic != "" {
			return topic
		}
	}
	return normalized
}

// InterruptionBudgetPolicy is a versioned human-attention budget. Zero limits
// disable the corresponding check. PolicyVersion identifies the exact config
// that produced gate decisions for audit and safe rollout.
type InterruptionBudgetPolicy struct {
	PolicyVersion         string        `json:"policy_version"`
	MaxPending            int           `json:"max_pending"`
	MaxAdmittedPerWindow  int           `json:"max_admitted_per_window"`
	MaxDeliveredPerWindow int           `json:"max_delivered_per_window"`
	Window                time.Duration `json:"window"`
}

func (p InterruptionBudgetPolicy) Validate() error {
	if strings.TrimSpace(p.PolicyVersion) == "" {
		return errors.New("interruption budget requires policy version")
	}
	if len(p.PolicyVersion) > 128 {
		return errors.New("interruption budget policy version exceeds byte limit")
	}
	if p.MaxPending < 0 || p.MaxAdmittedPerWindow < 0 || p.MaxDeliveredPerWindow < 0 || p.Window < 0 {
		return errors.New("interruption budget limits and window must not be negative")
	}
	if (p.MaxAdmittedPerWindow > 0 || p.MaxDeliveredPerWindow > 0) && p.Window == 0 {
		return errors.New("interruption budget window limits require a positive window")
	}
	return nil
}

// DigestPolicy groups non-urgent admissions into delayed outbox availability.
// Disabled when Hold is zero. MaxItems bounds how many held questions share a
// digest slot before additional proposals are deferred.
type DigestPolicy struct {
	Hold                  time.Duration `json:"hold"`
	MaxItems              int           `json:"max_items"`
	MinPriorityImmediate  uint8         `json:"min_priority_immediate"`
	AlignToHoldBoundaries bool          `json:"align_to_hold_boundaries"`
}

func (p DigestPolicy) Enabled() bool { return p.Hold > 0 }

func (p DigestPolicy) Validate() error {
	if p.Hold < 0 || p.MaxItems < 0 {
		return errors.New("digest hold and max items must not be negative")
	}
	if p.Hold > 0 && p.MaxItems == 0 {
		return errors.New("enabled digest requires positive max items")
	}
	return nil
}

// NextDigestAvailable returns when a non-urgent delivery may leave the outbox.
// Alignment uses the hold duration as a fixed bucket relative to Unix epoch so
// multiple admissions in the same window share one availability instant.
func (p DigestPolicy) NextDigestAvailable(now time.Time) time.Time {
	if !p.Enabled() || now.IsZero() {
		return time.Time{}
	}
	if !p.AlignToHoldBoundaries {
		return now.Add(p.Hold)
	}
	hold := p.Hold
	unix := now.UnixNano()
	bucket := int64(hold)
	if bucket <= 0 {
		return now.Add(hold)
	}
	next := ((unix / bucket) + 1) * bucket
	return time.Unix(0, next).UTC().In(now.Location())
}

// ReminderPolicy authorizes re-delivery of unanswered questions. The default
// zero value disables reminders. Reminders cease after MaxReminders, answer,
// expiration, supersession, or cancellation.
type ReminderPolicy struct {
	Enabled    bool          `json:"enabled"`
	MaxCount   uint32        `json:"max_count"`
	FirstAfter time.Duration `json:"first_after"`
	Interval   time.Duration `json:"interval"`
}

func (p ReminderPolicy) Validate() error {
	if !p.Enabled {
		if p.MaxCount != 0 || p.FirstAfter != 0 || p.Interval != 0 {
			return errors.New("disabled reminder policy must not carry schedule fields")
		}
		return nil
	}
	if p.MaxCount == 0 || p.FirstAfter <= 0 || p.Interval <= 0 {
		return errors.New("enabled reminder policy requires positive max count, first delay, and interval")
	}
	return nil
}

// QuestionReminderPlan is a pure scheduling result. Adapters create outbox
// work only when Due is true; the plan never mutates canonical state.
type QuestionReminderPlan struct {
	Due          bool      `json:"due"`
	ReminderIndex uint32   `json:"reminder_index,omitempty"`
	AvailableAt  time.Time `json:"available_at,omitempty"`
	StopReason   string    `json:"stop_reason,omitempty"`
}

// PlanQuestionReminder decides whether an unanswered delivered question may
// receive another reminder. deliveredAt is the first successful delivery;
// priorReminders counts completed reminder deliveries after that first send.
func PlanQuestionReminder(question OperatorQuestion, deliveredAt time.Time, priorReminders uint32, now time.Time, policy ReminderPolicy) (QuestionReminderPlan, error) {
	if err := policy.Validate(); err != nil {
		return QuestionReminderPlan{}, err
	}
	if err := question.Validate(); err != nil {
		return QuestionReminderPlan{}, fmt.Errorf("validate question for reminder: %w", err)
	}
	if now.IsZero() {
		return QuestionReminderPlan{}, errors.New("reminder planning requires current time")
	}
	if !policy.Enabled {
		return QuestionReminderPlan{StopReason: "REMINDERS_DISABLED"}, nil
	}
	if question.Status.Terminal() {
		return QuestionReminderPlan{StopReason: "QUESTION_" + string(question.Status)}, nil
	}
	if deliveredAt.IsZero() || deliveredAt.After(now) {
		return QuestionReminderPlan{StopReason: "NOT_DELIVERED"}, nil
	}
	if !question.ExpiresAt.IsZero() && !now.Before(question.ExpiresAt) {
		return QuestionReminderPlan{StopReason: "QUESTION_EXPIRED"}, nil
	}
	if priorReminders >= policy.MaxCount {
		return QuestionReminderPlan{StopReason: "MAX_REMINDERS"}, nil
	}
	nextIndex := priorReminders + 1
	var available time.Time
	if priorReminders == 0 {
		available = deliveredAt.Add(policy.FirstAfter)
	} else {
		available = deliveredAt.Add(policy.FirstAfter + time.Duration(priorReminders)*policy.Interval)
	}
	if now.Before(available) {
		return QuestionReminderPlan{ReminderIndex: nextIndex, AvailableAt: available}, nil
	}
	return QuestionReminderPlan{Due: true, ReminderIndex: nextIndex, AvailableAt: available}, nil
}

// ReminderDestinationRef derives a stable outbox route for reminder N without
// colliding with the primary delivery route key.
func ReminderDestinationRef(primary string, reminderIndex uint32) string {
	return fmt.Sprintf("%s#reminder:%d", primary, reminderIndex)
}
