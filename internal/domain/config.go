package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ConfigScope groups versioned operator configuration. Each scope has its own
// monotonic revision sequence and active pointer.
type ConfigScope string

const (
	ConfigScopeRuntime      ConfigScope = "RUNTIME"
	ConfigScopeScheduler    ConfigScope = "SCHEDULER"
	ConfigScopeHorizon      ConfigScope = "HORIZON"
	ConfigScopeInterruption ConfigScope = "INTERRUPTION"
	ConfigScopeChannels     ConfigScope = "CHANNELS"
)

func (s ConfigScope) Valid() bool {
	switch s {
	case ConfigScopeRuntime, ConfigScopeScheduler, ConfigScopeHorizon, ConfigScopeInterruption, ConfigScopeChannels:
		return true
	default:
		return false
	}
}

// ConfigApplicability describes when an accepted revision may take effect.
// IMMUTABLE scopes cannot be applied at runtime; they require a new mission or
// migration path outside this pipeline.
type ConfigApplicability string

const (
	ConfigHot             ConfigApplicability = "HOT"
	ConfigNextCycle       ConfigApplicability = "NEXT_CYCLE"
	ConfigRestartRequired ConfigApplicability = "RESTART_REQUIRED"
	ConfigImmutable       ConfigApplicability = "IMMUTABLE"
)

func (a ConfigApplicability) Valid() bool {
	switch a {
	case ConfigHot, ConfigNextCycle, ConfigRestartRequired, ConfigImmutable:
		return true
	default:
		return false
	}
}

// SecretRef is an indirect credential handle. Raw secret material must never
// appear in config payloads, drafts, events, or receipts.
type SecretRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func (s SecretRef) Validate() error {
	kind := strings.TrimSpace(s.Kind)
	name := strings.TrimSpace(s.Name)
	if kind == "" || name == "" {
		return errors.New("secret reference requires kind and name")
	}
	switch kind {
	case "env", "file", "store":
	default:
		return fmt.Errorf("unknown secret reference kind %q", kind)
	}
	if len(kind) > 32 || len(name) > 256 {
		return errors.New("secret reference exceeds byte limit")
	}
	// Reject values that look like inline secrets rather than names.
	if strings.ContainsAny(name, " \t\r\n") {
		return errors.New("secret reference name must not contain whitespace")
	}
	return nil
}

// RuntimeProcessConfig covers process-level knobs that do not belong to a
// mission revision. Secrets stay referenced, never embedded.
type RuntimeProcessConfig struct {
	Version             string `json:"version"`
	LogLevel            string `json:"log_level"`
	MetricsEnabled      bool   `json:"metrics_enabled"`
	TraceSamplePerMille int    `json:"trace_sample_per_mille"`
}

func (c RuntimeProcessConfig) Validate() error {
	if strings.TrimSpace(c.Version) == "" || len(c.Version) > 128 {
		return errors.New("runtime config version is required and must be bounded")
	}
	switch strings.ToLower(strings.TrimSpace(c.LogLevel)) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("unknown runtime log level %q", c.LogLevel)
	}
	if c.TraceSamplePerMille < 0 || c.TraceSamplePerMille > 1000 {
		return errors.New("runtime trace sample must be between 0 and 1000 per mille")
	}
	return nil
}

// SchedulerCadenceConfig is the durable cadence policy for the control loop.
// Zero MaxCycleDuration disables the soft deadline.
type SchedulerCadenceConfig struct {
	Version          string        `json:"version"`
	MinIdleSleep     time.Duration `json:"min_idle_sleep"`
	MaxIdleSleep     time.Duration `json:"max_idle_sleep"`
	MaxCycleDuration time.Duration `json:"max_cycle_duration"`
	MaxDispatches    int           `json:"max_dispatches_per_cycle"`
}

func (c SchedulerCadenceConfig) Validate() error {
	if strings.TrimSpace(c.Version) == "" || len(c.Version) > 128 {
		return errors.New("scheduler config version is required and must be bounded")
	}
	if c.MinIdleSleep < 0 || c.MaxIdleSleep < 0 || c.MaxCycleDuration < 0 || c.MaxDispatches < 0 {
		return errors.New("scheduler durations and limits must not be negative")
	}
	if c.MaxIdleSleep > 0 && c.MinIdleSleep > c.MaxIdleSleep {
		return errors.New("scheduler min idle sleep must not exceed max idle sleep")
	}
	if c.MaxDispatches == 0 {
		return errors.New("scheduler max dispatches must be positive")
	}
	return nil
}

// DefaultSchedulerCadenceConfig returns conservative MVP cadence for the
// control loop. Zero MaxCycleDuration would disable the soft deadline; the
// default keeps a positive bound so a single cycle cannot monopolize the process.
func DefaultSchedulerCadenceConfig() SchedulerCadenceConfig {
	return SchedulerCadenceConfig{
		Version:          "scheduler.default.v1",
		MinIdleSleep:     50 * time.Millisecond,
		MaxIdleSleep:     time.Second,
		MaxCycleDuration: 30 * time.Second,
		MaxDispatches:    8,
	}
}

