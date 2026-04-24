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
	"strings"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	manifest "github.com/lazuale/espocrm-ops/internal/manifest"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
)

type BackupResult struct {
	Manifest    string
	DBBackup    string
	FilesBackup string
}

type backupRuntime interface {
	Validate(ctx context.Context, target runtime.Target) error
	StopServices(ctx context.Context, target runtime.Target, services []string) error
	StartServices(ctx context.Context, target runtime.Target, services []string) error
	DumpDatabase(ctx context.Context, target runtime.Target, destPath string) error
}

type backupLayout struct {
	DBArtifact    string
	DBChecksum    string
	FilesArtifact string
	FilesChecksum string
	ManifestJSON  string
}

const serviceReturnTimeout = 30 * time.Second

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

	layout := newBackupLayout(cfg.BackupRoot, cfg.Scope, now)
	result = BackupResult{
		Manifest:    layout.ManifestJSON,
		DBBackup:    layout.DBArtifact,
		FilesBackup: layout.FilesArtifact,
	}

	cleanupPaths := []string{
		layout.ManifestJSON,
		layout.FilesChecksum,
		layout.FilesArtifact,
		layout.DBChecksum,
		layout.DBArtifact,
	}
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

	if err := rt.Validate(ctx, target); err != nil {
		return result, runtimeError("docker compose config failed", err)
	}
	if err := ensureBackupLayout(layout); err != nil {
		return result, ioError("failed to prepare backup directories", err)
	}
	if err := ensureTargetsAbsent(layout); err != nil {
		return result, artifactError("backup target already exists", err)
	}
	if err := rt.StopServices(ctx, target, cfg.AppServices); err != nil {
		return result, runtimeError("failed to stop app services", err)
	}
	servicesStopped = true

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
	serviceReturnAttempted = true
	startCtx, cancel := serviceReturnContext()
	if err := rt.StartServices(startCtx, target, cfg.AppServices); err != nil {
		cancel()
		return result, runtimeError("failed to return app services", err)
	}
	cancel()
	servicesReturned = true

	manifestTmp, err := newTempTarget(layout.ManifestJSON)
	if err != nil {
		return result, ioError("failed to allocate manifest temp file", err)
	}
	cleanupPaths = append(cleanupPaths, manifestTmp)
	if err := writeManifestJSON(manifestTmp, manifest.Manifest{
		Version:   1,
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
	}); err != nil {
		return result, ioError("failed to write backup manifest", err)
	}
	if err := promoteTempFile(manifestTmp, layout.ManifestJSON); err != nil {
		return result, ioError("failed to finalize backup manifest", err)
	}

	verifyResult, verifyErr := VerifyBackup(ctx, layout.ManifestJSON)
	if verifyErr != nil {
		return result, verifyErr
	}

	verified = true
	return BackupResult{
		Manifest:    verifyResult.Manifest,
		DBBackup:    verifyResult.DBBackup,
		FilesBackup: verifyResult.FilesBackup,
	}, nil
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
		{name: "BACKUP_ROOT", value: cfg.BackupRoot},
		{name: "ESPO_STORAGE_DIR", value: cfg.StorageDir},
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
	return nil
}

func newBackupLayout(root, scope string, createdAt time.Time) backupLayout {
	base := fmt.Sprintf("%s_%s", strings.TrimSpace(scope), createdAt.UTC().Format("2006-01-02_15-04-05"))
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

func removePaths(paths []string) error {
	var cleanupErr error
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
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
	return os.Rename(tempPath, finalPath)
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
	info, err := os.Stat(sourceDir)
	if err != nil {
		return err
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

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			header.Name += "/"
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

func runtimeError(message string, err error) error {
	return &VerifyError{Kind: ErrorKindRuntime, Message: message, Err: err}
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
