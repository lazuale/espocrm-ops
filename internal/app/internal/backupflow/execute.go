package backupflow

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	domainworkflow "github.com/lazuale/espocrm-ops/internal/domain/workflow"
)

func (s Service) Execute(req Request) (info ExecuteInfo, err error) {
	info = ExecuteInfo{
		Scope:              strings.TrimSpace(req.Scope),
		ProjectDir:         filepath.Clean(req.ProjectDir),
		ComposeFile:        filepath.Clean(req.ComposeFile),
		EnvFile:            strings.TrimSpace(req.EnvFile),
		BackupRoot:         strings.TrimSpace(req.BackupRoot),
		RetentionDays:      req.RetentionDays,
		ConsistentSnapshot: !req.NoStop,
		SkipDB:             req.SkipDB,
		SkipFiles:          req.SkipFiles,
		NoStop:             req.NoStop,
		Warnings:           flagWarnings(req),
	}

	defer func() {
		info.Warnings = dedupeStrings(info.Warnings)
		err = normalizeFailure(err, "backup_failed")
	}()

	if req.SkipDB && req.SkipFiles {
		err = domainfailure.Failure{
			Kind:    domainfailure.KindValidation,
			Code:    "backup_failed",
			Summary: "Nothing to back up",
			Action:  "Keep at least one backup step enabled before rerunning backup.",
			Err:     fmt.Errorf("nothing to back up: --skip-db and --skip-files cannot both be set"),
		}
		info.Steps = append(info.Steps,
			domainworkflow.NewStep("input_validation", domainworkflow.StatusFailed, failureSummary(err, "Backup input validation failed"), err.Error(), failureAction(err, "Fix the backup command flags before rerunning backup.")),
			blockedStep("artifact_allocation", "Artifact allocation did not run because backup input validation failed"),
			blockedStep("runtime_prepare", "Runtime preparation did not run because backup input validation failed"),
			blockedStep("db_backup", "Database backup did not run because backup input validation failed"),
			blockedStep("files_backup", "Files backup did not run because backup input validation failed"),
			blockedStep("finalize", "Manifest finalization did not run because backup input validation failed"),
			blockedStep("retention", "Retention cleanup did not run because backup input validation failed"),
		)
		return info, err
	}

	state, err := allocateBackupExecutionState(req)
	if err != nil {
		info.Steps = append(info.Steps,
			domainworkflow.NewStep("artifact_allocation", domainworkflow.StatusFailed, "Artifact allocation failed", err.Error(), "Resolve the backup directory or filesystem failure before rerunning backup."),
			blockedStep("runtime_prepare", "Runtime preparation did not run because artifact allocation failed"),
			blockedStep("db_backup", "Database backup did not run because artifact allocation failed"),
			blockedStep("files_backup", "Files backup did not run because artifact allocation failed"),
			blockedStep("finalize", "Manifest finalization did not run because artifact allocation failed"),
			blockedStep("retention", "Retention cleanup did not run because artifact allocation failed"),
		)
		return info, domainfailure.Failure{
			Kind:    domainfailure.KindIO,
			Code:    "backup_failed",
			Summary: "Artifact allocation failed",
			Action:  "Resolve the backup directory or filesystem failure before rerunning backup.",
			Err:     err,
		}
	}

	info.CreatedAt = state.createdAt.UTC().Format(time.RFC3339)
	info.ManifestTXTPath = state.set.ManifestTXT.Path
	info.ManifestJSONPath = state.set.ManifestJSON.Path
	if !req.SkipDB {
		info.DBBackupPath = state.set.DBBackup.Path
		info.DBSidecarPath = state.set.DBBackup.Path + ".sha256"
	}
	if !req.SkipFiles {
		info.FilesBackupPath = state.set.FilesBackup.Path
		info.FilesSidecarPath = state.set.FilesBackup.Path + ".sha256"
	}
	info.Steps = append(info.Steps, domainworkflow.NewStep("artifact_allocation", domainworkflow.StatusCompleted, "Artifact allocation completed", allocationDetails(state, info), ""))

	target := runtimeport.Target{ProjectDir: info.ProjectDir, ComposeFile: info.ComposeFile, EnvFile: info.EnvFile}

	var runtimePrep runtimePrepareInfo
	runtimeNeedsReturn := false
	runtimeReturnRecorded := false

	defer cleanupTemps(backupTempPaths(state, req))
	defer func() {
		if err == nil || !runtimeNeedsReturn || runtimeReturnRecorded {
			return
		}

		runtimeReturn, returnErr := s.returnRuntime(target, runtimePrep)
		if returnErr != nil {
			info.Steps = append(info.Steps, domainworkflow.NewStep("runtime_return", domainworkflow.StatusFailed, "Runtime return failed after backup failure", returnErr.Error(), "Restore the stopped application services manually before relying on the contour state."))
			err = errors.Join(err, domainfailure.Failure{
				Kind:    domainfailure.KindExternal,
				Code:    "backup_failed",
				Summary: "Runtime return failed after backup failure",
				Action:  "Restore the stopped application services manually before relying on the contour state.",
				Err:     returnErr,
			})
			return
		}

		info.Steps = append(info.Steps, domainworkflow.NewStep("runtime_return", domainworkflow.StatusCompleted, "Runtime return completed after backup failure", runtimeReturnDetails(runtimeReturn), ""))
		info.Warnings = append(info.Warnings, "Backup failed after stopping application services; the contour runtime was returned to its prior state.")
		runtimeReturnRecorded = true
	}()

	if req.NoStop {
		info.Steps = append(info.Steps, domainworkflow.NewStep("runtime_prepare", domainworkflow.StatusSkipped, "Runtime preparation skipped", runtimePrepareSkippedDetails(req), ""))
	} else {
		runtimePrep, err = s.prepareRuntime(target)
		if err != nil {
			info.Steps = append(info.Steps,
				domainworkflow.NewStep("runtime_prepare", domainworkflow.StatusFailed, "Runtime preparation failed", err.Error(), "Resolve the runtime preparation failure before rerunning backup."),
				blockedStep("db_backup", "Database backup did not run because runtime preparation failed"),
				blockedStep("files_backup", "Files backup did not run because runtime preparation failed"),
				blockedStep("finalize", "Manifest finalization did not run because runtime preparation failed"),
				blockedStep("retention", "Retention cleanup did not run because runtime preparation failed"),
			)
			return info, domainfailure.Failure{
				Kind:    domainfailure.KindExternal,
				Code:    "backup_failed",
				Summary: "Runtime preparation failed",
				Action:  "Resolve the runtime preparation failure before rerunning backup.",
				Err:     err,
			}
		}

		info.AppServicesWereRunning = runtimePrep.AppServicesWereRunning
		state.appServicesWereRunning = runtimePrep.AppServicesWereRunning
		runtimeNeedsReturn = runtimePrep.AppServicesWereRunning
		info.Steps = append(info.Steps, domainworkflow.NewStep("runtime_prepare", domainworkflow.StatusCompleted, "Runtime preparation completed", runtimePrepareDetails(runtimePrep), ""))
	}

	if req.SkipDB {
		info.Steps = append(info.Steps, domainworkflow.NewStep("db_backup", domainworkflow.StatusSkipped, "Database backup skipped", "The database backup was skipped because of --skip-db.", ""))
	} else {
		if err := s.runtime.DumpMySQLDumpGz(target, "db", req.DBUser, req.DBPassword, req.DBName, state.set.DBBackup.Path+".tmp"); err != nil {
			info.Steps = append(info.Steps,
				domainworkflow.NewStep("db_backup", domainworkflow.StatusFailed, "Database backup failed", err.Error(), "Resolve the database dump failure before rerunning backup."),
				blockedStep("files_backup", "Files backup did not run because database backup failed"),
				blockedStep("finalize", "Manifest finalization did not run because database backup failed"),
				blockedStep("retention", "Retention cleanup did not run because database backup failed"),
			)
			return info, domainfailure.Failure{
				Kind:    domainfailure.KindExternal,
				Code:    "backup_failed",
				Summary: "Database backup failed",
				Action:  "Resolve the database dump failure before rerunning backup.",
				Err:     err,
			}
		}
		if err := saveTempFile(state.set.DBBackup.Path+".tmp", state.set.DBBackup.Path, "save db backup"); err != nil {
			info.Steps = append(info.Steps,
				domainworkflow.NewStep("db_backup", domainworkflow.StatusFailed, "Database backup failed", err.Error(), "Resolve the database backup write failure before rerunning backup."),
				blockedStep("files_backup", "Files backup did not run because database backup failed"),
				blockedStep("finalize", "Manifest finalization did not run because database backup failed"),
				blockedStep("retention", "Retention cleanup did not run because database backup failed"),
			)
			return info, domainfailure.Failure{
				Kind:    domainfailure.KindIO,
				Code:    "backup_failed",
				Summary: "Database backup failed",
				Action:  "Resolve the database backup write failure before rerunning backup.",
				Err:     err,
			}
		}

		info.DBBackupCreated = true
		info.Steps = append(info.Steps, domainworkflow.NewStep("db_backup", domainworkflow.StatusCompleted, "Database backup completed", dbBackupDetails(state), ""))
	}

	if req.SkipFiles {
		info.Steps = append(info.Steps, domainworkflow.NewStep("files_backup", domainworkflow.StatusSkipped, "Files backup skipped", "The files backup was skipped because of --skip-files.", ""))
	} else {
		archiveInfo, archiveErr := s.createFilesBackupArchive(req, state.set.FilesBackup.Path+".tmp")
		if archiveErr != nil {
			info.Steps = append(info.Steps,
				domainworkflow.NewStep("files_backup", domainworkflow.StatusFailed, failureSummary(archiveErr, "Files backup failed"), archiveErr.Error(), failureAction(archiveErr, "Resolve the files backup failure before rerunning backup.")),
				blockedStep("finalize", "Manifest finalization did not run because files backup failed"),
				blockedStep("retention", "Retention cleanup did not run because files backup failed"),
			)
			return info, archiveErr
		}
		if err := saveTempFile(state.set.FilesBackup.Path+".tmp", state.set.FilesBackup.Path, "save files backup"); err != nil {
			info.Steps = append(info.Steps,
				domainworkflow.NewStep("files_backup", domainworkflow.StatusFailed, "Files backup failed", err.Error(), "Resolve the files backup write failure before rerunning backup."),
				blockedStep("finalize", "Manifest finalization did not run because files backup failed"),
				blockedStep("retention", "Retention cleanup did not run because files backup failed"),
			)
			return info, domainfailure.Failure{
				Kind:    domainfailure.KindIO,
				Code:    "backup_failed",
				Summary: "Files backup failed",
				Action:  "Resolve the files backup write failure before rerunning backup.",
				Err:     err,
			}
		}

		if archiveInfo.UsedDockerHelper {
			info.Warnings = append(info.Warnings, fmt.Sprintf("Files backup used the Docker helper fallback because local archiving failed for %s.", req.StorageDir))
		}

		info.FilesBackupCreated = true
		info.Steps = append(info.Steps, domainworkflow.NewStep("files_backup", domainworkflow.StatusCompleted, "Files backup completed", filesBackupDetails(state, archiveInfo), ""))
	}

	if err := s.finalizeArtifacts(req, &state, &info); err != nil {
		info.Steps = append(info.Steps,
			domainworkflow.NewStep("finalize", domainworkflow.StatusFailed, "Manifest finalization failed", err.Error(), "Resolve the manifest or checksum write failure before relying on this backup set."),
			blockedStep("retention", "Retention cleanup did not run because manifest finalization failed"),
		)
		return info, domainfailure.Failure{
			Kind:    domainfailure.KindIO,
			Code:    "backup_failed",
			Summary: "Manifest finalization failed",
			Action:  "Resolve the manifest or checksum write failure before relying on this backup set.",
			Err:     err,
		}
	}
	info.Steps = append(info.Steps, domainworkflow.NewStep("finalize", domainworkflow.StatusCompleted, "Manifest finalization completed", finalizeDetails(info), ""))

	if err := cleanupRetention(info.BackupRoot, req.RetentionDays, executeNow(req.Now)); err != nil {
		info.Steps = append(info.Steps, domainworkflow.NewStep("retention", domainworkflow.StatusFailed, "Retention cleanup failed", err.Error(), "Resolve the retention cleanup failure before relying on the backup root state."))
		return info, domainfailure.Failure{
			Kind:    domainfailure.KindIO,
			Code:    "backup_failed",
			Summary: "Retention cleanup failed",
			Action:  "Resolve the retention cleanup failure before relying on the backup root state.",
			Err:     err,
		}
	}
	info.Steps = append(info.Steps, domainworkflow.NewStep("retention", domainworkflow.StatusCompleted, "Retention cleanup completed", retentionDetails(info.BackupRoot, req.RetentionDays), ""))

	if runtimeNeedsReturn {
		runtimeReturn, returnErr := s.returnRuntime(target, runtimePrep)
		if returnErr != nil {
			info.Steps = append(info.Steps, domainworkflow.NewStep("runtime_return", domainworkflow.StatusFailed, "Runtime return failed", returnErr.Error(), "Restore the stopped application services manually before relying on the contour state."))
			return info, domainfailure.Failure{
				Kind:    domainfailure.KindExternal,
				Code:    "backup_failed",
				Summary: "Runtime return failed",
				Action:  "Restore the stopped application services manually before relying on the contour state.",
				Err:     returnErr,
			}
		}

		info.Steps = append(info.Steps, domainworkflow.NewStep("runtime_return", domainworkflow.StatusCompleted, "Runtime return completed", runtimeReturnDetails(runtimeReturn), ""))
		runtimeReturnRecorded = true
	} else {
		info.Steps = append(info.Steps, domainworkflow.NewStep("runtime_return", domainworkflow.StatusSkipped, "Runtime return skipped", runtimeReturnSkippedDetails(req, runtimePrep), ""))
		runtimeReturnRecorded = true
	}

	return info, nil
}

func (i ExecuteInfo) Counts() (completed, skipped, blocked, failed int) {
	for _, step := range i.Steps {
		switch step.Status {
		case domainworkflow.StatusCompleted:
			completed++
		case domainworkflow.StatusSkipped:
			skipped++
		case domainworkflow.StatusBlocked:
			blocked++
		case domainworkflow.StatusFailed:
			failed++
		}
	}

	return completed, skipped, blocked, failed
}

func (i ExecuteInfo) Ready() bool {
	for _, step := range i.Steps {
		if step.Status == domainworkflow.StatusFailed || step.Status == domainworkflow.StatusBlocked {
			return false
		}
	}

	return true
}
