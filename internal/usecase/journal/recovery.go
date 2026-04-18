package journal

import "strings"

const (
	RecoveryModeAuto   = "auto"
	RecoveryModeRetry  = "retry"
	RecoveryModeResume = "resume"

	RecoveryDecisionRetryFromStart       = "retry_from_start"
	RecoveryDecisionResumeFromCheckpoint = "resume_from_checkpoint"
	RecoveryDecisionRefused              = "refused"
)

type OperationRecovery struct {
	Retryable       bool   `json:"retryable"`
	Resumable       bool   `json:"resumable"`
	RecommendedMode string `json:"recommended_mode,omitempty"`
	Decision        string `json:"decision"`
	ResumeStep      string `json:"resume_step,omitempty"`
	Summary         string `json:"summary"`
	Details         string `json:"details,omitempty"`
	Action          string `json:"action,omitempty"`
}

type RecoverySelection struct {
	RequestedMode string `json:"requested_mode"`
	AppliedMode   string `json:"applied_mode"`
	Decision      string `json:"decision"`
	ResumeStep    string `json:"resume_step,omitempty"`
}

func explainRecovery(report OperationReport) *OperationRecovery {
	switch report.Command {
	case "update":
		return explainUpdateRecovery(report)
	case "rollback":
		return explainRollbackRecovery(report)
	default:
		return nil
	}
}

func explainUpdateRecovery(report OperationReport) *OperationRecovery {
	if report.DryRun {
		return refusedRecovery(
			"This update entry is a dry-run plan",
			"Retry and resume are only defined for real update executions, not for blocked dry-run plans.",
			"Run the canonical update command again when you are ready to execute it for real.",
		)
	}
	if report.OK {
		return refusedRecovery(
			"This update already completed",
			"Retry and resume are only available for failed or blocked update runs.",
			"Use show-operation for audit detail, or run a new update intentionally if you need another change window.",
		)
	}

	step, ok := failedOrBlockedStep(report.Steps)
	if !ok {
		return refusedRecovery(
			"The failed update checkpoint is unknown",
			"The journal entry does not identify a canonical failed or blocked update step to recover from.",
			"Inspect the raw logs and run a fresh update explicitly once the contour state is understood.",
		)
	}

	switch step.Code {
	case "operation_preflight":
		return retryRecovery(
			"Update can be retried from the start",
			"The source update failed before canonical preflight completed, so there is no trustworthy runtime checkpoint to resume from.",
			"Resolve the preflight error, then rerun the update from the start.",
		)
	case "doctor":
		return retryRecovery(
			"Update can be retried from the start",
			"The source update stopped in the doctor gate before any later execution checkpoint was established.",
			"Resolve the doctor failure, then retry the update from the start.",
		)
	case "backup_recovery_point":
		return retryRecovery(
			"Update can be retried from the start",
			"The recovery-point step failed before the runtime apply checkpoint was reached, so resume would be unsafe.",
			"Resolve the recovery-point failure, then retry the update from the start.",
		)
	case "runtime_apply":
		return resumeRecovery(
			"runtime_apply",
			"Update can resume from runtime apply",
			"The source update already cleared preflight, doctor, and recovery-point setup. Resume can restart the runtime apply step without redoing earlier completed work.",
			"Use resume to continue from runtime apply, or choose retry if you intentionally want the full update flow again.",
		)
	case "runtime_readiness":
		return resumeRecovery(
			"runtime_readiness",
			"Update can resume from runtime readiness",
			"The source update already completed runtime apply. Resume can continue from readiness verification without rerunning earlier completed steps.",
			"Use resume to continue from runtime readiness, or choose retry if you intentionally want the full update flow again.",
		)
	default:
		return refusedRecovery(
			"This update failure cannot be recovered canonically",
			"The journaled step sequence does not match a supported update retry or resume checkpoint.",
			"Inspect the report and rerun the update explicitly once the contour state is understood.",
		)
	}
}

