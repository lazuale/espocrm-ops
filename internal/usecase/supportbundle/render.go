package supportbundle

import (
	"fmt"
	"strings"

	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	doctorusecase "github.com/lazuale/espocrm-ops/internal/usecase/doctor"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
)

func renderDoctorReportText(report doctorusecase.Report) string {
	var body strings.Builder

	passed, warnings, failed := report.Counts()
	fmt.Fprintln(&body, "EspoCRM doctor")
	fmt.Fprintf(&body, "Target scope: %s\n", report.TargetScope)
	fmt.Fprintf(&body, "Ready: %t\n", report.Ready())
	fmt.Fprintf(&body, "Checks: %d\n", len(report.Checks))
	fmt.Fprintf(&body, "Passed: %d\n", passed)
	fmt.Fprintf(&body, "Warnings: %d\n", warnings)
	fmt.Fprintf(&body, "Failed: %d\n", failed)

	if len(report.Checks) == 0 {
		return body.String()
	}

	fmt.Fprintln(&body, "\nChecks:")
	for _, check := range report.Checks {
		prefix := fmt.Sprintf("[%s]", strings.ToUpper(check.Status))
		if strings.TrimSpace(check.Scope) != "" {
			prefix = fmt.Sprintf("[%s][%s]", check.Scope, strings.ToUpper(check.Status))
		}
		fmt.Fprintf(&body, "%s %s\n", prefix, check.Summary)
		if strings.TrimSpace(check.Details) != "" {
			fmt.Fprintf(&body, "  details: %s\n", check.Details)
		}
		if strings.TrimSpace(check.Action) != "" {
			fmt.Fprintf(&body, "  action: %s\n", check.Action)
		}
	}

	return body.String()
}

func renderOperationHistoryText(operations []journalusecase.OperationSummary) string {
	if len(operations) == 0 {
		return "no operations found\n"
	}

	var body strings.Builder
	for _, operation := range operations {
		fmt.Fprintln(&body, formatHistoryLine(operation))
	}
	return body.String()
}

func renderBackupCatalogText(info backupusecase.CatalogInfo) string {
	var body strings.Builder

	fmt.Fprintln(&body, "EspoCRM backup inventory")
	fmt.Fprintf(&body, "Backup directory: %s\n", info.BackupRoot)
	fmt.Fprintf(&body, "Checksum verification: %t\n", info.VerifyChecksum)
	fmt.Fprintf(&body, "Ready-only filter: %t\n", info.ReadyOnly)
	fmt.Fprintf(&body, "Total sets: %d\n", info.TotalSets)
	fmt.Fprintf(&body, "Shown sets: %d\n", info.ShownSets)

	if len(info.Items) == 0 {
		fmt.Fprintln(&body, "\nNo backup sets matched the current selection filters")
		return body.String()
	}

	for idx, item := range info.Items {
		fmt.Fprintf(&body, "\n[%d] %s | readiness=%s\n", idx+1, item.ID, item.RestoreReadiness)
		fmt.Fprintf(&body, "  prefix/stamp: %s | %s\n", item.Prefix, item.Stamp)
		fmt.Fprintf(&body, "  scope: %s\n", valueOrNA(item.Scope))
		fmt.Fprintf(&body, "  created_at: %s\n", valueOrNA(item.CreatedAt))
		fmt.Fprintf(&body, "  db: %s\n", valueOrMissing(item.DB.File))
		fmt.Fprintf(&body, "  files: %s\n", valueOrMissing(item.Files.File))
		fmt.Fprintf(&body, "  manifest_json: %s\n", valueOrMissing(item.ManifestJSON.File))
		fmt.Fprintf(&body, "  manifest_txt: %s\n", valueOrMissing(item.ManifestTXT.File))
	}

	return body.String()
}

