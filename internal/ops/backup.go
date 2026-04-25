package ops

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	manifest "github.com/lazuale/espocrm-ops/internal/manifest"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
)

type BackupResult struct {
	Manifest    string
	DBBackup    string
	FilesBackup string
	Warnings    []string
}

type backupRuntime interface {
	Validate(ctx context.Context, target runtime.Target) error
	StopServices(ctx context.Context, target runtime.Target, services []string) error
	RequireStoppedServices(ctx context.Context, target runtime.Target, services []string) error
	StartServices(ctx context.Context, target runtime.Target, services []string) error
	DumpDatabase(ctx context.Context, target runtime.Target, destPath string) error
	RequireHealthyServices(ctx context.Context, target runtime.Target, services []string) error
}

type backupLayout struct {
	DBArtifact    string
	DBChecksum    string
	FilesArtifact string
	FilesChecksum string
	ManifestJSON  string
}

const serviceReturnTimeout = 30 * time.Second

const backupSetTimestampFormat = "2006-01-02_15-04-05"

var (
	backupDiskFreeBytes = defaultBackupDiskFreeBytes
	backupRemovePath    = os.Remove
)

type retentionTarget struct {
	base          string
	createdAt     time.Time
	manifestPath  string
	dbPath        string
	dbSidecarPath string
	filesPath     string
	filesSidecar  string
}

