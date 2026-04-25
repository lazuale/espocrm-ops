package ops

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
)

type RestoreResult struct {
	Manifest         string
	SnapshotManifest string
}

type restoreRuntime interface {
	backupRuntime
	UpService(ctx context.Context, target runtime.Target, service string) error
	ResetDatabase(ctx context.Context, target runtime.Target) error
	RestoreDatabase(ctx context.Context, target runtime.Target, reader io.Reader) error
	DBPing(ctx context.Context, target runtime.Target) error
}

func Restore(ctx context.Context, cfg config.BackupConfig, manifestPath string, rt restoreRuntime, now time.Time) (result RestoreResult, err error) {
	return restoreWithOptions(ctx, cfg, manifestPath, rt, now, restoreOptions{
		scopeErrorMessage: "restore source scope is invalid",
	})
}

type restoreOptions struct {
	allowedSourceScope string
	scopeErrorMessage  string
}

const restoreStagingDirPattern = "espops-restore-staging-*"
const restoreRollbackDirPattern = "espops-restore-rollback-*"

var (
	createRestoreStagingDir    = defaultCreateRestoreStagingDir
	restoreExtractTarEntry     = extractTarEntry
	restoreValidateStorageTree = validateRestoredStorageTree
	restoreApplyOwnership      = applyRestoreOwnership
	restoreRenamePath          = os.Rename
	restoreRemoveAll           = os.RemoveAll
	restoreOwnershipFS         = restoreOwnershipOps{
		lstat:  os.Lstat,
		lchown: os.Lchown,
	}
)

type restoreOwnershipOps struct {
	lstat  func(string) (os.FileInfo, error)
	lchown func(string, int, int) error
}

func restoreWithOptions(ctx context.Context, cfg config.BackupConfig, manifestPath string, rt restoreRuntime, now time.Time, opts restoreOptions) (result RestoreResult, err error) {
	if rt == nil {
		return RestoreResult{}, runtimeError("restore runtime is required", nil)
	}
	if err := validateRestoreInputs(cfg, manifestPath); err != nil {
		return RestoreResult{}, &VerifyError{Kind: ErrorKindUsage, Message: err.Error()}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	return withOperationLocks(ctx, []operationLockSpec{{
		ProjectDir: cfg.ProjectDir,
		Scope:      cfg.Scope,
	}}, "restore lock failed", func(lockedCtx context.Context) (RestoreResult, error) {
		return restoreWithOptionsLocked(lockedCtx, cfg, manifestPath, rt, now, opts)
	})
}

func restoreWithOptionsLocked(ctx context.Context, cfg config.BackupConfig, manifestPath string, rt restoreRuntime, now time.Time, opts restoreOptions) (result RestoreResult, err error) {
	result.Manifest = manifestPath

	verifyResult, verifyErr := VerifyBackup(ctx, manifestPath)
	if verifyErr != nil {
		return result, verifyErr
	}
	result.Manifest = verifyResult.Manifest
	if err := validateRestoreSourceScope(cfg.Scope, verifyResult.Scope, opts); err != nil {
		return result, err
	}
	if err := requireRestoreRuntimeContract(cfg, verifyResult); err != nil {
		return result, err
	}
	if err := ensureRestoreStorageClearable(cfg.StorageDir); err != nil {
		return result, ioError("restore storage target is not clearable", err)
	}

	snapshotResult, snapshotErr := Backup(ctx, cfg, rt, now)
	if snapshotErr != nil {
		return result, snapshotErr
	}
	result.SnapshotManifest = snapshotResult.Manifest

	target := runtime.Target{
		ProjectDir:     cfg.ProjectDir,
		ComposeFile:    cfg.ComposeFile,
		EnvFile:        cfg.EnvFile,
		DBService:      cfg.DBService,
		DBUser:         cfg.DBUser,
		DBPassword:     cfg.DBPassword,
		DBRootPassword: cfg.DBRootPassword,
		DBName:         cfg.DBName,
	}

	servicesStopped := false
	servicesReturned := false
	serviceReturnAttempted := false
	defer func() {
		if !servicesStopped || servicesReturned || serviceReturnAttempted {
			return
		}
		serviceReturnAttempted = true
		startCtx, cancel := serviceReturnContext()
		startErr := rt.StartServices(startCtx, target, cfg.AppServices)
		cancel()
		if startErr == nil {
			servicesReturned = true
			return
		}
		err = combineServiceReturnError(err, startErr)
	}()

	if err := rt.StopServices(ctx, target, cfg.AppServices); err != nil {
		return result, runtimeError("failed to stop app services", err)
	}
	servicesStopped = true

	if err := rt.UpService(ctx, target, cfg.DBService); err != nil {
		return result, runtimeError("failed to ensure db service", err)
	}
	if err := resetDatabase(ctx, rt, target); err != nil {
		return result, err
	}
	if err := restoreDatabaseBackup(ctx, verifyResult.DBBackup, rt, target); err != nil {
		return result, err
	}
	if err := restoreFilesBackup(ctx, verifyResult.FilesBackup, cfg.StorageDir, cfg.RuntimeUID, cfg.RuntimeGID); err != nil {
		return result, err
	}

	serviceReturnAttempted = true
	startCtx, cancel := serviceReturnContext()
	if err := rt.StartServices(startCtx, target, cfg.AppServices); err != nil {
		cancel()
		return result, runtimeError("failed to return app services", err)
	}
	cancel()
	servicesReturned = true

	if err := waitForRuntimeServiceHealth(ctx, target, cfg.DBService, cfg.AppServices, rt); err != nil {
		return result, runtimeError("restore post-check failed", err)
	}

	if err := rt.DBPing(ctx, target); err != nil {
		return result, runtimeError("restore post-check failed", err)
	}

	return result, nil
}

func validateRestoreInputs(cfg config.BackupConfig, manifestPath string) error {
	if err := validateBackupConfig(cfg); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.DBRootPassword) == "" {
		return fmt.Errorf("DB_ROOT_PASSWORD is required")
	}
	if !cfg.RuntimeOwnershipConfigured {
		return fmt.Errorf("ESPO_RUNTIME_UID and ESPO_RUNTIME_GID are required")
	}
	if cfg.RuntimeUID < 0 {
		return fmt.Errorf("ESPO_RUNTIME_UID must be >= 0")
	}
	if cfg.RuntimeGID < 0 {
		return fmt.Errorf("ESPO_RUNTIME_GID must be >= 0")
	}
	if strings.TrimSpace(manifestPath) == "" {
		return fmt.Errorf("manifest path is required")
	}
	return nil
}

