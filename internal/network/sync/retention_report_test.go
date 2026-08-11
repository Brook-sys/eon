package peersync

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/storage/memory"
)

func TestRetentionReporter_NilStore(t *testing.T) {
	_, err := NewRetentionReporter(domain.DefaultStoreRetentionPolicy(), nil)
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestRetentionReporter_EmptyStore(t *testing.T) {
	store := memory.New()
	reporter, err := NewRetentionReporter(domain.DefaultStoreRetentionPolicy(), store)
	if err != nil {
		t.Fatal(err)
	}
	report, err := reporter.Report(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.EventHeadSequence != 0 {
		t.Errorf("expected head 0, got %d", report.EventHeadSequence)
	}
	if report.PruneAuthorized {
		t.Error("prune should not be authorized in MVP")
	}
	if report.EventHeadPressure != "" {
		t.Errorf("expected empty pressure at seq 0, got %q", report.EventHeadPressure)
	}
}

func TestRetentionReporter_String(t *testing.T) {
	r := RetentionPressureReport{
		EventHeadSequence: 42,
		EventHeadPressure: "info",
		PruneAuthorized:   false,
	}
	s := r.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
	expected := "event_head_sequence=42 pressure=info prune_authorized=false"
	if s != expected {
		t.Errorf("got %q, want %q", s, expected)
	}
}

func TestRetentionReporter_PruneMVPAlwaysFalse(t *testing.T) {
	store := memory.New()
	// Even if someone constructs a policy with AllowEventLogPrune=true,
	// Normalize() should force it to false.
	policy := domain.StoreRetentionPolicy{AllowEventLogPrune: true}
	reporter, err := NewRetentionReporter(policy, store)
	if err != nil {
		t.Fatal(err)
	}
	report, err := reporter.Report(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.PruneAuthorized {
		t.Error("MVP policy must never authorize prune even if input policy sets it true")
	}
}

func TestTickerAttachRetentionReporter(t *testing.T) {
	store := memory.New()
	reporter, err := NewRetentionReporter(domain.DefaultStoreRetentionPolicy(), store)
	if err != nil {
		t.Fatal(err)
	}

	svc := &mockService{}
	net := &mockNetwork{}
	ticker, err := NewTicker(svc, net, "node-a", "stream-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Before attach: should return zero report without error
	report, err := ticker.LatestRetentionReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.EventHeadSequence != 0 {
		t.Errorf("expected zero report before attach, got seq %d", report.EventHeadSequence)
	}

	ticker.AttachRetentionReporter(reporter)

	report, err = ticker.LatestRetentionReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Memory store at seq 0 - just verify no error and correct authorization
	if report.PruneAuthorized {
		t.Error("prune should not be authorized")
	}
}

func TestTickerTickInvokesRetentionReportPostSync(t *testing.T) {
	store := memory.New()
	reporter, err := NewRetentionReporter(domain.DefaultStoreRetentionPolicy(), store)
	if err != nil {
		t.Fatal(err)
	}

	net := &mockNetwork{
		peers: []domain.PeerRecord{
			{Identity: domain.NodeIdentity{ID: "peer-a"}, Capabilities: []string{Capability}},
		},
	}
	svc := &mockService{}
	ticker, err := NewTicker(svc, net, "node-local", "events", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ticker.AttachRetentionReporter(reporter)

	if err := ticker.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Tick should have called Report internally (no panic, no error propagation)
	// Verify reporter still works after tick
	report, err := ticker.LatestRetentionReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.PruneAuthorized {
		t.Error("prune should not be authorized after tick")
	}
}
