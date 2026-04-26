package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/runtime"
)

type RestoreResult struct {
	Manifest         string
	SnapshotManifest string
}

type restoreRuntime interface {
	backupRuntime
	ResetDatabase(ctx context.Context, target runtime.Target) error
	RestoreDatabase(ctx context.Context, target runtime.Target, sourcePath string) error
	DBPing(ctx context.Context, target runtime.Target) error
}

type preparedRestoreFiles struct {
	stagingDir string
	committed  bool
}

func Restore(ctx context.Context, cfg config.Config, manifestPath string, rt restoreRuntime, now time.Time) (RestoreResult, error) {
	if rt == nil {
		return RestoreResult{}, runtimeError("restore runtime is required", nil)
	}
	return withProjectLock(ctx, cfg.ProjectDir, func(lockedCtx context.Context) (RestoreResult, error) {
		return restoreLocked(lockedCtx, cfg, manifestPath, rt, now)
	})
}

func restoreLocked(ctx context.Context, cfg config.Config, manifestPath string, rt restoreRuntime, now time.Time) (result RestoreResult, err error) {
	result.Manifest = strings.TrimSpace(manifestPath)
	if err := validateConfig(cfg); err != nil {
		return result, usageError(err.Error())
	}
	if result.Manifest == "" {
		return result, usageError("--manifest is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	verifyResult, verifyErr := VerifyBackup(ctx, result.Manifest)
	if verifyErr != nil {
		return result, verifyErr
	}
	result.Manifest = verifyResult.Manifest
	if err := requireRestoreManifestMatchesTarget(cfg, verifyResult); err != nil {
		return result, err
	}

	snapshotResult, snapshotErr := backupCore(ctx, cfg, rt, now)
	if snapshotErr != nil {
		return result, snapshotErr
	}
	result.SnapshotManifest = snapshotResult.Manifest

	prepared, err := prepareRestoreFiles(ctx, verifyResult.FilesBackup, cfg.StorageDir)
	if err != nil {
		return result, err
	}
	defer func() {
		if cleanupErr := prepared.Cleanup(); cleanupErr != nil && err == nil {
			err = ioError("failed to clean restore staging directory", cleanupErr)
		}
	}()

	target := targetFromConfig(cfg)
	servicesStopped := false
	servicesReturned := false
	defer func() {
		if !servicesStopped || servicesReturned {
			return
		}
		startCtx, cancel := serviceReturnContext()
		startErr := rt.StartServices(startCtx, target, cfg.AppServices)
		cancel()
		if startErr == nil {
			return
		}
		err = errors.Join(err, runtimeError("failed to return app services", startErr))
	}()

	if err := rt.StopServices(ctx, target, cfg.AppServices); err != nil {
		return result, runtimeError("failed to stop app services", err)
	}
	servicesStopped = true

	if err := rt.ResetDatabase(ctx, target); err != nil {
		return result, runtimeError("database reset failed", err)
	}
	if err := rt.RestoreDatabase(ctx, target, verifyResult.DBBackup); err != nil {
		return result, runtimeError("database restore failed", err)
	}
	if err := switchStorageFromStaging(ctx, prepared, cfg.StorageDir); err != nil {
		return result, err
	}

	startCtx, cancel := serviceReturnContext()
	if err := rt.StartServices(startCtx, target, cfg.AppServices); err != nil {
		cancel()
		return result, runtimeError("failed to start app services", err)
	}
	cancel()
	servicesReturned = true

	if err := waitForRuntimeServiceHealth(ctx, target, cfg.DBService, cfg.AppServices, rt); err != nil {
		return result, runtimeError("restore health check failed", err)
	}
	if err := rt.DBPing(ctx, target); err != nil {
		return result, runtimeError("restore db ping failed", err)
	}

	return result, nil
}

func requireRestoreManifestMatchesTarget(cfg config.Config, result VerifyResult) error {
	if result.Scope != cfg.Scope {
		return manifestError("manifest scope does not match target", fmt.Errorf("manifest scope %q does not match target scope %q", result.Scope, cfg.Scope))
	}
	if result.DBName != cfg.DBName {
		return manifestError("manifest database does not match target", fmt.Errorf("manifest db_name %q does not match target DB_NAME %q", result.DBName, cfg.DBName))
	}
	return nil
}

func prepareRestoreFiles(ctx context.Context, archivePath, storageDir string) (*preparedRestoreFiles, error) {
	if err := ctx.Err(); err != nil {
		return nil, ioError("restore interrupted", err)
	}
	if err := ensureStorageDir(storageDir); err != nil {
		return nil, ioError("restore storage target is invalid", err)
	}
	stagingDir, err := os.MkdirTemp(filepath.Dir(storageDir), ".espops-restore-*")
	if err != nil {
		return nil, ioError("failed to create restore staging directory", err)
	}
	prepared := &preparedRestoreFiles{stagingDir: stagingDir}
	if err := runtime.ExtractStorageArchive(ctx, archivePath, stagingDir); err != nil {
		_ = prepared.Cleanup()
		return nil, archiveError("files restore extraction failed", err)
	}
	return prepared, nil
}

func (p *preparedRestoreFiles) Cleanup() error {
	if p == nil || p.committed || strings.TrimSpace(p.stagingDir) == "" {
		return nil
	}
	return os.RemoveAll(p.stagingDir)
}

func switchStorageFromStaging(ctx context.Context, prepared *preparedRestoreFiles, storageDir string) error {
	if err := ctx.Err(); err != nil {
		return ioError("restore interrupted", err)
	}
	if prepared == nil || strings.TrimSpace(prepared.stagingDir) == "" {
		return ioError("files switch staging is invalid", fmt.Errorf("prepared staging is required"))
	}
	if err := ensureStorageDir(prepared.stagingDir); err != nil {
		return ioError("files switch staging is invalid", err)
	}
	if err := ensureStorageDir(storageDir); err != nil {
		return ioError("files switch target is invalid", err)
	}
	if filepath.Clean(filepath.Dir(prepared.stagingDir)) != filepath.Clean(filepath.Dir(storageDir)) {
		return ioError("files switch staging is invalid", fmt.Errorf("staging must be next to storage dir"))
	}

	rollbackDir, err := newRollbackDir(storageDir)
	if err != nil {
		return ioError("files switch rollback allocation failed", err)
	}
	if err := os.Rename(storageDir, rollbackDir); err != nil {
		return ioError("files switch failed before target switch", err)
	}
	if err := os.Rename(prepared.stagingDir, storageDir); err != nil {
		switchErr := err
		if rollbackErr := rollbackStorage(storageDir, rollbackDir); rollbackErr != nil {
			return ioError("files switch failed during target switch; rollback failed", errors.Join(switchErr, rollbackErr))
		}
		return ioError("files switch failed during target switch; rolled back current storage", switchErr)
	}
	prepared.committed = true

	if err := os.RemoveAll(rollbackDir); err != nil {
		return ioError("files switch cleanup failed; restored storage is active and old storage rollback remains", err)
	}
	if err := ensureStorageDir(storageDir); err != nil {
		return ioError("files switch post-check failed", err)
	}
	return nil
}

func newRollbackDir(storageDir string) (string, error) {
	rollbackDir, err := os.MkdirTemp(filepath.Dir(storageDir), ".espops-rollback-*")
	if err != nil {
		return "", err
	}
	if err := os.Remove(rollbackDir); err != nil {
		return "", err
	}
	return rollbackDir, nil
}

func rollbackStorage(storageDir, rollbackDir string) error {
	if _, err := os.Stat(storageDir); err == nil {
		return fmt.Errorf("target path exists; old storage remains at rollback path %s", rollbackDir)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(rollbackDir, storageDir)
}
