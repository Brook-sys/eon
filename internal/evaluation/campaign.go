package evaluation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const CampaignSchemaVersion = 1

// CampaignManifest declares a bounded live campaign before any provider call.
// Secret values are referenced only by environment-variable name.
type CampaignManifest struct {
	SchemaVersion        int             `json:"schema_version"`
	Name                 string          `json:"name"`
	FixturePath          string          `json:"fixture_path"`
	ContextTokens        []int           `json:"context_tokens"`
	MaxCalls             int             `json:"max_calls"`
	MaxOutputTokens      int             `json:"max_output_tokens"`
	MaxTotalOutputTokens int             `json:"max_total_output_tokens"`
	TimeoutSeconds       int             `json:"timeout_seconds"`
	Models               []CampaignModel `json:"models"`
}

type CampaignModel struct {
	Provider          string `json:"provider"`
	BindingID         string `json:"binding_id"`
	BaseURL           string `json:"base_url"`
	Model             string `json:"model"`
	APIKeyEnvironment string `json:"api_key_env"`
	MaxOutputField    string `json:"max_output_field"`
	BaselineReport    string `json:"baseline_report,omitempty"`
}

func DecodeCampaignManifest(r io.Reader, maxBytes int64) (CampaignManifest, error) {
	if r == nil || maxBytes <= 0 {
		return CampaignManifest{}, errors.New("campaign reader and positive byte limit are required")
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return CampaignManifest{}, fmt.Errorf("read campaign manifest: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return CampaignManifest{}, errors.New("campaign manifest exceeds byte limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest CampaignManifest
	if err := decoder.Decode(&manifest); err != nil {
		return CampaignManifest{}, fmt.Errorf("decode campaign manifest: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return CampaignManifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return CampaignManifest{}, err
	}
	return manifest, nil
}

func (m CampaignManifest) Validate() error {
	if m.SchemaVersion != CampaignSchemaVersion || strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.FixturePath) == "" {
		return errors.New("campaign identity, fixture path, and supported schema version are required")
	}
	if err := (Matrix{ContextTokens: m.ContextTokens}).Validate(); err != nil {
		return fmt.Errorf("campaign contexts: %w", err)
	}
	if m.MaxCalls <= 0 || m.MaxOutputTokens <= 0 || m.MaxTotalOutputTokens <= 0 || m.TimeoutSeconds <= 0 || m.TimeoutSeconds > int((24*time.Hour).Seconds()) {
		return errors.New("campaign requires positive call/output/time bounds with timeout at most 24h")
	}
	if m.MaxTotalOutputTokens < m.MaxCalls*m.MaxOutputTokens {
		return errors.New("max_total_output_tokens must cover max_calls × max_output_tokens")
	}
	if len(m.Models) == 0 {
		return errors.New("campaign requires at least one model")
	}
	seen := map[string]bool{}
	for i, model := range m.Models {
		if err := model.Validate(); err != nil {
			return fmt.Errorf("campaign model %d: %w", i, err)
		}
		if seen[model.BindingID] {
			return fmt.Errorf("duplicate campaign binding_id %q", model.BindingID)
		}
		seen[model.BindingID] = true
	}
	return nil
}

func (m CampaignModel) Validate() error {
	for label, value := range map[string]string{
		"provider": m.Provider, "binding_id": m.BindingID, "base_url": m.BaseURL,
		"model": m.Model, "api_key_env": m.APIKeyEnvironment, "max_output_field": m.MaxOutputField,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
	}
	for _, r := range m.BindingID {
		if !(r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return errors.New("binding_id must be filesystem-safe")
		}
	}
	if m.MaxOutputField != "max_tokens" && m.MaxOutputField != "max_completion_tokens" {
		return errors.New("unsupported max_output_field")
	}
	return nil
}

func (m CampaignManifest) PlannedCalls(fixtures FixtureSet) int {
	cells := 0
	for _, c := range fixtures.Cases {
		cells += len(c.Formats) * len(m.ContextTokens)
	}
	return cells * len(m.Models)
}

func WriteCampaignManifest(path string, manifest CampaignManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWrite(path, body)
}

type CampaignReport struct {
	SchemaVersion int                   `json:"schema_version"`
	Name          string                `json:"name"`
	FixtureName   string                `json:"fixture_name"`
	PlannedCalls  int                   `json:"planned_calls"`
	MaxCalls      int                   `json:"max_calls"`
	Models        []CampaignModelReport `json:"models"`
}

// QualificationVerdict is an evidence-only deployment classification. It is
// deliberately separate from runtime configuration and routing authority.
type QualificationVerdict string

const (
	QualificationQualified    QualificationVerdict = "QUALIFIED"
	QualificationDegraded     QualificationVerdict = "DEGRADED"
	QualificationIncompatible QualificationVerdict = "INCOMPATIBLE"
)

type Qualification struct {
	Verdict QualificationVerdict `json:"verdict"`
	Reason  string               `json:"reason"`
}

type CampaignModelReport struct {
	Provider      string        `json:"provider"`
	BindingID     string        `json:"binding_id"`
	Model         string        `json:"model"`
	Report        Report        `json:"report"`
	Qualification Qualification `json:"qualification"`
	Regression    []Regression  `json:"regressions,omitempty"`
}

// QualifyReport applies conservative, reproducible thresholds to one bounded
// live report. A qualification never enables a binding or changes preference.
func QualifyReport(report Report) Qualification {
	if report.SchemaVersion != 1 || report.Summary.Total <= 0 {
		return Qualification{Verdict: QualificationIncompatible, Reason: "incomplete benchmark report"}
	}
	total := report.Summary.Total
	providerFailures := report.Summary.ProviderErrors + report.Summary.Timeouts
	if providerFailures*2 >= total {
		return Qualification{Verdict: QualificationIncompatible, Reason: "provider failures or timeouts affected at least half of runs"}
	}
	if report.Summary.SyntaxValid*2 < total {
		return Qualification{Verdict: QualificationIncompatible, Reason: "strict syntax validity was below 50%"}
	}
	if report.Summary.SemanticallyRight*3 >= total*2 && providerFailures == 0 {
		return Qualification{Verdict: QualificationQualified, Reason: "at least two-thirds correct with no provider failures"}
	}
	return Qualification{Verdict: QualificationDegraded, Reason: "compatible enough to observe, but below qualification threshold"}
}

type Regression struct {
	Dimension     string `json:"dimension"`
	Label         string `json:"label"`
	Metric        string `json:"metric"`
	Baseline      int    `json:"baseline"`
	Current       int    `json:"current"`
	Delta         int    `json:"delta"`
	BaselineTotal int    `json:"baseline_total,omitempty"`
	CurrentTotal  int    `json:"current_total,omitempty"`
}

func CompareReports(baseline, current Report) ([]Regression, error) {
	if baseline.SchemaVersion != 1 || current.SchemaVersion != 1 {
		return nil, errors.New("baseline and current reports must use the supported report schema")
	}
	var regressions []Regression
	compare := func(dimension string, before, after []Aggregate) {
		base := map[string]Aggregate{}
		for _, aggregate := range before {
			base[aggregate.Label] = aggregate
		}
		for _, aggregate := range after {
			old, ok := base[aggregate.Label]
			if !ok {
				continue
			}
			for _, metric := range []struct {
				name string
				b, c int
			}{
				{"syntax_valid", old.SyntaxValid, aggregate.SyntaxValid},
				{"semantically_correct", old.SemanticallyRight, aggregate.SemanticallyRight},
			} {
				// Compare rates by cross multiplication so an expanded corpus does
				// not look better merely because it contains more matrix cells.
				if old.Total > 0 && aggregate.Total > 0 && metric.c*old.Total < metric.b*aggregate.Total {
					regressions = append(regressions, Regression{Dimension: dimension, Label: aggregate.Label, Metric: metric.name, Baseline: metric.b, Current: metric.c, Delta: metric.c - metric.b, BaselineTotal: old.Total, CurrentTotal: aggregate.Total})
				}
			}
		}
	}
	compare("operation", baseline.Breakdown.ByOperation, current.Breakdown.ByOperation)
	compare("format", baseline.Breakdown.ByFormat, current.Breakdown.ByFormat)
	compare("context", baseline.Breakdown.ByContext, current.Breakdown.ByContext)
	sort.Slice(regressions, func(i, j int) bool {
		a, b := regressions[i], regressions[j]
		if a.Dimension != b.Dimension {
			return a.Dimension < b.Dimension
		}
		if a.Label != b.Label {
			return a.Label < b.Label
		}
		return a.Metric < b.Metric
	})
	return regressions, nil
}

func ReadReport(path string) (Report, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var report Report
	if err := decoder.Decode(&report); err != nil {
		return Report{}, fmt.Errorf("decode report: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Report{}, err
	}
	if report.SchemaVersion != 1 || report.FixtureName == "" || len(report.Runs) == 0 {
		return Report{}, errors.New("report is incomplete")
	}
	return report, nil
}

func WriteCampaignArtifacts(directory string, report CampaignReport) error {
	if strings.TrimSpace(directory) == "" || report.SchemaVersion != CampaignSchemaVersion || len(report.Models) == 0 {
		return errors.New("artifact directory and complete campaign report are required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := atomicWrite(filepath.Join(directory, "campaign.json"), body); err != nil {
		return err
	}
	var markdown strings.Builder
	fmt.Fprintf(&markdown, "# Cognitive campaign\n\n- Name: `%s`\n- Fixture: `%s`\n- Planned/max calls: %d/%d\n- Models: %d\n\n", report.Name, report.FixtureName, report.PlannedCalls, report.MaxCalls, len(report.Models))
	markdown.WriteString("| Provider | Binding | Model | Qualification | Correct | Syntax | Provider errors | 429 | Timeouts | Regressions |\n| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, model := range report.Models {
		s := model.Report.Summary
		fmt.Fprintf(&markdown, "| %s | %s | %s | %s | %d/%d | %d/%d | %d | %d | %d | %d |\n", model.Provider, model.BindingID, model.Model, model.Qualification.Verdict, s.SemanticallyRight, s.Total, s.SyntaxValid, s.Total, s.ProviderErrors, s.RateLimited, s.Timeouts, len(model.Regression))
	}
	markdown.WriteString("\nQualification is observational evidence only; it does not enable a binding or change runtime routing.\n")
	markdown.WriteString("\n## Regressions\n\n")
	for _, model := range report.Models {
		for _, regression := range model.Regression {
			fmt.Fprintf(&markdown, "- `%s` %s/%s %s: %d/%d → %d/%d\n", model.BindingID, regression.Dimension, regression.Label, regression.Metric, regression.Baseline, regression.BaselineTotal, regression.Current, regression.CurrentTotal)
		}
	}
	return atomicWrite(filepath.Join(directory, "campaign.md"), []byte(markdown.String()))
}
