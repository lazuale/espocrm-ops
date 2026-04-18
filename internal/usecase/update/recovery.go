package update

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
)

var updateStepOrder = []string{
	"operation_preflight",
	"doctor",
	"backup_recovery_point",
	"runtime_apply",
	"runtime_readiness",
}

type ExecuteRecovery struct {
	Info            operationusecase.RecoveryInfo
	SourceSteps     map[string]journalusecase.OperationStep
	CheckpointIndex int
	FailureIndex    int
}

func RecoverPlan(report journalusecase.OperationReport, selection journalusecase.RecoverySelection) (UpdatePlan, error) {
	req, err := buildRecoveryRequest(report, selection, nil)
	if err != nil {
		return UpdatePlan{}, err
	}

	plan := UpdatePlan{
		Scope:          strings.TrimSpace(req.Scope),
		ProjectDir:     req.ProjectDir,
		ComposeFile:    req.ComposeFile,
		EnvFile:        strings.TrimSpace(req.EnvFileOverride),
		TimeoutSeconds: req.TimeoutSeconds,
		SkipDoctor:     req.SkipDoctor,
		SkipBackup:     req.SkipBackup,
		SkipPull:       req.SkipPull,
		SkipHTTPProbe:  req.SkipHTTPProbe,
		Recovery:       req.Recovery.Info,
	}

	for _, code := range updateStepOrder {
		if req.Recovery.shouldSkip(code) {
			plan.Steps = append(plan.Steps, req.Recovery.skippedPlanStep(code))
			continue
		}
		plan.Steps = append(plan.Steps, updateRecoveryPlanStep(code, selection, req))
	}

	return plan, nil
}

func RecoverExecute(report journalusecase.OperationReport, selection journalusecase.RecoverySelection, logWriter io.Writer) (ExecuteInfo, error) {
	req, err := buildRecoveryRequest(report, selection, logWriter)
	if err != nil {
		return ExecuteInfo{}, err
	}

	return Execute(req)
}

func buildRecoveryRequest(report journalusecase.OperationReport, selection journalusecase.RecoverySelection, logWriter io.Writer) (ExecuteRequest, error) {
	if report.Command != "update" {
		return ExecuteRequest{}, apperr.Wrap(apperr.KindValidation, "update_recovery_refused", fmt.Errorf("operation %s is not an update run", report.OperationID))
	}

	projectDir := strings.TrimSpace(mapReportString(report.Artifacts, "project_dir"))
	composeFile := strings.TrimSpace(mapReportString(report.Artifacts, "compose_file"))
	envFile := strings.TrimSpace(mapReportString(report.Artifacts, "env_file"))
	if projectDir == "" || composeFile == "" || envFile == "" || strings.TrimSpace(report.Scope) == "" {
		return ExecuteRequest{}, apperr.Wrap(apperr.KindValidation, "update_recovery_refused", fmt.Errorf("operation %s is missing canonical update context needed for recovery", report.OperationID))
	}

	return ExecuteRequest{
		Scope:           strings.TrimSpace(report.Scope),
		ProjectDir:      projectDir,
		ComposeFile:     composeFile,
		EnvFileOverride: envFile,
		TimeoutSeconds:  mapReportInt(report.Details, "timeout_seconds"),
		SkipDoctor:      mapReportBool(report.Details, "skip_doctor"),
		SkipBackup:      mapReportBool(report.Details, "skip_backup"),
		SkipPull:        mapReportBool(report.Details, "skip_pull"),
		SkipHTTPProbe:   mapReportBool(report.Details, "skip_http_probe"),
		LogWriter:       logWriter,
		Recovery:        buildExecuteRecovery(report, selection),
	}, nil
}

func buildExecuteRecovery(report journalusecase.OperationReport, selection journalusecase.RecoverySelection) *ExecuteRecovery {
	sourceSteps := make(map[string]journalusecase.OperationStep, len(report.Steps))
	for _, step := range report.Steps {
		sourceSteps[step.Code] = step
	}

	return &ExecuteRecovery{
		Info: operationusecase.RecoveryInfo{
			SourceOperationID: report.OperationID,
			RequestedMode:     selection.RequestedMode,
			AppliedMode:       selection.AppliedMode,
			Decision:          selection.Decision,
			ResumeStep:        selection.ResumeStep,
		},
		SourceSteps:     sourceSteps,
		CheckpointIndex: updateStepIndex(selection.ResumeStep),
		FailureIndex:    updateFailureIndex(report.Steps),
	}
}

func updateFailureIndex(steps []journalusecase.OperationStep) int {
	for _, step := range steps {
		if step.Status == "failed" || step.Status == "blocked" {
			return updateStepIndex(step.Code)
		}
	}
	return len(updateStepOrder)
}

func (r *ExecuteRecovery) shouldSkip(code string) bool {
	if r == nil || r.Info.Decision != journalusecase.RecoveryDecisionResumeFromCheckpoint {
		return false
	}

	index := updateStepIndex(code)
	if index == -1 {
		return false
	}
	if index < r.CheckpointIndex {
		return true
	}
	if index == r.CheckpointIndex {
		return false
	}

	return index < r.FailureIndex
}

