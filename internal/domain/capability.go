package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Capability identity and policy decisions (ARCHITECTURE CapabilityRegistry /
// PolicyEngine, INV-AUTH-003, FR-RES-001). Pure types only — no I/O.

// PolicyDecisionKind is the deterministic outcome of PolicyEngine.
type PolicyDecisionKind string

const (
	PolicyAllow           PolicyDecisionKind = "ALLOW"
	PolicyDeny            PolicyDecisionKind = "DENY"
	PolicyRequireApproval PolicyDecisionKind = "REQUIRE_APPROVAL"
)

func (k PolicyDecisionKind) Valid() bool {
	switch k {
	case PolicyAllow, PolicyDeny, PolicyRequireApproval:
		return true
	default:
		return false
	}
}

// SideEffectClass classifies external effects declared by a capability.
type SideEffectClass string

const (
	SideEffectNone           SideEffectClass = "none"
	SideEffectReadLocal      SideEffectClass = "read_local"
	SideEffectReadNetwork    SideEffectClass = "read_network"
	SideEffectWriteLocal     SideEffectClass = "write_local"
	SideEffectWriteExternal  SideEffectClass = "write_external"
	SideEffectModelComplete  SideEffectClass = "model_complete"
	SideEffectArtifactRender SideEffectClass = "artifact_render"
)

func (c SideEffectClass) Valid() bool {
	switch c {
	case SideEffectNone, SideEffectReadLocal, SideEffectReadNetwork,
		SideEffectWriteLocal, SideEffectWriteExternal, SideEffectModelComplete,
		SideEffectArtifactRender:
		return true
	default:
		return false
	}
}

// CapabilityDescriptor is a versioned installable capability contract.
// Model output MUST NOT invent descriptors; only operator/kernel registration.
type CapabilityDescriptor struct {
	SchemaVersion       int               `json:"schema_version"`
	Name                string            `json:"name"`
	Version             uint64            `json:"version"`
	InputSchema         string            `json:"input_schema"`
	OutputSchema        string            `json:"output_schema"`
	SideEffects         []SideEffectClass `json:"side_effects"`
	Risk                RiskLevel         `json:"risk"`
	RequiredPermissions []string          `json:"required_permissions"`
	DefaultTimeout      time.Duration     `json:"default_timeout"`
	RetryPolicy         string            `json:"retry_policy"`
	// Resource is the ResourceGate key for this capability (e.g. web:searxng).
	Resource string `json:"resource,omitempty"`
	// HasVerifier marks that a deterministic verifier exists for outputs.
	HasVerifier bool `json:"has_verifier"`
}

func (d CapabilityDescriptor) Validate() error {
	if d.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported capability schema version %d", d.SchemaVersion)
	}
	if strings.TrimSpace(d.Name) == "" || d.Version == 0 {
		return errors.New("capability name and positive version are required")
	}
	if d.InputSchema == "" || d.OutputSchema == "" || d.RetryPolicy == "" {
		return errors.New("capability schemas and retry policy are required")
	}
	if d.DefaultTimeout < 0 {
		return errors.New("capability default timeout must not be negative")
	}
	switch d.Risk {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		return fmt.Errorf("unknown risk level %q", d.Risk)
	}
	if len(d.SideEffects) == 0 {
		return errors.New("capability must declare at least one side-effect class")
	}
	seen := make(map[SideEffectClass]struct{}, len(d.SideEffects))
	for _, se := range d.SideEffects {
		if !se.Valid() {
			return fmt.Errorf("unknown side effect %q", se)
		}
		if _, ok := seen[se]; ok {
			return fmt.Errorf("duplicate side effect %q", se)
		}
		seen[se] = struct{}{}
	}
	for _, p := range d.RequiredPermissions {
		if strings.TrimSpace(p) == "" {
			return errors.New("required permission entries must be non-empty")
		}
	}
	return nil
}

// Ref returns a stable name@version identity string.
func (d CapabilityDescriptor) Ref() string {
	return fmt.Sprintf("%s@%d", d.Name, d.Version)
}

// CapabilityCatalog is an immutable in-memory registry snapshot for pure
// evaluation. Kernel/runtime may wrap persistence later.
type CapabilityCatalog struct {
	// PolicyVersion labels the allow/deny/approval rules applied with this catalog.
	PolicyVersion string
	// MaxRiskWithoutApproval denies or requires approval when capability risk
	// exceeds this level. Empty means RiskHigh (all risks allowed by risk alone).
	MaxRiskWithoutApproval RiskLevel
	// RequireApprovalFor lists capability names that always need approval.
	RequireApprovalFor map[string]struct{}
	// Denied lists capability names that are globally denied.
	Denied map[string]struct{}
	byRef  map[string]CapabilityDescriptor
	byName map[string][]CapabilityDescriptor // sorted by version ascending
}

