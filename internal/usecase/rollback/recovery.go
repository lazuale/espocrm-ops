package rollback

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
)

var rollbackStepOrder = []string{
	"operation_preflight",
	"doctor",
	"target_selection",
	"runtime_prepare",
	"snapshot_recovery_point",
	"db_restore",
	"files_restore",
	"runtime_return",
}

type recoveryTarget struct {
	SelectionMode string
	Prefix        string
	Stamp         string
	ManifestJSON  string
	DBBackup      string
	FilesBackup   string
}

type recoverySnapshot struct {
	ManifestTXT   string
	ManifestJSON  string
	DBBackup      string
	FilesBackup   string
	DBChecksum    string
	FilesChecksum string
}

type ExecuteRecovery struct {
	Info                 operationusecase.RecoveryInfo
	SourceSteps          map[string]journalusecase.OperationStep
	CheckpointIndex      int
	FailureIndex         int
	SelectedTarget       recoveryTarget
	Snapshot             recoverySnapshot
	StartedDBTemporarily bool
}

func RecoverPlan(report journalusecase.OperationReport, selection journalusecase.RecoverySelection) (RollbackPlan, error) {
	req, err := buildRecoveryRequest(report, selection, nil)
	if err != nil {
		return RollbackPlan{}, err
	}

	plan := RollbackPlan{
		Scope:           strings.TrimSpace(req.Scope),
		ProjectDir:      req.ProjectDir,
		ComposeFile:     req.ComposeFile,
		EnvFile:         strings.TrimSpace(req.EnvFileOverride),
		TimeoutSeconds:  req.TimeoutSeconds,
		SnapshotEnabled: !req.NoSnapshot,
		NoStart:         req.NoStart,
		SkipHTTPProbe:   req.SkipHTTPProbe,
		SelectionMode:   req.Recovery.SelectedTarget.SelectionMode,
		SelectedPrefix:  req.Recovery.SelectedTarget.Prefix,
		SelectedStamp:   req.Recovery.SelectedTarget.Stamp,
		ManifestJSON:    req.Recovery.SelectedTarget.ManifestJSON,
		DBBackup:        req.Recovery.SelectedTarget.DBBackup,
		FilesBackup:     req.Recovery.SelectedTarget.FilesBackup,
		Recovery:        req.Recovery.Info,
	}

	for _, code := range rollbackStepOrder {
		if req.Recovery.shouldSkip(code) {
			plan.Steps = append(plan.Steps, req.Recovery.skippedPlanStep(code))
			continue
		}
		plan.Steps = append(plan.Steps, rollbackRecoveryPlanStep(code, selection, req))
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
	if report.Command != "rollback" {
		return ExecuteRequest{}, apperr.Wrap(apperr.KindValidation, "rollback_recovery_refused", fmt.Errorf("operation %s is not a rollback run", report.OperationID))
	}

	projectDir := strings.TrimSpace(mapReportString(report.Artifacts, "project_dir"))
	composeFile := strings.TrimSpace(mapReportString(report.Artifacts, "compose_file"))
	envFile := strings.TrimSpace(mapReportString(report.Artifacts, "env_file"))
	if projectDir == "" || composeFile == "" || envFile == "" || strings.TrimSpace(report.Scope) == "" {
		return ExecuteRequest{}, apperr.Wrap(apperr.KindValidation, "rollback_recovery_refused", fmt.Errorf("operation %s is missing canonical rollback context needed for recovery", report.OperationID))
	}

	requestedMode := rollbackRequestedSelectionMode(report)
	selected := selectedTargetFromReport(report)
	dbBackup, filesBackup := selected.DBBackup, selected.FilesBackup
	if dbBackup == "" || filesBackup == "" {
		dbBackup = mapReportString(report.Artifacts, "requested_db_backup")
		filesBackup = mapReportString(report.Artifacts, "requested_files_backup")
	}

	return ExecuteRequest{
		Scope:           strings.TrimSpace(report.Scope),
		ProjectDir:      projectDir,
		ComposeFile:     composeFile,
		EnvFileOverride: envFile,
		DBBackup:        dbBackup,
		FilesBackup:     filesBackup,
		NoSnapshot:      !mapReportBool(report.Details, "snapshot_enabled"),
		NoStart:         mapReportBool(report.Details, "no_start"),
		SkipHTTPProbe:   mapReportBool(report.Details, "skip_http_probe"),
		TimeoutSeconds:  mapReportInt(report.Details, "timeout_seconds"),
		LogWriter:       logWriter,
		Recovery: buildExecuteRecovery(report, selection, requestedMode, selected, recoverySnapshot{
			ManifestTXT:   mapReportString(report.Artifacts, "snapshot_manifest_txt"),
			ManifestJSON:  mapReportString(report.Artifacts, "snapshot_manifest_json"),
			DBBackup:      mapReportString(report.Artifacts, "snapshot_db_backup"),
			FilesBackup:   mapReportString(report.Artifacts, "snapshot_files_backup"),
			DBChecksum:    mapReportString(report.Artifacts, "snapshot_db_checksum"),
			FilesChecksum: mapReportString(report.Artifacts, "snapshot_files_checksum"),
		}),
	}, nil
}

func buildExecuteRecovery(report journalusecase.OperationReport, selection journalusecase.RecoverySelection, requestedMode string, selected recoveryTarget, snapshot recoverySnapshot) *ExecuteRecovery {
	sourceSteps := make(map[string]journalusecase.OperationStep, len(report.Steps))
	for _, step := range report.Steps {
		sourceSteps[step.Code] = step
	}

	if selected.SelectionMode == "" {
		selected.SelectionMode = requestedMode
	}

	return &ExecuteRecovery{
		Info: operationusecase.RecoveryInfo{
			SourceOperationID: report.OperationID,
			RequestedMode:     selection.RequestedMode,
			AppliedMode:       selection.AppliedMode,
			Decision:          selection.Decision,
			ResumeStep:        selection.ResumeStep,
		},
		SourceSteps:          sourceSteps,
		CheckpointIndex:      rollbackStepIndex(selection.ResumeStep),
		FailureIndex:         rollbackFailureIndex(report.Steps),
		SelectedTarget:       selected,
		Snapshot:             snapshot,
		StartedDBTemporarily: mapReportBool(report.Details, "started_db_temporarily"),
	}
}

func selectedTargetFromReport(report journalusecase.OperationReport) recoveryTarget {
	target := recoveryTarget{
		SelectionMode: rollbackRequestedSelectionMode(report),
	}
	if report.Target == nil {
		return target
	}

	target.SelectionMode = report.Target.SelectionMode
	target.Prefix = report.Target.Prefix
	target.Stamp = report.Target.Stamp
	target.ManifestJSON = report.Target.ManifestJSON
	target.DBBackup = report.Target.DBBackup
	target.FilesBackup = report.Target.FilesBackup
	return target
}

func rollbackRequestedSelectionMode(report journalusecase.OperationReport) string {
	mode := strings.TrimSpace(mapReportString(report.Details, "requested_selection_mode"))
	if mode != "" {
		return mode
	}
	if strings.TrimSpace(mapReportString(report.Artifacts, "requested_db_backup")) != "" {
		return "explicit"
	}
	if report.Target != nil && strings.TrimSpace(report.Target.SelectionMode) != "" {
		return report.Target.SelectionMode
	}
	if strings.TrimSpace(mapReportString(report.Details, "selection_mode")) != "" {
		return mapReportString(report.Details, "selection_mode")
	}
	return ""
}

func rollbackFailureIndex(steps []journalusecase.OperationStep) int {
	for _, step := range steps {
		if step.Status == "failed" || step.Status == "blocked" {
			return rollbackStepIndex(step.Code)
		}
	}
	return len(rollbackStepOrder)
}

func (r *ExecuteRecovery) shouldSkip(code string) bool {
	if r == nil || r.Info.Decision != journalusecase.RecoveryDecisionResumeFromCheckpoint {
		return false
	}

	index := rollbackStepIndex(code)
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
			Status:  RollbackStepStatusSkipped,
			Summary: rollbackRecoverySkipSummary(code),
			Details: fmt.Sprintf("Recovery reused this step from source operation %s.", r.Info.SourceOperationID),
		}
	}

	return ExecuteStep{
		Code:    code,
		Status:  RollbackStepStatusSkipped,
		Summary: rollbackRecoverySkipSummary(code),
		Details: rollbackRecoverySkipDetails(r.Info.SourceOperationID, source),
	}
}