// WithinCycleBudget reports whether now is still inside the soft cycle window.
// max <= 0 disables the deadline (always true). Equality at the exact boundary
// counts as exhausted so callers stop starting new work when the budget elapses.
func WithinCycleBudget(started, now time.Time, max time.Duration) bool {
	if max <= 0 {
		return true
	}
	if started.IsZero() || now.IsZero() {
		return true
	}
	return now.Before(started.Add(max))
}

// InterruptionRuntimePolicy is the versioned human-attention configuration that
// projects into the pure QuestionGate evaluation surface. It never embeds
// model authority.
type InterruptionRuntimePolicy struct {
	Version                       string         `json:"version"`
	MinPriority                   uint8          `json:"min_priority"`
	MaxPending                    int            `json:"max_pending"`
	MaxDeliveredPerWindow         int            `json:"max_delivered_per_window"`
	MaxAdmittedPerWindow          int            `json:"max_admitted_per_window"`
	Window                        time.Duration  `json:"window"`
	Cooldown                      time.Duration  `json:"cooldown"`
	TopicCooldown                 time.Duration  `json:"topic_cooldown"`
	QuietStartHour                int            `json:"quiet_start_hour"`
	QuietEndHour                  int            `json:"quiet_end_hour"`
	UrgentPriority                uint8          `json:"urgent_priority"`
	MinAlternativesTried          int            `json:"min_alternatives_tried"`
	SuppressSafeReversibleDefault bool           `json:"suppress_safe_reversible_default"`
	Digest                        DigestPolicy   `json:"digest"`
	Reminder                      ReminderPolicy `json:"reminder"`
}

func (p InterruptionRuntimePolicy) Validate() error {
	if strings.TrimSpace(p.Version) == "" || len(p.Version) > 128 {
		return errors.New("interruption policy version is required and must be bounded")
	}
	if p.MinPriority == 0 || p.UrgentPriority == 0 || p.UrgentPriority < p.MinPriority {
		return errors.New("interruption priorities must be positive and urgent must not be lower than minimum")
	}
	if p.MaxPending < 0 || p.MaxDeliveredPerWindow < 0 || p.MaxAdmittedPerWindow < 0 || p.MinAlternativesTried < 0 || p.Window < 0 || p.Cooldown < 0 || p.TopicCooldown < 0 {
		return errors.New("interruption limits and durations must not be negative")
	}
	if (p.MaxDeliveredPerWindow > 0 || p.MaxAdmittedPerWindow > 0) && p.Window == 0 {
		return errors.New("interruption rate/admission limits require a positive window")
	}
	if p.QuietStartHour < 0 || p.QuietStartHour > 23 || p.QuietEndHour < 0 || p.QuietEndHour > 23 {
		return errors.New("interruption quiet hours must be between 0 and 23")
	}
	if err := p.Digest.Validate(); err != nil {
		return err
	}
	return p.Reminder.Validate()
}

// DefaultInterruptionRuntimePolicy returns conservative MVP interruption marks.
func DefaultInterruptionRuntimePolicy() InterruptionRuntimePolicy {
	return InterruptionRuntimePolicy{
		Version:                       "interruption.v1",
		MinPriority:                   20,
		MaxPending:                    3,
		MaxDeliveredPerWindow:         2,
		MaxAdmittedPerWindow:          4,
		Window:                        time.Hour,
		Cooldown:                      6 * time.Hour,
		TopicCooldown:                 24 * time.Hour,
		QuietStartHour:                23,
		QuietEndHour:                  7,
		UrgentPriority:                90,
		MinAlternativesTried:          1,
		SuppressSafeReversibleDefault: true,
	}
}

// ChannelRouteConfig binds a logical destination to transport settings without
// embedding bot tokens or other secrets.
type ChannelRouteConfig struct {
	Channel         string    `json:"channel"`
	DestinationRef  string    `json:"destination_ref"`
	Enabled         bool      `json:"enabled"`
	Priority        uint8     `json:"priority"`
	CredentialRef   SecretRef `json:"credential_ref"`
	MaxDeliveriesPH int       `json:"max_deliveries_per_hour"`
}

func (c ChannelRouteConfig) Validate() error {
	if strings.TrimSpace(c.Channel) == "" || strings.TrimSpace(c.DestinationRef) == "" {
		return errors.New("channel route requires channel and destination")
	}
	if len(c.Channel) > 64 || len(c.DestinationRef) > 256 {
		return errors.New("channel route fields exceed byte limit")
	}
	if c.MaxDeliveriesPH < 0 {
		return errors.New("channel max deliveries must not be negative")
	}
	return c.CredentialRef.Validate()
}

// ChannelsConfig is the versioned channel surface for dashboard/Telegram.
type ChannelsConfig struct {
	Version string               `json:"version"`
	Routes  []ChannelRouteConfig `json:"routes"`
}

func (c ChannelsConfig) Validate() error {
	if strings.TrimSpace(c.Version) == "" || len(c.Version) > 128 {
		return errors.New("channels config version is required and must be bounded")
	}
	if len(c.Routes) == 0 {
		return errors.New("channels config requires at least one route")
	}
	if len(c.Routes) > 32 {
		return errors.New("channels config has too many routes")
	}
	seen := map[string]struct{}{}
	for _, route := range c.Routes {
		if err := route.Validate(); err != nil {
			return err
		}
		key := route.Channel + "\x00" + route.DestinationRef
		if _, ok := seen[key]; ok {
			return errors.New("channels config has duplicate route")
		}
		seen[key] = struct{}{}
	}
	return nil
}

