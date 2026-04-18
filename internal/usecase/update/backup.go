package update

import (
	"fmt"
	"io"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
)

type BackupApplyRequest struct {
	TimeoutSeconds int
	LogWriter      io.Writer
	Backup         backupusecase.ExecuteRequest
}

type BackupApplyInfo struct {
	TimeoutSeconds         int
	StartedDBTemporarily   bool
	CreatedAt              string
	ConsistentSnapshot     bool
	AppServicesWereRunning bool
	ManifestTXTPath        string
	ManifestJSONPath       string
	DBBackupPath           string
	FilesBackupPath        string
	DBSidecarPath          string
	FilesSidecarPath       string
}

func ApplyBackup(req BackupApplyRequest) (BackupApplyInfo, error) {
	info := BackupApplyInfo{
		TimeoutSeconds: req.TimeoutSeconds,
	}

	cfg := platformdocker.ComposeConfig{
		ProjectDir:  req.Backup.ProjectDir,
		ComposeFile: req.Backup.ComposeFile,
		EnvFile:     req.Backup.EnvFile,
	}

	state, err := platformdocker.ComposeServiceStateFor(cfg, "db")
	if err != nil {
		return info, apperr.Wrap(apperr.KindExternal, "update_backup_failed", err)
	}

	if state.Status != "running" && state.Status != "healthy" {
		info.StartedDBTemporarily = true
		if req.LogWriter != nil {
			if _, err := fmt.Fprintln(req.LogWriter, "[info] The DB container was not running, starting db temporarily for backup"); err != nil {
				return info, apperr.Wrap(apperr.KindIO, "update_backup_failed", err)
			}
		}
		if err := platformdocker.ComposeUp(cfg, "db"); err != nil {
			return info, apperr.Wrap(apperr.KindExternal, "update_backup_failed", err)
		}

		deadline := time.Now().UTC().Add(time.Duration(req.TimeoutSeconds) * time.Second)
		if err := waitForServiceReady(cfg, "db", deadline, req.TimeoutSeconds); err != nil {
			return info, apperr.Wrap(apperr.KindExternal, "update_backup_failed", err)
		}
	}

	backupInfo, err := backupusecase.ExecuteBackup(req.Backup)
	if err != nil {
		return info, wrapUpdateBackupError(err)
	}

	info.CreatedAt = backupInfo.CreatedAt
	info.ConsistentSnapshot = backupInfo.ConsistentSnapshot
	info.AppServicesWereRunning = backupInfo.AppServicesWereRunning
	info.ManifestTXTPath = backupInfo.ManifestTXTPath
	info.ManifestJSONPath = backupInfo.ManifestJSONPath
	info.DBBackupPath = backupInfo.DBBackupPath
	info.FilesBackupPath = backupInfo.FilesBackupPath
	info.DBSidecarPath = backupInfo.DBSidecarPath
	info.FilesSidecarPath = backupInfo.FilesSidecarPath

	return info, nil
}

func wrapUpdateBackupError(err error) error {
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "update_backup_failed", err)
	}

	return apperr.Wrap(apperr.KindExternal, "update_backup_failed", err)
}
