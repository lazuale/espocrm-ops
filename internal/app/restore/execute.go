package restore

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	domainworkflow "github.com/lazuale/espocrm-ops/internal/domain/workflow"
)

type ExecuteRequest struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
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
	Now             func() time.Time
}

type ExecuteStep struct {
	Code    string
	Status  domainworkflow.Status
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
	Kind    domainfailure.Kind
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

func (s Service) Execute(req ExecuteRequest) (ExecuteInfo, error) {
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

	ctx, err := s.operations.PrepareOperation(maintenanceusecase.OperationContextRequest{
		Scope:           info.Scope,
		Operation:       "restore",
		ProjectDir:      info.ProjectDir,
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		LogWriter:       req.LogWriter,
	})
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "operation_preflight",
				Status:  domainworkflow.StatusFailed,
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
		Status:  domainworkflow.StatusCompleted,
		Summary: "Restore preflight completed",
		Details: fmt.Sprintf("Using %s for contour %s.", info.EnvFile, info.Scope),
	})

	source, err := s.resolveExecuteSource(ctx.BackupRoot, req)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "source_resolution",
				Status:  domainworkflow.StatusFailed,
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
		return info, wrapRestoreExecuteError(executeFailure{Kind: domainfailure.KindValidation, Err: err})
	}

	info.SelectionMode = source.SelectionMode
	info.SourceKind = source.SourceKind
	info.ManifestJSONPath = source.ManifestJSON
	info.ManifestTXTPath = source.ManifestTXT
	info.DBBackupPath = source.DBBackup
	info.FilesBackupPath = source.FilesBackup
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "source_resolution",
		Status:  domainworkflow.StatusCompleted,
		Summary: restoreSourceSummary(source),
		Details: restoreSourceDetails(source),
	})

	runtimeInfo, err := s.inspectRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile, !req.SkipDB)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "runtime_prepare",
				Status:  domainworkflow.StatusFailed,
				Summary: "Runtime preparation planning failed",
				Details: err.Error(),
				Action:  "Resolve the runtime state inspection failure before rerunning restore.",
			},
			blockedRestoreStep("snapshot_recovery_point", "Emergency recovery point did not run because runtime preparation planning failed"),
			blockedRestoreStep("db_restore", "Database restore did not run because runtime preparation planning failed"),
			blockedRestoreStep("files_restore", "Files restore did not run because runtime preparation planning failed"),
			blockedRestoreStep("runtime_return", "Runtime return did not run because runtime preparation planning failed"),
		)
		return info, wrapRestoreExecuteError(executeFailure{Kind: domainfailure.KindExternal, Err: err})
	}
	info.AppServicesWereRunning = runtimeInfo.AppServicesWereRunning

	if req.DryRun {
		return s.buildDryRun(ctx, req, info, source, runtimeInfo)
	}

	runtimePrep, err := s.prepareRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile, !req.SkipDB, req.NoStop)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "runtime_prepare",
				Status:  domainworkflow.StatusFailed,
				Summary: "Runtime preparation failed",
				Details: err.Error(),
				Action:  "Resolve the runtime preparation failure before rerunning restore.",
			},
			blockedRestoreStep("snapshot_recovery_point", "Emergency recovery point did not run because runtime preparation failed"),
			blockedRestoreStep("db_restore", "Database restore did not run because runtime preparation failed"),
			blockedRestoreStep("files_restore", "Files restore did not run because runtime preparation failed"),
			blockedRestoreStep("runtime_return", "Runtime return did not run because runtime preparation failed"),
		)
		return info, wrapRestoreExecuteError(executeFailure{Kind: domainfailure.KindExternal, Err: err})
	}
	info.AppServicesWereRunning = runtimePrep.AppServicesWereRunning
	info.StartedDBTemporarily = runtimePrep.StartedDBTemporarily
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_prepare",
		Status:  domainworkflow.StatusCompleted,
		Summary: "Runtime preparation completed",
		Details: runtimePrepareDetails(runtimePrep, req),
	})

	if req.NoSnapshot {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "snapshot_recovery_point",
			Status:  domainworkflow.StatusSkipped,
			Summary: "Emergency recovery point skipped",
			Details: "The pre-restore emergency recovery point was skipped because of --no-snapshot.",
		})
	} else {
		snapshotReq, snapshotReqErr := s.buildSnapshotRequest(ctx, req)
		if snapshotReqErr != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "snapshot_recovery_point",
					Status:  domainworkflow.StatusFailed,
					Summary: "Emergency recovery point failed",
					Details: snapshotReqErr.Error(),
					Action:  "Resolve the snapshot backup configuration failure before rerunning restore.",
				},
				blockedRestoreStep("db_restore", "Database restore did not run because the emergency recovery point failed"),
				blockedRestoreStep("files_restore", "Files restore did not run because the emergency recovery point failed"),
				blockedRestoreStep("runtime_return", "Runtime return did not run because the emergency recovery point failed"),
			)
			return info, wrapRestoreExecuteError(snapshotReqErr)
		}
		snapshotInfo, err := s.applySnapshotBackup(snapshotReq)
		if err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "snapshot_recovery_point",
					Status:  domainworkflow.StatusFailed,
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
			Status:  domainworkflow.StatusCompleted,
			Summary: "Emergency recovery point completed",
			Details: snapshotDetails(snapshotInfo),
		})
	}

	if req.SkipDB {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_restore",
			Status:  domainworkflow.StatusSkipped,
			Summary: "Database restore skipped",
			Details: "The database restore was skipped because of --skip-db.",
		})
	} else {
		dbReq := s.flow.BuildDBRequest(ctx, source.ManifestJSON, source.DBBackup, runtimePrep.DBContainer)
		if _, err := s.flow.RestoreDB(dbReq); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "db_restore",
					Status:  domainworkflow.StatusFailed,
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
			Status:  domainworkflow.StatusCompleted,
			Summary: "Database restore completed",
			Details: dbRestoreDetails(ctx, source, runtimePrep.DBContainer),
		})
	}

	if req.SkipFiles {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_restore",
			Status:  domainworkflow.StatusSkipped,
			Summary: "Files restore skipped",
			Details: "The files restore was skipped because of --skip-files.",
		})
	} else {
		filesReq := s.flow.BuildFilesRequest(ctx, source.ManifestJSON, source.FilesBackup)
		if _, err := s.flow.RestoreFiles(filesReq); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_restore",
					Status:  domainworkflow.StatusFailed,
					Summary: "Files restore failed",
					Details: err.Error(),
					Action:  "Resolve the files restore failure before rerunning restore.",
				},
				blockedRestoreStep("runtime_return", "Runtime return did not run because the files restore failed"),
			)
			return info, wrapRestoreExecuteError(err)
		}
		if err := s.runtime.ReconcileEspoStoragePermissions(
			filesReq.TargetDir,
			strings.TrimSpace(ctx.Env.Value("MARIADB_TAG")),
			strings.TrimSpace(ctx.Env.Value("ESPOCRM_IMAGE")),
		); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_restore",
					Status:  domainworkflow.StatusFailed,
					Summary: "Files restore failed",
					Details: fmt.Sprintf("Files were restored but runtime permission reconciliation failed: %v", err),
					Action:  "Resolve the permission reconciliation failure before rerunning restore.",
				},
				blockedRestoreStep("runtime_return", "Runtime return did not run because the files restore failed"),
			)
			return info, wrapRestoreExecuteError(executeFailure{Kind: domainfailure.KindExternal, Err: err})
		}
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_restore",
			Status:  domainworkflow.StatusCompleted,
			Summary: "Files restore completed",
			Details: filesRestoreDetails(source, filesReq.TargetDir),
		})
	}

	runtimeReturn, err := s.returnRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile, runtimePrep, req.NoStart)
	if err != nil {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_return",
			Status:  domainworkflow.StatusFailed,
			Summary: "Runtime return failed",
			Details: err.Error(),
			Action:  "Resolve the contour return failure before relying on the restored state.",
		})
		return info, wrapRestoreExecuteError(executeFailure{Kind: domainfailure.KindExternal, Err: err})
	}
	validatedServices, err := s.validatePostRestoreRuntimeHealth(
		info.ProjectDir,
		info.ComposeFile,
		info.EnvFile,
		expectedPostRestoreServices(req, runtimePrep, runtimeReturn),
	)
	if err != nil {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_return",
			Status:  domainworkflow.StatusFailed,
			Summary: "Runtime return failed",
			Details: err.Error(),
			Action:  "Repair the restored contour health before treating this restore as successful.",
		})
		return info, wrapRestoreExecuteError(executeFailure{Kind: domainfailure.KindExternal, Err: err})
	}
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_return",
		Status:  runtimeReturnStatus(req, runtimePrep, runtimeReturn, validatedServices),
		Summary: runtimeReturnSummary(req, runtimePrep, runtimeReturn, validatedServices),
		Details: runtimeReturnDetails(req, runtimePrep, runtimeReturn, validatedServices),
	})

	info.Warnings = dedupeStrings(info.Warnings)
	return info, nil
}

func (i ExecuteInfo) Counts() (planned, completed, skipped, blocked, failed int) {
	for _, step := range i.Steps {
		switch step.Status {
		case domainworkflow.StatusPlanned:
			planned++
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
	return planned, completed, skipped, blocked, failed
}

func (i ExecuteInfo) Ready() bool {
	for _, step := range i.Steps {
		if step.Status == domainworkflow.StatusFailed || step.Status == domainworkflow.StatusBlocked {
			return false
		}
	}
	return true
}
