package restore

import (
	"errors"
	"fmt"
	"path/filepath"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type preparedFilesRestore struct {
	plan      FilesRestorePlan
	filesPath string
	lock      restoreLock
}

func (s Service) PlanFilesRestore(req RestoreFilesRequest) (plan FilesRestorePlan, err error) {
	prepared, err := s.prepareFilesRestore(req)
	if err != nil {
		return FilesRestorePlan{}, err
	}
	defer func() {
		if releaseErr := prepared.lock.Release(); releaseErr != nil {
			wrapped := fmt.Errorf("release files restore lock: %w", releaseErr)
			if err == nil {
				err = wrapped
			} else {
				err = errors.Join(err, wrapped)
			}
		}
	}()

	return prepared.plan, nil
}

func (s Service) RestoreFiles(req RestoreFilesRequest) (plan FilesRestorePlan, err error) {
	prepared, err := s.prepareFilesRestore(req)
	if err != nil {
		return FilesRestorePlan{}, err
	}
	defer func() {
		if releaseErr := prepared.lock.Release(); releaseErr != nil {
			wrapped := fmt.Errorf("release files restore lock: %w", releaseErr)
			if err == nil {
				err = wrapped
			} else {
				err = errors.Join(err, wrapped)
			}
		}
	}()

	if req.DryRun {
		return prepared.plan, nil
	}

	if err := s.executeFilesRestore(req, prepared.filesPath); err != nil {
		return FilesRestorePlan{}, err
	}

	return prepared.plan, nil
}

func (s Service) prepareFilesRestore(req RestoreFilesRequest) (preparedFilesRestore, error) {
	if err := req.Validate(); err != nil {
		return preparedFilesRestore{}, err
	}

	filesPath, err := s.PreflightFilesRestore(FilesPreflightRequest{
		ManifestPath: req.ManifestPath,
		FilesBackup:  req.FilesBackup,
		TargetDir:    req.TargetDir,
	})
	if err != nil {
		return preparedFilesRestore{}, err
	}

	lock, err := s.locks.AcquireRestoreFilesLock()
	if err != nil {
		return preparedFilesRestore{}, restoreFailure(domainfailure.KindConflict, "lock_acquire_failed", fmt.Errorf("files restore lock failed: %w", err))
	}

	return preparedFilesRestore{
		plan:      buildFilesRestorePlan(req, filesPath),
		filesPath: filesPath,
		lock:      lock,
	}, nil
}

func (s Service) executeFilesRestore(req RestoreFilesRequest, filesPath string) (err error) {
	stage, err := s.files.NewSiblingStage(req.TargetDir, "espops-restore-files")
	if err != nil {
		return restoreFailure(domainfailure.KindIO, "restore_files_failed", err)
	}
	defer func() {
		if cleanupErr := stage.Cleanup(); cleanupErr != nil {
			wrapped := restoreFailure(domainfailure.KindIO, "restore_files_failed", cleanupErr)
			if err == nil {
				err = wrapped
			} else {
				err = errors.Join(err, wrapped)
			}
		}
	}()

	if err := s.files.UnpackTarGz(filesPath, stage.PreparedDir(), domainbackup.ValidateFilesArchiveHeader); err != nil {
		return restoreFailure(domainfailure.KindRestore, "restore_files_failed", err)
	}

	preparedDir, err := s.preparedRestoreTreeRoot(stage.PreparedDir(), filepath.Base(req.TargetDir), req.FilesBackup != "")
	if err != nil {
		return restoreFailure(domainfailure.KindRestore, "restore_files_failed", err)
	}

	if err := s.files.ReplaceTree(req.TargetDir, preparedDir); err != nil {
		return restoreFailure(domainfailure.KindIO, "restore_files_failed", err)
	}

	return nil
}

func (s Service) preparedRestoreTreeRoot(stageDir, targetBase string, requireExactRoot bool) (string, error) {
	if requireExactRoot {
		return s.files.PreparedTreeRootExact(stageDir, targetBase)
	}

	return s.files.PreparedTreeRoot(stageDir, targetBase)
}
