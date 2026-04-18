package cli

import (
	"fmt"
	"io"
	"strings"

	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
)

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

func renderWarnings(w io.Writer, warnings []string) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(w, "WARNING: %s\n", warning); err != nil {
			return err
		}
	}

	return nil
}
