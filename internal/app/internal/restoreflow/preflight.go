package restoreflow

import (
	"fmt"
	"path/filepath"
	"strings"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type ManifestSource struct {
	ManifestPath string
	DBBackupPath string
	FilesPath    string
}

func (s Service) PreflightFiles(req FilesPreflightRequest) (string, error) {
	if err := req.Validate(); err != nil {
		return "", err
	}

	filesPath, err := s.filesRestoreSourcePath(req.ManifestPath, req.FilesBackup)
	if err != nil {
		return "", err
	}
	if req.FilesBackup != "" {
		if err := s.store.VerifyDirectFilesBackup(filesPath); err != nil {
			return "", err
		}
	}
	filesSize, err := s.files.EnsureNonEmptyFile("files backup", filesPath)
	if err != nil {
		return "", failure(domainfailure.KindIO, "filesystem_error", err)
	}

	parent := filepath.Dir(req.TargetDir)
	if err := s.files.EnsureWritableDir(parent); err != nil {
		return "", failure(domainfailure.KindIO, "filesystem_error", err)
	}
	if err := s.files.EnsureFreeSpace(parent, uint64(filesSize)); err != nil {
		return "", failure(domainfailure.KindIO, "filesystem_error", err)
	}

	return filesPath, nil
}

func (s Service) filesRestoreSourcePath(manifestPath, filesBackup string) (string, error) {
	if filesBackup != "" {
		return filesBackup, nil
	}

	info, err := s.ResolveManifestSource(manifestPath, false, true)
	if err != nil {
		return "", err
	}

	return info.FilesPath, nil
}

func (s Service) PreflightDB(req DBPreflightRequest) (string, error) {
	if err := req.Validate(); err != nil {
		return "", err
	}

	dbPath, err := s.dbRestoreSourcePath(req.ManifestPath, req.DBBackup)
	if err != nil {
		return "", err
	}
	if req.DBBackup != "" {
		if err := s.store.VerifyDirectDBBackup(dbPath); err != nil {
			return "", err
		}
	}

	if err := s.runtime.CheckDockerAvailable(); err != nil {
		return "", failure(domainfailure.KindExternal, "restore_db_failed", err)
	}

	if err := s.runtime.CheckContainerRunning(req.DBContainer); err != nil {
		return "", failure(domainfailure.KindExternal, "restore_db_failed", err)
	}

	return dbPath, nil
}

func (s Service) dbRestoreSourcePath(manifestPath, dbBackup string) (string, error) {
	if dbBackup != "" {
		return dbBackup, nil
	}

	info, err := s.ResolveManifestSource(manifestPath, true, false)
	if err != nil {
		return "", err
	}

	return info.DBBackupPath, nil
}

func (s Service) ResolveManifestSource(manifestPath string, needDB, needFiles bool) (ManifestSource, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return ManifestSource{}, fmt.Errorf("manifest path is required")
	}
	info, err := s.store.VerifyManifestSelection(manifestPath, needDB, needFiles)
	if err != nil {
		return ManifestSource{}, err
	}

	return ManifestSource{
		ManifestPath: info.ManifestPath,
		DBBackupPath: info.DBBackupPath,
		FilesPath:    info.FilesPath,
	}, nil
}