// NewCapabilityCatalog builds a catalog from descriptors. Duplicate name@version fails.
func NewCapabilityCatalog(policyVersion string, descriptors []CapabilityDescriptor) (CapabilityCatalog, error) {
	if strings.TrimSpace(policyVersion) == "" {
		return CapabilityCatalog{}, errors.New("policy version is required")
	}
	cat := CapabilityCatalog{
		PolicyVersion:          policyVersion,
		MaxRiskWithoutApproval: RiskHigh,
		RequireApprovalFor:     map[string]struct{}{},
		Denied:                 map[string]struct{}{},
		byRef:                  make(map[string]CapabilityDescriptor, len(descriptors)),
		byName:                 make(map[string][]CapabilityDescriptor),
	}
	for _, d := range descriptors {
		if err := d.Validate(); err != nil {
			return CapabilityCatalog{}, err
		}
		ref := d.Ref()
		if _, exists := cat.byRef[ref]; exists {
			return CapabilityCatalog{}, fmt.Errorf("duplicate capability %s", ref)
		}
		cat.byRef[ref] = d
		cat.byName[d.Name] = append(cat.byName[d.Name], d)
	}
	for name := range cat.byName {
		sort.Slice(cat.byName[name], func(i, j int) bool {
			return cat.byName[name][i].Version < cat.byName[name][j].Version
		})
	}
	return cat, nil
}

// Lookup returns the exact name@version descriptor when present.
func (c CapabilityCatalog) Lookup(name string, version uint64) (CapabilityDescriptor, bool) {
	d, ok := c.byRef[fmt.Sprintf("%s@%d", name, version)]
	return d, ok
}

// Latest returns the highest installed version for name.
func (c CapabilityCatalog) Latest(name string) (CapabilityDescriptor, bool) {
	list := c.byName[name]
	if len(list) == 0 {
		return CapabilityDescriptor{}, false
	}
	return list[len(list)-1], true
}

// List returns all descriptors sorted by name then version.
func (c CapabilityCatalog) List() []CapabilityDescriptor {
	out := make([]CapabilityDescriptor, 0, len(c.byRef))
	for _, d := range c.byRef {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Version < out[j].Version
	})
	return out
}

// AuthorizationRequest is the pure input to EvaluateCapability (INV-AUTH-003).
type AuthorizationRequest struct {
	Capability      string
	Version         uint64 // 0 selects latest installed
	ArgsDigest      string // hash or structural digest of validated args
	OperationID     OperationID
	MissionRevision MissionRevisionID
	EstimatedCost   Budget
	// GrantedPermissions is the operator/runtime permission set for this scope.
	GrantedPermissions []string
	// AvailableBudget is the remaining operation/mission budget before reserve.
	AvailableBudget Budget
	Now             time.Time
	// AuthorizationTTL bounds how long an ALLOW decision may be reused.
	// Zero means single-use at the decision instant only (ExpiresAt == Now).
	AuthorizationTTL time.Duration
}

// PolicyDecision is a durable-ready authorization outcome. It is not itself
// an effect — the kernel must still drive the capability behind a permit.
type PolicyDecision struct {
	SchemaVersion     int                `json:"schema_version"`
	Decision          PolicyDecisionKind `json:"decision"`
	Reason            string             `json:"reason"`
	PolicyVersion     string             `json:"policy_version"`
	Capability        string             `json:"capability"`
	CapabilityVersion uint64             `json:"capability_version"`
	CapabilityRef     string             `json:"capability_ref"`
	OperationID       OperationID        `json:"operation_id,omitempty"`
	MissionRevision   MissionRevisionID  `json:"mission_revision_id,omitempty"`
	ArgsDigest        string             `json:"args_digest,omitempty"`
	Risk              RiskLevel          `json:"risk,omitempty"`
	IssuedAt          time.Time          `json:"issued_at"`
	ExpiresAt         time.Time          `json:"expires_at"`
	// ReservedCost is the budget slice covered by AvailableBudget at decision time.
	ReservedCost Budget `json:"reserved_cost"`
	// Resource is the gate key when the descriptor declares one.
	Resource string `json:"resource,omitempty"`
}

func (d PolicyDecision) Validate() error {
	if d.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported policy decision schema version %d", d.SchemaVersion)
	}
	if !d.Decision.Valid() || d.Reason == "" || d.PolicyVersion == "" || d.Capability == "" || d.CapabilityVersion == 0 {
		return errors.New("policy decision is incomplete")
	}
	if d.IssuedAt.IsZero() || d.ExpiresAt.IsZero() {
		return errors.New("policy decision requires issued_at and expires_at")
	}
	if d.ExpiresAt.Before(d.IssuedAt) {
		return errors.New("policy decision expires_at precedes issued_at")
	}
	return d.ReservedCost.Validate()
}

