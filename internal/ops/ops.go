package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/runtime"
)

const (
	ManifestName          = "manifest.json"
	DBFileName            = "db.sql.gz"
	FilesFileName         = "files.tar.gz"
	backupTimestampFormat = "2006-01-02T15-04-05Z"
)

type BackupResult struct {
	Manifest string
}

type VerifyResult struct {
	Manifest    string
	DBBackup    string
	FilesBackup string
}

func Doctor(ctx context.Context, cfg config.Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if err := runtime.RequireCommands("docker", "tar", "gzip", "sha256sum"); err != nil {
		return err
	}
	if err := runtime.ComposeConfig(ctx, targetFromConfig(cfg)); err != nil {
		return err
	}
	if err := ensureBackupRootWritable(cfg.BackupRoot, false); err != nil {
		return err
	}
	if err := ensureStorageDir(cfg.StorageDir); err != nil {
		return err
	}
	return runtime.DBPing(ctx, targetFromConfig(cfg))
}

func Backup(ctx context.Context, cfg config.Config, now time.Time) (BackupResult, error) {
	if err := validateConfig(cfg); err != nil {
		return BackupResult{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := runtime.RequireCommands("docker", "tar", "gzip", "sha256sum"); err != nil {
		return BackupResult{}, err
	}
	if err := runtime.ComposeConfig(ctx, targetFromConfig(cfg)); err != nil {
		return BackupResult{}, err
	}
	if err := ensureBackupRootWritable(cfg.BackupRoot, true); err != nil {
		return BackupResult{}, err
	}
	if err := ensureStorageDir(cfg.StorageDir); err != nil {
		return BackupResult{}, err
	}

	layout := backupLayout(cfg.BackupRoot, cfg.Scope, now.UTC())
	if err := os.MkdirAll(filepath.Dir(layout.dir), 0o755); err != nil {
		return BackupResult{}, err
	}
	if err := os.Mkdir(layout.dir, 0o755); err != nil {
		return BackupResult{}, err
	}

	target := targetFromConfig(cfg)
	if err := runtime.StopServices(ctx, target, cfg.AppServices); err != nil {
		return BackupResult{}, err
	}
	if err := runtime.DumpDatabase(ctx, target, layout.db); err != nil {
		return BackupResult{}, err
	}
	if err := runtime.CreateStorageArchive(ctx, cfg.StorageDir, layout.files); err != nil {
		return BackupResult{}, err
	}
	if err := runtime.StartServices(ctx, target, cfg.AppServices); err != nil {
		return BackupResult{}, err
	}

	dbSHA, err := runtime.SHA256(ctx, layout.db)
	if err != nil {
		return BackupResult{}, err
	}
	filesSHA, err := runtime.SHA256(ctx, layout.files)
	if err != nil {
		return BackupResult{}, err
	}
	if err := writeManifest(layout.manifest, dbSHA, filesSHA); err != nil {
		return BackupResult{}, err
	}
	return BackupResult{Manifest: layout.manifest}, nil
}

func VerifyBackup(ctx context.Context, manifestPath string) (VerifyResult, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return VerifyResult{}, fmt.Errorf("manifest path is required")
	}
	manifestPath = filepath.Clean(manifestPath)
	if err := requireFile(manifestPath); err != nil {
		return VerifyResult{}, err
	}
	dbWant, filesWant, err := readManifest(manifestPath)
	if err != nil {
		return VerifyResult{}, err
	}

	dir := filepath.Dir(manifestPath)
	dbPath := filepath.Join(dir, DBFileName)
	filesPath := filepath.Join(dir, FilesFileName)
	if err := verifyGzipArtifact(ctx, dbPath, dbWant); err != nil {
		return VerifyResult{}, err
	}
	if err := verifyGzipArtifact(ctx, filesPath, filesWant); err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{Manifest: manifestPath, DBBackup: dbPath, FilesBackup: filesPath}, nil
}

func Restore(ctx context.Context, cfg config.Config, manifestPath string) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	verified, err := VerifyBackup(ctx, manifestPath)
	if err != nil {
		return err
	}
	if err := ensureStorageDir(cfg.StorageDir); err != nil {
		return err
	}

	target := targetFromConfig(cfg)
	if err := runtime.StopServices(ctx, target, cfg.AppServices); err != nil {
		return err
	}
	if err := runtime.ResetDatabase(ctx, target); err != nil {
		return err
	}
	if err := runtime.RestoreDatabase(ctx, target, verified.DBBackup); err != nil {
		return err
	}
	if err := resetStorageDir(cfg.StorageDir); err != nil {
		return err
	}
	if err := runtime.ExtractStorageArchive(ctx, verified.FilesBackup, cfg.StorageDir); err != nil {
		return err
	}
	return runtime.StartServices(ctx, target, cfg.AppServices)
}

type backupPaths struct {
	dir      string
	manifest string
	db       string
	files    string
}

func backupLayout(root, scope string, createdAt time.Time) backupPaths {
	dir := filepath.Join(root, scope, createdAt.Format(backupTimestampFormat))
	return backupPaths{
		dir:      dir,
		manifest: filepath.Join(dir, ManifestName),
		db:       filepath.Join(dir, DBFileName),
		files:    filepath.Join(dir, FilesFileName),
	}
}

func writeManifest(path, dbSHA, filesSHA string) error {
	raw, err := json.MarshalIndent(map[string]string{
		"db":    dbSHA,
		"files": filesSHA,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func readManifest(path string) (string, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return "", "", err
	}
	dbSHA := strings.ToLower(strings.TrimSpace(values["db"]))
	filesSHA := strings.ToLower(strings.TrimSpace(values["files"]))
	if dbSHA == "" {
		return "", "", fmt.Errorf("manifest db checksum is required")
	}
	if filesSHA == "" {
		return "", "", fmt.Errorf("manifest files checksum is required")
	}
	return dbSHA, filesSHA, nil
}

func verifyGzipArtifact(ctx context.Context, path, wantSHA string) error {
	if err := requireFile(path); err != nil {
		return err
	}
	gotSHA, err := runtime.SHA256(ctx, path)
	if err != nil {
		return err
	}
	if gotSHA != strings.ToLower(strings.TrimSpace(wantSHA)) {
		return fmt.Errorf("%s checksum mismatch", path)
	}
	return runtime.TestGzip(ctx, path)
}

func validateConfig(cfg config.Config) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "scope", value: cfg.Scope},
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
	if len(cfg.AppServices) == 0 || slices.ContainsFunc(cfg.AppServices, func(service string) bool {
		return strings.TrimSpace(service) == ""
	}) {
		return fmt.Errorf("APP_SERVICES is required")
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
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return err
	}
	return os.Remove(name)
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

func resetStorageDir(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." || path == string(filepath.Separator) {
		return fmt.Errorf("unsafe storage dir: %s", path)
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

func requireFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	return nil
}
