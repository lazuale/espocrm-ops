package restore

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
	"github.com/lazuale/espocrm-ops/internal/usecase/reporting"
)

const (
	RestoreStepStatusWouldRun  = "would_run"
	RestoreStepStatusCompleted = "completed"
	RestoreStepStatusSkipped   = "skipped"
	RestoreStepStatusBlocked   = "blocked"
	RestoreStepStatusFailed    = "failed"

	defaultRestoreReadinessTimeoutSeconds = 300
)

var restoreAppServices = []string{
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

type ExecuteRequest struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	EnvContourHint  string
	ManifestPath    string
	DBBackup        string
	FilesBackup     string
	SkipDB          bool
	SkipFiles       bool
	NoSnapshot      bool
	NoStop          bool
	NoStart         bool
	DryRun          bool
	LogWriter       io.Writer
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
	ManifestJSONPath       string
	ManifestTXTPath        string
	DBBackupPath           string
	FilesBackupPath        string
	SelectionMode          string
	SourceKind             string
	SnapshotEnabled        bool
	SkipDB                 bool
	SkipFiles              bool
	NoStop                 bool
	NoStart                bool
	DryRun                 bool
	AppServicesWereRunning bool
	StartedDBTemporarily   bool
	SnapshotManifestTXT    string
	SnapshotManifestJSON   string
	SnapshotDBBackup       string
	SnapshotFilesBackup    string
	SnapshotDBChecksum     string
	SnapshotFilesChecksum  string
	Warnings               []string
	Steps                  []ExecuteStep
}

type executeSource struct {
	SelectionMode string
	SourceKind    string
	ManifestJSON  string
	ManifestTXT   string
	DBBackup      string
	FilesBackup   string
}

type runtimePrepareInfo struct {
	StartedDBTemporarily   bool
	AppServicesWereRunning bool
	StoppedAppServices     []string
	DBContainer            string
}

type runtimeReturnInfo struct {
	RestartedAppServices []string
	StoppedDB            bool
}