// ConfigDraftStatus tracks the pure draft lifecycle before application.
type ConfigDraftStatus string

const (
	ConfigDraftOpen      ConfigDraftStatus = "OPEN"
	ConfigDraftValidated ConfigDraftStatus = "VALIDATED"
	ConfigDraftRejected  ConfigDraftStatus = "REJECTED"
	ConfigDraftApplied   ConfigDraftStatus = "APPLIED"
)

func (s ConfigDraftStatus) Valid() bool {
	switch s {
	case ConfigDraftOpen, ConfigDraftValidated, ConfigDraftRejected, ConfigDraftApplied:
		return true
	default:
		return false
	}
}

func (s ConfigDraftStatus) Terminal() bool {
	return s == ConfigDraftRejected || s == ConfigDraftApplied
}

// ConfigDraft is an operator-authored change proposal. Payload fields are
// mutually exclusive by Scope: exactly one concrete body must be set.
type ConfigDraft struct {
	SchemaVersion   int                        `json:"schema_version"`
	ID              ConfigDraftID              `json:"draft_id"`
	Scope           ConfigScope                `json:"scope"`
	BasedOnRevision uint64                     `json:"based_on_revision"`
	Applicability   ConfigApplicability        `json:"applicability"`
	Status          ConfigDraftStatus          `json:"status"`
	ActorType       ActorType                  `json:"actor_type"`
	ActorID         string                     `json:"actor_id"`
	Reason          string                     `json:"reason"`
	Runtime         *RuntimeProcessConfig      `json:"runtime,omitempty"`
	Scheduler       *SchedulerCadenceConfig    `json:"scheduler,omitempty"`
	Horizon         *HorizonPolicy             `json:"horizon,omitempty"`
	Interruption    *InterruptionRuntimePolicy `json:"interruption,omitempty"`
	Channels        *ChannelsConfig            `json:"channels,omitempty"`
	CreatedAt       time.Time                  `json:"created_at"`
	ValidatedAt     time.Time                  `json:"validated_at,omitempty"`
}

func (d ConfigDraft) Validate() error {
	if d.SchemaVersion != SchemaVersionV1 || d.ID == "" || d.ActorID == "" || strings.TrimSpace(d.Reason) == "" || d.CreatedAt.IsZero() {
		return errors.New("config draft is incomplete or has unsupported schema version")
	}
	if !d.Scope.Valid() || !d.Applicability.Valid() || !d.Status.Valid() {
		return errors.New("config draft has invalid scope, applicability, or status")
	}
	if !d.ActorType.valid() {
		return fmt.Errorf("unknown config draft actor type %q", d.ActorType)
	}
	if len(d.ActorID) > 128 || len(d.Reason) > MaxControlPayloadBytes {
		return errors.New("config draft actor or reason exceeds byte limit")
	}
	switch d.Status {
	case ConfigDraftValidated, ConfigDraftApplied:
		if d.ValidatedAt.IsZero() {
			return errors.New("validated or applied config draft requires validation time")
		}
	default:
		if !d.ValidatedAt.IsZero() {
			return errors.New("open or rejected config draft must not claim validation time")
		}
	}
	if d.Applicability == ConfigImmutable {
		return errors.New("immutable config cannot enter the draft/apply pipeline")
	}
	bodies := 0
	if d.Runtime != nil {
		bodies++
		if d.Scope != ConfigScopeRuntime {
			return errors.New("runtime payload requires RUNTIME scope")
		}
		if err := d.Runtime.Validate(); err != nil {
			return err
		}
	}
	if d.Scheduler != nil {
		bodies++
		if d.Scope != ConfigScopeScheduler {
			return errors.New("scheduler payload requires SCHEDULER scope")
		}
		if err := d.Scheduler.Validate(); err != nil {
			return err
		}
	}
	if d.Horizon != nil {
		bodies++
		if d.Scope != ConfigScopeHorizon {
			return errors.New("horizon payload requires HORIZON scope")
		}
		if err := d.Horizon.Validate(); err != nil {
			return err
		}
	}
	if d.Interruption != nil {
		bodies++
		if d.Scope != ConfigScopeInterruption {
			return errors.New("interruption payload requires INTERRUPTION scope")
		}
		if err := d.Interruption.Validate(); err != nil {
			return err
		}
	}
	if d.Channels != nil {
		bodies++
		if d.Scope != ConfigScopeChannels {
			return errors.New("channels payload requires CHANNELS scope")
		}
		if err := d.Channels.Validate(); err != nil {
			return err
		}
	}
	if bodies != 1 {
		return errors.New("config draft requires exactly one payload matching its scope")
	}
	return nil
}