func (r *ExecuteRecovery) skippedPlanStep(code string) PlanStep {
	source, ok := r.SourceSteps[code]
	if !ok {
		return PlanStep{
			Code:    code,
			Status:  PlanStatusSkipped,
			Summary: rollbackRecoverySkipSummary(code),
			Details: fmt.Sprintf("Recovery would reuse this step from source operation %s.", r.Info.SourceOperationID),
		}
	}

	return PlanStep{
		Code:    code,
		Status:  PlanStatusSkipped,
		Summary: rollbackRecoverySkipSummary(code),
		Details: rollbackRecoverySkipDetails(r.Info.SourceOperationID, source),
	}
}

func rollbackRecoveryPlanStep(code string, selection journalusecase.RecoverySelection, req ExecuteRequest) PlanStep {
	switch code {
	case "operation_preflight":
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: "Rollback preflight would run again",
			Details: fmt.Sprintf("Would reacquire the canonical env and operation lock for contour %s before recovery.", req.Scope),
		}
	case "doctor":
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: "Doctor would run again",
			Details: "Would rerun the canonical rollback doctor checks before continuing recovery.",
		}
	case "target_selection":
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: "Rollback target selection would run again",
			Details: "Would rebuild the rollback target selection from the source operation intent.",
		}
	case "runtime_prepare":
		summary := "Runtime preparation would run again"
		details := "Would rerun runtime preparation before continuing rollback recovery."
		if selection.AppliedMode == journalusecase.RecoveryModeResume && selection.ResumeStep == "runtime_prepare" {
			summary = "Runtime preparation would resume"
			details = "Would continue rollback recovery from runtime preparation using the source rollback target."
		}
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: summary,
			Details: details,
		}
	case "snapshot_recovery_point":
		if req.NoSnapshot {
			return PlanStep{
				Code:    code,
				Status:  PlanStatusSkipped,
				Summary: "Emergency recovery-point creation would remain skipped",
				Details: "Recovery would keep the source --no-snapshot decision.",
			}
		}
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: "Emergency recovery-point creation would run again",
			Details: "Would continue the emergency recovery-point step if it was not already completed in the source rollback.",
		}
	case "db_restore":
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: "Database restore would run again",
			Details: "Would continue rollback database restore with the source target backup set.",
		}
	case "files_restore":
		return PlanStep{
			Code:    code,
			Status:  PlanStatusWouldRun,
			Summary: "Files restore would run again",
			Details: "Would continue rollback files restore with the source target backup set.",
		}
	case "runtime_return":
		summary := "Contour return would run again"
		details := "Would rerun the contour return step after restore completion."
		if selection.AppliedMode == journalusecase.RecoveryModeResume && selection.ResumeStep == "runtime_return" {
			summary = "Contour return would resume"
			details = "Would continue rollback recovery from contour return without repeating earlier completed restore steps."
		}
		if req.NoStart {
			return PlanStep{
				Code:    code,
				Status:  PlanStatusSkipped,
				Summary: "Contour return would remain skipped",
				Details: "Recovery would keep the source --no-start decision.",
			}
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
			Details: "The rollback recovery flow encountered an unknown step code.",
			Action:  "Inspect the operation report and rerun rollback explicitly if needed.",
		}
	}
}

func rollbackRecoverySkipSummary(code string) string {
	switch code {
	case "doctor":
		return "Doctor skipped during recovery"
	case "target_selection":
		return "Rollback target selection skipped during recovery"
	case "runtime_prepare":
		return "Runtime preparation skipped during recovery"
	case "snapshot_recovery_point":
		return "Emergency recovery-point creation skipped during recovery"
	case "db_restore":
		return "Database restore skipped during recovery"
	case "files_restore":
		return "Files restore skipped during recovery"
	case "runtime_return":
		return "Contour return skipped during recovery"
	default:
		return "Recovery skipped this step"
	}
}

func rollbackRecoverySkipDetails(sourceOperationID string, source journalusecase.OperationStep) string {
	details := fmt.Sprintf("Recovery reused the source step from operation %s: [%s] %s.", sourceOperationID, strings.ToUpper(source.Status), source.Summary)
	if strings.TrimSpace(source.Details) != "" {
		details += " " + strings.TrimSpace(source.Details)
	}
	return details
}

func rollbackStepIndex(code string) int {
	for idx, stepCode := range rollbackStepOrder {
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