func Backup(ctx context.Context, cfg config.BackupConfig, rt backupRuntime, now time.Time) (result BackupResult, err error) {
	if rt == nil {
		return BackupResult{}, runtimeError("backup runtime is required", nil)
	}
	if err := validateBackupConfig(cfg); err != nil {
		return BackupResult{}, &VerifyError{Kind: ErrorKindUsage, Message: err.Error()}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	return withOperationLocks(ctx, []operationLockSpec{{
		ProjectDir: cfg.ProjectDir,
		Scope:      cfg.Scope,
	}}, "backup lock failed", func(lockedCtx context.Context) (BackupResult, error) {
		return backupLocked(lockedCtx, cfg, rt, now)
	})
}

func backupLocked(ctx context.Context, cfg config.BackupConfig, rt backupRuntime, now time.Time) (result BackupResult, err error) {
	layout := newBackupLayout(cfg.BackupRoot, cfg.BackupNamePrefix, now)
	result = BackupResult{
		Manifest:    layout.ManifestJSON,
		DBBackup:    layout.DBArtifact,
		FilesBackup: layout.FilesArtifact,
	}

	cleanupPaths := []string{}
	verified := false
	defer func() {
		if verified {
			return
		}
		if cleanupErr := removePaths(cleanupPaths); cleanupErr != nil {
			err = ioError("failed to remove incomplete backup set", errors.Join(err, cleanupErr))
		}
	}()

	target := runtime.Target{
		ProjectDir:  cfg.ProjectDir,
		ComposeFile: cfg.ComposeFile,
		EnvFile:     cfg.EnvFile,
		DBService:   cfg.DBService,
		DBUser:      cfg.DBUser,
		DBPassword:  cfg.DBPassword,
		DBName:      cfg.DBName,
	}
	serviceReturnNeeded := false
	servicesReturned := false
	serviceReturnAttempted := false
	defer func() {
		if !serviceReturnNeeded || servicesReturned || serviceReturnAttempted {
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

	if err := rt.Validate(ctx, target); err != nil {
		return result, runtimeError("docker compose config failed", err)
	}
	if err := ensureBackupLayout(layout); err != nil {
		return result, ioError("failed to prepare backup directories", err)
	}
	if err := ensureTargetsAbsent(layout); err != nil {
		return result, artifactError("backup target already exists", err)
	}
	if err := ensureBackupFreeDisk(cfg.BackupRoot, cfg.MinFreeDiskMB); err != nil {
		return result, ioError("backup free disk preflight failed", err)
	}
	serviceReturnNeeded = true
	if err := rt.StopServices(ctx, target, cfg.AppServices); err != nil {
		return result, runtimeError("failed to stop app services", err)
	}
	if err := rt.RequireStoppedServices(ctx, target, cfg.AppServices); err != nil {
		return result, runtimeError("app service stop check failed", err)
	}

	dbTmp, err := newTempTarget(layout.DBArtifact)
	if err != nil {
		return result, ioError("failed to allocate db backup temp file", err)
	}
	cleanupPaths = append(cleanupPaths, dbTmp)
	if err := rt.DumpDatabase(ctx, target, dbTmp); err != nil {
		return result, runtimeError("database backup failed", err)
	}
	dbChecksum, err := sha256File(dbTmp)
	if err != nil {
		return result, ioError("failed to checksum db backup", err)
	}
	if err := promoteTempFile(dbTmp, layout.DBArtifact); err != nil {
		return result, ioError("failed to finalize db backup", err)
	}
	cleanupPaths = append(cleanupPaths, layout.DBArtifact)

	dbSidecarTmp, err := newTempTarget(layout.DBChecksum)
	if err != nil {
		return result, ioError("failed to allocate db checksum temp file", err)
	}
	cleanupPaths = append(cleanupPaths, dbSidecarTmp)
	if err := writeSHA256Sidecar(dbSidecarTmp, filepath.Base(layout.DBArtifact), dbChecksum); err != nil {
		return result, ioError("failed to write db checksum sidecar", err)
	}
	if err := promoteTempFile(dbSidecarTmp, layout.DBChecksum); err != nil {
		return result, ioError("failed to finalize db checksum sidecar", err)
	}
	cleanupPaths = append(cleanupPaths, layout.DBChecksum)

	filesTmp, err := newTempTarget(layout.FilesArtifact)
	if err != nil {
		return result, ioError("failed to allocate files backup temp file", err)
	}
	cleanupPaths = append(cleanupPaths, filesTmp)
	if err := archiveStorageDir(cfg.StorageDir, filesTmp); err != nil {
		return result, archiveError("files backup failed", err)
	}
	filesChecksum, err := sha256File(filesTmp)
	if err != nil {
		return result, ioError("failed to checksum files backup", err)
	}
	if err := promoteTempFile(filesTmp, layout.FilesArtifact); err != nil {
		return result, ioError("failed to finalize files backup", err)
	}
	cleanupPaths = append(cleanupPaths, layout.FilesArtifact)

	filesSidecarTmp, err := newTempTarget(layout.FilesChecksum)
	if err != nil {
		return result, ioError("failed to allocate files checksum temp file", err)
	}
	cleanupPaths = append(cleanupPaths, filesSidecarTmp)
	if err := writeSHA256Sidecar(filesSidecarTmp, filepath.Base(layout.FilesArtifact), filesChecksum); err != nil {
		return result, ioError("failed to write files checksum sidecar", err)
	}
	if err := promoteTempFile(filesSidecarTmp, layout.FilesChecksum); err != nil {
		return result, ioError("failed to finalize files checksum sidecar", err)
	}
	cleanupPaths = append(cleanupPaths, layout.FilesChecksum)
	serviceReturnAttempted = true
	startCtx, cancel := serviceReturnContext()
	if err := rt.StartServices(startCtx, target, cfg.AppServices); err != nil {
		cancel()
		return result, runtimeError("failed to return app services", err)
	}
	cancel()
	servicesReturned = true
	if err := waitForRuntimeServiceHealth(ctx, target, cfg.DBService, cfg.AppServices, rt); err != nil {
		return result, runtimeError("backup post-check failed", err)
	}

	manifestTmp, err := newTempTarget(layout.ManifestJSON)
	if err != nil {
		return result, ioError("failed to allocate manifest temp file", err)
	}
	cleanupPaths = append(cleanupPaths, manifestTmp)
	if err := writeManifestJSON(manifestTmp, manifest.Manifest{
		Version:   manifest.VersionCurrent,
		Scope:     cfg.Scope,
		CreatedAt: now.Format(time.RFC3339),
		Artifacts: manifest.Artifacts{
			DBBackup:    filepath.Base(layout.DBArtifact),
			FilesBackup: filepath.Base(layout.FilesArtifact),
		},
		Checksums: manifest.Checksums{
			DBBackup:    dbChecksum,
			FilesBackup: filesChecksum,
		},
		Runtime: backupManifestRuntime(cfg),
	}); err != nil {
		return result, ioError("failed to write backup manifest", err)
	}
	if err := promoteTempFile(manifestTmp, layout.ManifestJSON); err != nil {
		return result, ioError("failed to finalize backup manifest", err)
	}
	cleanupPaths = append(cleanupPaths, layout.ManifestJSON)

	verifyResult, verifyErr := VerifyBackup(ctx, layout.ManifestJSON)
	if verifyErr != nil {
		return result, verifyErr
	}

	verified = true
	result = BackupResult{
		Manifest:    verifyResult.Manifest,
		DBBackup:    verifyResult.DBBackup,
		FilesBackup: verifyResult.FilesBackup,
	}
	if err := runBackupRetention(ctx, cfg, layout, now); err != nil {
		result.Warnings = append(result.Warnings, backupRetentionSkippedWarning(err))
	}
	return result, nil
}

func validateBackupConfig(cfg config.BackupConfig) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "--scope", value: cfg.Scope},
		{name: "project dir", value: cfg.ProjectDir},
		{name: "compose file", value: cfg.ComposeFile},
		{name: "env file", value: cfg.EnvFile},
		{name: "ESPOCRM_IMAGE", value: cfg.EspoCRMImage},
		{name: "MARIADB_IMAGE", value: cfg.MariaDBImage},
		{name: "BACKUP_ROOT", value: cfg.BackupRoot},
		{name: "BACKUP_NAME_PREFIX", value: cfg.BackupNamePrefix},
		{name: "ESPO_STORAGE_DIR", value: cfg.StorageDir},
		{name: "DB_SERVICE", value: cfg.DBService},
		{name: "DB_USER", value: cfg.DBUser},
		{name: "DB_PASSWORD", value: cfg.DBPassword},
		{name: "DB_NAME", value: cfg.DBName},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	if len(cfg.AppServices) == 0 {
		return fmt.Errorf("APP_SERVICES is required")
	}
	for _, service := range cfg.AppServices {
		if strings.TrimSpace(service) == "" {
			return fmt.Errorf("APP_SERVICES contains empty service name")
		}
	}
	if cfg.MinFreeDiskMB <= 0 {
		return fmt.Errorf("MIN_FREE_DISK_MB must be > 0")
	}
	if cfg.BackupRetentionDays < 0 {
		return fmt.Errorf("BACKUP_RETENTION_DAYS must be >= 0")
	}
	return nil
}

func newBackupLayout(root, prefix string, createdAt time.Time) backupLayout {
	base := fmt.Sprintf("%s_%s", prefix, createdAt.UTC().Format(backupSetTimestampFormat))
	dbArtifact := filepath.Join(root, "db", base+".sql.gz")
	filesArtifact := filepath.Join(root, "files", base+".tar.gz")
	return backupLayout{
		DBArtifact:    dbArtifact,
		DBChecksum:    dbArtifact + ".sha256",
		FilesArtifact: filesArtifact,
		FilesChecksum: filesArtifact + ".sha256",
		ManifestJSON:  filepath.Join(root, "manifests", base+".manifest.json"),
	}
}

func ensureBackupLayout(layout backupLayout) error {
	for _, dir := range []string{
		filepath.Dir(layout.DBArtifact),
		filepath.Dir(layout.FilesArtifact),
		filepath.Dir(layout.ManifestJSON),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func ensureTargetsAbsent(layout backupLayout) error {
	for _, path := range []string{
		layout.DBArtifact,
		layout.DBChecksum,
		layout.FilesArtifact,
		layout.FilesChecksum,
		layout.ManifestJSON,
	} {
		_, err := os.Lstat(path)
		if err == nil {
			return fmt.Errorf("target already exists: %s", path)
		}
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func ensureBackupFreeDisk(path string, minFreeDiskMB int) error {
	freeBytes, err := backupDiskFreeBytes(path)
	if err != nil {
		return err
	}

	requiredBytes := uint64(minFreeDiskMB) * 1024 * 1024
	if freeBytes < requiredBytes {
		return fmt.Errorf(
			"backup root free space %d MiB is below MIN_FREE_DISK_MB=%d",
			freeBytes/(1024*1024),
			minFreeDiskMB,
		)
	}

	return nil
}

func defaultBackupDiskFreeBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func removePaths(paths []string) error {
	var cleanupErr error
	for _, path := range paths {
		if err := backupRemovePath(path); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func newTempTarget(finalPath string) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(finalPath), "."+filepath.Base(finalPath)+".tmp-*")
	if err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func promoteTempFile(tempPath, finalPath string) error {
	if err := os.Link(tempPath, finalPath); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("target already exists: %s", finalPath)
		}
		return err
	}
	return os.Remove(tempPath)
}

func writeSHA256Sidecar(path, artifactName, checksum string) error {
	body := checksum + "  " + artifactName + "\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func writeManifestJSON(path string, manifestData manifest.Manifest) error {
	raw, err := json.MarshalIndent(manifestData, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func archiveStorageDir(sourceDir, destPath string) (err error) {
	info, err := os.Lstat(sourceDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("storage dir root is a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("storage dir must be a directory")
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer closeResource(out, &err)

	gz := gzip.NewWriter(out)
	defer closeResource(gz, &err)

	tw := tar.NewWriter(gz)
	defer closeResource(tw, &err)

	var found bool
	walkErr := filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("storage entry %s is a symlink", rel)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("storage entry %s has unsupported type", rel)
		}
		if info.Mode().IsRegular() {
			if err := ensureArchiveSourceNotHardlinked(rel, info); err != nil {
				return err
			}
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			header.Name += "/"
		}
		if err := validateTarHeader(header); err != nil {
			return err
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		found = true
		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if walkErr != nil {
		return walkErr
	}
	if !found {
		return fmt.Errorf("storage dir is empty")
	}

	return nil
}

func ensureArchiveSourceNotHardlinked(rel string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return fmt.Errorf("storage entry %s link metadata is unavailable", rel)
	}
	if stat.Nlink > 1 {
		return fmt.Errorf("storage entry %s has multiple hardlinks", rel)
	}
	return nil
}

func runtimeError(message string, err error) error {
	return &VerifyError{Kind: ErrorKindRuntime, Message: message, Err: err}
}

func backupRetentionSkippedWarning(err error) string {
	return "retention_skipped: " + err.Error()
}

func runBackupRetention(ctx context.Context, cfg config.BackupConfig, current backupLayout, now time.Time) error {
	if cfg.BackupRetentionDays == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return ioError("backup retention interrupted", err)
	}

	cutoff := now.UTC().AddDate(0, 0, -cfg.BackupRetentionDays)
	targets, err := planBackupRetention(cfg.BackupRoot, cfg.BackupNamePrefix, current, cutoff)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return ioError("backup retention interrupted", err)
		}
		if err := removeRetentionTarget(target); err != nil {
			return ioError("backup retention cleanup failed", err)
		}
	}
	return nil
}

func planBackupRetention(root, prefix string, current backupLayout, cutoff time.Time) ([]retentionTarget, error) {
	entries, err := os.ReadDir(filepath.Join(root, "manifests"))
	if err != nil {
		return nil, ioError("backup retention scan failed", err)
	}

	currentBase := strings.TrimSuffix(filepath.Base(current.ManifestJSON), ".manifest.json")
	targets := make([]retentionTarget, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".manifest.json") {
			continue
		}

		base := strings.TrimSuffix(name, ".manifest.json")
		createdAt, matchedPrefix, err := parseBackupSetTimestamp(prefix, base)
		if err != nil {
			return nil, artifactError("backup retention cleanup blocked", fmt.Errorf("manifest %q is suspicious: %w", name, err))
		}
		if !matchedPrefix || base == currentBase || !createdAt.Before(cutoff) {
			continue
		}

		target, err := validateRetentionTarget(root, filepath.Join(root, "manifests", name), base, createdAt)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}

	sort.Slice(targets, func(i, j int) bool {
		if targets[i].createdAt.Equal(targets[j].createdAt) {
			return targets[i].base < targets[j].base
		}
		return targets[i].createdAt.Before(targets[j].createdAt)
	})
	return targets, nil
}

func parseBackupSetTimestamp(prefix, base string) (time.Time, bool, error) {
	prefix = strings.TrimSpace(prefix)
	switch {
	case !strings.HasPrefix(base, prefix+"_"):
		return time.Time{}, false, nil
	case base == prefix+"_":
		return time.Time{}, true, fmt.Errorf("missing timestamp")
	}

	createdAt, err := time.Parse(backupSetTimestampFormat, strings.TrimPrefix(base, prefix+"_"))
	if err != nil {
		return time.Time{}, true, fmt.Errorf("invalid timestamp")
	}
	return createdAt.UTC(), true, nil
}

func validateRetentionTarget(root, manifestPath, base string, createdAt time.Time) (retentionTarget, error) {
	target := retentionTarget{
		base:          base,
		createdAt:     createdAt,
		manifestPath:  manifestPath,
		dbPath:        filepath.Join(root, "db", base+".sql.gz"),
		dbSidecarPath: filepath.Join(root, "db", base+".sql.gz.sha256"),
		filesPath:     filepath.Join(root, "files", base+".tar.gz"),
		filesSidecar:  filepath.Join(root, "files", base+".tar.gz.sha256"),
	}

	for _, path := range []string{
		target.manifestPath,
		target.dbPath,
		target.dbSidecarPath,
		target.filesPath,
		target.filesSidecar,
	} {
		if err := ensureNonEmptyFile(path); err != nil {
			return retentionTarget{}, artifactError("backup retention cleanup blocked", fmt.Errorf("backup set %q is incomplete: %w", base, err))
		}
	}

	loadedManifest, err := manifest.Load(target.manifestPath)
	if err != nil {
		return retentionTarget{}, manifestError("backup retention cleanup blocked", err)
	}
	if err := manifest.Validate(target.manifestPath, loadedManifest); err != nil {
		return retentionTarget{}, manifestError("backup retention cleanup blocked", err)
	}
	paths, err := manifest.ResolveArtifacts(target.manifestPath, loadedManifest)
	if err != nil {
		return retentionTarget{}, manifestError("backup retention cleanup blocked", err)
	}
	if paths.DBPath != target.dbPath || paths.DBSidecarPath != target.dbSidecarPath || paths.FilesPath != target.filesPath || paths.FilesSidecarPath != target.filesSidecar {
		return retentionTarget{}, artifactError("backup retention cleanup blocked", fmt.Errorf("backup set %q manifest does not match current layout", base))
	}

	return target, nil
}

func removeRetentionTarget(target retentionTarget) error {
	for _, path := range []string{
		target.dbSidecarPath,
		target.dbPath,
		target.filesSidecar,
		target.filesPath,
		target.manifestPath,
	} {
		if err := backupRemovePath(path); err != nil {
			return err
		}
	}
	return nil
}

func combineServiceReturnError(primary error, startErr error) error {
	if startErr == nil {
		return primary
	}

	serviceErr := fmt.Errorf("return app services failed: %w", startErr)
	var verifyErr *VerifyError
	if errors.As(primary, &verifyErr) {
		if verifyErr.Err == nil {
			return &VerifyError{
				Kind:    verifyErr.Kind,
				Message: verifyErr.Message,
				Err:     serviceErr,
			}
		}
		return &VerifyError{
			Kind:    verifyErr.Kind,
			Message: verifyErr.Message,
			Err:     errors.Join(verifyErr.Err, serviceErr),
		}
	}

	return runtimeError("backup failed and app services were not returned", errors.Join(primary, serviceErr))
}

func serviceReturnContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), serviceReturnTimeout)
}
