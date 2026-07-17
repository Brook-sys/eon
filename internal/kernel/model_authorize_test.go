package kernel

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestCompositeReserveModelComplete(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	now := time.Now().UTC()
	clock := source.NewManualClock(now)
	seedModelAgenda(t, store, now)

	auth, _ := NewMVPCapabilityAuthorizer(store, clock, "test-v1")
	auth.Limits[domain.ModelProviderResource("prov1")] = domain.ResourceLimit{Resource: domain.ModelProviderResource("prov1"), MaxConcurrent: 1}
	auth.Limits[domain.ModelBindingResource("bind1")] = domain.ResourceLimit{Resource: domain.ModelBindingResource("bind1"), MaxConcurrent: 1}

	var op domain.Operation
	var spec domain.OperationSpec
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_model")
		if err != nil { return err }
		spec, err = r.OperationSpec(op.SpecID)
		return err
	}); err != nil { t.Fatal(err) }

	out, err := auth.ReserveModelComplete(ctx, op, spec, 0, "prov1", "bind1")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Allowed || len(out.Permits) != 2 {
		t.Fatalf("expected 2 permits, got allowed=%v permits=%d", out.Allowed, len(out.Permits))
	}

	// Ensure throttle on a second reservation without double-counting policy budget.
	out2, _ := auth.ReserveModelComplete(ctx, op, spec, 0, "prov1", "bind1")
	if out2.Allowed {
		t.Fatal("expected second attempt to be throttled due to MaxConcurrent=1")
	}

	if err := auth.ReportModelComplete(ctx, op, out.Permits, true, nil); err != nil {
		t.Fatal(err)
	}
}
