package restore

import (
	"fmt"
	"strings"

	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	domainworkflow "github.com/lazuale/espocrm-ops/internal/domain/workflow"
)

func (s Service) buildDryRun(ctx maintenanceusecase.OperationContext, req ExecuteRequest, info ExecuteInfo, source executeSource, runtimeInfo runtimePrepareInfo) (ExecuteInfo, error) {
	info.StartedDBTemporarily = runtimeInfo.StartedDBTemporarily
	info.AppServicesWereRunning = runtimeInfo.AppServicesWereRunning
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_prepare",
		Status:  domainworkflow.StatusPlanned,
		Summary: "Runtime preparation planned",
		Details: runtimePrepareDetails(runtimeInfo, req),
	})

	if req.NoSnapshot {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "snapshot_recovery_point",
			Status:  domainworkflow.StatusSkipped,
			Summary: "Emergency recovery point skipped",
			Details: "The pre-restore emergency recovery point would be skipped because of --no-snapshot.",
		})
	} else {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "snapshot_recovery_point",
			Status:  domainworkflow.StatusPlanned,
			Summary: "Emergency recovery point planned",
			Details: snapshotPlanDetails(req),
		})
	}

	if req.SkipDB {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_restore",
			Status:  domainworkflow.StatusSkipped,
			Summary: "Database restore skipped",
			Details: "The database restore would be skipped because of --skip-db.",
		})
	} else {
		if err := s.dryRunDBChecks(ctx, source, runtimeInfo); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "db_restore",
					Status:  domainworkflow.StatusFailed,
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
			Status:  domainworkflow.StatusPlanned,
			Summary: "Database restore planned",
			Details: dryRunDBDetails(ctx, source, runtimeInfo),
		})
	}

	if req.SkipFiles {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_restore",
			Status:  domainworkflow.StatusSkipped,
			Summary: "Files restore skipped",
			Details: "The files restore would be skipped because of --skip-files.",
		})
	} else {
		filesReq := s.BuildFilesRestoreRequest(ctx, source.ManifestJSON, source.FilesBackup)
		if err := s.dryRunFilesChecks(source, filesReq); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_restore",
					Status:  domainworkflow.StatusFailed,
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
			Status:  domainworkflow.StatusPlanned,
			Summary: "Files restore planned",
			Details: dryRunFilesDetails(source, filesReq.TargetDir),
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
	lock, err := s.locks.AcquireRestoreDBLock()
	if err != nil {
		return restoreFailure(domainfailure.KindConflict, "lock_acquire_failed", fmt.Errorf("db restore lock failed: %w", err))
	}
	if releaseErr := lock.Release(); releaseErr != nil {
		return fmt.Errorf("release db restore lock: %w", releaseErr)
	}

	req := s.BuildDBRestoreRequest(ctx, source.ManifestJSON, source.DBBackup, runtimeInfo.DBContainer)
	req.DryRun = true
	if _, err := s.resolveDBPassword(req); err != nil {
		return err
	}
	if _, _, err := s.resolveDBRootPasswordForPlan(req); err != nil {
		return err
	}

	if runtimeInfo.StartedDBTemporarily {
		if err := s.runtime.CheckDockerAvailable(); err != nil {
			return restoreFailure(domainfailure.KindExternal, "restore_db_failed", err)
		}
		return nil
	}

	_, err = s.PreflightDBRestore(DBPreflightRequest{
		DBBackup:    source.DBBackup,
		DBContainer: runtimeInfo.DBContainer,
	})
	return err
}

func (s Service) dryRunFilesChecks(source executeSource, filesReq RestoreFilesRequest) error {
	lock, err := s.locks.AcquireRestoreFilesLock()
	if err != nil {
		return restoreFailure(domainfailure.KindConflict, "lock_acquire_failed", fmt.Errorf("files restore lock failed: %w", err))
	}
	if releaseErr := lock.Release(); releaseErr != nil {
		return fmt.Errorf("release files restore lock: %w", releaseErr)
	}

	_, err = s.PreflightFilesRestore(FilesPreflightRequest{
		ManifestPath: filesReq.ManifestPath,
		FilesBackup:  filesReq.FilesBackup,
		TargetDir:    filesReq.TargetDir,
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

func dryRunFilesDetails(source executeSource, targetDir string) string {
	return fmt.Sprintf("Would replace %s from %s and then reconcile the storage permissions to the runtime image contract.", targetDir, source.FilesBackup)
}

func dryRunRuntimeReturnStatus(runtimeInfo runtimePrepareInfo, noStart bool) domainworkflow.Status {
	if len(runtimeInfo.StoppedAppServices) == 0 && !runtimeInfo.StartedDBTemporarily {
		return domainworkflow.StatusSkipped
	}
	if len(runtimeInfo.StoppedAppServices) != 0 && noStart {
		return domainworkflow.StatusSkipped
	}
	return domainworkflow.StatusPlanned
}

func dryRunRuntimeReturnSummary(runtimeInfo runtimePrepareInfo, noStart bool) string {
	if dryRunRuntimeReturnStatus(runtimeInfo, noStart) == domainworkflow.StatusPlanned {
		return "Runtime return planned"
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
