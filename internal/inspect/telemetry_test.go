package inspect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/observability"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestTelemetryAndAlertsHTTP(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "alerts", Purpose: "alerts test", Status: domain.MissionActive,
		Provenance: "fixture", AcceptedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		return tx.ActivateMissionRevision(mission.MissionID, mission.ID)
	}); err != nil {
		t.Fatal(err)
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }
	projector.SetTelemetry(true, false, observability.ExportRetention{
		TraceMaxQueueSize: 256,
		MetricInterval:    10 * time.Second,
	})

	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	// Telemetry endpoint.
	telResp, err := http.Get(server.URL + "/telemetry")
	if err != nil {
		t.Fatal(err)
	}
	defer telResp.Body.Close()
	if telResp.StatusCode != http.StatusOK {
		t.Fatalf("telemetry status = %d", telResp.StatusCode)
	}
	var tel inspect.TelemetryStatus
	if err := json.NewDecoder(telResp.Body).Decode(&tel); err != nil {
		t.Fatal(err)
	}
	if !tel.Enabled || tel.HasOTLP || tel.Canonical {
		t.Fatalf("telemetry = %#v", tel)
	}
	if tel.Retention.TraceMaxQueueSize != 256 || tel.Retention.Canonical {
		t.Fatalf("retention = %#v", tel.Retention)
	}

	// Alerts endpoint includes telemetry posture + process signals.
	alResp, err := http.Get(server.URL + "/alerts?mission_id=mission_1")
	if err != nil {
		t.Fatal(err)
	}
	defer alResp.Body.Close()
	if alResp.StatusCode != http.StatusOK {
		t.Fatalf("alerts status = %d", alResp.StatusCode)
	}
	var snap observability.AlertSnapshot
	if err := json.NewDecoder(alResp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if snap.Canonical || snap.Total < 1 {
		t.Fatalf("alerts = %#v", snap)
	}
	foundNoOTLP := false
	for _, a := range snap.Alerts {
		if a.Code == observability.AlertCodeTelemetryNoOTLP {
			foundNoOTLP = true
		}
		if a.Canonical {
			t.Fatalf("alert canonical: %#v", a)
		}
	}
	if !foundNoOTLP {
		t.Fatalf("expected enabled_no_otlp alert: %#v", snap.Alerts)
	}

	// Overview embeds telemetry + alerts.
	ovResp, err := http.Get(server.URL + "/overview?mission_id=mission_1")
	if err != nil {
		t.Fatal(err)
	}
	defer ovResp.Body.Close()
	var overview inspect.Overview
	if err := json.NewDecoder(ovResp.Body).Decode(&overview); err != nil {
		t.Fatal(err)
	}
	if overview.Telemetry == nil || !overview.Telemetry.Enabled {
		t.Fatalf("overview telemetry = %#v", overview.Telemetry)
	}
	if overview.Alerts == nil || overview.Alerts.Total < 1 {
		t.Fatalf("overview alerts = %#v", overview.Alerts)
	}

	// Health carries telemetry posture.
	hResp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer hResp.Body.Close()
	var health inspect.Health
	if err := json.NewDecoder(hResp.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health.Telemetry == nil || health.Telemetry.Retention.TraceMaxQueueSize != 256 {
		t.Fatalf("health telemetry = %#v", health.Telemetry)
	}
}

func TestUnsettledReceiptAlertSurfaces(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 28, 16, 0, 0, 0, time.UTC)
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "unsettled receipt alert", Purpose: "test", Status: domain.MissionActive,
		Provenance: "fixture", AcceptedAt: now,
	}

	// Build a valid receipt that will remain unsettled.
	result := domain.ModelCompletionResult{
		Text: "{}", InputTokens: 100, OutputTokens: 50, Model: "test/model", FinishReason: "stop",
	}
	hash, err := result.Hash()
	if err != nil {
		t.Fatal(err)
	}
	receipt := domain.ModelCompletionReceipt{
		SchemaVersion: domain.SchemaVersionV1,
		OperationID:   "op_1",
		Attempt:       1,
		ModelCall:     1,
		Result:        result,
		PayloadHash:   hash,
		RecordedAt:    now.Add(-10 * time.Minute), // 10 minutes ago
		Permits: []domain.ResourcePermit{{
			Resource:  "binding:test",
			Cost:      domain.ResourceCost{Slots: 1, Tokens: 150},
			GrantedAt: now.Add(-10 * time.Minute),
		}},
	}

	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.ActivateMissionRevision(mission.MissionID, mission.ID); err != nil {
			return err
		}
		return tx.AppendModelCompletionReceipt(receipt)
	}); err != nil {
		t.Fatal(err)
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }

	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	alResp, err := http.Get(server.URL + "/alerts?mission_id=mission_1")
	if err != nil {
		t.Fatal(err)
	}
	defer alResp.Body.Close()
	if alResp.StatusCode != http.StatusOK {
		t.Fatalf("alerts status = %d", alResp.StatusCode)
	}
	var snap observability.AlertSnapshot
	if err := json.NewDecoder(alResp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}

	var found *observability.Alert
	for i, a := range snap.Alerts {
		if a.Code == observability.AlertCodeUnsettledReceipts {
			found = &snap.Alerts[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected unsettled receipt alert; got codes: %v", snap.Alerts)
	}
	if found.Severity != observability.AlertSeverityWarning {
		t.Fatalf("expected warning severity for 10m-old receipt, got %s", found.Severity)
	}
	if found.Canonical {
		t.Fatalf("unsettled receipt alert must not be canonical")
	}
}
