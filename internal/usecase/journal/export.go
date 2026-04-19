package journal

import (
	"strings"
	"time"

	platformclock "github.com/lazuale/espocrm-ops/internal/platform/clock"
)

const (
	OperationBundleKind    = "operation_incident_bundle"
	OperationBundleVersion = 1

	OperationBundleSectionIdentity      = "identity"
	OperationBundleSectionType          = "type"
	OperationBundleSectionScope         = "scope"
	OperationBundleSectionSummary       = "summary"
	OperationBundleSectionTiming        = "timing"
	OperationBundleSectionTargetSummary = "target_summary"
	OperationBundleSectionStepOutcomes  = "step_outcomes"
	OperationBundleSectionWarnings      = "warnings"
	OperationBundleSectionFailure       = "failure"
	OperationBundleSectionRecovery      = "recovery"
	OperationBundleSectionDetails       = "details"
	OperationBundleSectionArtifacts     = "artifacts"
	OperationBundleSectionJournalRead   = "journal_read"
)

type ExportInput struct {
	JournalDir string
	ID         string
}

type ExportOutput struct {
	Bundle OperationBundle
}

type OperationBundle struct {
	BundleKind       string           `json:"bundle_kind"`
	BundleVersion    int              `json:"bundle_version"`
	ExportedAt       string           `json:"exported_at"`
	IncludedSections []string         `json:"included_sections"`
	OmittedSections  []string         `json:"omitted_sections,omitempty"`
	Summary          OperationSummary `json:"summary"`
	Report           OperationReport  `json:"report"`
	JournalRead      ReadStats        `json:"journal_read"`
}

func Export(in ExportInput) (ExportOutput, error) {
	operation, err := ShowOperation(ShowOperationInput(in))
	if err != nil {
		return ExportOutput{}, err
	}

	report := Explain(operation.Entry)
	summary := summarizeOperation(operation.Entry)
	included, omitted := bundleSections(report)

	return ExportOutput{
		Bundle: OperationBundle{
			BundleKind:       OperationBundleKind,
			BundleVersion:    OperationBundleVersion,
			ExportedAt:       platformclock.Now().UTC().Format(time.RFC3339),
			IncludedSections: included,
			OmittedSections:  omitted,
			Summary:          summary,
			Report:           report,
			JournalRead:      operation.Stats,
		},
	}, nil
}

func bundleSections(report OperationReport) ([]string, []string) {
	sections := []struct {
		name    string
		include bool
	}{
		{name: OperationBundleSectionIdentity, include: strings.TrimSpace(report.OperationID) != ""},
		{name: OperationBundleSectionType, include: strings.TrimSpace(report.Command) != ""},
		{name: OperationBundleSectionScope, include: strings.TrimSpace(report.Scope) != ""},
		{name: OperationBundleSectionSummary, include: true},
		{name: OperationBundleSectionTiming, include: strings.TrimSpace(report.StartedAt) != "" || strings.TrimSpace(report.FinishedAt) != "" || report.DurationMS != 0},
		{name: OperationBundleSectionTargetSummary, include: report.Target != nil},
		{name: OperationBundleSectionStepOutcomes, include: len(report.Steps) != 0},
		{name: OperationBundleSectionWarnings, include: len(report.Warnings) != 0},
		{name: OperationBundleSectionFailure, include: report.Failure != nil},
		{name: OperationBundleSectionRecovery, include: report.Recovery != nil},
		{name: OperationBundleSectionDetails, include: len(report.Details) != 0},
		{name: OperationBundleSectionArtifacts, include: len(report.Artifacts) != 0},
		{name: OperationBundleSectionJournalRead, include: true},
	}

	included := make([]string, 0, len(sections))
	omitted := make([]string, 0, len(sections))
	for _, section := range sections {
		if section.include {
			included = append(included, section.name)
			continue
		}
		omitted = append(omitted, section.name)
	}

	return included, omitted
}
