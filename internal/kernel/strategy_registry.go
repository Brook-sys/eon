package kernel

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"motor-autonomo/internal/domain"
)

// DefaultContinuityCatalogVersion is the explicit portfolio revision installed by
// RegisterDefaultContinuityFamilies. Catalogue evolution is a code change that
// MUST bump this string (or register a new named version) so diagnosis/inspect
// can attribute strategy behaviour without model authority.
const DefaultContinuityCatalogVersion = "continuity-catalog.v2"

// StrategyDescriptor is the versioned metadata for a continuity family.
// Registration is deterministic and independent from model authority.
type StrategyDescriptor struct {
	Name            string
	Family          domain.WorkFamily
	Version         string
	Priority        uint8
	RequiresModel   bool
	RequiresNetwork bool
	LocalOnly       bool
}

// Ref returns the stable name@version token used in diagnosis and audit trails.
func (d StrategyDescriptor) Ref() string {
	name := strings.TrimSpace(d.Name)
	version := strings.TrimSpace(d.Version)
	if name == "" {
		return ""
	}
	if version == "" {
		return name
	}
	return name + "@" + version
}

func (d StrategyDescriptor) Validate() error {
	if strings.TrimSpace(d.Name) == "" || strings.TrimSpace(d.Version) == "" || d.Priority == 0 {
		return errors.New("strategy descriptor is incomplete")
	}
	if !d.Family.Valid() {
		return fmt.Errorf("strategy family %q is not registered", d.Family)
	}
	if d.LocalOnly && (d.RequiresModel || d.RequiresNetwork) {
		return errors.New("local-only strategy cannot require model or network")
	}
	return nil
}

// RegisteredStrategy pairs descriptor metadata with a replenisher implementation.
type RegisteredStrategy struct {
	Descriptor StrategyDescriptor
	Strategy   ContinuityStrategy
}

// StrategyRegistry is an ordered, versioned catalogue of continuity families.
type StrategyRegistry struct {
	catalogVersion string
	byName         map[string]RegisteredStrategy
	order          []string
}

func NewStrategyRegistry() *StrategyRegistry {
	return &StrategyRegistry{byName: make(map[string]RegisteredStrategy)}
}

// SetCatalogVersion records the portfolio revision for Snapshot/inspect. Empty
// clears any prior version (tests may leave it unset).
func (r *StrategyRegistry) SetCatalogVersion(version string) {
	if r == nil {
		return
	}
	r.catalogVersion = strings.TrimSpace(version)
}

// CatalogVersion returns the portfolio revision string, if any.
func (r *StrategyRegistry) CatalogVersion() string {
	if r == nil {
		return ""
	}
	return r.catalogVersion
}

// Register inserts a strategy. Duplicate names are rejected so catalogue
// evolution remains explicit and versioned by callers.
func (r *StrategyRegistry) Register(descriptor StrategyDescriptor, strategy ContinuityStrategy) error {
	if r == nil {
		return errors.New("strategy registry is nil")
	}
	if strategy == nil {
		return errors.New("continuity strategy implementation is required")
	}
	if err := descriptor.Validate(); err != nil {
		return err
	}
	if strategy.Name() != descriptor.Name {
		return fmt.Errorf("strategy name %q disagrees with descriptor %q", strategy.Name(), descriptor.Name)
	}
	if _, exists := r.byName[descriptor.Name]; exists {
		return fmt.Errorf("strategy %q already registered", descriptor.Name)
	}
	r.byName[descriptor.Name] = RegisteredStrategy{Descriptor: descriptor, Strategy: strategy}
	r.order = append(r.order, descriptor.Name)
	sort.SliceStable(r.order, func(i, j int) bool {
		left, right := r.byName[r.order[i]].Descriptor, r.byName[r.order[j]].Descriptor
		if left.Priority == right.Priority {
			return left.Name < right.Name
		}
		// Higher priority first.
		return left.Priority > right.Priority
	})
	return nil
}

// StrategyCatalogSnapshot is a read-only view of the ordered registry used by
// diagnosis, inspect, and tests. It never grants model authority.
type StrategyCatalogSnapshot struct {
	CatalogVersion string
	Descriptors    []StrategyDescriptor
}

// Snapshot clones ordered descriptors and the catalogue version.
func (r *StrategyRegistry) Snapshot() StrategyCatalogSnapshot {
	if r == nil {
		return StrategyCatalogSnapshot{}
	}
	descriptors := r.Descriptors()
	out := StrategyCatalogSnapshot{
		CatalogVersion: r.catalogVersion,
		Descriptors:    append([]StrategyDescriptor(nil), descriptors...),
	}
	return out
}

// Descriptor returns the metadata for a registered strategy name.
func (r *StrategyRegistry) Descriptor(name string) (StrategyDescriptor, bool) {
	if r == nil {
		return StrategyDescriptor{}, false
	}
	item, ok := r.byName[strings.TrimSpace(name)]
	if !ok {
		return StrategyDescriptor{}, false
	}
	return item.Descriptor, true
}

// StrategyRefs returns ordered name@version tokens for diagnosis trails.
func (r *StrategyRegistry) StrategyRefs() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name].Descriptor.Ref())
	}
	return out
}

func (r *StrategyRegistry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.order)
}

func (r *StrategyRegistry) Strategies() []ContinuityStrategy {
	if r == nil {
		return nil
	}
	out := make([]ContinuityStrategy, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name].Strategy)
	}
	return out
}

func (r *StrategyRegistry) Descriptors() []StrategyDescriptor {
	if r == nil {
		return nil
	}
	out := make([]StrategyDescriptor, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name].Descriptor)
	}
	return out
}

// ContinuityPlan is the non-dispatch branch chosen before or after strategy
// attempts. EXPAND means replenish; DIAGNOSE means persist ContinuityBlocked.
type ContinuityPlan struct {
	Action   domain.ContinuityAction
	Reason   string
	Horizon  domain.ExecutableHorizon
	Strategy string
}

// PlanContinuityAction chooses EXPAND while strategies remain to replenish an
// empty or insufficient dispatch set; otherwise DIAGNOSE for ContinuityBlocked.
func PlanContinuityAction(horizon domain.ExecutableHorizon, remainingStrategies int, nextStrategy string) (ContinuityPlan, error) {
	if err := horizon.Validate(); err != nil {
		return ContinuityPlan{}, err
	}
	if remainingStrategies < 0 {
		return ContinuityPlan{}, errors.New("remaining strategies must not be negative")
	}
	if remainingStrategies > 0 {
		if strings.TrimSpace(nextStrategy) == "" {
			return ContinuityPlan{}, errors.New("expand plan requires next strategy name")
		}
		reason := "no ready work; expand continuity family"
		if horizon.NeedsReplenishment() {
			reason = "ready horizon at or below low watermark"
		}
		return ContinuityPlan{
			Action:   domain.ContinuityExpand,
			Reason:   reason,
			Horizon:  horizon,
			Strategy: nextStrategy,
		}, nil
	}
	return ContinuityPlan{
		Action:  domain.ContinuityDiagnose,
		Reason:  "no executable work after continuity expansion",
		Horizon: horizon,
	}, nil
}

// StrategyCooldown and StrategyCooldownBook live in cooldown.go so the
// registry stays focused on ordered catalogue metadata.
