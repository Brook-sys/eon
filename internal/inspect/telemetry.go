package inspect

import (
	"context"
	"errors"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/observability"
	"motor-autonomo/internal/port"
)

// TelemetryStatus is process-local, non-authoritative export posture for inspect.
type TelemetryStatus struct {
	Enabled   bool                        `json:"enabled"`
	HasOTLP   bool                        `json:"has_otlp"`
	Canonical bool                        `json:"canonical"` // always false
	Retention observability.RetentionView `json:"retention"`
}

// SetTelemetry installs disposable telemetry posture on the projector.
// Never used by kernel; inspect/dashboard only.
func (p *Projector) SetTelemetry(enabled, hasOTLP bool, retention observability.ExportRetention) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	view := retention.View()
	p.telemetry = &TelemetryStatus{
		Enabled:   enabled,
		HasOTLP:   hasOTLP,
		Canonical: false,
		Retention: view,
	}
}

// ClearTelemetry removes telemetry posture (tests / disabled assembly).
func (p *Projector) ClearTelemetry() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.telemetry = nil
}

// Telemetry returns a copy of process telemetry posture when configured.
func (p *Projector) Telemetry() (TelemetryStatus, bool) {
	if p == nil {
		return TelemetryStatus{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.telemetry == nil {
		return TelemetryStatus{}, false
	}
	out := *p.telemetry
	return out, true
}

// BuildAlerts derives presentation alerts from health + overview (read-only).
func (p *Projector) BuildAlerts(ctx context.Context, missionID domain.MissionID) (observability.AlertSnapshot, error) {
	now := p.Clock().UTC()
	in := observability.AlertInput{ObservedAt: now, StoreReachable: true}

	if tel, ok := p.Telemetry(); ok {
		in.TelemetryEnabled = tel.Enabled
		in.TelemetryHasOTLP = tel.HasOTLP
	}

	health, err := p.HealthProbe(ctx)
	if err != nil {
		// HealthProbe returns degraded payload + error on store failure.
		in.StoreReachable = false
		in.ProcessMode = string(health.ProcessMode)
		return observability.EvaluateAlerts(in), nil
	}
	in.StoreReachable = health.StoreReachable
	in.ProcessMode = string(health.ProcessMode)

	overview, err := p.BuildOverview(ctx, missionID)
	if err != nil {
		// Fall back to process-only alerts when mission is missing.
		if errors.Is(err, port.ErrNotFound) {
			return observability.EvaluateAlerts(in), nil
		}
		return observability.AlertSnapshot{}, err
	}
	in.PendingCommands = overview.PendingCommands
	in.PendingQuestions = overview.PendingQuestions
	if overview.Mission != nil {
		m := overview.Mission
		if m.Horizon != nil {
			in.HorizonPresent = true
			in.HorizonReady = m.Horizon.ReadyCount
			in.HorizonLowWatermark = m.Horizon.LowWatermark
		}
		if m.Frontier != nil {
			in.FrontierNeedsHygiene = m.Frontier.NeedsHygiene
		}
		if m.LatestDiagnosis != nil {
			in.ContinuityBlocked = true
			in.ContinuityBlockedDetail = m.LatestDiagnosis.SafeDetail
		}
		if m.ContinuityFindings != nil && m.ContinuityFindings.Latest != nil && m.ContinuityFindings.Latest.Stale {
			in.ContinuityFindingsStale = true
		}
	}
	return observability.EvaluateAlerts(in), nil
}

func (p *Projector) defaultRetentionView() observability.RetentionView {
	return observability.ExportRetention{}.View()
}
