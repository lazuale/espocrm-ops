package restoreflow

import (
	"errors"
	"fmt"
	"path/filepath"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type preparedFilesRestore struct {
	plan      FilesPlan
	filesPath string
	lock      restoreLock
}

func (s Service) PlanFiles(req FilesRequest) (plan FilesPlan, err error) {
	prepared, err := s.prepareFiles(req)
	if err != nil {
		return FilesPlan{}, err
	}
	defer func() {
		if releaseErr := releaseLockError(domainfailure.KindIO, "restore_files_failed", "files restore lock", prepared.lock.Release()); releaseErr != nil {
			if err == nil {
				err = releaseErr
			} else {
				err = errors.Join(err, releaseErr)
			}
		}
	}()

	return prepared.plan, nil
}

func (s Service) RestoreFiles(req FilesRequest) (plan FilesPlan, err error) {
	prepared, err := s.prepareFiles(req)
	if err != nil {
		return FilesPlan{}, err
	}
	defer func() {
		if releaseErr := releaseLockError(domainfailure.KindIO, "restore_files_failed", "files restore lock", prepared.lock.Release()); releaseErr != nil {
			if err == nil {
				err = releaseErr
			} else {
				err = errors.Join(err, releaseErr)
			}
		}
	}()

	if req.DryRun {
		return prepared.plan, nil
	}

	if err := s.executeFiles(req, prepared.filesPath); err != nil {
		return FilesPlan{}, err
	}

	return prepared.plan, nil
}

func (s Service) prepareFiles(req FilesRequest) (preparedFilesRestore, error) {
	if err := req.Validate(); err != nil {
		return preparedFilesRestore{}, err
	}

	filesPath, err := s.PreflightFiles(FilesPreflightRequest{
		ManifestPath: req.ManifestPath,
		FilesBackup:  req.FilesBackup,
		TargetDir:    req.TargetDir,
	})
	if err != nil {
		return preparedFilesRestore{}, err
	}

	lock, err := s.locks.AcquireRestoreFilesLock()
	if err != nil {
		return preparedFilesRestore{}, failure(domainfailure.KindConflict, "lock_acquire_failed", fmt.Errorf("files restore lock failed: %w", err))
	}

	return preparedFilesRestore{
		plan:      buildFilesPlan(req, filesPath),
		filesPath: filesPath,
		lock:      lock,
	}, nil
}

func (s Service) executeFiles(req FilesRequest, filesPath string) (err error) {
	stage, err := s.files.NewSiblingStage(req.TargetDir, "espops-restore-files")
	if err != nil {
		return failure(domainfailure.KindIO, "restore_files_failed", err)
	}
	defer func() {
		if cleanupErr := stage.Cleanup(); cleanupErr != nil {
			wrapped := failure(domainfailure.KindIO, "restore_files_failed", cleanupErr)
			if err == nil {
				err = wrapped
			} else {
				err = errors.Join(err, wrapped)
			}
		}
	}()

	if err := s.files.UnpackTarGz(filesPath, stage.PreparedDir(), domainbackup.ValidateFilesArchiveHeader); err != nil {
		return failure(domainfailure.KindRestore, "restore_files_failed", err)
	}

	preparedDir, err := s.preparedRestoreTreeRoot(stage.PreparedDir(), filepath.Base(req.TargetDir), req.FilesBackup != "")
	if err != nil {
		return failure(domainfailure.KindRestore, "restore_files_failed", err)
	}

	if err := s.files.ReplaceTree(req.TargetDir, preparedDir); err != nil {
		return failure(domainfailure.KindIO, "restore_files_failed", err)
	}

	return nil
}

func (s Service) preparedRestoreTreeRoot(stageDir, targetBase string, requireExactRoot bool) (string, error) {
	if requireExactRoot {
		return s.files.PreparedTreeRootExact(stageDir, targetBase)
	}

	return s.files.PreparedTreeRoot(stageDir, targetBase)
}
