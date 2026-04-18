package journal

import (
	"time"
)

const (
	operationStepStatusWouldRun  = "would_run"
	operationStepStatusCompleted = "completed"
	operationStepStatusSkipped   = "skipped"
	operationStepStatusBlocked   = "blocked"
	operationStepStatusFailed    = "failed"
	operationStepStatusUnknown   = "unknown"
	operationStepStatusNotRun    = "not_run"
)

type OperationReport struct {
	OperationID  string                `json:"operation_id"`
	Command      string                `json:"command"`
	StartedAt    string                `json:"started_at"`
	FinishedAt   string                `json:"finished_at,omitempty"`
	DurationMS   int64                 `json:"duration_ms,omitempty"`
	OK           bool                  `json:"ok"`
	DryRun       bool                  `json:"dry_run,omitempty"`
	Message      string                `json:"message,omitempty"`
	Scope        string                `json:"scope,omitempty"`
	Counts       OperationReportCounts `json:"counts"`
	Target       *OperationTarget      `json:"target,omitempty"`
	Steps        []OperationStep       `json:"steps,omitempty"`
	Details      map[string]any        `json:"details,omitempty"`
	Artifacts    map[string]any        `json:"artifacts,omitempty"`
	Warnings     []string              `json:"warnings,omitempty"`
	ErrorCode    string                `json:"error_code,omitempty"`
	ErrorMessage string                `json:"error_message,omitempty"`
	Failure      *OperationFailure     `json:"failure,omitempty"`
}

type OperationReportCounts struct {
	Steps     int `json:"steps"`
	WouldRun  int `json:"would_run,omitempty"`
	Completed int `json:"completed,omitempty"`
	Skipped   int `json:"skipped,omitempty"`
	Blocked   int `json:"blocked,omitempty"`
	Failed    int `json:"failed,omitempty"`
	Unknown   int `json:"unknown,omitempty"`
}

type OperationTarget struct {
	SelectionMode string `json:"selection_mode,omitempty"`
	Prefix        string `json:"prefix,omitempty"`
	Stamp         string `json:"stamp,omitempty"`
	ManifestJSON  string `json:"manifest_json,omitempty"`
	DBBackup      string `json:"db_backup,omitempty"`
	FilesBackup   string `json:"files_backup,omitempty"`
}

type OperationStep struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

type OperationFailure struct {
	Code        string `json:"code,omitempty"`
	Message     string `json:"message,omitempty"`
	StepCode    string `json:"step_code,omitempty"`
	StepSummary string `json:"step_summary,omitempty"`
	Action      string `json:"action,omitempty"`
}

func Explain(entry Entry) OperationReport {
	steps := explainSteps(entry.Items)

	report := OperationReport{
		OperationID:  entry.OperationID,
		Command:      entry.Command,
		StartedAt:    entry.StartedAt,
		FinishedAt:   entry.FinishedAt,
		DurationMS:   explainDurationMS(entry.StartedAt, entry.FinishedAt),
		OK:           entry.OK,
		DryRun:       entry.DryRun,
		Message:      entry.Message,
		Scope:        mapString(entry.Details, "scope"),
		Counts:       countOperationSteps(steps),
		Target:       explainTarget(entry),
		Steps:        steps,
		Details:      entry.Details,
		Artifacts:    entry.Artifacts,
		Warnings:     append([]string(nil), entry.Warnings...),
		ErrorCode:    entry.ErrorCode,
		ErrorMessage: entry.ErrorMessage,
	}
	report.Failure = explainFailure(report)

	return report
}

func explainSteps(items []any) []OperationStep {
	if len(items) == 0 {
		return nil
	}

	steps := make([]OperationStep, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		code := mapString(item, "code")
		status := normalizeOperationStepStatus(mapString(item, "status"))
		summary := mapString(item, "summary")
		if code == "" || status == "" || summary == "" {
			continue
		}

		steps = append(steps, OperationStep{
			Code:    code,
			Status:  status,
			Summary: summary,
			Details: mapString(item, "details"),
			Action:  mapString(item, "action"),
		})
	}

	return steps
}

func normalizeOperationStepStatus(status string) string {
	switch status {
	case operationStepStatusWouldRun,
		operationStepStatusCompleted,
		operationStepStatusSkipped,
		operationStepStatusBlocked,
		operationStepStatusFailed,
		operationStepStatusUnknown:
		return status
	case operationStepStatusNotRun:
		return operationStepStatusBlocked
	default:
		return operationStepStatusUnknown
	}
}

func countOperationSteps(steps []OperationStep) OperationReportCounts {
	counts := OperationReportCounts{
		Steps: len(steps),
	}

	for _, step := range steps {
		switch step.Status {
		case operationStepStatusWouldRun:
			counts.WouldRun++
		case operationStepStatusCompleted:
			counts.Completed++
		case operationStepStatusSkipped:
			counts.Skipped++
		case operationStepStatusBlocked:
			counts.Blocked++
		case operationStepStatusFailed:
			counts.Failed++
		default:
			counts.Unknown++
		}
	}

	return counts
}

func explainTarget(entry Entry) *OperationTarget {
	if entry.Command != "rollback" {
		return nil
	}

	target := &OperationTarget{
		SelectionMode: mapString(entry.Details, "selection_mode"),
		Prefix:        mapString(entry.Artifacts, "selected_prefix"),
		Stamp:         mapString(entry.Artifacts, "selected_stamp"),
		ManifestJSON:  mapString(entry.Artifacts, "manifest_json"),
		DBBackup:      mapString(entry.Artifacts, "db_backup"),
		FilesBackup:   mapString(entry.Artifacts, "files_backup"),
	}
	if target.SelectionMode == "" &&
		target.Prefix == "" &&
		target.Stamp == "" &&
		target.ManifestJSON == "" &&
		target.DBBackup == "" &&
		target.FilesBackup == "" {
		return nil
	}

	return target
}

func explainFailure(report OperationReport) *OperationFailure {
	if report.ErrorCode == "" && report.ErrorMessage == "" {
		return nil
	}

	failure := &OperationFailure{
		Code:    report.ErrorCode,
		Message: report.ErrorMessage,
	}

	for _, step := range report.Steps {
		if step.Status != operationStepStatusFailed && step.Status != operationStepStatusBlocked {
			continue
		}
		failure.StepCode = step.Code
		failure.StepSummary = step.Summary
		failure.Action = step.Action
		break
	}

	return failure
}

func explainDurationMS(startedAt, finishedAt string) int64 {
	if startedAt == "" || finishedAt == "" {
		return 0
	}

	started, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return 0
	}
	finished, err := time.Parse(time.RFC3339, finishedAt)
	if err != nil {
		return 0
	}
	if finished.Before(started) {
		return 0
	}

	return finished.Sub(started).Milliseconds()
}

func mapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}

	raw, ok := values[key]
	if !ok {
		return ""
	}

	value, _ := raw.(string)
	return value
}
