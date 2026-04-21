package backup

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
)

const (
	BackupStepStatusCompleted = "completed"
	BackupStepStatusSkipped   = "skipped"
	BackupStepStatusFailed    = "failed"
	BackupStepStatusNotRun    = "not_run"
)

type ExecuteRequest struct {
	Scope          string
	ProjectDir     string
	ComposeFile    string
	EnvFile        string
	BackupRoot     string
	StorageDir     string
	NamePrefix     string
	RetentionDays  int
	ComposeProject string
	DBUser         string
	DBPassword     string
	DBName         string
	EspoCRMImage   string
	MariaDBTag     string
	SkipDB         bool
	SkipFiles      bool
	NoStop         bool
	Now            func() time.Time
}

type ExecuteStep struct {
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type ExecuteInfo struct {
	Scope                  string
	ProjectDir             string
	ComposeFile            string
	EnvFile                string
	BackupRoot             string
	CreatedAt              string
	RetentionDays          int
	ConsistentSnapshot     bool
	AppServicesWereRunning bool
	DBBackupCreated        bool
	FilesBackupCreated     bool
	SkipDB                 bool
	SkipFiles              bool
	NoStop                 bool
	ManifestTXTPath        string
	ManifestJSONPath       string
	DBBackupPath           string
	FilesBackupPath        string
	DBSidecarPath          string
	FilesSidecarPath       string
	Warnings               []string
	Steps                  []ExecuteStep
}

type executeFailure struct {
	Kind    apperr.Kind
	Summary string
	Action  string
	Err     error
}

func (e executeFailure) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e executeFailure) Unwrap() error {
	return e.Err
}