// ConfigRevision is an immutable accepted configuration for one scope.
type ConfigRevision struct {
	SchemaVersion int                        `json:"schema_version"`
	ID            ConfigRevisionID           `json:"revision_id"`
	Scope         ConfigScope                `json:"scope"`
	Revision      uint64                     `json:"revision"`
	Applicability ConfigApplicability        `json:"applicability"`
	ParentID      ConfigRevisionID           `json:"parent_id,omitempty"`
	ContentHash   string                     `json:"content_hash"`
	ActorType     ActorType                  `json:"actor_type"`
	ActorID       string                     `json:"actor_id"`
	Reason        string                     `json:"reason"`
	DraftID       ConfigDraftID              `json:"draft_id"`
	Runtime       *RuntimeProcessConfig      `json:"runtime,omitempty"`
	Scheduler     *SchedulerCadenceConfig    `json:"scheduler,omitempty"`
	Horizon       *HorizonPolicy             `json:"horizon,omitempty"`
	Interruption  *InterruptionRuntimePolicy `json:"interruption,omitempty"`
	Channels      *ChannelsConfig            `json:"channels,omitempty"`
	AcceptedAt    time.Time                  `json:"accepted_at"`
}

func (r ConfigRevision) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 || r.ID == "" || r.Revision == 0 || r.DraftID == "" || r.ActorID == "" || strings.TrimSpace(r.Reason) == "" || r.AcceptedAt.IsZero() || strings.TrimSpace(r.ContentHash) == "" {
		return errors.New("config revision is incomplete or has unsupported schema version")
	}
	if !r.Scope.Valid() || !r.Applicability.Valid() {
		return errors.New("config revision has invalid scope or applicability")
	}
	if r.Applicability == ConfigImmutable {
		return errors.New("immutable config cannot be stored as an applied revision")
	}
	if !r.ActorType.valid() {
		return fmt.Errorf("unknown config revision actor type %q", r.ActorType)
	}
	if len(r.ContentHash) > 128 {
		return errors.New("config revision content hash exceeds byte limit")
	}
	bodies := 0
	if r.Runtime != nil {
		bodies++
		if r.Scope != ConfigScopeRuntime {
			return errors.New("runtime payload requires RUNTIME scope")
		}
		if err := r.Runtime.Validate(); err != nil {
			return err
		}
	}
	if r.Scheduler != nil {
		bodies++
		if r.Scope != ConfigScopeScheduler {
			return errors.New("scheduler payload requires SCHEDULER scope")
		}
		if err := r.Scheduler.Validate(); err != nil {
			return err
		}
	}
	if r.Horizon != nil {
		bodies++
		if r.Scope != ConfigScopeHorizon {
			return errors.New("horizon payload requires HORIZON scope")
		}
		if err := r.Horizon.Validate(); err != nil {
			return err
		}
	}
	if r.Interruption != nil {
		bodies++
		if r.Scope != ConfigScopeInterruption {
			return errors.New("interruption payload requires INTERRUPTION scope")
		}
		if err := r.Interruption.Validate(); err != nil {
			return err
		}
	}
	if r.Channels != nil {
		bodies++
		if r.Scope != ConfigScopeChannels {
			return errors.New("channels payload requires CHANNELS scope")
		}
		if err := r.Channels.Validate(); err != nil {
			return err
		}
	}
	if bodies != 1 {
		return errors.New("config revision requires exactly one payload matching its scope")
	}
	hash, err := ConfigPayloadHash(r.Scope, r.Runtime, r.Scheduler, r.Horizon, r.Interruption, r.Channels)
	if err != nil {
		return err
	}
	if r.ContentHash != hash {
		return errors.New("config revision content hash does not match payload")
	}
	return nil
}

// ConfigApplyState distinguishes acceptance of a draft from confirmed apply.
type ConfigApplyState string

const (
	ConfigApplyReceived   ConfigApplyState = "RECEIVED"
	ConfigApplyValidating ConfigApplyState = "VALIDATING"
	ConfigApplyAccepted   ConfigApplyState = "ACCEPTED"
	ConfigApplyRejected   ConfigApplyState = "REJECTED"
	ConfigApplyApplying   ConfigApplyState = "APPLYING"
	ConfigApplyApplied    ConfigApplyState = "APPLIED"
	ConfigApplyFailed     ConfigApplyState = "FAILED"
)

func (s ConfigApplyState) Valid() bool {
	switch s {
	case ConfigApplyReceived, ConfigApplyValidating, ConfigApplyAccepted, ConfigApplyRejected,
		ConfigApplyApplying, ConfigApplyApplied, ConfigApplyFailed:
		return true
	default:
		return false
	}
}

func (s ConfigApplyState) Terminal() bool {
	switch s {
	case ConfigApplyRejected, ConfigApplyApplied, ConfigApplyFailed:
		return true
	default:
		return false
	}
}

// ConfigApplyReceipt records the apply pipeline for one draft.
type ConfigApplyReceipt struct {
	SchemaVersion int              `json:"schema_version"`
	ID            ReceiptID        `json:"receipt_id"`
	DraftID       ConfigDraftID    `json:"draft_id"`
	RevisionID    ConfigRevisionID `json:"revision_id,omitempty"`
	State         ConfigApplyState `json:"state"`
	ResultRef     string           `json:"result_ref,omitempty"`
	FailureCode   string           `json:"failure_code,omitempty"`
	RecordedAt    time.Time        `json:"recorded_at"`
}

