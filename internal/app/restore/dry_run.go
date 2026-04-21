package restore

import (
	"fmt"
	"strings"

	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
)

func (s Service) buildDryRun(ctx maintenanceusecase.OperationContext, req ExecuteRequest, info ExecuteInfo, source executeSource, runtimeInfo runtimePrepareInfo) (ExecuteInfo, error) {
	info.StartedDBTemporarily = runtimeInfo.StartedDBTemporarily
	info.AppServicesWereRunning = runtimeInfo.AppServicesWereRunning
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_prepare",
		Status:  RestoreStepStatusWouldRun,
		Summary: "Runtime preparation would run",
		Details: runtimePrepareDetails(runtimeInfo, req),
	})

	if req.NoSnapshot {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "snapshot_recovery_point",
			Status:  RestoreStepStatusSkipped,
			Summary: "Emergency recovery point skipped",
			Details: "The pre-restore emergency recovery point would be skipped because of --no-snapshot.",
		})
	} else {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "snapshot_recovery_point",
			Status:  RestoreStepStatusWouldRun,
			Summary: "Emergency recovery point would run",
			Details: snapshotPlanDetails(req),
		})
	}

	if req.SkipDB {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_restore",
			Status:  RestoreStepStatusSkipped,
			Summary: "Database restore skipped",
			Details: "The database restore would be skipped because of --skip-db.",
		})
	} else {
		if err := s.dryRunDBChecks(ctx, source, runtimeInfo); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "db_restore",
					Status:  RestoreStepStatusFailed,
					Summary: "Database restore planning failed",
					Details: err.Error(),
					Action:  "Resolve the database restore planning failure before rerunning restore.",
				},
				blockedRestoreStep("files_restore", "Files restore did not run because database restore planning failed"),
				blockedRestoreStep("runtime_return", "Runtime return did not run because database restore planning failed"),
			)
			return info, wrapRestoreExecuteError(err)
		}
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_restore",
			Status:  RestoreStepStatusWouldRun,
			Summary: "Database restore would run",
			Details: dryRunDBDetails(ctx, source, runtimeInfo),
		})
	}

	if req.SkipFiles {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_restore",
			Status:  RestoreStepStatusSkipped,
			Summary: "Files restore skipped",
			Details: "The files restore would be skipped because of --skip-files.",
		})
	} else {
		if err := s.dryRunFilesChecks(ctx, source); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_restore",
					Status:  RestoreStepStatusFailed,
					Summary: "Files restore planning failed",
					Details: err.Error(),
					Action:  "Resolve the files restore planning failure before rerunning restore.",
				},
				blockedRestoreStep("runtime_return", "Runtime return did not run because files restore planning failed"),
			)
			return info, wrapRestoreExecuteError(err)
		}
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_restore",
			Status:  RestoreStepStatusWouldRun,
			Summary: "Files restore would run",
			Details: dryRunFilesDetails(ctx, source),
		})
	}

	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_return",
		Status:  dryRunRuntimeReturnStatus(runtimeInfo, req.NoStart),
		Summary: dryRunRuntimeReturnSummary(runtimeInfo, req.NoStart),
		Details: dryRunRuntimeReturnDetails(runtimeInfo, req.NoStart),
	})
	info.Warnings = dedupeStrings(info.Warnings)
	return info, nil
}

func (s Service) dryRunDBChecks(ctx maintenanceusecase.OperationContext, source executeSource, runtimeInfo runtimePrepareInfo) error {
	lock, err := platformlocks.AcquireRestoreDBLock()
	if err != nil {
		return LockError{Err: fmt.Errorf("db restore lock failed: %w", err)}
	}
	if releaseErr := lock.Release(); releaseErr != nil {
		return fmt.Errorf("release db restore lock: %w", releaseErr)
	}

	req := buildDBRestoreRequest(ctx, source, runtimeInfo.DBContainer)
	req.DryRun = true
	if _, err := configResolveDBPassword(req); err != nil {
		return err
	}
	if _, _, err := resolveDBRootPasswordForPlan(req); err != nil {
		return err
	}

	if runtimeInfo.StartedDBTemporarily {
		if err := platformdocker.CheckDockerAvailable(); err != nil {
			return PreflightError{Err: err}
		}
		return nil
	}

	_, err = s.PreflightDBRestore(DBPreflightRequest{
		DBBackup:    source.DBBackup,
		DBContainer: runtimeInfo.DBContainer,
	})
	return err
}

