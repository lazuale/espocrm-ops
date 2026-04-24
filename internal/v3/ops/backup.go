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

	v3config "github.com/lazuale/espocrm-ops/internal/v3/config"
	v3manifest "github.com/lazuale/espocrm-ops/internal/v3/manifest"
	v3runtime "github.com/lazuale/espocrm-ops/internal/v3/runtime"
)

type BackupResult struct {
	Manifest    string
	Scope       string
	CreatedAt   string
	DBBackup    string
	FilesBackup string
}

type backupRuntime interface {
	Validate(ctx context.Context, target v3runtime.Target) error
	RunningServices(ctx context.Context, target v3runtime.Target) ([]string, error)
	StopServices(ctx context.Context, target v3runtime.Target, services ...string) error
	StartServices(ctx context.Context, target v3runtime.Target, services ...string) error
	DumpDatabase(ctx context.Context, target v3runtime.Target, destPath string) error
}

type backupLayout struct {
	DBArtifact    string
	DBChecksum    string
	FilesArtifact string
	FilesChecksum string
	ManifestJSON  string
}

func Backup(ctx context.Context, cfg v3config.BackupConfig, rt backupRuntime, now time.Time) (result BackupResult, err error) {
	if rt == nil {
		return BackupResult{}, &VerifyError{Kind: ErrorKindRuntime, Message: "backup runtime is required"}
	}
	if err := validateBackupConfig(cfg); err != nil {
		return BackupResult{}, &VerifyError{Kind: ErrorKindUsage, Message: err.Error()}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	layout := newBackupLayout(cfg.BackupRoot, cfg.NamePrefix, now)
	result = BackupResult{
		Manifest:    layout.ManifestJSON,
		Scope:       cfg.Scope,
		CreatedAt:   now.Format(time.RFC3339),
		DBBackup:    layout.DBArtifact,
		FilesBackup: layout.FilesArtifact,
	}

	target := v3runtime.Target{
		ProjectDir:  cfg.ProjectDir,
		ComposeFile: cfg.ComposeFile,
		EnvFile:     cfg.EnvFile,
		DBService:   cfg.DBService,
		DBUser:      cfg.DBUser,
		DBPassword:  cfg.DBPassword,
		DBName:      cfg.DBName,
	}

	verified := false
	needsRuntimeReturn := false
	runtimeReturned := false
	stoppedServices := []string{}

	defer func() {
		if err != nil && !verified {
			cleanupErr := cleanupBackupSet(layout)
			if cleanupErr != nil {
				err = ioError("failed to remove incomplete backup set", errors.Join(err, cleanupErr))
			}
		}
		if !needsRuntimeReturn || runtimeReturned {
			return
		}
		startErr := rt.StartServices(ctx, target, stoppedServices...)
		runtimeReturned = true
		if startErr == nil {
			return
		}
		if err == nil {
			err = runtimeError("runtime return failed after backup", startErr)
			return
		}
		err = runtimeError("backup failed and runtime return also failed", errors.Join(err, startErr))
	}()

	if err := rt.Validate(ctx, target); err != nil {
		return result, runtimeError("compose configuration is invalid", err)
	}
	if err := ensureBackupLayout(layout); err != nil {
		return result, ioError("failed to prepare backup directories", err)
	}
	if err := ensureTargetsAbsent(layout); err != nil {
		return result, artifactError("backup set already exists", err)
	}

	running, err := rt.RunningServices(ctx, target)
	if err != nil {
		return result, runtimeError("could not inspect running services", err)
	}
	stoppedServices = runningApplicationServices(running)
	if len(stoppedServices) != 0 {
		if err := rt.StopServices(ctx, target, stoppedServices...); err != nil {
			return result, runtimeError("could not stop application services", err)
		}
		needsRuntimeReturn = true
	}

	if err := rt.DumpDatabase(ctx, target, layout.DBArtifact); err != nil {
		return result, runtimeError("database backup failed", err)
	}
	dbChecksum, err := sha256File(layout.DBArtifact)
	if err != nil {
		return result, ioError("failed to checksum db backup", err)
	}
	if err := writeSHA256Sidecar(layout.DBChecksum, filepath.Base(layout.DBArtifact), dbChecksum); err != nil {
		return result, ioError("failed to write db checksum sidecar", err)
	}

	if err := archiveStorageDir(cfg.StorageDir, layout.FilesArtifact); err != nil {
		return result, archiveError("files backup failed", err)
	}
	filesChecksum, err := sha256File(layout.FilesArtifact)
	if err != nil {
		return result, ioError("failed to checksum files backup", err)
	}
	if err := writeSHA256Sidecar(layout.FilesChecksum, filepath.Base(layout.FilesArtifact), filesChecksum); err != nil {
		return result, ioError("failed to write files checksum sidecar", err)
	}

	manifestData := v3manifest.Manifest{
		Version:   1,
		Scope:     cfg.Scope,
		CreatedAt: now.Format(time.RFC3339),
		Artifacts: v3manifest.Artifacts{
			DBBackup:    filepath.Base(layout.DBArtifact),
			FilesBackup: filepath.Base(layout.FilesArtifact),
		},
		Checksums: v3manifest.Checksums{
			DBBackup:    dbChecksum,
			FilesBackup: filesChecksum,
		},
	}
	if err := writeManifestJSON(layout.ManifestJSON, manifestData); err != nil {
		return result, ioError("failed to write backup manifest", err)
	}

	verifyResult, verifyErr := VerifyBackup(ctx, layout.ManifestJSON)
	if verifyErr != nil {
		return result, verifyErr
	}
	verified = true
	result = BackupResult(verifyResult)

	if len(stoppedServices) != 0 {
		if err := rt.StartServices(ctx, target, stoppedServices...); err != nil {
			needsRuntimeReturn = false
			runtimeReturned = true
			return result, runtimeError("runtime return failed after backup", err)
		}
		runtimeReturned = true
		needsRuntimeReturn = false
	}

	return result, nil
}

func validateBackupConfig(cfg v3config.BackupConfig) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "--scope", value: cfg.Scope},
		{name: "project dir", value: cfg.ProjectDir},
		{name: "compose file", value: cfg.ComposeFile},
		{name: "env file", value: cfg.EnvFile},
		{name: "BACKUP_ROOT", value: cfg.BackupRoot},
		{name: "BACKUP_NAME_PREFIX", value: cfg.NamePrefix},
		{name: "ESPO_STORAGE_DIR", value: cfg.StorageDir},
		{name: "DB_USER", value: cfg.DBUser},
		{name: "DB_PASSWORD", value: cfg.DBPassword},
		{name: "DB_NAME", value: cfg.DBName},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	return nil
}

