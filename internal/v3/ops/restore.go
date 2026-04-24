package ops

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	v3config "github.com/lazuale/espocrm-ops/internal/v3/config"
	v3runtime "github.com/lazuale/espocrm-ops/internal/v3/runtime"
)

type RestoreResult struct {
	Manifest         string
	SnapshotManifest string
}

type restoreRuntime interface {
	backupRuntime
	UpService(ctx context.Context, target v3runtime.Target, service string) error
	RestoreDatabase(ctx context.Context, target v3runtime.Target, reader io.Reader) error
	DBPing(ctx context.Context, target v3runtime.Target) error
}

func Restore(ctx context.Context, cfg v3config.BackupConfig, manifestPath string, rt restoreRuntime, now time.Time) (result RestoreResult, err error) {
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

	result.Manifest = manifestPath

	verifyResult, verifyErr := VerifyBackup(ctx, manifestPath)
	if verifyErr != nil {
		return result, verifyErr
	}
	result.Manifest = verifyResult.Manifest

	snapshotResult, snapshotErr := Backup(ctx, cfg, rt, now)
	if snapshotErr != nil {
		return result, snapshotErr
	}
	result.SnapshotManifest = snapshotResult.Manifest

	target := v3runtime.Target{
		ProjectDir:  cfg.ProjectDir,
		ComposeFile: cfg.ComposeFile,
		EnvFile:     cfg.EnvFile,
		DBService:   cfg.DBService,
		DBUser:      cfg.DBUser,
		DBPassword:  cfg.DBPassword,
		DBName:      cfg.DBName,
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
	if err := restoreDatabaseBackup(ctx, verifyResult.DBBackup, rt, target); err != nil {
		return result, err
	}
	if err := restoreFilesBackup(ctx, verifyResult.FilesBackup, cfg.StorageDir); err != nil {
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

	if err := rt.DBPing(ctx, target); err != nil {
		return result, runtimeError("restore post-check failed", err)
	}

	return result, nil
}

func validateRestoreInputs(cfg v3config.BackupConfig, manifestPath string) error {
	if err := validateBackupConfig(cfg); err != nil {
		return err
	}
	if strings.TrimSpace(manifestPath) == "" {
		return fmt.Errorf("manifest path is required")
	}
	return nil
}

func restoreDatabaseBackup(ctx context.Context, artifactPath string, rt restoreRuntime, target v3runtime.Target) (err error) {
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

func restoreFilesBackup(ctx context.Context, artifactPath, storageDir string) (err error) {
	if err := ctx.Err(); err != nil {
		return ioError("restore interrupted", err)
	}
	if err := ensureRestoreStorageDir(storageDir); err != nil {
		return ioError("restore storage target is invalid", err)
	}
	if err := validateFilesArchiveForRestore(artifactPath); err != nil {
		return err
	}

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
	if err := clearDirectoryContents(storageDir); err != nil {
		return ioError("files restore failed", err)
	}

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
		if err := validateTarHeader(header); err != nil {
			return archiveError("files restore source is unsafe", err)
		}
		found = true
		if err := extractTarEntry(storageDir, header, tarReader); err != nil {
			return ioError("files restore failed", err)
		}
	}
	if !found {
		return archiveError("files restore source is empty", nil)
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
	var found bool
	for {
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return archiveError("files restore source is unreadable", nextErr)
		}
		if err := validateTarHeader(header); err != nil {
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

func clearDirectoryContents(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		info, err := os.Lstat(entryPath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("storage dir entry must not be a symlink: %s", entry.Name())
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("storage dir entry type is not supported: %s", entry.Name())
		}
		paths = append(paths, entryPath)
	}
	for _, entryPath := range paths {
		if err := os.RemoveAll(entryPath); err != nil {
			return err
		}
	}
	return nil
}

func extractTarEntry(root string, header *tar.Header, reader io.Reader) error {
	relative := filepath.Clean(filepath.FromSlash(header.Name))
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
	case tar.TypeReg, legacyTarRegularTypeflag:
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