func (s Service) dryRunFilesChecks(ctx maintenanceusecase.OperationContext, source executeSource) error {
	lock, err := platformlocks.AcquireRestoreFilesLock()
	if err != nil {
		return LockError{Err: fmt.Errorf("files restore lock failed: %w", err)}
	}
	if releaseErr := lock.Release(); releaseErr != nil {
		return fmt.Errorf("release files restore lock: %w", releaseErr)
	}

	targetDir := platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir())
	_, err = s.PreflightFilesRestore(FilesPreflightRequest{
		FilesBackup: source.FilesBackup,
		TargetDir:   targetDir,
	})
	return err
}

func snapshotPlanDetails(req ExecuteRequest) string {
	switch {
	case req.SkipDB:
		return "Would create a files-only emergency recovery point before the destructive restore step."
	case req.SkipFiles:
		return "Would create a database-only emergency recovery point before the destructive restore step."
	default:
		return "Would create a full emergency recovery point before the destructive restore steps."
	}
}

func dryRunDBDetails(ctx maintenanceusecase.OperationContext, source executeSource, runtimeInfo runtimePrepareInfo) string {
	details := fmt.Sprintf("Would restore database %s from %s.", strings.TrimSpace(ctx.Env.Value("DB_NAME")), source.DBBackup)
	if strings.TrimSpace(runtimeInfo.DBContainer) != "" {
		details += fmt.Sprintf(" The current db container resolves to %s.", runtimeInfo.DBContainer)
	} else if runtimeInfo.StartedDBTemporarily {
		details += " Runtime preparation would start the db service first."
	}
	return details
}

func dryRunFilesDetails(ctx maintenanceusecase.OperationContext, source executeSource) string {
	targetDir := platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir())
	return fmt.Sprintf("Would replace %s from %s and then reconcile the storage permissions to the runtime image contract.", targetDir, source.FilesBackup)
}

func dryRunRuntimeReturnStatus(runtimeInfo runtimePrepareInfo, noStart bool) string {
	if len(runtimeInfo.StoppedAppServices) == 0 && !runtimeInfo.StartedDBTemporarily {
		return RestoreStepStatusSkipped
	}
	if len(runtimeInfo.StoppedAppServices) != 0 && noStart {
		return RestoreStepStatusSkipped
	}
	return RestoreStepStatusWouldRun
}

func dryRunRuntimeReturnSummary(runtimeInfo runtimePrepareInfo, noStart bool) string {
	if dryRunRuntimeReturnStatus(runtimeInfo, noStart) == RestoreStepStatusWouldRun {
		return "Runtime return would run"
	}
	return "Runtime return skipped"
}

func dryRunRuntimeReturnDetails(runtimeInfo runtimePrepareInfo, noStart bool) string {
	switch {
	case len(runtimeInfo.StoppedAppServices) != 0 && !noStart:
		return fmt.Sprintf("Would restart application services after restore: %s.", strings.Join(runtimeInfo.StoppedAppServices, ", "))
	case runtimeInfo.StartedDBTemporarily && len(runtimeInfo.StoppedAppServices) == 0:
		return "Would stop the db service again after restore to return the contour to its prior stopped state."
	case len(runtimeInfo.StoppedAppServices) != 0 && noStart:
		return "Application services would remain stopped because of --no-start."
	default:
		return "The contour runtime state would already match the requested post-restore state."
	}
}