// Execute runs the prepared backup workflow.
//
// Backup is intentionally kept as a prepared worker boundary: the CLI owns
// maintenance preflight and env-derived config assembly so restore can reuse
// the same backup execution path for emergency recovery points without a second
// wrapper layer.
func Execute(req ExecuteRequest) (info ExecuteInfo, err error) {
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
	}()

	if req.SkipDB && req.SkipFiles {
		err = executeFailure{
			Kind:    apperr.KindValidation,
			Summary: "Nothing to back up",
			Action:  "Keep at least one backup step enabled before rerunning backup.",
			Err:     fmt.Errorf("nothing to back up: --skip-db and --skip-files cannot both be set"),
		}
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "input_validation",
				Status:  BackupStepStatusFailed,
				Summary: failureSummary(err, "Backup input validation failed"),
				Details: err.Error(),
				Action:  failureAction(err, "Fix the backup command flags before rerunning backup."),
			},
			notRunBackupStep("artifact_allocation", "Artifact allocation did not run because backup input validation failed"),
			notRunBackupStep("runtime_prepare", "Runtime preparation did not run because backup input validation failed"),
			notRunBackupStep("db_backup", "Database backup did not run because backup input validation failed"),
			notRunBackupStep("files_backup", "Files backup did not run because backup input validation failed"),
			notRunBackupStep("finalize", "Manifest finalization did not run because backup input validation failed"),
			notRunBackupStep("retention", "Retention cleanup did not run because backup input validation failed"),
		)
		return info, wrapBackupExecuteError(err)
	}

	state, err := allocateBackupExecutionState(req)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "artifact_allocation",
				Status:  BackupStepStatusFailed,
				Summary: "Artifact allocation failed",
				Details: err.Error(),
				Action:  "Resolve the backup directory or filesystem failure before rerunning backup.",
			},
			notRunBackupStep("runtime_prepare", "Runtime preparation did not run because artifact allocation failed"),
			notRunBackupStep("db_backup", "Database backup did not run because artifact allocation failed"),
			notRunBackupStep("files_backup", "Files backup did not run because artifact allocation failed"),
			notRunBackupStep("finalize", "Manifest finalization did not run because artifact allocation failed"),
			notRunBackupStep("retention", "Retention cleanup did not run because artifact allocation failed"),
		)
		return info, wrapBackupExecuteError(executeFailure{
			Kind:    apperr.KindIO,
			Summary: "Artifact allocation failed",
			Action:  "Resolve the backup directory or filesystem failure before rerunning backup.",
			Err:     err,
		})
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
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "artifact_allocation",
		Status:  BackupStepStatusCompleted,
		Summary: "Artifact allocation completed",
		Details: allocationDetails(state, info),
	})

	cfg := platformdocker.ComposeConfig{
		ProjectDir:  info.ProjectDir,
		ComposeFile: info.ComposeFile,
		EnvFile:     info.EnvFile,
	}

	var runtimePrep runtimePrepareInfo
	runtimeNeedsReturn := false
	runtimeReturnRecorded := false

	defer cleanupBackupTemps(backupTempPaths(state, req))
	defer func() {
		if err == nil || !runtimeNeedsReturn || runtimeReturnRecorded {
			return
		}

		runtimeReturn, returnErr := returnRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile, runtimePrep)
		if returnErr != nil {
			info.Steps = append(info.Steps, ExecuteStep{
				Code:    "runtime_return",
				Status:  BackupStepStatusFailed,
				Summary: "Runtime return failed after backup failure",
				Details: returnErr.Error(),
				Action:  "Restore the stopped application services manually before relying on the contour state.",
			})
			err = errors.Join(err, executeFailure{
				Kind:    apperr.KindExternal,
				Summary: "Runtime return failed after backup failure",
				Action:  "Restore the stopped application services manually before relying on the contour state.",
				Err:     returnErr,
			})
			return
		}

		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_return",
			Status:  BackupStepStatusCompleted,
			Summary: "Runtime return completed after backup failure",
			Details: runtimeReturnDetails(runtimeReturn),
		})
		info.Warnings = append(info.Warnings, "Backup failed after stopping application services; the contour runtime was returned to its prior state.")
		runtimeReturnRecorded = true
	}()

	if req.NoStop {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_prepare",
			Status:  BackupStepStatusSkipped,
			Summary: "Runtime preparation skipped",
			Details: runtimePrepareSkippedDetails(req),
		})
	} else {
		runtimePrep, err = prepareRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile)
		if err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "runtime_prepare",
					Status:  BackupStepStatusFailed,
					Summary: "Runtime preparation failed",
					Details: err.Error(),
					Action:  "Resolve the runtime preparation failure before rerunning backup.",
				},
				notRunBackupStep("db_backup", "Database backup did not run because runtime preparation failed"),
				notRunBackupStep("files_backup", "Files backup did not run because runtime preparation failed"),
				notRunBackupStep("finalize", "Manifest finalization did not run because runtime preparation failed"),
				notRunBackupStep("retention", "Retention cleanup did not run because runtime preparation failed"),
			)
			return info, wrapBackupExecuteError(executeFailure{
				Kind:    apperr.KindExternal,
				Summary: "Runtime preparation failed",
				Action:  "Resolve the runtime preparation failure before rerunning backup.",
				Err:     err,
			})
		}

		info.AppServicesWereRunning = runtimePrep.AppServicesWereRunning
		state.appServicesWereRunning = runtimePrep.AppServicesWereRunning
		runtimeNeedsReturn = runtimePrep.AppServicesWereRunning
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_prepare",
			Status:  BackupStepStatusCompleted,
			Summary: "Runtime preparation completed",
			Details: runtimePrepareDetails(runtimePrep),
		})
	}

	if req.SkipDB {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_backup",
			Status:  BackupStepStatusSkipped,
			Summary: "Database backup skipped",
			Details: "The database backup was skipped because of --skip-db.",
		})
	} else {
		if err := platformdocker.DumpMySQLDumpGz(cfg, "db", req.DBUser, req.DBPassword, req.DBName, state.set.DBBackup.Path+".tmp"); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "db_backup",
					Status:  BackupStepStatusFailed,
					Summary: "Database backup failed",
					Details: err.Error(),
					Action:  "Resolve the database dump failure before rerunning backup.",
				},
				notRunBackupStep("files_backup", "Files backup did not run because database backup failed"),
				notRunBackupStep("finalize", "Manifest finalization did not run because database backup failed"),
				notRunBackupStep("retention", "Retention cleanup did not run because database backup failed"),
			)
			return info, wrapBackupExecuteError(executeFailure{
				Kind:    apperr.KindExternal,
				Summary: "Database backup failed",
				Action:  "Resolve the database dump failure before rerunning backup.",
				Err:     err,
			})
		}
		if err := saveTempFile(state.set.DBBackup.Path+".tmp", state.set.DBBackup.Path, "save db backup"); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "db_backup",
					Status:  BackupStepStatusFailed,
					Summary: "Database backup failed",
					Details: err.Error(),
					Action:  "Resolve the database backup write failure before rerunning backup.",
				},
				notRunBackupStep("files_backup", "Files backup did not run because database backup failed"),
				notRunBackupStep("finalize", "Manifest finalization did not run because database backup failed"),
				notRunBackupStep("retention", "Retention cleanup did not run because database backup failed"),
			)
			return info, wrapBackupExecuteError(executeFailure{
				Kind:    apperr.KindIO,
				Summary: "Database backup failed",
				Action:  "Resolve the database backup write failure before rerunning backup.",
				Err:     err,
			})
		}

		info.DBBackupCreated = true
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_backup",
			Status:  BackupStepStatusCompleted,
			Summary: "Database backup completed",
			Details: dbBackupDetails(state),
		})
	}

	if req.SkipFiles {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_backup",
			Status:  BackupStepStatusSkipped,
			Summary: "Files backup skipped",
			Details: "The files backup was skipped because of --skip-files.",
		})
	} else {
		archiveInfo, archiveErr := createFilesBackupArchive(req, state.set.FilesBackup.Path+".tmp")
		if archiveErr != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_backup",
					Status:  BackupStepStatusFailed,
					Summary: failureSummary(archiveErr, "Files backup failed"),
					Details: archiveErr.Error(),
					Action:  failureAction(archiveErr, "Resolve the files backup failure before rerunning backup."),
				},
				notRunBackupStep("finalize", "Manifest finalization did not run because files backup failed"),
				notRunBackupStep("retention", "Retention cleanup did not run because files backup failed"),
			)
			return info, wrapBackupExecuteError(archiveErr)
		}
		if err := saveTempFile(state.set.FilesBackup.Path+".tmp", state.set.FilesBackup.Path, "save files backup"); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_backup",
					Status:  BackupStepStatusFailed,
					Summary: "Files backup failed",
					Details: err.Error(),
					Action:  "Resolve the files backup write failure before rerunning backup.",
				},
				notRunBackupStep("finalize", "Manifest finalization did not run because files backup failed"),
				notRunBackupStep("retention", "Retention cleanup did not run because files backup failed"),
			)
			return info, wrapBackupExecuteError(executeFailure{
				Kind:    apperr.KindIO,
				Summary: "Files backup failed",
				Action:  "Resolve the files backup write failure before rerunning backup.",
				Err:     err,
			})
		}

		if archiveInfo.UsedDockerHelper {
			info.Warnings = append(info.Warnings, fmt.Sprintf("Files backup used the Docker helper fallback because local archiving failed for %s.", req.StorageDir))
		}

		info.FilesBackupCreated = true
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_backup",
			Status:  BackupStepStatusCompleted,
			Summary: "Files backup completed",
			Details: filesBackupDetails(state, archiveInfo),
		})
	}

	if err := finalizeBackupArtifacts(req, &state, &info); err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "finalize",
				Status:  BackupStepStatusFailed,
				Summary: "Manifest finalization failed",
				Details: err.Error(),
				Action:  "Resolve the manifest or checksum write failure before relying on this backup set.",
			},
			notRunBackupStep("retention", "Retention cleanup did not run because manifest finalization failed"),
		)
		return info, wrapBackupExecuteError(executeFailure{
			Kind:    apperr.KindIO,
			Summary: "Manifest finalization failed",
			Action:  "Resolve the manifest or checksum write failure before relying on this backup set.",
			Err:     err,
		})
	}
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "finalize",
		Status:  BackupStepStatusCompleted,
		Summary: "Manifest finalization completed",
		Details: finalizeDetails(info),
	})

	if err := cleanupBackupRetention(info.BackupRoot, req.RetentionDays, executeNow(req.Now)); err != nil {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "retention",
			Status:  BackupStepStatusFailed,
			Summary: "Retention cleanup failed",
			Details: err.Error(),
			Action:  "Resolve the retention cleanup failure before relying on the backup root state.",
		})
		return info, wrapBackupExecuteError(executeFailure{
			Kind:    apperr.KindIO,
			Summary: "Retention cleanup failed",
			Action:  "Resolve the retention cleanup failure before relying on the backup root state.",
			Err:     err,
		})
	}
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "retention",
		Status:  BackupStepStatusCompleted,
		Summary: "Retention cleanup completed",
		Details: retentionDetails(info.BackupRoot, req.RetentionDays),
	})

	if runtimeNeedsReturn {
		runtimeReturn, returnErr := returnRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile, runtimePrep)
		if returnErr != nil {
			info.Steps = append(info.Steps, ExecuteStep{
				Code:    "runtime_return",
				Status:  BackupStepStatusFailed,
				Summary: "Runtime return failed",
				Details: returnErr.Error(),
				Action:  "Restore the stopped application services manually before relying on the contour state.",
			})
			return info, wrapBackupExecuteError(executeFailure{
				Kind:    apperr.KindExternal,
				Summary: "Runtime return failed",
				Action:  "Restore the stopped application services manually before relying on the contour state.",
				Err:     returnErr,
			})
		}

		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_return",
			Status:  BackupStepStatusCompleted,
			Summary: "Runtime return completed",
			Details: runtimeReturnDetails(runtimeReturn),
		})
		runtimeReturnRecorded = true
	} else {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_return",
			Status:  BackupStepStatusSkipped,
			Summary: "Runtime return skipped",
			Details: runtimeReturnSkippedDetails(req, runtimePrep),
		})
		runtimeReturnRecorded = true
	}

	return info, nil
}

func (i ExecuteInfo) Counts() (completed, skipped, failed, notRun int) {
	for _, step := range i.Steps {
		switch step.Status {
		case BackupStepStatusCompleted:
			completed++
		case BackupStepStatusSkipped:
			skipped++
		case BackupStepStatusFailed:
			failed++
		case BackupStepStatusNotRun:
			notRun++
		}
	}

	return completed, skipped, failed, notRun
}

func (i ExecuteInfo) Ready() bool {
	for _, step := range i.Steps {
		if step.Status == BackupStepStatusFailed {
			return false
		}
	}

	return true
}

func wrapBackupExecuteError(err error) error {
	var failure executeFailure
	if errors.As(err, &failure) && failure.Kind != "" {
		return apperr.Wrap(failure.Kind, "backup_failed", err)
	}
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "backup_failed", err)
	}

	return apperr.Wrap(apperr.KindInternal, "backup_failed", err)
}