type executeFailure struct {
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

func Execute(req ExecuteRequest) (ExecuteInfo, error) {
	info := ExecuteInfo{
		Scope:           strings.TrimSpace(req.Scope),
		ProjectDir:      filepath.Clean(req.ProjectDir),
		ComposeFile:     filepath.Clean(req.ComposeFile),
		SnapshotEnabled: !req.NoSnapshot,
		SkipDB:          req.SkipDB,
		SkipFiles:       req.SkipFiles,
		NoStop:          req.NoStop,
		NoStart:         req.NoStart,
		DryRun:          req.DryRun,
		Warnings:        flagWarnings(req),
	}

	ctx, err := maintenanceusecase.PrepareOperation(maintenanceusecase.OperationContextRequest{
		Scope:           info.Scope,
		Operation:       "restore",
		ProjectDir:      info.ProjectDir,
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		EnvContourHint:  strings.TrimSpace(req.EnvContourHint),
		LogWriter:       req.LogWriter,
	})
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "operation_preflight",
				Status:  RestoreStepStatusFailed,
				Summary: "Restore preflight failed",
				Details: err.Error(),
				Action:  "Resolve env, lock, or filesystem readiness before rerunning restore.",
			},
			blockedRestoreStep("source_resolution", "Source resolution did not run because restore preflight failed"),
			blockedRestoreStep("runtime_prepare", "Runtime preparation did not run because restore preflight failed"),
			blockedRestoreStep("snapshot_recovery_point", "Emergency recovery point did not run because restore preflight failed"),
			blockedRestoreStep("db_restore", "Database restore did not run because restore preflight failed"),
			blockedRestoreStep("files_restore", "Files restore did not run because restore preflight failed"),
			blockedRestoreStep("runtime_return", "Runtime return did not run because restore preflight failed"),
		)
		return info, wrapRestoreExecuteError(err)
	}
	defer func() {
		_ = ctx.Release()
	}()

	info.EnvFile = ctx.Env.FilePath
	info.BackupRoot = ctx.BackupRoot
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "operation_preflight",
		Status:  RestoreStepStatusCompleted,
		Summary: "Restore preflight completed",
		Details: fmt.Sprintf("Using %s for contour %s.", info.EnvFile, info.Scope),
	})

	source, err := resolveExecuteSource(ctx.BackupRoot, req)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "source_resolution",
				Status:  RestoreStepStatusFailed,
				Summary: failureSummary(err, "Restore source resolution failed"),
				Details: err.Error(),
				Action:  failureAction(err, "Resolve the restore source selection first and rerun restore."),
			},
			blockedRestoreStep("runtime_prepare", "Runtime preparation did not run because source resolution failed"),
			blockedRestoreStep("snapshot_recovery_point", "Emergency recovery point did not run because source resolution failed"),
			blockedRestoreStep("db_restore", "Database restore did not run because source resolution failed"),
			blockedRestoreStep("files_restore", "Files restore did not run because source resolution failed"),
			blockedRestoreStep("runtime_return", "Runtime return did not run because source resolution failed"),
		)
		return info, apperr.Wrap(apperr.KindValidation, "restore_failed", err)
	}

	info.SelectionMode = source.SelectionMode
	info.SourceKind = source.SourceKind
	info.ManifestJSONPath = source.ManifestJSON
	info.ManifestTXTPath = source.ManifestTXT
	info.DBBackupPath = source.DBBackup
	info.FilesBackupPath = source.FilesBackup
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "source_resolution",
		Status:  RestoreStepStatusCompleted,
		Summary: restoreSourceSummary(source),
		Details: restoreSourceDetails(source),
	})

	runtimeInfo, err := inspectRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile, !req.SkipDB)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "runtime_prepare",
				Status:  RestoreStepStatusFailed,
				Summary: "Runtime preparation planning failed",
				Details: err.Error(),
				Action:  "Resolve the runtime state inspection failure before rerunning restore.",
			},
			blockedRestoreStep("snapshot_recovery_point", "Emergency recovery point did not run because runtime preparation planning failed"),
			blockedRestoreStep("db_restore", "Database restore did not run because runtime preparation planning failed"),
			blockedRestoreStep("files_restore", "Files restore did not run because runtime preparation planning failed"),
			blockedRestoreStep("runtime_return", "Runtime return did not run because runtime preparation planning failed"),
		)
		return info, wrapRestoreExternalError(err)
	}
	info.AppServicesWereRunning = runtimeInfo.AppServicesWereRunning

	if req.DryRun {
		return buildDryRun(ctx, req, info, source, runtimeInfo)
	}

	runtimePrep, err := prepareRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile, !req.SkipDB, req.NoStop)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "runtime_prepare",
				Status:  RestoreStepStatusFailed,
				Summary: "Runtime preparation failed",
				Details: err.Error(),
				Action:  "Resolve the runtime preparation failure before rerunning restore.",
			},
			blockedRestoreStep("snapshot_recovery_point", "Emergency recovery point did not run because runtime preparation failed"),
			blockedRestoreStep("db_restore", "Database restore did not run because runtime preparation failed"),
			blockedRestoreStep("files_restore", "Files restore did not run because runtime preparation failed"),
			blockedRestoreStep("runtime_return", "Runtime return did not run because runtime preparation failed"),
		)
		return info, wrapRestoreExternalError(err)
	}
	info.AppServicesWereRunning = runtimePrep.AppServicesWereRunning
	info.StartedDBTemporarily = runtimePrep.StartedDBTemporarily
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_prepare",
		Status:  RestoreStepStatusCompleted,
		Summary: "Runtime preparation completed",
		Details: runtimePrepareDetails(runtimePrep, req),
	})

	if req.NoSnapshot {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "snapshot_recovery_point",
			Status:  RestoreStepStatusSkipped,
			Summary: "Emergency recovery point skipped",
			Details: "The pre-restore emergency recovery point was skipped because of --no-snapshot.",
		})
	} else {
		snapshotInfo, err := applySnapshotBackup(buildSnapshotRequest(ctx, req))
		if err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "snapshot_recovery_point",
					Status:  RestoreStepStatusFailed,
					Summary: "Emergency recovery point failed",
					Details: err.Error(),
					Action:  "Resolve the recovery-point backup failure before rerunning restore.",
				},
				blockedRestoreStep("db_restore", "Database restore did not run because the emergency recovery point failed"),
				blockedRestoreStep("files_restore", "Files restore did not run because the emergency recovery point failed"),
				blockedRestoreStep("runtime_return", "Runtime return did not run because the emergency recovery point failed"),
			)
			return info, wrapRestoreExecuteError(err)
		}
		info.StartedDBTemporarily = info.StartedDBTemporarily || snapshotInfo.StartedDBTemporarily
		info.SnapshotManifestTXT = snapshotInfo.ManifestTXTPath
		info.SnapshotManifestJSON = snapshotInfo.ManifestJSONPath
		info.SnapshotDBBackup = snapshotInfo.DBBackupPath
		info.SnapshotFilesBackup = snapshotInfo.FilesBackupPath
		info.SnapshotDBChecksum = snapshotInfo.DBSidecarPath
		info.SnapshotFilesChecksum = snapshotInfo.FilesSidecarPath
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "snapshot_recovery_point",
			Status:  RestoreStepStatusCompleted,
			Summary: "Emergency recovery point completed",
			Details: snapshotDetails(snapshotInfo),
		})
	}

	if req.SkipDB {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_restore",
			Status:  RestoreStepStatusSkipped,
			Summary: "Database restore skipped",
			Details: "The database restore was skipped because of --skip-db.",
		})
	} else {
		if _, err := RestoreDB(buildDBRestoreRequest(ctx, source, runtimePrep.DBContainer)); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "db_restore",
					Status:  RestoreStepStatusFailed,
					Summary: "Database restore failed",
					Details: err.Error(),
					Action:  "Resolve the database restore failure before rerunning restore.",
				},
				blockedRestoreStep("files_restore", "Files restore did not run because the database restore failed"),
				blockedRestoreStep("runtime_return", "Runtime return did not run because the database restore failed"),
			)
			return info, wrapRestoreExecuteError(err)
		}
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_restore",
			Status:  RestoreStepStatusCompleted,
			Summary: "Database restore completed",
			Details: dbRestoreDetails(ctx, source, runtimePrep.DBContainer),
		})
	}

	if req.SkipFiles {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_restore",
			Status:  RestoreStepStatusSkipped,
			Summary: "Files restore skipped",
			Details: "The files restore was skipped because of --skip-files.",
		})
	} else {
		filesReq := buildFilesRestoreRequest(ctx, source)
		if _, err := RestoreFiles(filesReq); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_restore",
					Status:  RestoreStepStatusFailed,
					Summary: "Files restore failed",
					Details: err.Error(),
					Action:  "Resolve the files restore failure before rerunning restore.",
				},
				blockedRestoreStep("runtime_return", "Runtime return did not run because the files restore failed"),
			)
			return info, wrapRestoreExecuteError(err)
		}
		if err := platformdocker.ReconcileEspoStoragePermissions(
			filesReq.TargetDir,
			strings.TrimSpace(ctx.Env.Value("MARIADB_TAG")),
			strings.TrimSpace(ctx.Env.Value("ESPOCRM_IMAGE")),
		); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_restore",
					Status:  RestoreStepStatusFailed,
					Summary: "Files restore failed",
					Details: fmt.Sprintf("Files were restored but runtime permission reconciliation failed: %v", err),
					Action:  "Resolve the permission reconciliation failure before rerunning restore.",
				},
				blockedRestoreStep("runtime_return", "Runtime return did not run because the files restore failed"),
			)
			return info, wrapRestoreExternalError(err)
		}
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_restore",
			Status:  RestoreStepStatusCompleted,
			Summary: "Files restore completed",
			Details: filesRestoreDetails(ctx, source),
		})
	}

	runtimeReturn, err := returnRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile, runtimePrep, req.NoStart)
	if err != nil {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_return",
			Status:  RestoreStepStatusFailed,
			Summary: "Runtime return failed",
			Details: err.Error(),
			Action:  "Resolve the contour return failure before relying on the restored state.",
		})
		return info, wrapRestoreExternalError(err)
	}
	validatedServices, err := validatePostRestoreRuntimeHealth(
		info.ProjectDir,
		info.ComposeFile,
		info.EnvFile,
		expectedPostRestoreServices(req, runtimePrep, runtimeReturn),
	)
	if err != nil {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_return",
			Status:  RestoreStepStatusFailed,
			Summary: "Runtime return failed",
			Details: err.Error(),
			Action:  "Repair the restored contour health before treating this restore as successful.",
		})
		return info, wrapRestoreExternalError(err)
	}
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_return",
		Status:  runtimeReturnStatus(req, runtimePrep, runtimeReturn, validatedServices),
		Summary: runtimeReturnSummary(req, runtimePrep, runtimeReturn, validatedServices),
		Details: runtimeReturnDetails(req, runtimePrep, runtimeReturn, validatedServices),
	})

	info.Warnings = reporting.DedupeStrings(info.Warnings)
	return info, nil
}