func validateRestoreSourceScope(targetScope, sourceScope string, opts restoreOptions) error {
	wantScope := strings.TrimSpace(targetScope)
	if allowed := strings.TrimSpace(opts.allowedSourceScope); allowed != "" {
		wantScope = allowed
	}
	if sourceScope == wantScope {
		return nil
	}

	message := strings.TrimSpace(opts.scopeErrorMessage)
	if message == "" {
		message = "restore source scope is invalid"
	}
	return &VerifyError{
		Kind:    ErrorKindUsage,
		Message: message,
		Err: fmt.Errorf(
			"manifest scope %q does not match required scope %q",
			sourceScope,
			wantScope,
		),
	}
}

func resetDatabase(ctx context.Context, rt restoreRuntime, target runtime.Target) error {
	if err := ctx.Err(); err != nil {
		return ioError("restore interrupted", err)
	}
	if err := rt.ResetDatabase(ctx, target); err != nil {
		return runtimeError("database reset failed", err)
	}
	return nil
}

func restoreDatabaseBackup(ctx context.Context, artifactPath string, rt restoreRuntime, target runtime.Target) (err error) {
	if err := ctx.Err(); err != nil {
		return ioError("restore interrupted", err)
	}

	file, err := os.Open(artifactPath)
	if err != nil {
		return ioError("failed to open source db backup", err)
	}
	defer closeResource(file, &err)

	reader, err := gzip.NewReader(file)
	if err != nil {
		return archiveError("database restore source is unreadable", err)
	}
	defer closeResource(reader, &err)

	if err := rt.RestoreDatabase(ctx, target, reader); err != nil {
		return runtimeError("database restore failed", err)
	}

	return nil
}

func restoreFilesBackup(ctx context.Context, artifactPath, storageDir string, runtimeUID, runtimeGID int) (err error) {
	if err := ctx.Err(); err != nil {
		return ioError("restore interrupted", err)
	}
	if err := ensureRestoreStorageDir(storageDir); err != nil {
		return ioError("restore storage target is invalid", err)
	}
	if err := validateFilesArchiveForRestore(artifactPath); err != nil {
		return err
	}

	stagingDir, err := createRestoreStagingDir(storageDir)
	if err != nil {
		return ioError("failed to create restore staging directory", err)
	}
	defer func() {
		cleanupErr := restoreRemoveAll(stagingDir)
		if cleanupErr == nil || err != nil {
			return
		}
		err = ioError("failed to clean restore staging directory", cleanupErr)
	}()

	if err := extractFilesArchiveToStaging(ctx, artifactPath, stagingDir); err != nil {
		return err
	}
	if err := restoreValidateStorageTree(stagingDir); err != nil {
		return archiveError("files restore staging is invalid", err)
	}
	if err := restoreApplyOwnership(stagingDir, runtimeUID, runtimeGID); err != nil {
		return ioError("files ownership restore failed", err)
	}
	if err := replaceRestoreStorageFromStaging(ctx, stagingDir, storageDir); err != nil {
		return err
	}
	if err := restoreValidateStorageTree(storageDir); err != nil {
		return ioError("files restore post-check failed", err)
	}

	return nil
}

