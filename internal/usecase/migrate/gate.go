package migrate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
	"github.com/lazuale/espocrm-ops/internal/usecase/reporting"
)

const (
	GateCheckPassed  = "passed"
	GateCheckBlocked = "blocked"
)

type GateRequest struct {
	SourceScope string
	TargetScope string
	ProjectDir  string
	ComposeFile string
	DBBackup    string
	FilesBackup string
	SkipDB      bool
	SkipFiles   bool
	NoStart     bool
}

type GateCheck struct {
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type GateInfo struct {
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
	Warnings               []string
	Checks                 []GateCheck
}

func Preflight(req GateRequest) (GateInfo, error) {
	info := GateInfo{
		SourceScope:            strings.TrimSpace(req.SourceScope),
		TargetScope:            strings.TrimSpace(req.TargetScope),
		ProjectDir:             filepath.Clean(req.ProjectDir),
		ComposeFile:            filepath.Clean(req.ComposeFile),
		RequestedDBBackup:      strings.TrimSpace(req.DBBackup),
		RequestedFilesBackup:   strings.TrimSpace(req.FilesBackup),
		RequestedSelectionMode: requestedSelectionMode(ExecuteRequest{DBBackup: req.DBBackup, FilesBackup: req.FilesBackup, SkipDB: req.SkipDB, SkipFiles: req.SkipFiles}),
		Warnings:               flagWarnings(ExecuteRequest{SkipDB: req.SkipDB, SkipFiles: req.SkipFiles, NoStart: req.NoStart}),
	}

	sourceEnv, err := platformconfig.LoadOperationEnv(info.ProjectDir, info.SourceScope, "", "")
	if err != nil {
		info.Checks = append(info.Checks, GateCheck{
			Code:    "source_preflight",
			Status:  GateCheckBlocked,
			Summary: "Source contour preflight failed",
			Details: err.Error(),
			Action:  "Resolve the source env file or source backup root settings before rerunning migrate-backup.",
		})
		return info, wrapMigrationEnvError(err)
	}

	info.SourceEnvFile = sourceEnv.FilePath
	info.SourceBackupRoot = platformconfig.ResolveProjectPath(info.ProjectDir, sourceEnv.BackupRoot())
	info.Checks = append(info.Checks, GateCheck{
		Code:    "source_preflight",
		Status:  GateCheckPassed,
		Summary: "Source contour preflight passed",
		Details: fmt.Sprintf("Using %s with backup root %s.", info.SourceEnvFile, info.SourceBackupRoot),
	})

	targetCtx, err := maintenanceusecase.PrepareOperation(maintenanceusecase.OperationContextRequest{
		Scope:      info.TargetScope,
		Operation:  "migrate-backup",
		ProjectDir: info.ProjectDir,
	})
	if err != nil {
		info.Checks = append(info.Checks, GateCheck{
			Code:    "target_preflight",
			Status:  GateCheckBlocked,
			Summary: "Target contour preflight failed",
			Details: err.Error(),
			Action:  "Resolve env, lock, or filesystem readiness before rerunning migrate-backup.",
		})
		return info, wrapExecuteError(err)
	}
	defer func() {
		_ = targetCtx.Release()
	}()

	info.TargetEnvFile = targetCtx.Env.FilePath
	info.TargetBackupRoot = targetCtx.BackupRoot
	info.Checks = append(info.Checks, GateCheck{
		Code:    "target_preflight",
		Status:  GateCheckPassed,
		Summary: "Target contour preflight passed",
		Details: fmt.Sprintf("Using %s with backup root %s.", info.TargetEnvFile, info.TargetBackupRoot),
	})

	selection, err := resolveSourceSelection(sourceEnv, ExecuteRequest{
		ProjectDir:  info.ProjectDir,
		DBBackup:    req.DBBackup,
		FilesBackup: req.FilesBackup,
		SkipDB:      req.SkipDB,
		SkipFiles:   req.SkipFiles,
	})
	if err != nil {
		info.Checks = append(info.Checks, GateCheck{
			Code:    "source_selection",
			Status:  GateCheckBlocked,
			Summary: failureSummary(err, "Source backup selection failed"),
			Details: err.Error(),
			Action:  failureAction(err, "Resolve the source backup selection error before rerunning migrate-backup."),
		})
		return info, apperr.Wrap(apperr.KindValidation, "migrate_backup_failed", err)
	}

	info.SelectionMode = selection.SelectionMode
	info.SelectedPrefix = selection.Prefix
	info.SelectedStamp = selection.Stamp
	info.ManifestTXTPath = selection.ManifestTXT
	info.ManifestJSONPath = selection.ManifestJSON
	info.DBBackupPath = selection.DBBackup
	info.FilesBackupPath = selection.FilesBackup
	info.Warnings = append(info.Warnings, selection.Warnings...)
	info.Checks = append(info.Checks, GateCheck{
		Code:    "source_selection",
		Status:  GateCheckPassed,
		Summary: sourceSelectionSummary(selection),
		Details: sourceSelectionDetails(selection),
	})

	if err := requireMigrationCompatibility(sourceEnv, targetCtx.Env, info.SourceScope, info.TargetScope); err != nil {
		info.Checks = append(info.Checks, GateCheck{
			Code:    "compatibility",
			Status:  GateCheckBlocked,
			Summary: failureSummary(err, "Migration compatibility contract failed"),
			Details: err.Error(),
			Action:  failureAction(err, "Align the shared settings first and rerun ./scripts/doctor.sh all."),
		})
		return info, apperr.Wrap(apperr.KindValidation, "migrate_backup_failed", err)
	}
	info.Checks = append(info.Checks, GateCheck{
		Code:    "compatibility",
		Status:  GateCheckPassed,
		Summary: "Migration compatibility contract passed",
		Details: fmt.Sprintf("The source contour %s and target contour %s match on the governed migration settings.", info.SourceScope, info.TargetScope),
	})

	info.Warnings = reporting.DedupeStrings(info.Warnings)
	return info, nil
}
