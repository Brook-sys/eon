package observability

import (
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
)

// Alert severity for derived process signals. Never drives kernel decisions.
const (
	AlertSeverityInfo     = "info"
	AlertSeverityWarning  = "warning"
	AlertSeverityCritical = "critical"
)

// Alert codes are stable, machine-readable identifiers for UI/inspect filters.
const (
	AlertCodeTelemetryDisabled       = "telemetry.disabled"
	AlertCodeTelemetryNoOTLP         = "telemetry.enabled_no_otlp"
	AlertCodeTelemetryActive         = "telemetry.export_active"
	AlertCodeProcessStopping         = "process.stopping"
	AlertCodeProcessStopped          = "process.stopped"
	AlertCodeStoreUnreachable        = "store.unreachable"
	AlertCodePendingCommandsHigh     = "control.pending_commands_high"
	AlertCodePendingQuestionsHigh    = "control.pending_operator_questions_high"
	AlertCodeHorizonNeedsReplenish   = "horizon.needs_replenish"
	AlertCodeFrontierNeedsHygiene    = "frontier.needs_hygiene"
	AlertCodeContinuityBlocked       = "continuity.blocked"
	AlertCodeContinuityFindingsStale = "continuity.findings_stale"
	AlertCodeEventHeadGrowth         = "store.event_head_growth"
	AlertCodeStaleArtifactsHigh      = "store.stale_artifacts_high"
)

// Default soft thresholds for control backlog pressure (presentation only).
const (
	DefaultPendingCommandsWarn  = 50
	DefaultPendingQuestionsWarn = 20
)

// Alert is a derived, non-authoritative operator signal.
// Secrets, prompts, and free-form model bodies must never appear in Detail.
type Alert struct {
	Code       string    `json:"code"`
	Severity   string    `json:"severity"`
	Summary    string    `json:"summary"`
	Detail     string    `json:"detail,omitempty"`
	Canonical  bool      `json:"canonical"` // always false
	ObservedAt time.Time `json:"observed_at"`
}