func explainRollbackRecovery(report OperationReport) *OperationRecovery {
	if report.DryRun {
		return refusedRecovery(
			"This rollback entry is a dry-run plan",
			"Retry and resume are only defined for real rollback executions, not for blocked dry-run plans.",
			"Run the canonical rollback command again when you are ready to execute it for real.",
		)
	}
	if report.OK {
		return refusedRecovery(
			"This rollback already completed",
			"Retry and resume are only available for failed or blocked rollback runs.",
			"Use show-operation for audit detail, or run a new rollback intentionally if you need another recovery window.",
		)
	}

	step, ok := failedOrBlockedStep(report.Steps)
	if !ok {
		return refusedRecovery(
			"The failed rollback checkpoint is unknown",
			"The journal entry does not identify a canonical failed or blocked rollback step to recover from.",
			"Inspect the raw logs and run a fresh rollback explicitly once the contour state is understood.",
		)
	}

	switch step.Code {
	case "operation_preflight":
		return retryRecovery(
			"Rollback can be retried from the start",
			"The source rollback failed before canonical preflight completed, so there is no trustworthy checkpoint to resume from.",
			"Resolve the preflight error, then retry the rollback from the start.",
		)
	case "doctor":
		return retryRecovery(
			"Rollback can be retried from the start",
			"The source rollback stopped in the doctor gate before a destructive checkpoint was established.",
			"Resolve the doctor failure, then retry the rollback from the start.",
		)
	case "target_selection":
		if !rollbackRequestedTargetKnown(report) {
			return refusedRecovery(
				"Rollback retry is refused because the original target request is ambiguous",
				"The journal entry does not contain enough target-request detail to retry this failed selection without guessing a new rollback target.",
				"Inspect the report and rerun rollback explicitly with the intended target selection.",
			)
		}
		return retryRecovery(
			"Rollback can be retried from the start",
			"The source rollback failed before runtime mutation began, so retry from the start is safe once the original target request is known.",
			"Resolve the target selection issue, then retry the rollback from the start.",
		)
	case "runtime_prepare":
		if !rollbackSelectedTargetKnown(report) {
			return refusedRecovery(
				"Rollback resume is refused because the selected target is incomplete",
				"The journal entry does not contain the selected rollback target needed to resume safely from runtime preparation.",
				"Inspect the report and rerun rollback explicitly with the intended backup set.",
			)
		}
		return resumeRecovery(
			"runtime_prepare",
			"Rollback can resume from runtime preparation",
			"The source rollback already established a canonical target. Resume can continue from runtime preparation without reselecting a new rollback target.",
			"Use resume to continue from runtime preparation, or choose retry if you intentionally want the full rollback flow again.",
		)
	case "snapshot_recovery_point", "db_restore":
		if !rollbackSelectedTargetKnown(report) {
			return refusedRecovery(
				"Rollback resume is refused because the selected target is incomplete",
				"The journal entry does not contain the selected rollback target needed to resume safely after runtime preparation.",
				"Inspect the report and rerun rollback explicitly with the intended backup set.",
			)
		}
		return resumeRecovery(
			"runtime_prepare",
			"Rollback can resume from runtime preparation",
			"The source rollback already selected a canonical target. Resume can re-establish runtime preparation and continue the destructive path without guessing a new target.",
			"Use resume to continue from runtime preparation, or choose retry if you intentionally want the full rollback flow again.",
		)
	case "files_restore":
		return refusedRecovery(
			"Rollback resume is refused after a files-restore failure",
			"The source rollback already changed state past database restore, and the remaining continuation path is ambiguous for the filesystem tree.",
			"Inspect the contour state first, then run an explicit rollback or manual restore plan once it is safe.",
		)
	case "runtime_return":
		if !rollbackSelectedTargetKnown(report) {
			return refusedRecovery(
				"Rollback resume is refused because the selected target is incomplete",
				"The journal entry does not contain the selected rollback target needed to reason about the completed destructive steps safely.",
				"Inspect the report and rerun rollback explicitly with the intended backup set.",
			)
		}
		return resumeRecovery(
			"runtime_return",
			"Rollback can resume from contour return",
			"The source rollback completed restore work and only needs to return the contour to service again.",
			"Use resume to retry the contour return step, or choose retry only if you intentionally want the full rollback flow again.",
		)
	default:
		return refusedRecovery(
			"This rollback failure cannot be recovered canonically",
			"The journaled step sequence does not match a supported rollback retry or resume checkpoint.",
			"Inspect the report and rerun the rollback explicitly once the contour state is understood.",
		)
	}
}