func buildDryRun(ctx maintenanceusecase.OperationContext, req ExecuteRequest, info ExecuteInfo, source executeSource, runtimeInfo runtimePrepareInfo) (ExecuteInfo, error) {
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
		if err := dryRunDBChecks(ctx, source, runtimeInfo); err != nil {
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
		if err := dryRunFilesChecks(ctx, source); err != nil {
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
	info.Warnings = reporting.DedupeStrings(info.Warnings)
	return info, nil
}

func resolveExecuteSource(backupRoot string, req ExecuteRequest) (executeSource, error) {
	manifestPath := strings.TrimSpace(req.ManifestPath)
	dbBackup := strings.TrimSpace(req.DBBackup)
	filesBackup := strings.TrimSpace(req.FilesBackup)

	if manifestPath != "" {
		info, err := backupstore.VerifyManifestDetailed(manifestPath)
		if err != nil {
			return executeSource{}, executeFailure{
				Summary: "The selected restore manifest is not valid",
				Action:  "Choose a valid manifest JSON that references readable, verified backup artifacts.",
				Err:     err,
			}
		}
		return executeSource{
			SelectionMode: manifestSelectionMode(req),
			SourceKind:    "manifest",
			ManifestJSON:  filepath.Clean(manifestPath),
			ManifestTXT:   matchingManifestTXT(manifestPath),
			DBBackup:      info.DBBackupPath,
			FilesBackup:   info.FilesPath,
		}, nil
	}

	switch {
	case req.SkipDB:
		filesBackup = filepath.Clean(filesBackup)
		if err := backupstore.VerifyDirectFilesBackup(filesBackup); err != nil {
			return executeSource{}, executeFailure{
				Summary: "The selected files backup is not valid",
				Action:  "Choose a readable .tar.gz files backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		return executeSource{
			SelectionMode: "direct_files_only",
			SourceKind:    "direct",
			FilesBackup:   filesBackup,
		}, nil
	case req.SkipFiles:
		dbBackup = filepath.Clean(dbBackup)
		if err := backupstore.VerifyDirectDBBackup(dbBackup); err != nil {
			return executeSource{}, executeFailure{
				Summary: "The selected database backup is not valid",
				Action:  "Choose a readable .sql.gz database backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		return executeSource{
			SelectionMode: "direct_db_only",
			SourceKind:    "direct",
			DBBackup:      dbBackup,
		}, nil
	default:
		dbBackup = filepath.Clean(dbBackup)
		filesBackup = filepath.Clean(filesBackup)
		if err := backupstore.VerifyDirectDBBackup(dbBackup); err != nil {
			return executeSource{}, executeFailure{
				Summary: "The selected database backup is not valid",
				Action:  "Choose a readable .sql.gz database backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		if err := backupstore.VerifyDirectFilesBackup(filesBackup); err != nil {
			return executeSource{}, executeFailure{
				Summary: "The selected files backup is not valid",
				Action:  "Choose a readable .tar.gz files backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		if err := validateDirectPair(dbBackup, filesBackup); err != nil {
			return executeSource{}, err
		}
		return executeSource{
			SelectionMode: "direct_pair",
			SourceKind:    "direct",
			DBBackup:      dbBackup,
			FilesBackup:   filesBackup,
		}, nil
	}
}

func validateDirectPair(dbPath, filesPath string) error {
	dbGroup, err := domainbackup.ParseDBBackupName(dbPath)
	if err != nil {
		return executeFailure{
			Summary: "The selected database backup name is unsupported",
			Action:  "Choose a canonical .sql.gz backup path or use a manifest-backed restore.",
			Err:     err,
		}
	}
	filesGroup, err := domainbackup.ParseFilesBackupName(filesPath)
	if err != nil {
		return executeFailure{
			Summary: "The selected files backup name is unsupported",
			Action:  "Choose a canonical .tar.gz backup path or use a manifest-backed restore.",
			Err:     err,
		}
	}
	if dbGroup != filesGroup {
		return executeFailure{
			Summary: "The selected database and files backups do not belong to the same backup set",
			Action:  "Pass database and files backups from the same backup set or use a manifest-backed restore.",
			Err:     fmt.Errorf("database backup resolves to %s %s, but files backup resolves to %s %s", dbGroup.Prefix, dbGroup.Stamp, filesGroup.Prefix, filesGroup.Stamp),
		}
	}

	return nil
}

func inspectRuntime(projectDir, composeFile, envFile string, needDB bool) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	if needDB {
		dbState, err := platformdocker.ComposeServiceStateFor(cfg, "db")
		if err != nil {
			return info, err
		}
		if dbState.Status != "running" && dbState.Status != "healthy" {
			info.StartedDBTemporarily = true
		} else {
			container, err := platformdocker.ComposeServiceContainerID(cfg, "db")
			if err != nil {
				return info, err
			}
			info.DBContainer = strings.TrimSpace(container)
		}
	}

	runningServices, err := platformdocker.ComposeRunningServices(cfg)
	if err != nil {
		return info, err
	}
	info.AppServicesWereRunning = restoreAppServicesRunning(runningServices)
	info.StoppedAppServices = runningAppServices(runningServices)
	return info, nil
}

func prepareRuntime(projectDir, composeFile, envFile string, needDB bool, noStop bool) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	if needDB {
		dbState, err := platformdocker.ComposeServiceStateFor(cfg, "db")
		if err != nil {
			return info, err
		}
		if dbState.Status != "running" && dbState.Status != "healthy" {
			info.StartedDBTemporarily = true
			if err := platformdocker.ComposeUp(cfg, "db"); err != nil {
				return info, err
			}
		}
		if err := platformdocker.WaitForServicesReady(cfg, defaultRestoreReadinessTimeoutSeconds, "db"); err != nil {
			return info, err
		}
		container, err := platformdocker.ComposeServiceContainerID(cfg, "db")
		if err != nil {
			return info, err
		}
		info.DBContainer = strings.TrimSpace(container)
		if info.DBContainer == "" {
			return info, fmt.Errorf("could not resolve the db container after runtime preparation")
		}
	}

	runningServices, err := platformdocker.ComposeRunningServices(cfg)
	if err != nil {
		return info, err
	}
	info.AppServicesWereRunning = restoreAppServicesRunning(runningServices)
	info.StoppedAppServices = runningAppServices(runningServices)
	if noStop || len(info.StoppedAppServices) == 0 {
		return info, nil
	}

	if err := platformdocker.ComposeStop(cfg, info.StoppedAppServices...); err != nil {
		return info, err
	}
	return info, nil
}

func returnRuntime(projectDir, composeFile, envFile string, prep runtimePrepareInfo, noStart bool) (runtimeReturnInfo, error) {
	info := runtimeReturnInfo{}
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	if len(prep.StoppedAppServices) != 0 && !noStart {
		if err := platformdocker.ComposeUp(cfg, prep.StoppedAppServices...); err != nil {
			return info, err
		}
		info.RestartedAppServices = append(info.RestartedAppServices, prep.StoppedAppServices...)
	}

	if prep.StartedDBTemporarily && len(prep.StoppedAppServices) == 0 {
		if err := platformdocker.ComposeStop(cfg, "db"); err != nil {
			return info, err
		}
		info.StoppedDB = true
	}

	return info, nil
}

func dryRunDBChecks(ctx maintenanceusecase.OperationContext, source executeSource, runtimeInfo runtimePrepareInfo) error {
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

	_, err = PreflightDBRestore(DBPreflightRequest{
		DBBackup:    source.DBBackup,
		DBContainer: runtimeInfo.DBContainer,
	})
	return err
}

func dryRunFilesChecks(ctx maintenanceusecase.OperationContext, source executeSource) error {
	lock, err := platformlocks.AcquireRestoreFilesLock()
	if err != nil {
		return LockError{Err: fmt.Errorf("files restore lock failed: %w", err)}
	}
	if releaseErr := lock.Release(); releaseErr != nil {
		return fmt.Errorf("release files restore lock: %w", releaseErr)
	}

	targetDir := platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir())
	_, err = PreflightFilesRestore(FilesPreflightRequest{
		FilesBackup: source.FilesBackup,
		TargetDir:   targetDir,
	})
	return err
}

func configResolveDBPassword(req RestoreDBRequest) (string, error) {
	password, err := platformconfig.ResolveDBPassword(platformconfig.DBConfig{
		Container:    req.DBContainer,
		Name:         req.DBName,
		User:         req.DBUser,
		Password:     req.DBPassword,
		PasswordFile: req.DBPasswordFile,
	})
	if err != nil {
		return "", PreflightError{Err: fmt.Errorf("resolve db password: %w", err)}
	}
	return password, nil
}

func buildSnapshotRequest(ctx maintenanceusecase.OperationContext, req ExecuteRequest) snapshotBackupRequest {
	return snapshotBackupRequest{
		TimeoutSeconds: defaultRestoreReadinessTimeoutSeconds,
		LogWriter:      req.LogWriter,
		Backup: backupusecase.ExecuteRequest{
			Scope:          ctx.Scope,
			ProjectDir:     ctx.ProjectDir,
			ComposeFile:    filepath.Clean(req.ComposeFile),
			EnvFile:        ctx.Env.FilePath,
			BackupRoot:     ctx.BackupRoot,
			StorageDir:     platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
			NamePrefix:     resolvedBackupNamePrefix(ctx.Env),
			RetentionDays:  resolvedRetentionDays(ctx.Env),
			ComposeProject: ctx.ComposeProject,
			DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
			DBPassword:     ctx.Env.Value("DB_PASSWORD"),
			DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
			EspoCRMImage:   strings.TrimSpace(ctx.Env.Value("ESPOCRM_IMAGE")),
			MariaDBTag:     strings.TrimSpace(ctx.Env.Value("MARIADB_TAG")),
			SkipDB:         req.SkipDB,
			SkipFiles:      req.SkipFiles,
			NoStop:         req.NoStop,
			LogWriter:      req.LogWriter,
			ErrWriter:      req.LogWriter,
		},
	}
}

func buildDBRestoreRequest(ctx maintenanceusecase.OperationContext, source executeSource, dbContainer string) RestoreDBRequest {
	return RestoreDBRequest{
		DBBackup:       source.DBBackup,
		DBContainer:    dbContainer,
		DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
		DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
		DBPassword:     ctx.Env.Value("DB_PASSWORD"),
		DBRootPassword: ctx.Env.Value("DB_ROOT_PASSWORD"),
	}
}

func buildFilesRestoreRequest(ctx maintenanceusecase.OperationContext, source executeSource) RestoreFilesRequest {
	return RestoreFilesRequest{
		FilesBackup: source.FilesBackup,
		TargetDir:   platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
	}
}

func matchingManifestTXT(manifestJSON string) string {
	if !strings.HasSuffix(manifestJSON, ".manifest.json") {
		return ""
	}
	return strings.TrimSuffix(manifestJSON, ".manifest.json") + ".manifest.txt"
}

func manifestSelectionMode(req ExecuteRequest) string {
	switch {
	case req.SkipDB:
		return "manifest_files_only"
	case req.SkipFiles:
		return "manifest_db_only"
	default:
		return "manifest_full"
	}
}

func restoreSourceSummary(source executeSource) string {
	if source.SourceKind == "manifest" {
		return "Restore source resolution completed from manifest"
	}

	switch source.SelectionMode {
	case "direct_db_only":
		return "Restore source resolution completed from a direct database backup"
	case "direct_files_only":
		return "Restore source resolution completed from a direct files backup"
	default:
		return "Restore source resolution completed from a direct backup pair"
	}
}

func restoreSourceDetails(source executeSource) string {
	switch source.SourceKind {
	case "manifest":
		return fmt.Sprintf("Using manifest %s with database backup %s and files backup %s.", source.ManifestJSON, source.DBBackup, source.FilesBackup)
	case "direct_db_only":
		return fmt.Sprintf("Using direct database backup %s.", source.DBBackup)
	case "direct_files_only":
		return fmt.Sprintf("Using direct files backup %s.", source.FilesBackup)
	default:
		return fmt.Sprintf("Using direct database backup %s and files backup %s.", source.DBBackup, source.FilesBackup)
	}
}

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

func resolvedBackupNamePrefix(env platformconfig.OperationEnv) string {
	if value := strings.TrimSpace(env.Value("BACKUP_NAME_PREFIX")); value != "" {
		return value
	}
	return strings.TrimSpace(env.ComposeProject())
}

func resolvedRetentionDays(env platformconfig.OperationEnv) int {
	value := strings.TrimSpace(env.Value("BACKUP_RETENTION_DAYS"))
	if value == "" {
		return 7
	}
	var days int
	if _, err := fmt.Sscanf(value, "%d", &days); err != nil || days <= 0 {
		return 7
	}
	return days
}

func expectedPostRestoreServices(req ExecuteRequest, prep runtimePrepareInfo, ret runtimeReturnInfo) []string {
	services := []string{}

	switch {
	case req.NoStop:
		services = append(services, prep.StoppedAppServices...)
	default:
		services = append(services, ret.RestartedAppServices...)
	}

	if len(services) != 0 || (!req.SkipDB && !prep.StartedDBTemporarily) {
		services = append(services, "db")
	}

	out := make([]string, 0, len(services))
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" || slices.Contains(out, service) {
			continue
		}
		out = append(out, service)
	}

	return out
}

func validatePostRestoreRuntimeHealth(projectDir, composeFile, envFile string, services []string) ([]string, error) {
	if len(services) == 0 {
		return nil, nil
	}

	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}
	if err := platformdocker.WaitForServicesReady(cfg, defaultRestoreReadinessTimeoutSeconds, services...); err != nil {
		return nil, err
	}

	return append([]string(nil), services...), nil
}