// UsableAt reports whether an ALLOW decision may still authorize an effect.
// DENY/REQUIRE_APPROVAL are never usable as execution permits.
// Expired, wrong args digest, wrong operation, or wrong mission revision fail closed.
func (d PolicyDecision) UsableAt(now time.Time, operationID OperationID, mission MissionRevisionID, argsDigest string) bool {
	if d.Decision != PolicyAllow {
		return false
	}
	if now.Before(d.IssuedAt) || now.After(d.ExpiresAt) {
		return false
	}
	if d.OperationID != "" && operationID != "" && d.OperationID != operationID {
		return false
	}
	if d.MissionRevision != "" && mission != "" && d.MissionRevision != mission {
		return false
	}
	if d.ArgsDigest != "" && argsDigest != "" && d.ArgsDigest != argsDigest {
		return false
	}
	return true
}

// EvaluateCapability is the pure PolicyEngine entry point (INV-AUTH-003).
// It never performs I/O and never mutates the catalog.
func EvaluateCapability(cat CapabilityCatalog, req AuthorizationRequest) (PolicyDecision, error) {
	if err := req.EstimatedCost.Validate(); err != nil {
		return PolicyDecision{}, err
	}
	if err := req.AvailableBudget.Validate(); err != nil {
		return PolicyDecision{}, err
	}
	if req.Now.IsZero() {
		return PolicyDecision{}, errors.New("authorization requires now")
	}
	if strings.TrimSpace(req.Capability) == "" {
		return PolicyDecision{}, errors.New("capability name is required")
	}

	var desc CapabilityDescriptor
	var ok bool
	if req.Version == 0 {
		desc, ok = cat.Latest(req.Capability)
	} else {
		desc, ok = cat.Lookup(req.Capability, req.Version)
	}
	if !ok {
		return denyDecision(cat, req, 0, "capability not installed"), nil
	}

	if _, denied := cat.Denied[desc.Name]; denied {
		return denyDecision(cat, req, desc.Version, "capability denied by policy"), nil
	}

	if !permissionsCover(req.GrantedPermissions, desc.RequiredPermissions) {
		return denyDecision(cat, req, desc.Version, "missing required permission"), nil
	}

	if !req.AvailableBudget.Covers(req.EstimatedCost) {
		return denyDecision(cat, req, desc.Version, "insufficient budget"), nil
	}

	if riskExceeds(desc.Risk, effectiveMaxRisk(cat.MaxRiskWithoutApproval)) {
		return approvalDecision(cat, req, desc, "risk exceeds policy ceiling"), nil
	}
	if _, need := cat.RequireApprovalFor[desc.Name]; need {
		return approvalDecision(cat, req, desc, "capability requires approval"), nil
	}

	expires := req.Now
	if req.AuthorizationTTL > 0 {
		expires = req.Now.Add(req.AuthorizationTTL)
	}
	dec := PolicyDecision{
		SchemaVersion:     SchemaVersionV1,
		Decision:          PolicyAllow,
		Reason:            "authorized",
		PolicyVersion:     cat.PolicyVersion,
		Capability:        desc.Name,
		CapabilityVersion: desc.Version,
		CapabilityRef:     desc.Ref(),
		OperationID:       req.OperationID,
		MissionRevision:   req.MissionRevision,
		ArgsDigest:        req.ArgsDigest,
		Risk:              desc.Risk,
		IssuedAt:          req.Now.UTC(),
		ExpiresAt:         expires.UTC(),
		ReservedCost:      req.EstimatedCost,
		Resource:          desc.Resource,
	}
	if err := dec.Validate(); err != nil {
		return PolicyDecision{}, err
	}
	return dec, nil
}

