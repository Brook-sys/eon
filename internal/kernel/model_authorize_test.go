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
	now := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
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
		if err != nil {
			return err
		}
		spec, err = r.OperationSpec(op.SpecID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

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

func TestGroqBindingQuotaIsolation(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	seedModelAgenda(t, store, now)

	auth, err := NewMVPCapabilityAuthorizer(store, clock, "test-v1")
	if err != nil {
		t.Fatal(err)
	}
	provider := domain.ModelProviderResource("groq")
	bindingA := domain.ModelBindingResource("groq-a")
	bindingB := domain.ModelBindingResource("groq-b")
	auth.Limits[provider] = domain.ResourceLimit{Resource: provider, MaxPerMinute: 10}
	auth.Limits[bindingA] = domain.ResourceLimit{Resource: bindingA, MaxPerMinute: 1}
	auth.Limits[bindingB] = domain.ResourceLimit{Resource: bindingB, MaxPerMinute: 1}

	var op domain.Operation
	var spec domain.OperationSpec
	if err := store.View(ctx, func(r port.Reader) error {
		var readErr error
		op, readErr = r.Operation("operation_model")
		if readErr != nil {
			return readErr
		}
		spec, readErr = r.OperationSpec(op.SpecID)
		return readErr
	}); err != nil {
		t.Fatal(err)
	}

	first, err := auth.ReserveModelComplete(ctx, op, spec, 0, "groq", "groq-a")
	if err != nil || !first.Allowed {
		t.Fatalf("first binding A reserve: %+v err=%v", first, err)
	}
	if err := auth.ReportModelComplete(ctx, op, first.Permits, true, nil); err != nil {
		t.Fatal(err)
	}

	blockedA, err := auth.ReserveModelComplete(ctx, op, spec, 0, "groq", "groq-a")
	if err != nil {
		t.Fatal(err)
	}
	if blockedA.Allowed || !blockedA.Throttled {
		t.Fatalf("saturated binding A must throttle: %+v", blockedA)
	}

	allowedB, err := auth.ReserveModelComplete(ctx, op, spec, 0, "groq", "groq-b")
	if err != nil {
		t.Fatal(err)
	}
	if !allowedB.Allowed {
		t.Fatalf("binding B must remain available when only A is saturated: %+v", allowedB)
	}
	if err := auth.ReportModelComplete(ctx, op, allowedB.Permits, true, nil); err != nil {
		t.Fatal(err)
	}
}

func TestNIMProviderRetryAfterBlocksAllBindings(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	now := time.Date(2026, 7, 17, 12, 10, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	seedModelAgenda(t, store, now)

	auth, err := NewMVPCapabilityAuthorizer(store, clock, "test-v1")
	if err != nil {
		t.Fatal(err)
	}
	provider := domain.ModelProviderResource("nvidia-nim")
	bindingA := domain.ModelBindingResource("nim-a")
	bindingB := domain.ModelBindingResource("nim-b")
	auth.Limits[provider] = domain.ResourceLimit{Resource: provider, MaxPerMinute: 30, FailureThreshold: 10}
	auth.Limits[bindingA] = domain.ResourceLimit{Resource: bindingA, MaxPerMinute: 30}
	auth.Limits[bindingB] = domain.ResourceLimit{Resource: bindingB, MaxPerMinute: 30}

	var op domain.Operation
	var spec domain.OperationSpec
	if err := store.View(ctx, func(r port.Reader) error {
		var readErr error
		op, readErr = r.Operation("operation_model")
		if readErr != nil {
			return readErr
		}
		spec, readErr = r.OperationSpec(op.SpecID)
		return readErr
	}); err != nil {
		t.Fatal(err)
	}

	first, err := auth.ReserveModelComplete(ctx, op, spec, 0, "nvidia-nim", "nim-a")
	if err != nil || !first.Allowed {
		t.Fatalf("first NIM reserve: %+v err=%v", first, err)
	}
	retryAt := now.Add(45 * time.Second)
	if err := auth.ReportModelComplete(ctx, op, first.Permits, false, &retryAt); err != nil {
		t.Fatal(err)
	}

	blockedB, err := auth.ReserveModelComplete(ctx, op, spec, 0, "nvidia-nim", "nim-b")
	if err != nil {
		t.Fatal(err)
	}
	if blockedB.Allowed || !blockedB.Throttled || blockedB.Acquire == nil || blockedB.Acquire.WaitUntil == nil {
		t.Fatalf("provider retry-after must block every NIM binding: %+v", blockedB)
	}
	if !blockedB.Acquire.WaitUntil.Equal(retryAt) {
		t.Fatalf("wait until=%v want=%v", blockedB.Acquire.WaitUntil, retryAt)
	}

	clock.Set(retryAt)
	allowedB, err := auth.ReserveModelComplete(ctx, op, spec, 0, "nvidia-nim", "nim-b")
	if err != nil {
		t.Fatal(err)
	}
	if !allowedB.Allowed {
		t.Fatalf("binding B must recover when provider retry-after expires: %+v", allowedB)
	}
}
