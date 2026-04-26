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
	Warnings         []string
}

type restoreRuntime interface {
	backupRuntime
	UpService(ctx context.Context, target runtime.Target, service string) error
	ResetDatabase(ctx context.Context, target runtime.Target) error
	RestoreDatabase(ctx context.Context, target runtime.Target, reader io.Reader) error
	DBPing(ctx context.Context, target runtime.Target) error
}

func Restore(ctx context.Context, cfg config.BackupConfig, manifestPath string, rt restoreRuntime, now time.Time) (result RestoreResult, err error) {
	return restoreWithAllowedSourceScope(ctx, cfg, manifestPath, rt, now, "")
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

func restoreWithAllowedSourceScope(ctx context.Context, cfg config.BackupConfig, manifestPath string, rt restoreRuntime, now time.Time, allowedSourceScope string) (result RestoreResult, err error) {
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
		return restoreWithAllowedSourceScopeLocked(lockedCtx, cfg, manifestPath, rt, now, allowedSourceScope)
	})
}

func restoreWithAllowedSourceScopeLocked(ctx context.Context, cfg config.BackupConfig, manifestPath string, rt restoreRuntime, now time.Time, allowedSourceScope string) (result RestoreResult, err error) {
	result.Manifest = manifestPath

	verifyResult, verifyErr := VerifyBackup(ctx, manifestPath)
	if verifyErr != nil {
		return result, verifyErr
	}
	result.Manifest = verifyResult.Manifest
	result.Warnings = append(result.Warnings, verifyResult.Warnings...)
	if err := validateRestoreSourceScope(cfg.Scope, verifyResult.Scope, allowedSourceScope); err != nil {
		return result, err
	}
	if err := requireRestoreRuntimeContract(cfg, verifyResult); err != nil {
		return result, err
	}
	if err := ensureRestoreStorageClearable(cfg.StorageDir); err != nil {
		return result, ioError("restore storage target is not clearable", err)
	}

	snapshotResult, snapshotErr := snapshotBackup(ctx, cfg, rt, now)
	if snapshotErr != nil {
		return result, snapshotErr
	}
	result.SnapshotManifest = snapshotResult.Manifest
	if err := ensureRestoreStagingFreeDisk(cfg.StorageDir, verifyResult.FilesExpandedBytes, cfg.MinFreeDiskMB); err != nil {
		return result, ioError("restore free disk preflight failed", err)
	}
	preparedFiles, err := prepareRestoreFilesBackup(ctx, verifyResult.FilesBackup, cfg.StorageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	if err != nil {
		return result, err
	}
	defer func() {
		cleanupErr := preparedFiles.Cleanup()
		if cleanupErr == nil || err != nil {
			return
		}
		err = ioError("failed to clean restore staging directory", cleanupErr)
	}()

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
	if err := commitRestoreFilesBackup(ctx, preparedFiles, cfg.StorageDir); err != nil {
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

func validateRestoreSourceScope(targetScope, sourceScope, allowedSourceScope string) error {
	wantScope := strings.TrimSpace(targetScope)
	if allowed := strings.TrimSpace(allowedSourceScope); allowed != "" {
		wantScope = allowed
	}
	if sourceScope == wantScope {
		return nil
	}

	return &VerifyError{
		Kind:    ErrorKindUsage,
		Message: "manifest scope is invalid for requested operation",
		Err: fmt.Errorf(
			"manifest scope %q does not match required scope %q",
			sourceScope,
			wantScope,
		),
	}
}

func ensureRestoreStagingFreeDisk(storageDir string, filesExpandedBytes int64, minFreeDiskMB int) error {
	storageParent := filepath.Dir(storageDir)
	freeBytes, err := backupDiskFreeBytes(storageParent)
	if err != nil {
		return fmt.Errorf("check storage parent free space %s: %w", storageParent, err)
	}

	requiredBytes, err := restoreStagingRequiredBytes(filesExpandedBytes, minFreeDiskMB)
	if err != nil {
		return err
	}
	if freeBytes >= requiredBytes {
		return nil
	}

	return fmt.Errorf(
		"storage parent %s has %d MiB available; restore staging requires at least %d MiB (files backup expands to %d bytes plus MIN_FREE_DISK_MB=%d)",
		storageParent,
		freeBytes/bytesPerMiB,
		bytesToMiBCeil(requiredBytes),
		filesExpandedBytes,
		minFreeDiskMB,
	)
}

func restoreStagingRequiredBytes(filesExpandedBytes int64, minFreeDiskMB int) (uint64, error) {
	if filesExpandedBytes < 0 {
		return 0, fmt.Errorf("files backup expanded size is invalid: %d", filesExpandedBytes)
	}
	if minFreeDiskMB <= 0 {
		return 0, fmt.Errorf("MIN_FREE_DISK_MB must be > 0")
	}

	expandedBytes := uint64(filesExpandedBytes)
	reserveBytes := uint64(minFreeDiskMB) * bytesPerMiB
	if reserveBytes/bytesPerMiB != uint64(minFreeDiskMB) {
		return 0, fmt.Errorf("MIN_FREE_DISK_MB is too large")
	}
	if expandedBytes > ^uint64(0)-reserveBytes {
		return 0, fmt.Errorf("restore staging required space is too large")
	}
	return expandedBytes + reserveBytes, nil
}

func bytesToMiBCeil(value uint64) uint64 {
	if value == 0 {
		return 0
	}
	return ((value - 1) / bytesPerMiB) + 1
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

type preparedRestoreFilesBackup struct {
	stagingDir string
	committed  bool
}

func (p *preparedRestoreFilesBackup) Cleanup() error {
	if p == nil || p.committed || strings.TrimSpace(p.stagingDir) == "" {
		return nil
	}
	return restoreRemoveAll(p.stagingDir)
}

func (p *preparedRestoreFilesBackup) markCommitted() {
	if p == nil {
		return
	}
	p.committed = true
}

func prepareRestoreFilesBackup(ctx context.Context, artifactPath, storageDir string, runtimeUID, runtimeGID int) (result *preparedRestoreFilesBackup, err error) {
	if err := ctx.Err(); err != nil {
		return nil, ioError("restore interrupted", err)
	}
	if err := ensureRestoreStorageDir(storageDir); err != nil {
		return nil, ioError("restore storage target is invalid", err)
	}
	if err := validateFilesArchiveForRestore(artifactPath); err != nil {
		return nil, err
	}

	stagingDir, err := createRestoreStagingDir(storageDir)
	if err != nil {
		return nil, ioError("failed to create restore staging directory", err)
	}
	prepared := &preparedRestoreFilesBackup{stagingDir: stagingDir}
	defer func() {
		if err == nil {
			return
		}
		_ = prepared.Cleanup()
	}()

	if err := extractFilesArchiveToStaging(ctx, artifactPath, stagingDir); err != nil {
		return nil, err
	}
	if err := restoreValidateStorageTree(stagingDir); err != nil {
		return nil, archiveError("files restore staging is invalid", err)
	}
	if err := restoreApplyOwnership(stagingDir, runtimeUID, runtimeGID); err != nil {
		return nil, ioError("files ownership restore failed", err)
	}

	return prepared, nil
}

func commitRestoreFilesBackup(ctx context.Context, prepared *preparedRestoreFilesBackup, storageDir string) error {
	if prepared == nil {
		return ioError("files restore staging is invalid", fmt.Errorf("prepared files staging is required"))
	}
	if err := replaceRestoreStorageFromStagingWithCommit(ctx, prepared.stagingDir, storageDir, prepared.markCommitted); err != nil {
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

	return nil
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

func replaceRestoreStorageFromStagingWithCommit(ctx context.Context, stagingDir, storageDir string, markCommitted func()) error {
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
	if markCommitted != nil {
		markCommitted()
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