func runningAppServices(services []string) []string {
	items := make([]string, 0, len(restoreAppServices))
	for _, service := range restoreAppServices {
		if slices.Contains(services, service) {
			items = append(items, service)
		}
	}
	return items
}

func restoreAppServicesRunning(services []string) bool {
	for _, service := range services {
		if slices.Contains(restoreAppServices, service) {
			return true
		}
	}
	return false
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

func (i ExecuteInfo) Counts() (wouldRun, completed, skipped, blocked, failed int) {
	for _, step := range i.Steps {
		switch step.Status {
		case RestoreStepStatusWouldRun:
			wouldRun++
		case RestoreStepStatusCompleted:
			completed++
		case RestoreStepStatusSkipped:
			skipped++
		case RestoreStepStatusBlocked:
			blocked++
		case RestoreStepStatusFailed:
			failed++
		}
	}
	return wouldRun, completed, skipped, blocked, failed
}

func (i ExecuteInfo) Ready() bool {
	for _, step := range i.Steps {
		if step.Status == RestoreStepStatusFailed || step.Status == RestoreStepStatusBlocked {
			return false
		}
	}
	return true
}

func wrapRestoreExecuteError(err error) error {
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "restore_failed", err)
	}
	return apperr.Wrap(apperr.KindInternal, "restore_failed", err)
}

func wrapRestoreExternalError(err error) error {
	return apperr.Wrap(apperr.KindExternal, "restore_failed", err)
}
