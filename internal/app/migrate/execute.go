package migrate

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	domainruntime "github.com/lazuale/espocrm-ops/internal/domain/runtime"
	domainworkflow "github.com/lazuale/espocrm-ops/internal/domain/workflow"
)

type ExecuteRequest struct {
	SourceScope string
	TargetScope string
	ProjectDir  string
	ComposeFile string
	DBBackup    string
	FilesBackup string
	SkipDB      bool
	SkipFiles   bool
	NoStart     bool
	LogWriter   io.Writer
}

type ExecuteStep struct {
	Code    string
	Status  domainworkflow.Status
	Summary string
	Details string
	Action  string
}

type ExecuteInfo struct {
	SourceScope            string
	TargetScope            string
	ProjectDir             string
	ComposeFile            string
	SourceEnvFile          string
	TargetEnvFile          string
	SourceBackupRoot       string
	TargetBackupRoot       string
	RequestedDBBackup      string
	RequestedFilesBackup   string
	RequestedSelectionMode string
	SelectionMode          string
	SelectedPrefix         string
	SelectedStamp          string
	ManifestTXTPath        string
	ManifestJSONPath       string
	DBBackupPath           string
	FilesBackupPath        string
	SkipDB                 bool
	SkipFiles              bool
	NoStart                bool
	StartedDBTemporarily   bool
	Warnings               []string
	Steps                  []ExecuteStep
}

type runtimePrepareInfo struct {
	StartedDBTemporarily bool
	StoppedAppServices   []string
}