func validateFilesArchiveForRestore(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return ioError("failed to open source files backup", err)
	}
	defer closeResource(file, &err)

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return archiveError("files restore source is unreadable", err)
	}
	defer closeResource(gzipReader, &err)

	tarReader := tar.NewReader(gzipReader)
	validator := newFilesArchiveValidator()
	var found bool
	for {
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return archiveError("files restore source is unreadable", nextErr)
		}
		if err := validator.validate(header); err != nil {
			return archiveError("files restore source is unsafe", err)
		}
		found = true
		if _, err := io.Copy(io.Discard, tarReader); err != nil {
			return archiveError("files restore source is unreadable", err)
		}
	}
	if !found {
		return archiveError("files restore source is empty", nil)
	}

	return nil
}

func ensureRestoreStorageDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("storage dir root must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("storage dir must be a directory")
	}
	return nil
}

func ensureRestoreStorageClearable(path string) error {
	if err := ensureRestoreStorageDir(path); err != nil {
		return err
	}
	if err := ensureRestoreStorageParentWritable(path); err != nil {
		return err
	}

	return filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}

		probe, err := os.CreateTemp(current, ".espops-clearable-*")
		if err != nil {
			return err
		}
		probePath := probe.Name()
		if err := probe.Close(); err != nil {
			_ = os.Remove(probePath)
			return err
		}
		if err := os.Remove(probePath); err != nil {
			return err
		}
		return nil
	})
}

func ensureRestoreStorageParentWritable(path string) error {
	probe, err := os.MkdirTemp(filepath.Dir(path), "espops-restore-switch-probe-*")
	if err != nil {
		return fmt.Errorf("storage dir parent must allow adjacent restore staging: %w", err)
	}
	if err := os.Remove(probe); err != nil {
		return err
	}
	return nil
}

func defaultCreateRestoreStagingDir(storageDir string) (string, error) {
	return os.MkdirTemp(filepath.Dir(storageDir), restoreStagingDirPattern)
}

func extractFilesArchiveToStaging(ctx context.Context, artifactPath, stagingDir string) (err error) {
	file, err := os.Open(artifactPath)
	if err != nil {
		return ioError("failed to open source files backup", err)
	}
	defer closeResource(file, &err)

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return archiveError("files restore source is unreadable", err)
	}
	defer closeResource(gzipReader, &err)

	tarReader := tar.NewReader(gzipReader)
	validator := newFilesArchiveValidator()
	var found bool
	for {
		if err := ctx.Err(); err != nil {
			return ioError("restore interrupted", err)
		}

		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return archiveError("files restore source is unreadable", nextErr)
		}
		if err := validator.validate(header); err != nil {
			return archiveError("files restore source is unsafe", err)
		}
		found = true
		if err := restoreExtractTarEntry(stagingDir, header, tarReader); err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return archiveError("files restore source is unreadable", err)
			}
			return ioError("files staging extraction failed", err)
		}
	}
	if !found {
		return archiveError("files restore source is empty", nil)
	}

	return nil
}

func validateRestoredStorageTree(path string) error {
	if err := ensureRestoreStorageDir(path); err != nil {
		return err
	}

	var found bool
	walkErr := filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == path {
			return nil
		}

		rel, err := filepath.Rel(path, current)
		if err != nil {
			return err
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("storage entry %s is a symlink", filepath.ToSlash(rel))
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("storage entry %s has unsupported type", filepath.ToSlash(rel))
		}
		found = true
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if !found {
		return fmt.Errorf("storage dir is empty")
	}

	return nil
}

func applyRestoreOwnership(root string, uid, gid int) error {
	return applyRestoreOwnershipWithOps(root, uid, gid, restoreOwnershipFS)
}

func applyRestoreOwnershipWithOps(root string, uid, gid int, fs restoreOwnershipOps) error {
	if err := ensureRestoreStorageDir(root); err != nil {
		return err
	}
	if fs.lstat == nil {
		return fmt.Errorf("restore ownership lstat is required")
	}
	if fs.lchown == nil {
		return fmt.Errorf("restore ownership lchown is required")
	}

	return filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		info, err := fs.lstat(current)
		if err != nil {
			return err
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("restore ownership path %s is a symlink", current)
		}
		if !info.IsDir() && !mode.IsRegular() {
			return fmt.Errorf("restore ownership path %s has unsupported type", current)
		}

		currentUID, currentGID, err := fileOwner(info)
		if err != nil {
			return fmt.Errorf("restore ownership path %s owner metadata is unavailable: %w", current, err)
		}
		if currentUID == uid && currentGID == gid {
			return nil
		}
		if err := fs.lchown(current, uid, gid); err != nil {
			return fmt.Errorf(
				"restore ownership path %s to uid=%d gid=%d failed: %w",
				current,
				uid,
				gid,
				err,
			)
		}
		return nil
	})
}

