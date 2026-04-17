package restore

import (
	"fmt"
	"path/filepath"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
)

type preparedFilesRestore struct {
	plan      FilesRestorePlan
	filesPath string
	lock      *platformlocks.FileLock
}

func PlanFilesRestore(req RestoreFilesRequest) (FilesRestorePlan, error) {
	prepared, err := prepareFilesRestore(req)
	if err != nil {
		return FilesRestorePlan{}, err
	}
	defer prepared.lock.Release()

	return prepared.plan, nil
}

func RestoreFiles(req RestoreFilesRequest) (FilesRestorePlan, error) {
	prepared, err := prepareFilesRestore(req)
	if err != nil {
		return FilesRestorePlan{}, err
	}
	defer prepared.lock.Release()

	if req.DryRun {
		return prepared.plan, nil
	}

	if err := executeFilesRestore(req, prepared.filesPath); err != nil {
		return FilesRestorePlan{}, err
	}

	return prepared.plan, nil
}

func prepareFilesRestore(req RestoreFilesRequest) (preparedFilesRestore, error) {
	if err := req.Validate(); err != nil {
		return preparedFilesRestore{}, err
	}

	filesPath, err := PreflightFilesRestore(FilesPreflightRequest{
		ManifestPath: req.ManifestPath,
		FilesBackup:  req.FilesBackup,
		TargetDir:    req.TargetDir,
	})
	if err != nil {
		return preparedFilesRestore{}, err
	}

	lock, err := platformlocks.AcquireRestoreFilesLock()
	if err != nil {
		return preparedFilesRestore{}, LockError{Err: fmt.Errorf("files restore lock failed: %w", err)}
	}

	return preparedFilesRestore{
		plan:      buildFilesRestorePlan(req, filesPath),
		filesPath: filesPath,
		lock:      lock,
	}, nil
}

func executeFilesRestore(req RestoreFilesRequest, filesPath string) error {
	stage, err := platformfs.NewSiblingStage(req.TargetDir, "espops-restore-files")
	if err != nil {
		return OperationError{Err: err, FallbackCode: "restore_files_failed"}
	}
	defer stage.Cleanup()

	if err := platformfs.UnpackTarGz(filesPath, stage.PreparedDir, domainbackup.ValidateFilesArchiveHeader); err != nil {
		return OperationError{Err: err, FallbackCode: "restore_files_failed"}
	}

	preparedDir, err := preparedRestoreTreeRoot(stage.PreparedDir, filepath.Base(req.TargetDir), req.FilesBackup != "")
	if err != nil {
		return OperationError{Err: err, FallbackCode: "restore_files_failed"}
	}

	if err := platformfs.ReplaceTree(req.TargetDir, preparedDir); err != nil {
		return OperationError{Err: err, FallbackCode: "restore_files_failed"}
	}

	return nil
}

func preparedRestoreTreeRoot(stageDir, targetBase string, requireExactRoot bool) (string, error) {
	if requireExactRoot {
		return platformfs.PreparedTreeRootExact(stageDir, targetBase)
	}

	return platformfs.PreparedTreeRoot(stageDir, targetBase)
}