// AlertSnapshot is the inspect/dashboard projection of current derived alerts.
type AlertSnapshot struct {
	SchemaVersion int       `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Canonical     bool      `json:"canonical"` // always false
	Total         int       `json:"total"`
	Warnings      int       `json:"warnings"`
	Critical      int       `json:"critical"`
	Alerts        []Alert   `json:"alerts"`
}

// AlertInput is the pure input bag for EvaluateAlerts (no store I/O).
// Callers assemble this from health/overview projections.
type AlertInput struct {
	ObservedAt time.Time

	TelemetryEnabled bool
	TelemetryHasOTLP bool

	StoreReachable bool
	ProcessMode    string // domain.ProcessMode string form

	PendingCommands  int
	PendingQuestions int

	// HorizonReady/HorizonLowWatermark: when both >0 and ready <= low, warn.
	HorizonReady        int
	HorizonLowWatermark int
	HorizonPresent      bool

	FrontierNeedsHygiene    bool
	ContinuityBlocked       bool
	ContinuityBlockedDetail string
	ContinuityFindingsStale bool

	// Soft store growth signals (append-only log / derived stale views).
	// Never authorize GC; presentation and operator hygiene only.
	EventHeadSequence  uint64
	StaleArtifactCount int
	// Optional override; zero values use domain.DefaultStoreRetentionPolicy.
	StoreRetention domain.StoreRetentionPolicy
}

// EvaluateAlerts derives a stable, sorted alert list from pure inputs.
// Deterministic for the same input (stable code order).
func EvaluateAlerts(in AlertInput) AlertSnapshot {
	now := in.ObservedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	out := AlertSnapshot{
		SchemaVersion: 1,
		GeneratedAt:   now,
		Canonical:     false,
		Alerts:        make([]Alert, 0, 8),
	}

	// Telemetry posture (info only — never critical).
	if !in.TelemetryEnabled {
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeTelemetryDisabled, Severity: AlertSeverityInfo,
			Summary:   "derived telemetry export is disabled",
			Detail:    "optional OTel path is off; kernel and store are unaffected",
			Canonical: false, ObservedAt: now,
		})
	} else if !in.TelemetryHasOTLP {
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeTelemetryNoOTLP, Severity: AlertSeverityInfo,
			Summary:   "telemetry enabled without OTLP endpoint",
			Detail:    "spans/metrics stay in-process until an OTLP exporter is configured",
			Canonical: false, ObservedAt: now,
		})
	} else {
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeTelemetryActive, Severity: AlertSeverityInfo,
			Summary:   "OTLP export is configured",
			Detail:    "export buffers are disposable; never authoritative for kernel decisions",
			Canonical: false, ObservedAt: now,
		})
	}

	if !in.StoreReachable {
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeStoreUnreachable, Severity: AlertSeverityCritical,
			Summary:   "store is unreachable from inspect health probe",
			Detail:    "control API projections may be incomplete until the store recovers",
			Canonical: false, ObservedAt: now,
		})
	}

	mode := strings.ToUpper(strings.TrimSpace(in.ProcessMode))
	switch mode {
	case "STOPPING":
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeProcessStopping, Severity: AlertSeverityWarning,
			Summary:   "process is stopping",
			Detail:    "new work should drain; operators may wait for STOPPED",
			Canonical: false, ObservedAt: now,
		})
	case "STOPPED":
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeProcessStopped, Severity: AlertSeverityWarning,
			Summary:   "process is stopped",
			Detail:    "control loop is not advancing; restart or resume per ops policy",
			Canonical: false, ObservedAt: now,
		})
	}

	if in.PendingCommands >= DefaultPendingCommandsWarn {
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodePendingCommandsHigh, Severity: AlertSeverityWarning,
			Summary:   "pending operator commands backlog is elevated",
			Detail:    "count exceeds soft presentation threshold; not a kernel admission decision",
			Canonical: false, ObservedAt: now,
		})
	}
	if in.PendingQuestions >= DefaultPendingQuestionsWarn {
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodePendingQuestionsHigh, Severity: AlertSeverityWarning,
			Summary:   "pending operator questions backlog is elevated",
			Detail:    "count exceeds soft presentation threshold; answer via control API",
			Canonical: false, ObservedAt: now,
		})
	}

	if in.HorizonPresent && in.HorizonLowWatermark > 0 && in.HorizonReady <= in.HorizonLowWatermark {
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeHorizonNeedsReplenish, Severity: AlertSeverityWarning,
			Summary:   "executable horizon ready count is at or below low watermark",
			Detail:    "replenishment is a continuity strategy concern; this alert is derived only",
			Canonical: false, ObservedAt: now,
		})
	}
	if in.FrontierNeedsHygiene {
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeFrontierNeedsHygiene, Severity: AlertSeverityWarning,
			Summary:   "frontier reservoir needs hygiene",
			Detail:    "dry-run PlanFrontierReservoirHygiene would emit actions; apply via frontier_management family",
			Canonical: false, ObservedAt: now,
		})
	}
	if in.ContinuityBlocked {
		detail := strings.TrimSpace(in.ContinuityBlockedDetail)
		if len(detail) > 160 {
			detail = detail[:160]
		}
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeContinuityBlocked, Severity: AlertSeverityWarning,
			Summary:   "latest continuity diagnosis reports blocked progress",
			Detail:    detail,
			Canonical: false, ObservedAt: now,
		})
	}
	if in.ContinuityFindingsStale {
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeContinuityFindingsStale, Severity: AlertSeverityInfo,
			Summary:   "latest continuity audit findings are marked stale",
			Detail:    "re-run continuity audit families for a fresh artifact",
			Canonical: false, ObservedAt: now,
		})
	}

	// Soft append-only growth / derived-stale pressure (never GC triggers).
	policy := in.StoreRetention.Normalize()
	if pressure := policy.EventHeadPressure(in.EventHeadSequence); pressure != "" {
		sev := AlertSeverityInfo
		if pressure == "warn" {
			sev = AlertSeverityWarning
		}
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeEventHeadGrowth, Severity: sev,
			Summary:   "append-only event log head sequence is elevated",
			Detail:    fmt.Sprintf("head_sequence=%d policy=%s; prune is not authorized — use backup/export", in.EventHeadSequence, policy.Version),
			Canonical: false, ObservedAt: now,
		})
	}
	if pressure := policy.StaleArtifactPressure(in.StaleArtifactCount); pressure != "" {
		sev := AlertSeverityInfo
		if pressure == "warn" {
			sev = AlertSeverityWarning
		}
		out.Alerts = append(out.Alerts, Alert{
			Code: AlertCodeStaleArtifactsHigh, Severity: sev,
			Summary:   "stale derived knowledge artifacts exceed soft threshold",
			Detail:    fmt.Sprintf("stale_count=%d; authorized action is refresh via artifact_refresh / FR-KNOW-005 cascade", in.StaleArtifactCount),
			Canonical: false, ObservedAt: now,
		})
	}

	out.Total = len(out.Alerts)
	for _, a := range out.Alerts {
		switch a.Severity {
		case AlertSeverityWarning:
			out.Warnings++
		case AlertSeverityCritical:
			out.Critical++
		}
	}
	return out
}
