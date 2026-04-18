package journal

import "strings"

const (
	OperationStatusCompleted = "completed"
	OperationStatusFailed    = "failed"
	OperationStatusBlocked   = "blocked"
	OperationStatusRunning   = "running"
	OperationStatusUnknown   = "unknown"
)

type OperationSummary struct {
	OperationID     string                    `json:"operation_id"`
	Command         string                    `json:"command"`
	Status          string                    `json:"status"`
	StartedAt       string                    `json:"started_at"`
	FinishedAt      string                    `json:"finished_at,omitempty"`
	DurationMS      int64                     `json:"duration_ms,omitempty"`
	DryRun          bool                      `json:"dry_run,omitempty"`
	Scope           string                    `json:"scope,omitempty"`
	RecoveryRun     bool                      `json:"recovery_run"`
	Summary         string                    `json:"summary,omitempty"`
	Counts          OperationReportCounts     `json:"counts"`
	WarningCount    int                       `json:"warning_count,omitempty"`
	ErrorCode       string                    `json:"error_code,omitempty"`
	Target          *OperationSummaryTarget   `json:"target,omitempty"`
	Failure         *OperationFailure         `json:"failure,omitempty"`
	RecoveryAttempt *OperationRecoveryAttempt `json:"recovery_attempt,omitempty"`
}

type OperationSummaryTarget struct {
	SelectionMode string `json:"selection_mode,omitempty"`
	Prefix        string `json:"prefix,omitempty"`
	Stamp         string `json:"stamp,omitempty"`
}

type OperationRecoveryAttempt struct {
	SourceOperationID string `json:"source_operation_id"`
	RequestedMode     string `json:"requested_mode,omitempty"`
	AppliedMode       string `json:"applied_mode,omitempty"`
	Decision          string `json:"decision,omitempty"`
	ResumeStep        string `json:"resume_step,omitempty"`
}

func summarizeOperations(entries []Entry, filters Filters) []OperationSummary {
	operations := make([]OperationSummary, 0, len(entries))
	for _, entry := range entries {
		operation := summarizeOperation(entry)
		if !matchesOperationSummaryFilters(operation, filters) {
			continue
		}

		operations = append(operations, operation)
		if filters.Limit > 0 && len(operations) >= filters.Limit {
			break
		}
	}

	return operations
}

func summarizeOperation(entry Entry) OperationSummary {
	report := Explain(entry)
	recoveryAttempt := explainRecoveryAttempt(report)
	summary := OperationSummary{
		OperationID:     report.OperationID,
		Command:         report.Command,
		Status:          summarizeOperationStatus(report),
		StartedAt:       report.StartedAt,
		FinishedAt:      report.FinishedAt,
		DurationMS:      report.DurationMS,
		DryRun:          report.DryRun,
		Scope:           report.Scope,
		RecoveryRun:     recoveryAttempt != nil,
		Summary:         summarizeOperationText(report),
		Counts:          report.Counts,
		WarningCount:    len(report.Warnings),
		ErrorCode:       report.ErrorCode,
		Target:          summarizeOperationTarget(report.Target),
		Failure:         report.Failure,
		RecoveryAttempt: recoveryAttempt,
	}
	if summary.Status == "" {
		summary.Status = OperationStatusUnknown
	}

	return summary
}

func summarizeOperationStatus(report OperationReport) string {
	if strings.TrimSpace(report.FinishedAt) == "" {
		return OperationStatusRunning
	}
	if report.DryRun {
		if report.OK && report.Counts.Blocked == 0 && report.ErrorCode == "" {
			return OperationStatusCompleted
		}
		return OperationStatusBlocked
	}
	if report.OK {
		return OperationStatusCompleted
	}
	if report.Counts.Blocked > 0 && report.Counts.Failed == 0 && report.ErrorCode == "" && report.Failure == nil {
		return OperationStatusBlocked
	}
	if !report.OK {
		return OperationStatusFailed
	}
	return OperationStatusUnknown
}

func summarizeOperationText(report OperationReport) string {
	if report.Failure != nil {
		if summary := strings.TrimSpace(report.Failure.StepSummary); summary != "" {
			return summary
		}
		if message := strings.TrimSpace(report.Failure.Message); message != "" {
			return message
		}
	}

	if message := strings.TrimSpace(report.Message); message != "" {
		return message
	}

	for idx := len(report.Steps) - 1; idx >= 0; idx-- {
		if summary := strings.TrimSpace(report.Steps[idx].Summary); summary != "" {
			return summary
		}
	}

	return ""
}

func summarizeOperationTarget(target *OperationTarget) *OperationSummaryTarget {
	if target == nil {
		return nil
	}

	summary := &OperationSummaryTarget{
		SelectionMode: target.SelectionMode,
		Prefix:        target.Prefix,
		Stamp:         target.Stamp,
	}
	if summary.SelectionMode == "" && summary.Prefix == "" && summary.Stamp == "" {
		return nil
	}

	return summary
}

func explainRecoveryAttempt(report OperationReport) *OperationRecoveryAttempt {
	values := mapMap(report.Details, "recovery")
	if len(values) == 0 {
		return nil
	}

	recovery := &OperationRecoveryAttempt{
		SourceOperationID: mapString(values, "source_operation_id"),
		RequestedMode:     mapString(values, "requested_mode"),
		AppliedMode:       mapString(values, "applied_mode"),
		Decision:          mapString(values, "decision"),
		ResumeStep:        mapString(values, "resume_step"),
	}
	if recovery.SourceOperationID == "" &&
		recovery.RequestedMode == "" &&
		recovery.AppliedMode == "" &&
		recovery.Decision == "" &&
		recovery.ResumeStep == "" {
		return nil
	}

	return recovery
}

func matchesOperationSummaryFilters(operation OperationSummary, filters Filters) bool {
	if filters.Status != "" && operation.Status != filters.Status {
		return false
	}
	if filters.Scope != "" && operation.Scope != filters.Scope {
		return false
	}
	if filters.RecoveryOnly && !operation.RecoveryRun {
		return false
	}
	if filters.TargetPrefix != "" {
		if operation.Target == nil || operation.Target.Prefix != filters.TargetPrefix {
			return false
		}
	}

	return true
}

func mapMap(values map[string]any, key string) map[string]any {
	if values == nil {
		return nil
	}

	raw, ok := values[key]
	if !ok {
		return nil
	}

	out, _ := raw.(map[string]any)
	return out
}
