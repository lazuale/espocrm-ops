package restore

import (
	"errors"
	"fmt"
	"strings"

	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
)

func runtimePrepareDetails(info runtimePrepareInfo, req ExecuteRequest) string {
	parts := []string{}
	if info.StartedDBTemporarily {
		parts = append(parts, "The db service was started temporarily for restore readiness.")
	} else if strings.TrimSpace(info.DBContainer) != "" {
		parts = append(parts, fmt.Sprintf("Using database container %s.", info.DBContainer))
	}
	if info.AppServicesWereRunning {
		if req.NoStop {
			parts = append(parts, fmt.Sprintf("Application services remained running because of --no-stop: %s.", strings.Join(info.StoppedAppServices, ", ")))
		} else {
			parts = append(parts, fmt.Sprintf("Stopped application services before restore: %s.", strings.Join(info.StoppedAppServices, ", ")))
		}
	} else {
		parts = append(parts, "Application services were already stopped before restore.")
	}

	return strings.Join(parts, " ")
}

func snapshotDetails(info snapshotBackupInfo) string {
	parts := []string{fmt.Sprintf("Created emergency recovery point at %s.", info.ManifestJSONPath)}
	if info.DBBackupPath != "" {
		parts = append(parts, fmt.Sprintf("Database snapshot: %s.", info.DBBackupPath))
	}
	if info.FilesBackupPath != "" {
		parts = append(parts, fmt.Sprintf("Files snapshot: %s.", info.FilesBackupPath))
	}
	return strings.Join(parts, " ")
}

func dbRestoreDetails(ctx maintenanceusecase.OperationContext, source executeSource, dbContainer string) string {
	details := fmt.Sprintf("Restored database %s in container %s from %s.", strings.TrimSpace(ctx.Env.Value("DB_NAME")), dbContainer, source.DBBackup)
	if strings.TrimSpace(source.ManifestJSON) != "" {
		details += fmt.Sprintf(" The manifest %s anchored the selected backup set.", source.ManifestJSON)
	}
	return details
}

func filesRestoreDetails(ctx maintenanceusecase.OperationContext, source executeSource) string {
	targetDir := platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir())
	details := fmt.Sprintf("Replaced %s from %s and reconciled the storage permissions to the runtime image contract.", targetDir, source.FilesBackup)
	if strings.TrimSpace(source.ManifestJSON) != "" {
		details += fmt.Sprintf(" The manifest %s anchored the selected backup set.", source.ManifestJSON)
	}
	return details
}

func runtimeReturnStatus(req ExecuteRequest, prep runtimePrepareInfo, ret runtimeReturnInfo, validatedServices []string) string {
	if len(validatedServices) != 0 || len(ret.RestartedAppServices) != 0 || ret.StoppedDB {
		return RestoreStepStatusCompleted
	}
	if len(prep.StoppedAppServices) != 0 && req.NoStart && !req.NoStop {
		return RestoreStepStatusSkipped
	}
	return RestoreStepStatusSkipped
}

func runtimeReturnSummary(req ExecuteRequest, prep runtimePrepareInfo, ret runtimeReturnInfo, validatedServices []string) string {
	switch {
	case len(validatedServices) != 0:
		return "Runtime return completed"
	case len(ret.RestartedAppServices) != 0:
		return "Runtime return completed"
	case ret.StoppedDB:
		return "Runtime return completed"
	case len(prep.StoppedAppServices) != 0 && req.NoStart && !req.NoStop:
		return "Runtime return skipped"
	default:
		return "Runtime return skipped"
	}
}

func runtimeReturnDetails(req ExecuteRequest, prep runtimePrepareInfo, ret runtimeReturnInfo, validatedServices []string) string {
	var details string
	switch {
	case len(ret.RestartedAppServices) != 0:
		details = fmt.Sprintf("Restarted application services after restore: %s.", strings.Join(ret.RestartedAppServices, ", "))
	case req.NoStop && len(prep.StoppedAppServices) != 0:
		details = fmt.Sprintf("Application services remained running because of --no-stop: %s.", strings.Join(prep.StoppedAppServices, ", "))
	case ret.StoppedDB:
		details = "Stopped the db service again because restore had started it temporarily and the contour had been stopped beforehand."
	case len(prep.StoppedAppServices) != 0 && req.NoStart && !req.NoStop:
		details = "Application services were left stopped because of --no-start."
	default:
		details = "The contour runtime state already matched the requested post-restore state."
	}

	if len(validatedServices) != 0 {
		details += fmt.Sprintf(" Post-restore health validation passed for: %s.", strings.Join(validatedServices, ", "))
	}

	return details
}

func flagWarnings(req ExecuteRequest) []string {
	warnings := []string{}
	if req.NoSnapshot {
		warnings = append(warnings, "Restore will skip the emergency recovery point because of --no-snapshot.")
	}
	if req.NoStop {
		warnings = append(warnings, "Restore will run without stopping application services because of --no-stop.")
	}
	if req.NoStart {
		warnings = append(warnings, "Restore will leave application services stopped after completion because of --no-start.")
	}
	if req.SkipDB {
		warnings = append(warnings, "Restore will skip the database step because of --skip-db.")
	}
	if req.SkipFiles {
		warnings = append(warnings, "Restore will skip the files step because of --skip-files.")
	}
	return warnings
}

func blockedRestoreStep(code, summary string) ExecuteStep {
	return ExecuteStep{
		Code:    code,
		Status:  RestoreStepStatusBlocked,
		Summary: summary,
	}
}

func failureSummary(err error, fallback string) string {
	var failure executeFailure
	if errors.As(err, &failure) && strings.TrimSpace(failure.Summary) != "" {
		return failure.Summary
	}
	return fallback
}

func failureAction(err error, fallback string) string {
	var failure executeFailure
	if errors.As(err, &failure) && strings.TrimSpace(failure.Action) != "" {
		return failure.Action
	}
	return fallback
}
