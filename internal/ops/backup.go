package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/manifest"
	"github.com/lazuale/espocrm-ops/internal/runtime"
)

const backupTimestampFormat = "2006-01-02T15-04-05.000000000Z"
const serviceReturnTimeout = 30 * time.Second

type BackupResult struct {
	Manifest    string
	DBBackup    string
	FilesBackup string
}

type backupRuntime interface {
	ComposeConfig(ctx context.Context, target runtime.Target) error
	StopServices(ctx context.Context, target runtime.Target, services []string) error
	StartServices(ctx context.Context, target runtime.Target, services []string) error
	DumpDatabase(ctx context.Context, target runtime.Target, destPath string) error
	RequireHealthyServices(ctx context.Context, target runtime.Target, services []string) error
}

type backupLayout struct {
	Dir           string
	ManifestJSON  string
	DBArtifact    string
	FilesArtifact string
}

func Backup(ctx context.Context, cfg config.Config, rt backupRuntime, now time.Time) (BackupResult, error) {
	if rt == nil {
		return BackupResult{}, runtimeError("backup runtime is required", nil)
	}
	return withProjectLock(ctx, cfg.ProjectDir, func(lockedCtx context.Context) (BackupResult, error) {
		return backupCore(lockedCtx, cfg, rt, now)
	})
}

func backupCore(ctx context.Context, cfg config.Config, rt backupRuntime, now time.Time) (result BackupResult, err error) {
	if err := validateConfig(cfg); err != nil {
		return BackupResult{}, usageError(err.Error())
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

	verified := false
	defer func() {
		if verified {
			return
		}
		if cleanupErr := os.RemoveAll(layout.Dir); cleanupErr != nil {
			err = ioError("failed to remove incomplete backup set", errors.Join(err, cleanupErr))
		}
	}()

	target := targetFromConfig(cfg)
	if err := rt.ComposeConfig(ctx, target); err != nil {
		return result, runtimeError("docker compose config failed", err)
	}
	if err := ensureBackupRootWritable(cfg.BackupRoot, true); err != nil {
		return result, ioError("backup root is not writable", err)
	}
	if err := ensureStorageDir(cfg.StorageDir); err != nil {
		return result, ioError("storage dir is invalid", err)
	}
	if err := runtime.TarExists(); err != nil {
		return result, runtimeError("tar check failed", err)
	}
	if err := createBackupDir(layout.Dir); err != nil {
		return result, ioError("failed to create backup set", err)
	}

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

	if err := rt.DumpDatabase(ctx, target, layout.DBArtifact); err != nil {
		return result, runtimeError("database backup failed", err)
	}
	if err := runtime.CreateStorageArchive(ctx, cfg.StorageDir, layout.FilesArtifact); err != nil {
		return result, archiveError("files backup failed", err)
	}

	startCtx, cancel := serviceReturnContext()
	if err := rt.StartServices(startCtx, target, cfg.AppServices); err != nil {
		cancel()
		return result, runtimeError("failed to start app services", err)
	}
	cancel()
	servicesReturned = true

	if err := waitForRuntimeServiceHealth(ctx, target, cfg.DBService, cfg.AppServices, rt); err != nil {
		return result, runtimeError("backup health check failed", err)
	}

	dbChecksum, err := sha256File(layout.DBArtifact)
	if err != nil {
		return result, ioError("failed to checksum db backup", err)
	}
	filesChecksum, err := sha256File(layout.FilesArtifact)
	if err != nil {
		return result, ioError("failed to checksum files backup", err)
	}
	if err := manifest.Write(layout.ManifestJSON, manifest.Manifest{
		Version:   manifest.Version,
		Scope:     cfg.Scope,
		CreatedAt: now.Format(time.RFC3339Nano),
		DB: manifest.Artifact{
			File:   manifest.DBFileName,
			SHA256: dbChecksum,
		},
		Files: manifest.Artifact{
			File:   manifest.FilesFileName,
			SHA256: filesChecksum,
		},
		DBName:      cfg.DBName,
		DBService:   cfg.DBService,
		AppServices: append([]string(nil), cfg.AppServices...),
	}); err != nil {
		return result, ioError("failed to write backup manifest", err)
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

func validateConfig(cfg config.Config) error {
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
		{name: "DB_SERVICE", value: cfg.DBService},
		{name: "DB_USER", value: cfg.DBUser},
		{name: "DB_PASSWORD", value: cfg.DBPassword},
		{name: "DB_ROOT_PASSWORD", value: cfg.DBRootPassword},
		{name: "DB_NAME", value: cfg.DBName},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	if len(cfg.AppServices) == 0 {
		return fmt.Errorf("APP_SERVICES is required")
	}
	if slices.ContainsFunc(cfg.AppServices, func(service string) bool {
		return strings.TrimSpace(service) == ""
	}) {
		return fmt.Errorf("APP_SERVICES contains empty service name")
	}
	return nil
}

func newBackupLayout(root, scope string, createdAt time.Time) backupLayout {
	dir := filepath.Join(root, scope, createdAt.UTC().Format(backupTimestampFormat))
	return backupLayout{
		Dir:           dir,
		ManifestJSON:  filepath.Join(dir, manifest.ManifestName),
		DBArtifact:    filepath.Join(dir, manifest.DBFileName),
		FilesArtifact: filepath.Join(dir, manifest.FilesFileName),
	}
}

func createBackupDir(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		return err
	}
	return nil
}

func ensureBackupRootWritable(path string, create bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("backup root is required")
	}
	if create {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("backup root must be a directory")
	}
	probe, err := os.CreateTemp(path, ".espops-write-probe-*")
	if err != nil {
		return err
	}
	probePath := probe.Name()
	closeErr := probe.Close()
	removeErr := os.Remove(probePath)
	return errors.Join(closeErr, removeErr)
}

func ensureStorageDir(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("storage dir is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("storage dir must be a directory")
	}
	return nil
}

func targetFromConfig(cfg config.Config) runtime.Target {
	return runtime.Target{
		ProjectDir:     cfg.ProjectDir,
		ComposeFile:    cfg.ComposeFile,
		EnvFile:        cfg.EnvFile,
		DBService:      cfg.DBService,
		DBUser:         cfg.DBUser,
		DBPassword:     cfg.DBPassword,
		DBRootPassword: cfg.DBRootPassword,
		DBName:         cfg.DBName,
	}
}

func serviceReturnContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), serviceReturnTimeout)
}
