package journal

import (
	"testing"
)

func TestExportBuildsBundleFromShowOperationData(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, Entry{
		OperationID:  "op-export-1",
		Command:      "rollback",
		StartedAt:    "2026-04-19T10:00:00Z",
		FinishedAt:   "2026-04-19T10:00:04Z",
		OK:           false,
		Message:      "rollback failed",
		ErrorCode:    "rollback_failed",
		ErrorMessage: "target selection failed",
		Warnings:     []string{"final probe skipped"},
		Details: map[string]any{
			"scope":          "prod",
			"selection_mode": "auto_latest_valid",
		},
		Artifacts: map[string]any{
			"selected_prefix": "espocrm-prod",
			"selected_stamp":  "2026-04-19_09-00-00",
			"manifest_json":   "/backups/manifest.json",
		},
		Items: []any{
			map[string]any{
				"code":    "target_selection",
				"status":  "failed",
				"summary": "Rollback target selection failed",
				"action":  "Resolve the target selection error before retrying rollback.",
			},
			map[string]any{
				"code":    "runtime_prepare",
				"status":  "not_run",
				"summary": "Runtime preparation did not run because target selection failed",
			},
		},
	})
	writeRaw(t, dir+"/corrupt.json", "{not-json")

	out, err := Export(ExportInput{
		JournalDir: dir,
		ID:         "op-export-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	bundle := out.Bundle
	if bundle.BundleKind != OperationBundleKind || bundle.BundleVersion != OperationBundleVersion {
		t.Fatalf("unexpected bundle metadata: %#v", bundle)
	}
	if bundle.Summary.OperationID != "op-export-1" || bundle.Summary.Status != OperationStatusFailed {
		t.Fatalf("unexpected operation summary: %#v", bundle.Summary)
	}
	if bundle.Report.Scope != "prod" {
		t.Fatalf("unexpected report scope: %#v", bundle.Report)
	}
	if bundle.JournalRead.TotalFilesSeen != 2 || bundle.JournalRead.LoadedEntries != 1 || bundle.JournalRead.SkippedCorrupt != 1 {
		t.Fatalf("unexpected journal read stats: %#v", bundle.JournalRead)
	}
	for _, want := range []string{
		OperationBundleSectionIdentity,
		OperationBundleSectionType,
		OperationBundleSectionScope,
		OperationBundleSectionSummary,
		OperationBundleSectionTiming,
		OperationBundleSectionTargetSummary,
		OperationBundleSectionStepOutcomes,
		OperationBundleSectionWarnings,
		OperationBundleSectionFailure,
		OperationBundleSectionRecovery,
		OperationBundleSectionDetails,
		OperationBundleSectionArtifacts,
		OperationBundleSectionJournalRead,
	} {
		if !containsString(bundle.IncludedSections, want) {
			t.Fatalf("expected included section %q, got %#v", want, bundle.IncludedSections)
		}
	}
}

func TestExportListsMissingOptionalSections(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, Entry{
		OperationID: "op-export-2",
		Command:     "verify-backup",
		StartedAt:   "2026-04-19T10:00:00Z",
		FinishedAt:  "2026-04-19T10:00:01Z",
		OK:          true,
	})

	out, err := Export(ExportInput{
		JournalDir: dir,
		ID:         "op-export-2",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		OperationBundleSectionScope,
		OperationBundleSectionTargetSummary,
		OperationBundleSectionStepOutcomes,
		OperationBundleSectionWarnings,
		OperationBundleSectionFailure,
		OperationBundleSectionRecovery,
		OperationBundleSectionDetails,
		OperationBundleSectionArtifacts,
	} {
		if !containsString(out.Bundle.OmittedSections, want) {
			t.Fatalf("expected omitted section %q, got %#v", want, out.Bundle.OmittedSections)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
