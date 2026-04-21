package restore

import (
	"path/filepath"

	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

func (s Service) PreflightFilesRestore(req FilesPreflightRequest) (string, error) {
	if err := req.Validate(); err != nil {
		return "", err
	}

	filesPath, err := filesRestoreSourcePath(req.ManifestPath, req.FilesBackup)
	if err != nil {
		return "", err
	}
	if req.FilesBackup != "" {
		if err := backupstore.VerifyDirectFilesBackup(filesPath); err != nil {
			return "", PreflightError{Err: err}
		}
	}
	filesSize, err := platformfs.EnsureNonEmptyFile("files backup", filesPath)
	if err != nil {
		return "", PreflightError{Err: err}
	}

	parent := filepath.Dir(req.TargetDir)
	if err := platformfs.EnsureWritableDir(parent); err != nil {
		return "", PreflightError{Err: err}
	}
	if err := platformfs.EnsureFreeSpace(parent, uint64(filesSize)); err != nil {
		return "", PreflightError{Err: err}
	}

	return filesPath, nil
}

func filesRestoreSourcePath(manifestPath, filesBackup string) (string, error) {
	if filesBackup != "" {
		return filesBackup, nil
	}

	info, err := backupstore.VerifyManifestDetailed(manifestPath)
	if err != nil {
		return "", PreflightError{Err: err}
	}

	return info.FilesPath, nil
}

func (s Service) PreflightDBRestore(req DBPreflightRequest) (string, error) {
	if err := req.Validate(); err != nil {
		return "", err
	}

	dbPath, err := dbRestoreSourcePath(req.ManifestPath, req.DBBackup)
	if err != nil {
		return "", err
	}
	if req.DBBackup != "" {
		if err := backupstore.VerifyDirectDBBackup(dbPath); err != nil {
			return "", PreflightError{Err: err}
		}
	}

	if err := platformdocker.CheckDockerAvailable(); err != nil {
		return "", PreflightError{Err: err}
	}

	if err := platformdocker.CheckContainerRunning(req.DBContainer); err != nil {
		return "", PreflightError{Err: err}
	}

	return dbPath, nil
}

func dbRestoreSourcePath(manifestPath, dbBackup string) (string, error) {
	if dbBackup != "" {
		return dbBackup, nil
	}

	info, err := backupstore.VerifyManifestDetailed(manifestPath)
	if err != nil {
		return "", PreflightError{Err: err}
	}

	return info.DBBackupPath, nil
}