func renderBundleSummary(info Info) string {
	var body strings.Builder

	fmt.Fprintln(&body, "EspoCRM support bundle")
	fmt.Fprintf(&body, "Scope: %s\n", info.Scope)
	fmt.Fprintf(&body, "Generated at: %s\n", info.GeneratedAt)
	fmt.Fprintf(&body, "Output path: %s\n", info.OutputPath)
	fmt.Fprintf(&body, "Bundle: %s v%d\n", info.BundleKind, info.BundleVersion)
	fmt.Fprintf(&body, "Tail lines: %d\n", info.TailLines)
	fmt.Fprintf(&body, "Included: %s\n", strings.Join(info.IncludedSections, ", "))

	omitted := "none"
	if len(info.OmittedSections) != 0 {
		omitted = strings.Join(info.OmittedSections, ", ")
	}
	fmt.Fprintf(&body, "Omitted: %s\n", omitted)

	fmt.Fprintln(&body, "\nSections:")
	for _, section := range info.Sections {
		fmt.Fprintf(&body, "[%s] %s - %s\n", strings.ToUpper(section.Status), section.Code, section.Summary)
		if strings.TrimSpace(section.Details) != "" {
			fmt.Fprintf(&body, "  details: %s\n", section.Details)
		}
		if len(section.Files) != 0 {
			fmt.Fprintf(&body, "  files: %s\n", strings.Join(section.Files, ", "))
		}
		if strings.TrimSpace(section.Action) != "" {
			fmt.Fprintf(&body, "  action: %s\n", section.Action)
		}
	}

	if len(info.Warnings) != 0 {
		fmt.Fprintln(&body, "\nWarnings:")
		for _, warning := range info.Warnings {
			fmt.Fprintf(&body, "- %s\n", warning)
		}
	}

	return body.String()
}

func formatHistoryLine(operation journalusecase.OperationSummary) string {
	parts := []string{
		operation.StartedAt,
		operation.Command,
		strings.ToUpper(operation.Status),
	}
	if operation.DryRun {
		parts = append(parts, "mode=dry-run")
	}
	if operation.Scope != "" {
		parts = append(parts, "scope="+operation.Scope)
	}
	if target := formatHistoryTarget(operation.Target); target != "" {
		parts = append(parts, "target="+target)
	}
	if recovery := formatHistoryRecovery(operation); recovery != "" {
		parts = append(parts, recovery)
	}
	if operation.ErrorCode != "" {
		parts = append(parts, "error="+operation.ErrorCode)
	}
	if operation.WarningCount > 0 {
		parts = append(parts, fmt.Sprintf("warnings=%d", operation.WarningCount))
	}
	if operation.Summary != "" {
		parts = append(parts, fmt.Sprintf("summary=%q", operation.Summary))
	}
	parts = append(parts, "id="+operation.OperationID)

	return strings.Join(parts, "  ")
}

func formatHistoryTarget(target *journalusecase.OperationSummaryTarget) string {
	if target == nil {
		return ""
	}

	value := target.Prefix
	if target.Stamp != "" {
		if value != "" {
			value += "@"
		}
		value += target.Stamp
	}
	if value == "" {
		value = target.SelectionMode
	}

	return value
}

func formatHistoryRecovery(operation journalusecase.OperationSummary) string {
	if !operation.RecoveryRun {
		return ""
	}
	if operation.RecoveryAttempt == nil {
		return "recovery=true"
	}

	value := operation.RecoveryAttempt.AppliedMode
	if value == "" {
		value = "true"
	}
	if operation.RecoveryAttempt.SourceOperationID != "" {
		value += ":" + operation.RecoveryAttempt.SourceOperationID
	}
	if operation.RecoveryAttempt.ResumeStep != "" {
		value += "@" + operation.RecoveryAttempt.ResumeStep
	}

	return "recovery=" + value
}

func valueOrNA(value string) string {
	if strings.TrimSpace(value) == "" {
		return "n/a"
	}
	return value
}

func valueOrMissing(value string) string {
	if strings.TrimSpace(value) == "" {
		return "missing"
	}
	return value
}