func newBackupLayout(root, prefix string, createdAt time.Time) backupLayout {
	stamp := createdAt.UTC().Format("2006-01-02_15-04-05")
	dbArtifact := filepath.Join(root, "db", fmt.Sprintf("%s_%s.sql.gz", prefix, stamp))
	filesArtifact := filepath.Join(root, "files", fmt.Sprintf("%s_files_%s.tar.gz", prefix, stamp))
	return backupLayout{
		DBArtifact:    dbArtifact,
		DBChecksum:    dbArtifact + ".sha256",
		FilesArtifact: filesArtifact,
		FilesChecksum: filesArtifact + ".sha256",
		ManifestJSON:  filepath.Join(root, "manifests", fmt.Sprintf("%s_%s.manifest.json", prefix, stamp)),
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

func cleanupBackupSet(layout backupLayout) error {
	var cleanupErr error
	for _, path := range []string{
		layout.ManifestJSON,
		layout.FilesChecksum,
		layout.FilesArtifact,
		layout.DBChecksum,
		layout.DBArtifact,
	} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func writeSHA256Sidecar(path, artifactName, checksum string) error {
	body := checksum + "  " + artifactName + "\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func writeManifestJSON(path string, manifestData v3manifest.Manifest) error {
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

func runningApplicationServices(running []string) []string {
	seen := map[string]struct{}{}
	for _, service := range running {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		seen[service] = struct{}{}
	}

	out := []string{}
	for _, service := range []string{"espocrm", "espocrm-daemon", "espocrm-websocket"} {
		if _, ok := seen[service]; ok {
			out = append(out, service)
		}
	}
	return out
}

func runtimeError(message string, err error) error {
	return &VerifyError{Kind: ErrorKindRuntime, Message: message, Err: err}
}