func fileOwner(info os.FileInfo) (int, int, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, 0, fmt.Errorf("unsupported stat payload")
	}
	return int(stat.Uid), int(stat.Gid), nil
}

func replaceRestoreStorageFromStaging(ctx context.Context, stagingDir, storageDir string) error {
	if err := ctx.Err(); err != nil {
		return ioError("restore interrupted", err)
	}
	if err := ensureRestoreStorageDir(stagingDir); err != nil {
		return ioError("files switch staging is invalid", err)
	}
	if err := ensureRestoreStorageDir(storageDir); err != nil {
		return ioError("files switch target is invalid", err)
	}
	if err := ensureStagingNextToStorage(stagingDir, storageDir); err != nil {
		return ioError("files switch staging is invalid", err)
	}

	rollbackDir, err := newRestoreRollbackDir(storageDir)
	if err != nil {
		return ioError("files switch rollback allocation failed", err)
	}

	if err := restoreRenamePath(storageDir, rollbackDir); err != nil {
		return ioError("files switch failed before target switch", fmt.Errorf(
			"move current storage %s to rollback %s in the same parent directory: %w",
			storageDir,
			rollbackDir,
			err,
		))
	}

	if err := ctx.Err(); err != nil {
		if rollbackErr := rollbackRestoreStorage(storageDir, rollbackDir); rollbackErr != nil {
			return ioError("files switch interrupted after target move; rollback failed", errors.Join(err, rollbackErr))
		}
		return ioError("files switch interrupted after target move; rolled back current storage", err)
	}

	if err := restoreRenamePath(stagingDir, storageDir); err != nil {
		switchErr := fmt.Errorf(
			"move staged storage %s to target %s in the same parent directory: %w",
			stagingDir,
			storageDir,
			err,
		)
		if rollbackErr := rollbackRestoreStorage(storageDir, rollbackDir); rollbackErr != nil {
			return ioError("files switch failed during target switch; rollback failed", errors.Join(switchErr, rollbackErr))
		}
		return ioError("files switch failed during target switch; rolled back current storage", switchErr)
	}

	if err := restoreRemoveAll(rollbackDir); err != nil {
		return ioError("files switch cleanup failed; restored storage is active and old storage rollback remains", err)
	}
	return nil
}

func ensureStagingNextToStorage(stagingDir, storageDir string) error {
	stagingClean := filepath.Clean(stagingDir)
	storageClean := filepath.Clean(storageDir)
	if stagingClean == storageClean {
		return fmt.Errorf("restore staging dir must differ from storage dir")
	}
	if filepath.Clean(filepath.Dir(stagingClean)) != filepath.Clean(filepath.Dir(storageClean)) {
		return fmt.Errorf(
			"restore staging dir must be next to storage dir for same-filesystem rename: staging=%s storage=%s",
			stagingDir,
			storageDir,
		)
	}
	return nil
}

func newRestoreRollbackDir(storageDir string) (string, error) {
	rollbackDir, err := os.MkdirTemp(filepath.Dir(storageDir), restoreRollbackDirPattern)
	if err != nil {
		return "", err
	}
	if err := os.Remove(rollbackDir); err != nil {
		return "", err
	}
	return rollbackDir, nil
}

func rollbackRestoreStorage(storageDir, rollbackDir string) error {
	if _, err := os.Lstat(storageDir); err == nil {
		return fmt.Errorf("target path exists; old storage remains at rollback path %s", rollbackDir)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := restoreRenamePath(rollbackDir, storageDir); err != nil {
		return fmt.Errorf("move rollback %s back to storage %s: %w", rollbackDir, storageDir, err)
	}
	return nil
}

func extractTarEntry(root string, header *tar.Header, reader io.Reader) error {
	relativeSlash, _, err := validateTarHeaderEntry(header)
	if err != nil {
		return err
	}
	relative := filepath.FromSlash(relativeSlash)
	targetPath := filepath.Join(root, relative)
	if !pathWithinRoot(root, targetPath) {
		return fmt.Errorf("tar entry escapes restore root: %s", header.Name)
	}

	mode := os.FileMode(header.Mode) & os.ModePerm
	switch header.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(targetPath, mode); err != nil {
			return err
		}
		return os.Chmod(targetPath, mode)
	case tar.TypeReg, tarRegularTypeflagZero:
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}

		file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		if _, err := io.Copy(file, reader); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		return os.Chmod(targetPath, mode)
	default:
		return fmt.Errorf("tar entry type is not allowed: %d", header.Typeflag)
	}
}

func pathWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