func (r ConfigApplyReceipt) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 || r.ID == "" || r.DraftID == "" || r.RecordedAt.IsZero() {
		return errors.New("config apply receipt is incomplete or has unsupported schema version")
	}
	if !r.State.Valid() {
		return fmt.Errorf("unknown config apply state %q", r.State)
	}
	if r.ResultRef != "" && r.FailureCode != "" {
		return errors.New("config apply receipt cannot contain both result and failure")
	}
	switch r.State {
	case ConfigApplyApplied:
		if r.RevisionID == "" || r.ResultRef == "" {
			return errors.New("applied config receipt requires revision and result reference")
		}
	case ConfigApplyRejected, ConfigApplyFailed:
		if r.FailureCode == "" {
			return errors.New("rejected or failed config receipt requires failure code")
		}
		if r.RevisionID != "" {
			return errors.New("rejected or failed config receipt must not claim a revision")
		}
	default:
		if r.ResultRef != "" || r.FailureCode != "" || r.RevisionID != "" {
			return errors.New("non-terminal config receipt must not claim result, failure, or revision")
		}
	}
	return nil
}

// ConfigFieldChange is one path in a deterministic config diff.
type ConfigFieldChange struct {
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Secret bool   `json:"secret,omitempty"`
}

// ConfigDiff is a sorted, deterministic field-level comparison.
type ConfigDiff struct {
	Scope        ConfigScope         `json:"scope"`
	BaseRevision uint64              `json:"base_revision"`
	Changes      []ConfigFieldChange `json:"changes"`
	Empty        bool                `json:"empty"`
}

func (d ConfigDiff) Validate() error {
	if !d.Scope.Valid() {
		return errors.New("config diff has invalid scope")
	}
	if d.Empty && len(d.Changes) != 0 {
		return errors.New("empty config diff must not list changes")
	}
	if !d.Empty && len(d.Changes) == 0 {
		return errors.New("non-empty config diff requires changes")
	}
	for i := 1; i < len(d.Changes); i++ {
		if d.Changes[i-1].Path >= d.Changes[i].Path {
			return errors.New("config diff changes must be strictly sorted by path")
		}
	}
	return nil
}

// ConfigImpactPreview is the pure impact analysis before apply.
type ConfigImpactPreview struct {
	Scope           ConfigScope         `json:"scope"`
	Applicability   ConfigApplicability `json:"applicability"`
	FieldsChanged   []string            `json:"fields_changed"`
	RestartRequired bool                `json:"restart_required"`
	NextCycleOnly   bool                `json:"next_cycle_only"`
	Blocked         bool                `json:"blocked"`
	Notes           []string            `json:"notes,omitempty"`
}

func (p ConfigImpactPreview) Validate() error {
	if !p.Scope.Valid() || !p.Applicability.Valid() {
		return errors.New("config impact has invalid scope or applicability")
	}
	if p.RestartRequired && p.Applicability != ConfigRestartRequired {
		return errors.New("restart required impact requires RESTART_REQUIRED applicability")
	}
	if p.NextCycleOnly && p.Applicability != ConfigNextCycle {
		return errors.New("next-cycle impact requires NEXT_CYCLE applicability")
	}
	return nil
}