func (r *ExecuteRecovery) skippedExecuteStep(code string) ExecuteStep {
	source, ok := r.SourceSteps[code]
	if !ok {
		return ExecuteStep{
			Code:    code,
			Status:  UpdateStepStatusSkipped,
			Summary: updateRecoverySkipSummary(code),
			Details: fmt.Sprintf("Recovery reused this step from source operation %s.", r.Info.SourceOperationID),
		}
	}

	return ExecuteStep{
		Code:    code,
		Status:  UpdateStepStatusSkipped,
		Summary: updateRecoverySkipSummary(code),
		Details: updateRecoverySkipDetails(r.Info.SourceOperationID, source),
	}
}

func (r *ExecuteRecovery) skippedPlanStep(code string) PlanStep {
	source, ok := r.SourceSteps[code]
	if !ok {
		return PlanStep{
			Code:    code,
			Status:  PlanStatusSkipped,
			Summary: updateRecoverySkipSummary(code),
			Details: fmt.Sprintf("Recovery would reuse this step from source operation %s.", r.Info.SourceOperationID),
		}
	}

	return PlanStep{
		Code:    code,
		Status:  PlanStatusSkipped,
		Summary: updateRecoverySkipSummary(code),
		Details: updateRecoverySkipDetails(r.Info.SourceOperationID, source),
	}
}

func updateRecoveryPlanStep(code string, selection journalusecase.RecoverySelection, req ExecuteRequest) PlanStep {
	switch code {
	case "operation_preflight":
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: "Update preflight would run again",
			Details: fmt.Sprintf("Would reacquire the canonical env and operation lock for contour %s before recovery.", req.Scope),
		}
	case "doctor":
		if req.SkipDoctor {
			return PlanStep{
				Code:    code,
				Status:  PlanStatusSkipped,
				Summary: "Doctor would remain skipped",
				Details: "Recovery would keep the source --skip-doctor decision.",
			}
		}
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: "Doctor would run again",
			Details: "Would rerun the canonical doctor checks before continuing recovery.",
		}
	case "backup_recovery_point":
		if req.SkipBackup {
			return PlanStep{
				Code:    code,
				Status:  PlanStatusSkipped,
				Summary: "Recovery-point creation would remain skipped",
				Details: "Recovery would keep the source --skip-backup decision.",
			}
		}
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: "Recovery-point creation would run again",
			Details: "Would rerun the canonical pre-update recovery-point step before continuing recovery.",
		}
	case "runtime_apply":
		summary := "Runtime apply would run again"
		details := "Would rerun runtime apply with the source update settings."
		if selection.AppliedMode == journalusecase.RecoveryModeResume && selection.ResumeStep == "runtime_apply" {
			summary = "Runtime apply would resume"
			details = "Would continue the update from the runtime apply checkpoint without rerunning earlier completed steps."
		}
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: summary,
			Details: details,
		}
	case "runtime_readiness":
		summary := "Runtime readiness checks would run again"
		details := "Would rerun runtime readiness and HTTP probe verification with the source update settings."
		if selection.AppliedMode == journalusecase.RecoveryModeResume && selection.ResumeStep == "runtime_readiness" {
			summary = "Runtime readiness checks would resume"
			details = "Would continue the update from the runtime readiness checkpoint without rerunning earlier completed steps."
		}
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: summary,
			Details: details,
		}
	default:
		return PlanStep{
			Code:    code,
			Status:  PlanStatusUnknown,
			Summary: "Recovery plan could not classify this step",
			Details: "The update recovery flow encountered an unknown step code.",
			Action:  "Inspect the operation report and rerun the update explicitly if needed.",
		}
	}
}

func updateRecoverySkipSummary(code string) string {
	switch code {
	case "doctor":
		return "Doctor skipped during recovery"
	case "backup_recovery_point":
		return "Recovery-point creation skipped during recovery"
	case "runtime_apply":
		return "Runtime apply skipped during recovery"
	case "runtime_readiness":
		return "Runtime readiness checks skipped during recovery"
	default:
		return "Recovery skipped this step"
	}
}

func updateRecoverySkipDetails(sourceOperationID string, source journalusecase.OperationStep) string {
	details := fmt.Sprintf("Recovery reused the source step from operation %s: [%s] %s.", sourceOperationID, strings.ToUpper(source.Status), source.Summary)
	if strings.TrimSpace(source.Details) != "" {
		details += " " + strings.TrimSpace(source.Details)
	}
	return details
}

func updateStepIndex(code string) int {
	for idx, stepCode := range updateStepOrder {
		if stepCode == code {
			return idx
		}
	}
	return -1
}

func mapReportString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return value
}

func mapReportInt(values map[string]any, key string) int {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func mapReportBool(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	value, _ := values[key].(bool)
	return value
}