type sourceSelection struct {
	SelectionMode string
	Prefix        string
	Stamp         string
	ManifestTXT   string
	ManifestJSON  string
	DBBackup      string
	FilesBackup   string
	Warnings      []string
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
		SourceScope:            strings.TrimSpace(req.SourceScope),
		TargetScope:            strings.TrimSpace(req.TargetScope),
		ProjectDir:             filepath.Clean(req.ProjectDir),
		ComposeFile:            filepath.Clean(req.ComposeFile),
		RequestedDBBackup:      strings.TrimSpace(req.DBBackup),
		RequestedFilesBackup:   strings.TrimSpace(req.FilesBackup),
		RequestedSelectionMode: requestedSelectionMode(req),
		SkipDB:                 req.SkipDB,
		SkipFiles:              req.SkipFiles,
		NoStart:                req.NoStart,
		Warnings:               flagWarnings(req),
	}

	sourceEnv, err := s.env.LoadOperationEnv(info.ProjectDir, info.SourceScope, "")
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "source_preflight",
				Status:  domainworkflow.StatusFailed,
				Summary: "Source contour preflight failed",
				Details: err.Error(),
				Action:  "Resolve the source env file or source backup root settings before rerunning migrate.",
			},
			blockedMigrateStep("target_preflight", "Target contour preflight did not run because source contour preflight failed"),
			blockedMigrateStep("source_selection", "Source backup selection did not run because source contour preflight failed"),
			blockedMigrateStep("compatibility", "Migration compatibility checks did not run because source contour preflight failed"),
			blockedMigrateStep("runtime_prepare", "Target runtime preparation did not run because source contour preflight failed"),
			blockedMigrateStep("db_restore", "Database restore did not run because source contour preflight failed"),
			blockedMigrateStep("files_restore", "Files restore did not run because source contour preflight failed"),
			blockedMigrateStep("target_start", "Target contour start did not run because source contour preflight failed"),
		)
		return info, wrapExecuteError(err)
	}

	if _, err := sourceEnv.RuntimeContract(); err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "source_preflight",
				Status:  domainworkflow.StatusFailed,
				Summary: "Source contour preflight failed",
				Details: err.Error(),
				Action:  "Fix the explicit helper image and runtime ownership settings in the source env file before rerunning migrate.",
			},
			blockedMigrateStep("target_preflight", "Target contour preflight did not run because source contour preflight failed"),
			blockedMigrateStep("source_selection", "Source backup selection did not run because source contour preflight failed"),
			blockedMigrateStep("compatibility", "Migration compatibility checks did not run because source contour preflight failed"),
			blockedMigrateStep("runtime_prepare", "Target runtime preparation did not run because source contour preflight failed"),
			blockedMigrateStep("db_restore", "Database restore did not run because source contour preflight failed"),
			blockedMigrateStep("files_restore", "Files restore did not run because source contour preflight failed"),
			blockedMigrateStep("target_start", "Target contour start did not run because source contour preflight failed"),
		)
		return info, wrapExecuteError(executeFailure{Kind: domainfailure.KindValidation, Err: err})
	}

	info.SourceEnvFile = sourceEnv.FilePath
	info.SourceBackupRoot = s.env.ResolveProjectPath(info.ProjectDir, sourceEnv.BackupRoot())
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "source_preflight",
		Status:  domainworkflow.StatusCompleted,
		Summary: "Source contour preflight completed",
		Details: fmt.Sprintf("Using %s with backup root %s.", info.SourceEnvFile, info.SourceBackupRoot),
	})

	targetCtx, err := s.operations.PrepareOperation(maintenanceusecase.OperationContextRequest{
		Scope:      info.TargetScope,
		Operation:  "migrate",
		ProjectDir: info.ProjectDir,
		LogWriter:  req.LogWriter,
	})
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "target_preflight",
				Status:  domainworkflow.StatusFailed,
				Summary: "Target contour preflight failed",
				Details: err.Error(),
				Action:  "Resolve env, lock, or filesystem readiness before rerunning migrate.",
			},
			blockedMigrateStep("source_selection", "Source backup selection did not run because target contour preflight failed"),
			blockedMigrateStep("compatibility", "Migration compatibility checks did not run because target contour preflight failed"),
			blockedMigrateStep("runtime_prepare", "Target runtime preparation did not run because target contour preflight failed"),
			blockedMigrateStep("db_restore", "Database restore did not run because target contour preflight failed"),
			blockedMigrateStep("files_restore", "Files restore did not run because target contour preflight failed"),
			blockedMigrateStep("target_start", "Target contour start did not run because target contour preflight failed"),
		)
		return info, wrapExecuteError(err)
	}
	defer func() {
		_ = targetCtx.Release()
	}()

	targetRuntimeContract, err := targetCtx.Env.RuntimeContract()
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "target_preflight",
				Status:  domainworkflow.StatusFailed,
				Summary: "Target contour preflight failed",
				Details: err.Error(),
				Action:  "Fix the explicit helper image and runtime ownership settings in the target env file before rerunning migrate.",
			},
			blockedMigrateStep("source_selection", "Source backup selection did not run because target contour preflight failed"),
			blockedMigrateStep("compatibility", "Migration compatibility checks did not run because target contour preflight failed"),
			blockedMigrateStep("runtime_prepare", "Target runtime preparation did not run because target contour preflight failed"),
			blockedMigrateStep("db_restore", "Database restore did not run because target contour preflight failed"),
			blockedMigrateStep("files_restore", "Files restore did not run because target contour preflight failed"),
			blockedMigrateStep("target_start", "Target contour start did not run because target contour preflight failed"),
		)
		return info, wrapExecuteError(executeFailure{Kind: domainfailure.KindValidation, Err: err})
	}

	info.TargetEnvFile = targetCtx.Env.FilePath
	info.TargetBackupRoot = targetCtx.BackupRoot
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "target_preflight",
		Status:  domainworkflow.StatusCompleted,
		Summary: "Target contour preflight completed",
		Details: fmt.Sprintf("Using %s with backup root %s.", info.TargetEnvFile, info.TargetBackupRoot),
	})

	selection, err := s.resolveSourceSelection(sourceEnv, req)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "source_selection",
				Status:  domainworkflow.StatusFailed,
				Summary: failureSummary(err, "Source backup selection failed"),
				Details: err.Error(),
				Action:  failureAction(err, "Resolve the source backup selection error before rerunning migrate."),
			},
			blockedMigrateStep("compatibility", "Migration compatibility checks did not run because source backup selection failed"),
			blockedMigrateStep("runtime_prepare", "Target runtime preparation did not run because source backup selection failed"),
			blockedMigrateStep("db_restore", "Database restore did not run because source backup selection failed"),
			blockedMigrateStep("files_restore", "Files restore did not run because source backup selection failed"),
			blockedMigrateStep("target_start", "Target contour start did not run because source backup selection failed"),
		)
		return info, wrapExecuteError(executeFailure{Kind: domainfailure.KindValidation, Err: err})
	}

	info.SelectionMode = selection.SelectionMode
	info.SelectedPrefix = selection.Prefix
	info.SelectedStamp = selection.Stamp
	info.ManifestTXTPath = selection.ManifestTXT
	info.ManifestJSONPath = selection.ManifestJSON
	info.DBBackupPath = selection.DBBackup
	info.FilesBackupPath = selection.FilesBackup
	info.Warnings = append(info.Warnings, selection.Warnings...)
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "source_selection",
		Status:  domainworkflow.StatusCompleted,
		Summary: sourceSelectionSummary(selection),
		Details: sourceSelectionDetails(selection),
	})

	if err := requireMigrationCompatibility(sourceEnv, targetCtx.Env, info.SourceScope, info.TargetScope); err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "compatibility",
				Status:  domainworkflow.StatusFailed,
				Summary: failureSummary(err, "Migration compatibility contract failed"),
				Details: err.Error(),
				Action:  failureAction(err, "Align the shared settings first and rerun espops doctor --scope all --project-dir <repo>."),
			},
			blockedMigrateStep("runtime_prepare", "Target runtime preparation did not run because migration compatibility checks failed"),
			blockedMigrateStep("db_restore", "Database restore did not run because migration compatibility checks failed"),
			blockedMigrateStep("files_restore", "Files restore did not run because migration compatibility checks failed"),
			blockedMigrateStep("target_start", "Target contour start did not run because migration compatibility checks failed"),
		)
		return info, wrapExecuteError(err)
	}
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "compatibility",
		Status:  domainworkflow.StatusCompleted,
		Summary: "Migration compatibility contract passed",
		Details: fmt.Sprintf("The source contour %s and target contour %s match on the governed migration settings.", info.SourceScope, info.TargetScope),
	})

	runtimePrep, err := s.prepareRuntime(info.ProjectDir, info.ComposeFile, info.TargetEnvFile)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "runtime_prepare",
				Status:  domainworkflow.StatusFailed,
				Summary: "Target runtime preparation failed",
				Details: err.Error(),
				Action:  "Resolve the target runtime preparation failure before rerunning migrate.",
			},
			blockedMigrateStep("db_restore", "Database restore did not run because target runtime preparation failed"),
			blockedMigrateStep("files_restore", "Files restore did not run because target runtime preparation failed"),
			blockedMigrateStep("target_start", "Target contour start did not run because target runtime preparation failed"),
		)
		return info, wrapExecuteError(err)
	}
	info.StartedDBTemporarily = runtimePrep.StartedDBTemporarily
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_prepare",
		Status:  domainworkflow.StatusCompleted,
		Summary: "Target runtime preparation completed",
		Details: runtimePrepareDetails(runtimePrep),
	})

	if req.SkipDB {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_restore",
			Status:  domainworkflow.StatusSkipped,
			Summary: "Database restore skipped",
			Details: "The database restore was skipped because of --skip-db.",
		})
	} else {
		dbContainer, err := s.resolveDBContainer(info.ProjectDir, info.ComposeFile, info.TargetEnvFile)
		if err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "db_restore",
					Status:  domainworkflow.StatusFailed,
					Summary: "Database restore failed",
					Details: err.Error(),
					Action:  "Resolve the target db container state before rerunning migrate.",
				},
				blockedMigrateStep("files_restore", "Files restore did not run because database restore failed"),
				blockedMigrateStep("target_start", "Target contour start did not run because database restore failed"),
			)
			return info, wrapExecuteError(err)
		}

		dbReq := s.restore.BuildDBRequest(targetCtx, info.ManifestJSONPath, info.DBBackupPath, dbContainer)
		if _, err := s.restore.RestoreDB(dbReq); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "db_restore",
					Status:  domainworkflow.StatusFailed,
					Summary: "Database restore failed",
					Details: err.Error(),
					Action:  "Resolve the database restore failure before rerunning migrate.",
				},
				blockedMigrateStep("files_restore", "Files restore did not run because database restore failed"),
				blockedMigrateStep("target_start", "Target contour start did not run because database restore failed"),
			)
			return info, wrapExecuteError(err)
		}

		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_restore",
			Status:  domainworkflow.StatusCompleted,
			Summary: "Database restore completed",
			Details: dbRestoreDetails(info, targetCtx.Env.Value("DB_NAME")),
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
		filesReq := s.restore.BuildFilesRequest(targetCtx, info.ManifestJSONPath, info.FilesBackupPath)
		if _, err := s.restore.RestoreFiles(filesReq); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_restore",
					Status:  domainworkflow.StatusFailed,
					Summary: "Files restore failed",
					Details: err.Error(),
					Action:  "Resolve the files restore failure before rerunning migrate.",
				},
				blockedMigrateStep("target_start", "Target contour start did not run because files restore failed"),
			)
			return info, wrapExecuteError(err)
		}
		if err := s.runtime.ReconcileEspoStoragePermissions(
			filesReq.TargetDir,
			targetRuntimeContract.HelperImage,
			targetRuntimeContract.UID,
			targetRuntimeContract.GID,
		); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_restore",
					Status:  domainworkflow.StatusFailed,
					Summary: "Files restore failed",
					Details: fmt.Sprintf("Files were restored but runtime permission reconciliation failed: %v", err),
					Action:  "Resolve the permission reconciliation failure before rerunning migrate.",
				},
				blockedMigrateStep("target_start", "Target contour start did not run because files restore failed"),
			)
			return info, wrapExecuteError(executeFailure{Kind: domainfailure.KindExternal, Err: err})
		}

		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_restore",
			Status:  domainworkflow.StatusCompleted,
			Summary: "Files restore completed",
			Details: filesRestoreDetails(info, filesReq.TargetDir),
		})
	}

	if req.NoStart {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "target_start",
			Status:  domainworkflow.StatusSkipped,
			Summary: "Target contour start skipped",
			Details: "The target application services were left stopped because of --no-start.",
		})
	} else {
		target := runtimeport.Target{
			ProjectDir:  info.ProjectDir,
			ComposeFile: info.ComposeFile,
			EnvFile:     info.TargetEnvFile,
		}
		if err := s.runtime.Up(target); err != nil {
			info.Steps = append(info.Steps, ExecuteStep{
				Code:    "target_start",
				Status:  domainworkflow.StatusFailed,
				Summary: "Target contour start failed",
				Details: err.Error(),
				Action:  "Resolve the target contour start failure before rerunning migrate.",
			})
			return info, wrapExecuteError(executeFailure{Kind: domainfailure.KindExternal, Err: err})
		}
		validatedServices := expectedStartedTargetServices()
		if err := s.runtime.WaitForServicesReady(target, domainruntime.DefaultReadinessTimeoutSeconds, validatedServices...); err != nil {
			info.Steps = append(info.Steps, ExecuteStep{
				Code:    "target_start",
				Status:  domainworkflow.StatusFailed,
				Summary: "Target contour start failed",
				Details: err.Error(),
				Action:  "Repair the target contour runtime health before treating this migration as successful.",
			})
			return info, wrapExecuteError(executeFailure{Kind: domainfailure.KindExternal, Err: err})
		}

		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "target_start",
			Status:  domainworkflow.StatusCompleted,
			Summary: "Target contour start completed",
			Details: fmt.Sprintf("Started the target contour %s with docker compose up -d and confirmed runtime health for: %s.", info.TargetScope, strings.Join(validatedServices, ", ")),
		})
	}

	info.Warnings = dedupeStrings(info.Warnings)
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