// ConfigPayloadHash produces a stable content hash for one scoped payload.
func ConfigPayloadHash(scope ConfigScope, runtime *RuntimeProcessConfig, scheduler *SchedulerCadenceConfig, horizon *HorizonPolicy, interruption *InterruptionRuntimePolicy, channels *ChannelsConfig) (string, error) {
	body := struct {
		Scope        ConfigScope                `json:"scope"`
		Runtime      *RuntimeProcessConfig      `json:"runtime,omitempty"`
		Scheduler    *SchedulerCadenceConfig    `json:"scheduler,omitempty"`
		Horizon      *HorizonPolicy             `json:"horizon,omitempty"`
		Interruption *InterruptionRuntimePolicy `json:"interruption,omitempty"`
		Channels     *ChannelsConfig            `json:"channels,omitempty"`
	}{Scope: scope, Runtime: runtime, Scheduler: scheduler, Horizon: horizon, Interruption: interruption, Channels: channels}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal config payload: %w", err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// DiffConfig compares the active revision (optional) with a draft payload.
func DiffConfig(active *ConfigRevision, draft ConfigDraft) (ConfigDiff, error) {
	if err := draft.Validate(); err != nil {
		return ConfigDiff{}, fmt.Errorf("validate draft for diff: %w", err)
	}
	if active != nil {
		if err := active.Validate(); err != nil {
			return ConfigDiff{}, fmt.Errorf("validate active revision for diff: %w", err)
		}
		if active.Scope != draft.Scope {
			return ConfigDiff{}, errors.New("active config scope disagrees with draft")
		}
	}
	before := map[string]string{}
	after := map[string]string{}
	secret := map[string]bool{}
	if active != nil {
		collectConfigFields(before, secret, active.Scope, active.Runtime, active.Scheduler, active.Horizon, active.Interruption, active.Channels)
	}
	collectConfigFields(after, secret, draft.Scope, draft.Runtime, draft.Scheduler, draft.Horizon, draft.Interruption, draft.Channels)
	paths := map[string]struct{}{}
	for p := range before {
		paths[p] = struct{}{}
	}
	for p := range after {
		paths[p] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for p := range paths {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)
	changes := make([]ConfigFieldChange, 0)
	for _, path := range ordered {
		b, a := before[path], after[path]
		if b == a {
			continue
		}
		change := ConfigFieldChange{Path: path, Before: b, After: a, Secret: secret[path]}
		if change.Secret {
			if b != "" {
				change.Before = "[secret-ref]"
			}
			if a != "" {
				change.After = "[secret-ref]"
			}
		}
		changes = append(changes, change)
	}
	base := uint64(0)
	if active != nil {
		base = active.Revision
	}
	diff := ConfigDiff{Scope: draft.Scope, BaseRevision: base, Changes: changes, Empty: len(changes) == 0}
	if err := diff.Validate(); err != nil {
		return ConfigDiff{}, err
	}
	return diff, nil
}

// PreviewConfigImpact derives applicability consequences from a validated draft
// and its diff. Empty diffs are blocked as no-ops.
func PreviewConfigImpact(draft ConfigDraft, diff ConfigDiff) (ConfigImpactPreview, error) {
	if err := draft.Validate(); err != nil {
		return ConfigImpactPreview{}, err
	}
	if err := diff.Validate(); err != nil {
		return ConfigImpactPreview{}, err
	}
	if draft.Scope != diff.Scope {
		return ConfigImpactPreview{}, errors.New("impact scope disagrees with draft")
	}
	fields := make([]string, 0, len(diff.Changes))
	for _, change := range diff.Changes {
		fields = append(fields, change.Path)
	}
	preview := ConfigImpactPreview{
		Scope:         draft.Scope,
		Applicability: draft.Applicability,
		FieldsChanged: fields,
	}
	switch draft.Applicability {
	case ConfigHot:
		preview.Notes = append(preview.Notes, "applies at next safe read of active config")
	case ConfigNextCycle:
		preview.NextCycleOnly = true
		preview.Notes = append(preview.Notes, "applies at next scheduler cycle boundary")
	case ConfigRestartRequired:
		preview.RestartRequired = true
		preview.Notes = append(preview.Notes, "requires coordinated process restart after apply")
	case ConfigImmutable:
		preview.Blocked = true
		preview.Notes = append(preview.Notes, "immutable configuration cannot be applied")
	}
	if diff.Empty {
		preview.Blocked = true
		preview.Notes = append(preview.Notes, "no-op draft has no field changes")
	}
	if err := preview.Validate(); err != nil {
		return ConfigImpactPreview{}, err
	}
	return preview, nil
}

// DraftFromConfigRevision builds an OPEN draft that re-proposes an existing
// revision payload. Semantic rollback uses this against the current active
// base so the lineage still advances (no pointer rewind).
func DraftFromConfigRevision(source ConfigRevision, draftID ConfigDraftID, basedOn uint64, actorType ActorType, actorID, reason string, now time.Time) (ConfigDraft, error) {
	if err := source.Validate(); err != nil {
		return ConfigDraft{}, err
	}
	if draftID == "" || actorID == "" || strings.TrimSpace(reason) == "" || now.IsZero() {
		return ConfigDraft{}, errors.New("rollback draft requires id, actor, reason, and time")
	}
	if !actorType.valid() {
		return ConfigDraft{}, fmt.Errorf("unknown rollback actor type %q", actorType)
	}
	draft := ConfigDraft{
		SchemaVersion:   SchemaVersionV1,
		ID:              draftID,
		Scope:           source.Scope,
		BasedOnRevision: basedOn,
		Applicability:   source.Applicability,
		Status:          ConfigDraftOpen,
		ActorType:       actorType,
		ActorID:         actorID,
		Reason:          strings.TrimSpace(reason),
		Runtime:         cloneRuntimeConfig(source.Runtime),
		Scheduler:       cloneSchedulerConfig(source.Scheduler),
		Horizon:         cloneHorizonPolicy(source.Horizon),
		Interruption:    cloneInterruptionPolicy(source.Interruption),
		Channels:        cloneChannelsConfig(source.Channels),
		CreatedAt:       now.UTC(),
	}
	if err := draft.Validate(); err != nil {
		return ConfigDraft{}, err
	}
	return draft, nil
}

// ConfigRevisionsEqualPayload reports whether two revisions carry the same
// scoped content hash (identity of config body, not revision metadata).
func ConfigRevisionsEqualPayload(a, b ConfigRevision) bool {
	return a.Scope == b.Scope && a.ContentHash != "" && a.ContentHash == b.ContentHash
}

// MarkConfigDraftValidated is a pure status advance after validation succeeds.
func MarkConfigDraftValidated(draft ConfigDraft, now time.Time) (ConfigDraft, error) {
	if err := draft.Validate(); err != nil {
		return ConfigDraft{}, err
	}
	if draft.Status != ConfigDraftOpen {
		return ConfigDraft{}, fmt.Errorf("%w: only OPEN drafts can be validated", ErrConflict)
	}
	if now.IsZero() {
		return ConfigDraft{}, errors.New("config validation requires time")
	}
	next := draft
	next.Status = ConfigDraftValidated
	next.ValidatedAt = now.UTC()
	if err := next.Validate(); err != nil {
		return ConfigDraft{}, err
	}
	return next, nil
}

// ApplyConfigDraft is the pure transition from a validated draft to an
// immutable revision. Callers enforce optimistic concurrency on BasedOnRevision.
func ApplyConfigDraft(active *ConfigRevision, draft ConfigDraft, revisionID ConfigRevisionID, receiptID ReceiptID, now time.Time) (ConfigRevision, ConfigDraft, ConfigApplyReceipt, error) {
	if err := draft.Validate(); err != nil {
		return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, err
	}
	if draft.Status != ConfigDraftValidated {
		return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, errors.New("only validated drafts may be applied")
	}
	if revisionID == "" || receiptID == "" || now.IsZero() {
		return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, errors.New("apply requires revision id, receipt id, and time")
	}
	if active != nil {
		if err := active.Validate(); err != nil {
			return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, err
		}
		if active.Scope != draft.Scope {
			return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, errors.New("active config scope disagrees with draft")
		}
		if draft.BasedOnRevision != active.Revision {
			return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, fmt.Errorf("%w: stale config base revision", ErrConflict)
		}
	} else if draft.BasedOnRevision != 0 {
		return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, fmt.Errorf("%w: draft expects existing config revision", ErrConflict)
	}

	diff, err := DiffConfig(active, draft)
	if err != nil {
		return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, err
	}
	impact, err := PreviewConfigImpact(draft, diff)
	if err != nil {
		return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, err
	}
	if impact.Blocked {
		return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, errors.New("config apply blocked by impact preview")
	}

	hash, err := ConfigPayloadHash(draft.Scope, draft.Runtime, draft.Scheduler, draft.Horizon, draft.Interruption, draft.Channels)
	if err != nil {
		return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, err
	}
	nextRevision := uint64(1)
	var parent ConfigRevisionID
	if active != nil {
		nextRevision = active.Revision + 1
		parent = active.ID
	}
	revision := ConfigRevision{
		SchemaVersion: SchemaVersionV1,
		ID:            revisionID,
		Scope:         draft.Scope,
		Revision:      nextRevision,
		Applicability: draft.Applicability,
		ParentID:      parent,
		ContentHash:   hash,
		ActorType:     draft.ActorType,
		ActorID:       draft.ActorID,
		Reason:        draft.Reason,
		DraftID:       draft.ID,
		Runtime:       cloneRuntimeConfig(draft.Runtime),
		Scheduler:     cloneSchedulerConfig(draft.Scheduler),
		Horizon:       cloneHorizonPolicy(draft.Horizon),
		Interruption:  cloneInterruptionPolicy(draft.Interruption),
		Channels:      cloneChannelsConfig(draft.Channels),
		AcceptedAt:    now.UTC(),
	}
	if err := revision.Validate(); err != nil {
		return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, err
	}
	appliedDraft := draft
	appliedDraft.Status = ConfigDraftApplied
	// ValidatedAt retained; applied is terminal.
	receipt := ConfigApplyReceipt{
		SchemaVersion: SchemaVersionV1,
		ID:            receiptID,
		DraftID:       draft.ID,
		RevisionID:    revision.ID,
		State:         ConfigApplyApplied,
		ResultRef:     fmt.Sprintf("%s@%d", draft.Scope, revision.Revision),
		RecordedAt:    now.UTC(),
	}
	if err := receipt.Validate(); err != nil {
		return ConfigRevision{}, ConfigDraft{}, ConfigApplyReceipt{}, err
	}
	return revision, appliedDraft, receipt, nil
}

// AdvanceConfigApplyReceipt enforces monotonic receipt transitions.
func AdvanceConfigApplyReceipt(current, next ConfigApplyReceipt) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.ID != next.ID || current.DraftID != next.DraftID {
		return errors.New("config apply receipt identity changed")
	}
	if next.RecordedAt.Before(current.RecordedAt) {
		return errors.New("config apply receipt time must not go backwards")
	}
	if current.State == next.State {
		return fmt.Errorf("%w: config apply receipt changed without state advance", ErrConflict)
	}
	if current.State.Terminal() {
		return fmt.Errorf("%w: terminal config apply receipt cannot advance", ErrConflict)
	}
	legal := map[ConfigApplyState][]ConfigApplyState{
		ConfigApplyReceived:   {ConfigApplyValidating, ConfigApplyRejected},
		ConfigApplyValidating: {ConfigApplyAccepted, ConfigApplyRejected},
		ConfigApplyAccepted:   {ConfigApplyApplying, ConfigApplyRejected},
		ConfigApplyApplying:   {ConfigApplyApplied, ConfigApplyFailed},
	}
	for _, allowed := range legal[current.State] {
		if next.State == allowed {
			return nil
		}
	}
	return fmt.Errorf("%w: illegal config apply receipt transition %s → %s", ErrConflict, current.State, next.State)
}

// DefaultApplicabilityForScope returns the conservative default for each scope.
func DefaultApplicabilityForScope(scope ConfigScope) ConfigApplicability {
	switch scope {
	case ConfigScopeInterruption, ConfigScopeHorizon, ConfigScopeScheduler:
		return ConfigNextCycle
	case ConfigScopeChannels:
		return ConfigHot
	case ConfigScopeRuntime:
		return ConfigRestartRequired
	default:
		return ConfigImmutable
	}
}

func collectConfigFields(dst map[string]string, secret map[string]bool, scope ConfigScope, runtime *RuntimeProcessConfig, scheduler *SchedulerCadenceConfig, horizon *HorizonPolicy, interruption *InterruptionRuntimePolicy, channels *ChannelsConfig) {
	switch scope {
	case ConfigScopeRuntime:
		if runtime == nil {
			return
		}
		dst["runtime.version"] = runtime.Version
		dst["runtime.log_level"] = runtime.LogLevel
		dst["runtime.metrics_enabled"] = fmt.Sprintf("%t", runtime.MetricsEnabled)
		dst["runtime.trace_sample_per_mille"] = fmt.Sprintf("%d", runtime.TraceSamplePerMille)
	case ConfigScopeScheduler:
		if scheduler == nil {
			return
		}
		dst["scheduler.version"] = scheduler.Version
		dst["scheduler.min_idle_sleep"] = scheduler.MinIdleSleep.String()
		dst["scheduler.max_idle_sleep"] = scheduler.MaxIdleSleep.String()
		dst["scheduler.max_cycle_duration"] = scheduler.MaxCycleDuration.String()
		dst["scheduler.max_dispatches_per_cycle"] = fmt.Sprintf("%d", scheduler.MaxDispatches)
	case ConfigScopeHorizon:
		if horizon == nil {
			return
		}
		dst["horizon.version"] = horizon.Version
		dst["horizon.target_ready"] = fmt.Sprintf("%d", horizon.TargetReady)
		dst["horizon.low_watermark"] = fmt.Sprintf("%d", horizon.LowWatermark)
		dst["horizon.max_ready"] = fmt.Sprintf("%d", horizon.MaxReady)
		dst["horizon.max_candidates"] = fmt.Sprintf("%d", horizon.MaxCandidates)
		dst["horizon.max_children"] = fmt.Sprintf("%d", horizon.MaxChildren)
		dst["horizon.max_depth"] = fmt.Sprintf("%d", horizon.MaxDepth)
		dst["horizon.strategy_cooldown"] = horizon.StrategyCooldown.String()
	case ConfigScopeInterruption:
		if interruption == nil {
			return
		}
		dst["interruption.version"] = interruption.Version
		dst["interruption.min_priority"] = fmt.Sprintf("%d", interruption.MinPriority)
		dst["interruption.max_pending"] = fmt.Sprintf("%d", interruption.MaxPending)
		dst["interruption.max_delivered_per_window"] = fmt.Sprintf("%d", interruption.MaxDeliveredPerWindow)
		dst["interruption.max_admitted_per_window"] = fmt.Sprintf("%d", interruption.MaxAdmittedPerWindow)
		dst["interruption.window"] = interruption.Window.String()
		dst["interruption.cooldown"] = interruption.Cooldown.String()
		dst["interruption.topic_cooldown"] = interruption.TopicCooldown.String()
		dst["interruption.quiet_start_hour"] = fmt.Sprintf("%d", interruption.QuietStartHour)
		dst["interruption.quiet_end_hour"] = fmt.Sprintf("%d", interruption.QuietEndHour)
		dst["interruption.urgent_priority"] = fmt.Sprintf("%d", interruption.UrgentPriority)
		dst["interruption.min_alternatives_tried"] = fmt.Sprintf("%d", interruption.MinAlternativesTried)
		dst["interruption.suppress_safe_reversible_default"] = fmt.Sprintf("%t", interruption.SuppressSafeReversibleDefault)
		dst["interruption.digest.hold"] = interruption.Digest.Hold.String()
		dst["interruption.digest.max_items"] = fmt.Sprintf("%d", interruption.Digest.MaxItems)
		dst["interruption.digest.min_priority_immediate"] = fmt.Sprintf("%d", interruption.Digest.MinPriorityImmediate)
		dst["interruption.digest.align_to_hold_boundaries"] = fmt.Sprintf("%t", interruption.Digest.AlignToHoldBoundaries)
		dst["interruption.reminder.enabled"] = fmt.Sprintf("%t", interruption.Reminder.Enabled)
		dst["interruption.reminder.max_count"] = fmt.Sprintf("%d", interruption.Reminder.MaxCount)
		dst["interruption.reminder.first_after"] = interruption.Reminder.FirstAfter.String()
		dst["interruption.reminder.interval"] = interruption.Reminder.Interval.String()
	case ConfigScopeChannels:
		if channels == nil {
			return
		}
		dst["channels.version"] = channels.Version
		for i, route := range channels.Routes {
			prefix := fmt.Sprintf("channels.routes[%d]", i)
			dst[prefix+".channel"] = route.Channel
			dst[prefix+".destination_ref"] = route.DestinationRef
			dst[prefix+".enabled"] = fmt.Sprintf("%t", route.Enabled)
			dst[prefix+".priority"] = fmt.Sprintf("%d", route.Priority)
			dst[prefix+".max_deliveries_per_hour"] = fmt.Sprintf("%d", route.MaxDeliveriesPH)
			dst[prefix+".credential_ref"] = route.CredentialRef.Kind + ":" + route.CredentialRef.Name
			secret[prefix+".credential_ref"] = true
		}
	}
}

func cloneRuntimeConfig(v *RuntimeProcessConfig) *RuntimeProcessConfig {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneSchedulerConfig(v *SchedulerCadenceConfig) *SchedulerCadenceConfig {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneHorizonPolicy(v *HorizonPolicy) *HorizonPolicy {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneInterruptionPolicy(v *InterruptionRuntimePolicy) *InterruptionRuntimePolicy {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneChannelsConfig(v *ChannelsConfig) *ChannelsConfig {
	if v == nil {
		return nil
	}
	cp := *v
	cp.Routes = append([]ChannelRouteConfig(nil), v.Routes...)
	return &cp
}
