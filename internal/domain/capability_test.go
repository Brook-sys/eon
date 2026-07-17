package domain

import (
	"testing"
	"time"
)

func TestMVPCapabilityCatalog(t *testing.T) {
	cat, err := NewCapabilityCatalog("policy@mvp-1", MVPCapabilityDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	list := cat.List()
	if len(list) != 7 {
		t.Fatalf("mvp size = %d", len(list))
	}
	web, ok := cat.Lookup("web.search", 1)
	if !ok || web.Resource != "web:searxng" {
		t.Fatalf("web.search = %+v ok=%v", web, ok)
	}
	latest, ok := cat.Latest("model.complete")
	if !ok || latest.Version != 1 {
		t.Fatalf("latest model.complete = %+v", latest)
	}
}

func TestEvaluateCapabilityAllowDenyApproval(t *testing.T) {
	cat, err := NewCapabilityCatalog("policy@mvp-1", MVPCapabilityDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	base := AuthorizationRequest{
		Capability:         "web.search",
		Version:            1,
		ArgsDigest:         "sha256:args1",
		OperationID:        "op_1",
		MissionRevision:    "mr_1",
		EstimatedCost:      Budget{ModelCalls: 0, Tokens: 0, Bytes: 1024, Attempts: 1},
		GrantedPermissions: []string{"web:search"},
		AvailableBudget:    Budget{Bytes: 4096, Attempts: 2},
		Now:                now,
		AuthorizationTTL:   time.Minute,
	}

	allow, err := EvaluateCapability(cat, base)
	if err != nil {
		t.Fatal(err)
	}
	if allow.Decision != PolicyAllow || allow.CapabilityRef != "web.search@1" {
		t.Fatalf("allow = %+v", allow)
	}
	if !allow.UsableAt(now.Add(30*time.Second), "op_1", "mr_1", "sha256:args1") {
		t.Fatal("expected usable permit")
	}
	// Expired
	if allow.UsableAt(now.Add(2*time.Minute), "op_1", "mr_1", "sha256:args1") {
		t.Fatal("expired must not be usable")
	}
	// Wrong args
	if allow.UsableAt(now.Add(time.Second), "op_1", "mr_1", "sha256:other") {
		t.Fatal("args mismatch must not be usable")
	}
	// Wrong operation
	if allow.UsableAt(now.Add(time.Second), "op_2", "mr_1", "sha256:args1") {
		t.Fatal("operation mismatch must not be usable")
	}

	// Unknown capability
	unknown := base
	unknown.Capability = "shell.exec"
	dec, err := EvaluateCapability(cat, unknown)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != PolicyDeny || dec.Reason != "capability not installed" {
		t.Fatalf("unknown = %+v", dec)
	}
	if dec.UsableAt(now, "op_1", "mr_1", "sha256:args1") {
		t.Fatal("deny never usable")
	}

	// Missing permission
	noperm := base
	noperm.GrantedPermissions = nil
	dec, err = EvaluateCapability(cat, noperm)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != PolicyDeny || dec.Reason != "missing required permission" {
		t.Fatalf("noperm = %+v", dec)
	}

	// Insufficient budget
	broke := base
	broke.AvailableBudget = Budget{Bytes: 1, Attempts: 1}
	broke.EstimatedCost = Budget{Bytes: 2048, Attempts: 1}
	dec, err = EvaluateCapability(cat, broke)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != PolicyDeny || dec.Reason != "insufficient budget" {
		t.Fatalf("broke = %+v", dec)
	}

	// Risk ceiling → require approval
	strict := cat
	strict.MaxRiskWithoutApproval = RiskLow
	dec, err = EvaluateCapability(strict, base)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != PolicyRequireApproval {
		t.Fatalf("risk = %+v", dec)
	}

	// Explicit require-approval list
	need := cat
	need.RequireApprovalFor = map[string]struct{}{"web.search": {}}
	dec, err = EvaluateCapability(need, base)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != PolicyRequireApproval {
		t.Fatalf("require list = %+v", dec)
	}

	// Global deny
	denied := cat
	denied.Denied = map[string]struct{}{"web.search": {}}
	dec, err = EvaluateCapability(denied, base)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != PolicyDeny {
		t.Fatalf("denied = %+v", dec)
	}
}

func TestCapabilityDescriptorValidation(t *testing.T) {
	bad := CapabilityDescriptor{SchemaVersion: SchemaVersionV1, Name: "x", Version: 1}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected incomplete descriptor error")
	}
	_, err := NewCapabilityCatalog("p", []CapabilityDescriptor{
		MVPCapabilityDescriptors()[0],
		MVPCapabilityDescriptors()[0],
	})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestEvaluateCapabilityLatestVersion(t *testing.T) {
	d1 := MVPCapabilityDescriptors()[5] // model.complete@1
	d2 := d1
	d2.Version = 2
	cat, err := NewCapabilityCatalog("p@1", []CapabilityDescriptor{d1, d2})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	dec, err := EvaluateCapability(cat, AuthorizationRequest{
		Capability:         "model.complete",
		Version:            0,
		GrantedPermissions: []string{"model:complete"},
		AvailableBudget:    Budget{ModelCalls: 1, Tokens: 100, Attempts: 1},
		EstimatedCost:      Budget{ModelCalls: 1, Tokens: 10, Attempts: 1},
		Now:                now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != PolicyAllow || dec.CapabilityVersion != 2 {
		t.Fatalf("latest = %+v", dec)
	}
}