func ResolveRecoverySelection(report OperationReport, requestedMode string) (RecoverySelection, error) {
	mode := normalizeRecoveryMode(requestedMode)
	recovery := explainRecovery(report)
	if recovery == nil {
		return RecoverySelection{}, ValidationError{
			Code:    "operation_recovery_refused",
			Message: "retry and resume are only supported for update and rollback operations",
		}
	}

	switch mode {
	case RecoveryModeAuto:
		if recovery.Decision == RecoveryDecisionRefused {
			return RecoverySelection{}, ValidationError{
				Code:    "operation_recovery_refused",
				Message: recovery.Summary,
			}
		}

		selection := RecoverySelection{
			RequestedMode: RecoveryModeAuto,
			AppliedMode:   recovery.RecommendedMode,
			Decision:      recovery.Decision,
			ResumeStep:    recovery.ResumeStep,
		}
		if selection.AppliedMode == "" {
			selection.AppliedMode = modeForDecision(recovery.Decision)
		}
		return selection, nil
	case RecoveryModeRetry:
		if !recovery.Retryable {
			return RecoverySelection{}, ValidationError{
				Code:    "operation_recovery_refused",
				Message: recovery.Summary,
			}
		}
		return RecoverySelection{
			RequestedMode: RecoveryModeRetry,
			AppliedMode:   RecoveryModeRetry,
			Decision:      RecoveryDecisionRetryFromStart,
		}, nil
	case RecoveryModeResume:
		if !recovery.Resumable {
			return RecoverySelection{}, ValidationError{
				Code:    "operation_recovery_refused",
				Message: recovery.Summary,
			}
		}
		return RecoverySelection{
			RequestedMode: RecoveryModeResume,
			AppliedMode:   RecoveryModeResume,
			Decision:      RecoveryDecisionResumeFromCheckpoint,
			ResumeStep:    recovery.ResumeStep,
		}, nil
	default:
		return RecoverySelection{}, ValidationError{
			Code:    "usage_error",
			Message: "recovery mode must be auto, retry, or resume",
		}
	}
}

func normalizeRecoveryMode(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return RecoveryModeAuto
	}
	return trimmed
}

func modeForDecision(decision string) string {
	switch decision {
	case RecoveryDecisionRetryFromStart:
		return RecoveryModeRetry
	case RecoveryDecisionResumeFromCheckpoint:
		return RecoveryModeResume
	default:
		return ""
	}
}

func retryRecovery(summary, details, action string) *OperationRecovery {
	return &OperationRecovery{
		Retryable:       true,
		RecommendedMode: RecoveryModeRetry,
		Decision:        RecoveryDecisionRetryFromStart,
		Summary:         summary,
		Details:         details,
		Action:          action,
	}
}

func resumeRecovery(resumeStep, summary, details, action string) *OperationRecovery {
	return &OperationRecovery{
		Retryable:       true,
		Resumable:       true,
		RecommendedMode: RecoveryModeResume,
		Decision:        RecoveryDecisionResumeFromCheckpoint,
		ResumeStep:      resumeStep,
		Summary:         summary,
		Details:         details,
		Action:          action,
	}
}

func refusedRecovery(summary, details, action string) *OperationRecovery {
	return &OperationRecovery{
		Decision: RecoveryDecisionRefused,
		Summary:  summary,
		Details:  details,
		Action:   action,
	}
}

func failedOrBlockedStep(steps []OperationStep) (OperationStep, bool) {
	for _, step := range steps {
		if step.Status == operationStepStatusFailed || step.Status == operationStepStatusBlocked {
			return step, true
		}
	}
	return OperationStep{}, false
}

func rollbackSelectedTargetKnown(report OperationReport) bool {
	return report.Target != nil &&
		strings.TrimSpace(report.Target.DBBackup) != "" &&
		strings.TrimSpace(report.Target.FilesBackup) != ""
}

func rollbackRequestedTargetKnown(report OperationReport) bool {
	mode := mapString(report.Details, "requested_selection_mode")
	if mode == "" {
		mode = mapString(report.Details, "selection_mode")
	}
	switch mode {
	case "explicit":
		return strings.TrimSpace(mapString(report.Artifacts, "requested_db_backup")) != "" &&
			strings.TrimSpace(mapString(report.Artifacts, "requested_files_backup")) != ""
	case "auto_latest_valid":
		return true
	default:
		return false
	}
}

type ValidationError struct {
	Code    string
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}