// MVPCapabilityDescriptors returns the experimental MVP capability set from
// ARCHITECTURE.md. Versions start at 1; resource keys match ResourceGate ids.
func MVPCapabilityDescriptors() []CapabilityDescriptor {
	return []CapabilityDescriptor{
		{
			SchemaVersion: SchemaVersionV1, Name: "file.discover", Version: 1,
			InputSchema: "file.discover.input.v1", OutputSchema: "file.discover.output.v1",
			SideEffects: []SideEffectClass{SideEffectReadLocal}, Risk: RiskLow,
			RequiredPermissions: []string{"file:authorized-root"},
			DefaultTimeout:      5 * time.Second, RetryPolicy: "no_retry",
			Resource: "file:authorized-root", HasVerifier: true,
		},
		{
			SchemaVersion: SchemaVersionV1, Name: "file.read", Version: 1,
			InputSchema: "file.read.input.v1", OutputSchema: "file.read.output.v1",
			SideEffects: []SideEffectClass{SideEffectReadLocal}, Risk: RiskLow,
			RequiredPermissions: []string{"file:authorized-root"},
			DefaultTimeout:      10 * time.Second, RetryPolicy: "no_retry",
			Resource: "file:authorized-root", HasVerifier: true,
		},
		{
			SchemaVersion: SchemaVersionV1, Name: "web.search", Version: 1,
			InputSchema: "web.search.input.v1", OutputSchema: "web.search.output.v1",
			SideEffects: []SideEffectClass{SideEffectReadNetwork}, Risk: RiskMedium,
			RequiredPermissions: []string{"web:search"},
			DefaultTimeout:      15 * time.Second, RetryPolicy: "retry_after",
			Resource: "web:searxng", HasVerifier: true,
		},
		{
			SchemaVersion: SchemaVersionV1, Name: "web.fetch", Version: 1,
			InputSchema: "web.fetch.input.v1", OutputSchema: "web.fetch.output.v1",
			SideEffects: []SideEffectClass{SideEffectReadNetwork}, Risk: RiskMedium,
			RequiredPermissions: []string{"web:fetch"},
			DefaultTimeout:      20 * time.Second, RetryPolicy: "retry_after",
			Resource: "web:http", HasVerifier: true,
		},
		{
			SchemaVersion: SchemaVersionV1, Name: "source.snapshot", Version: 1,
			InputSchema: "source.snapshot.input.v1", OutputSchema: "source.snapshot.output.v1",
			SideEffects: []SideEffectClass{SideEffectWriteLocal}, Risk: RiskLow,
			RequiredPermissions: []string{"source:snapshot"},
			DefaultTimeout:      5 * time.Second, RetryPolicy: "no_retry",
			Resource: "store:knowledge", HasVerifier: true,
		},
		{
			SchemaVersion: SchemaVersionV1, Name: "model.complete", Version: 1,
			InputSchema: "model.complete.input.v1", OutputSchema: "model.complete.output.v1",
			SideEffects: []SideEffectClass{SideEffectModelComplete}, Risk: RiskMedium,
			RequiredPermissions: []string{"model:complete"},
			DefaultTimeout:      60 * time.Second, RetryPolicy: "retry_after",
			Resource: "model:default", HasVerifier: false,
		},
		{
			SchemaVersion: SchemaVersionV1, Name: "artifact.render", Version: 1,
			InputSchema: "artifact.render.input.v1", OutputSchema: "artifact.render.output.v1",
			SideEffects: []SideEffectClass{SideEffectArtifactRender}, Risk: RiskLow,
			RequiredPermissions: []string{"artifact:render"},
			DefaultTimeout:      5 * time.Second, RetryPolicy: "no_retry",
			Resource: "store:artifact", HasVerifier: true,
		},
	}
}

func denyDecision(cat CapabilityCatalog, req AuthorizationRequest, version uint64, reason string) PolicyDecision {
	if version == 0 {
		version = req.Version
	}
	ref := ""
	if req.Capability != "" && version > 0 {
		ref = fmt.Sprintf("%s@%d", req.Capability, version)
	}
	return PolicyDecision{
		SchemaVersion:     SchemaVersionV1,
		Decision:          PolicyDeny,
		Reason:            reason,
		PolicyVersion:     cat.PolicyVersion,
		Capability:        req.Capability,
		CapabilityVersion: version,
		CapabilityRef:     ref,
		OperationID:       req.OperationID,
		MissionRevision:   req.MissionRevision,
		ArgsDigest:        req.ArgsDigest,
		IssuedAt:          req.Now.UTC(),
		ExpiresAt:         req.Now.UTC(),
		ReservedCost:      Budget{},
	}
}

func approvalDecision(cat CapabilityCatalog, req AuthorizationRequest, desc CapabilityDescriptor, reason string) PolicyDecision {
	return PolicyDecision{
		SchemaVersion:     SchemaVersionV1,
		Decision:          PolicyRequireApproval,
		Reason:            reason,
		PolicyVersion:     cat.PolicyVersion,
		Capability:        desc.Name,
		CapabilityVersion: desc.Version,
		CapabilityRef:     desc.Ref(),
		OperationID:       req.OperationID,
		MissionRevision:   req.MissionRevision,
		ArgsDigest:        req.ArgsDigest,
		Risk:              desc.Risk,
		IssuedAt:          req.Now.UTC(),
		ExpiresAt:         req.Now.UTC(),
		ReservedCost:      Budget{},
		Resource:          desc.Resource,
	}
}

func permissionsCover(granted, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(granted))
	for _, g := range granted {
		set[g] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}

func effectiveMaxRisk(r RiskLevel) RiskLevel {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return r
	default:
		return RiskHigh
	}
}

func riskExceeds(have, max RiskLevel) bool {
	return riskRank(have) > riskRank(max)
}

func riskRank(r RiskLevel) int {
	switch r {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	default:
		return 99
	}
}
