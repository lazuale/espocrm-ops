package ops

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
)

func TestBackupWritesArtifactsAndVerifies(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(filepath.Join(storageDir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "data", "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rt.lastServices, ",") != strings.Join(cfg.AppServices, ",") {
		t.Fatalf("unexpected app services: %v", rt.lastServices)
	}
	if result.Manifest == "" || result.DBBackup == "" || result.FilesBackup == "" {
		t.Fatalf("unexpected result: %#v", result)
	}

	for _, path := range []string{
		result.Manifest,
		result.DBBackup,
		result.DBBackup + ".sha256",
		result.FilesBackup,
		result.FilesBackup + ".sha256",
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}
	if _, err := VerifyBackup(context.Background(), result.Manifest); err != nil {
		t.Fatalf("VerifyBackup on produced set failed: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(cfg.BackupRoot, "*", "*.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("unexpected temporary files after success: %v", matches)
	}
}

func TestBackupUsesConfiguredNamePrefixNotScope(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := backupTestConfig(root, storageDir)
	cfg.BackupNamePrefix = "ops.snapshot-01"

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	wantBase := "ops.snapshot-01_2026-04-24_12-00-00"
	if got := filepath.Base(result.DBBackup); got != wantBase+".sql.gz" {
		t.Fatalf("unexpected db backup name: %s", got)
	}
	if got := filepath.Base(result.FilesBackup); got != wantBase+".tar.gz" {
		t.Fatalf("unexpected files backup name: %s", got)
	}
	if got := filepath.Base(result.Manifest); got != wantBase+".manifest.json" {
		t.Fatalf("unexpected manifest name: %s", got)
	}
	if strings.Contains(filepath.Base(result.Manifest), cfg.Scope+"_") {
		t.Fatalf("manifest unexpectedly used scope-based name: %s", result.Manifest)
	}
}

func TestBackupFailsClosedWhenSelfVerifyFails(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump: []byte("not gzip"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupLowDiskFailsBeforeStoppingServices(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restoreBackupDiskFreeBytes(t, func(string) (uint64, error) {
		return 1024 * 1024, nil
	})

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := backupTestConfig(root, storageDir)
	cfg.MinFreeDiskMB = 2

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "backup free disk preflight failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.requireCalls("validate"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupFailsWhenStopServicesFails(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump:  gzipBytes(t, "create table test(id int);\n"),
		stopErr: errf("stop failed"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "failed to stop app services") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.requireCalls("validate", "stop_services"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupFailureAfterStopAttemptsStart(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dumpErr: errf("dump failed"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database backup failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupStartFailureAfterSnapshotFailsBackup(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump:   gzipBytes(t, "create table test(id int);\n"),
		startErr: errf("start failed"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "failed to return app services") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupStartFailureAfterPriorFailureKeepsOriginalErrorVisible(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dumpErr:  errf("dump failed"),
		startErr: errf("start failed"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database backup failed") {
		t.Fatalf("original error was lost: %v", err)
	}
	if !strings.Contains(err.Error(), "return app services failed") {
		t.Fatalf("service return error missing: %v", err)
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupCancellationAfterStopStillAttemptsStart(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rt := &fakeBackupRuntime{
		dbDump:       gzipBytes(t, "create table test(id int);\n"),
		cancelOnStop: cancel,
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(ctx, cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database backup failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation in error chain: %v", err)
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	if len(rt.startContextErrs) != 1 || rt.startContextErrs[0] != nil {
		t.Fatalf("expected uncanceled service return context, got %v", rt.startContextErrs)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupCancellationAfterStopAndStartFailureIncludesBothErrors(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rt := &fakeBackupRuntime{
		dbDump:       gzipBytes(t, "create table test(id int);\n"),
		startErr:     errf("start failed"),
		cancelOnStop: cancel,
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(ctx, cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database backup failed") {
		t.Fatalf("original error was lost: %v", err)
	}
	if !strings.Contains(err.Error(), "return app services failed") {
		t.Fatalf("service return error missing: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation in error chain: %v", err)
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	if len(rt.startContextErrs) != 1 || rt.startContextErrs[0] != nil {
		t.Fatalf("expected uncanceled service return context, got %v", rt.startContextErrs)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupFailsWhenStorageDirIsBroken(t *testing.T) {
	root := t.TempDir()
	storagePath := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(filepath.Dir(storagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storagePath, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := backupTestConfig(root, storagePath)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupRetentionDeletesOldCompleteSamePrefixSetAfterSelfVerify(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	setupCfg := backupTestConfig(root, storageDir)
	setupCfg.BackupRetentionDays = 0
	oldResult, err := Backup(context.Background(), setupCfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table old_snapshot(id int);\n"),
	}, time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("setup backup failed: %v", err)
	}

	cfg := backupTestConfig(root, storageDir)
	result, err := Backup(context.Background(), cfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table fresh_snapshot(id int);\n"),
	}, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	assertBackupSetRemoved(t, oldResult)
	assertBackupSetPresent(t, result)
}

func TestBackupRetentionDoesNotRunWhenBackupFails(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	setupCfg := backupTestConfig(root, storageDir)
	setupCfg.BackupRetentionDays = 0
	oldResult, err := Backup(context.Background(), setupCfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table old_snapshot(id int);\n"),
	}, time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("setup backup failed: %v", err)
	}

	cfg := backupTestConfig(root, storageDir)
	result, err := Backup(context.Background(), cfg, &fakeBackupRuntime{
		dbDump: []byte("not gzip"),
	}, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	assertBackupSetPresent(t, oldResult)
	assertBackupSetRemoved(t, result)
}

func TestBackupRetentionKeepsDifferentPrefixSet(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	foreignCfg := backupTestConfig(root, storageDir)
	foreignCfg.BackupRetentionDays = 0
	foreignCfg.BackupNamePrefix = "other-prefix"
	foreignResult, err := Backup(context.Background(), foreignCfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table foreign_snapshot(id int);\n"),
	}, time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("foreign setup backup failed: %v", err)
	}

	cfg := backupTestConfig(root, storageDir)
	result, err := Backup(context.Background(), cfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table fresh_snapshot(id int);\n"),
	}, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	assertBackupSetPresent(t, foreignResult)
	assertBackupSetPresent(t, result)
}

func TestBackupRetentionRefusesIncompleteSamePrefixSet(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	setupCfg := backupTestConfig(root, storageDir)
	setupCfg.BackupRetentionDays = 0
	oldResult, err := Backup(context.Background(), setupCfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table old_snapshot(id int);\n"),
	}, time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("setup backup failed: %v", err)
	}
	if err := os.Remove(oldResult.FilesBackup + ".sha256"); err != nil {
		t.Fatal(err)
	}

	cfg := backupTestConfig(root, storageDir)
	result, err := Backup(context.Background(), cfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table fresh_snapshot(id int);\n"),
	}, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindArtifact)
	if !strings.Contains(err.Error(), "backup retention cleanup blocked") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertBackupSetPresent(t, result)
	if _, statErr := os.Stat(oldResult.Manifest); statErr != nil {
		t.Fatalf("expected incomplete manifest to remain for operator review: %v", statErr)
	}
}

func TestBackupRetentionCleanupErrorFailsBackupVisibly(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	setupCfg := backupTestConfig(root, storageDir)
	setupCfg.BackupRetentionDays = 0
	oldResult, err := Backup(context.Background(), setupCfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table old_snapshot(id int);\n"),
	}, time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("setup backup failed: %v", err)
	}

	restoreBackupRemovePath(t, func(path string) error {
		if path == oldResult.DBBackup+".sha256" {
			return errf("remove failed")
		}
		return os.Remove(path)
	})

	cfg := backupTestConfig(root, storageDir)
	result, err := Backup(context.Background(), cfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table fresh_snapshot(id int);\n"),
	}, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "backup retention cleanup failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertBackupSetPresent(t, result)
	assertBackupSetPresent(t, oldResult)
}

func backupTestConfig(root, storageDir string) config.BackupConfig {
	return config.BackupConfig{
		Scope:                      "prod",
		ProjectDir:                 root,
		ComposeFile:                filepath.Join(root, "compose.yaml"),
		EnvFile:                    filepath.Join(root, ".env.prod"),
		BackupRoot:                 filepath.Join(root, "backups", "prod"),
		BackupNamePrefix:           "espocrm-prod",
		BackupRetentionDays:        7,
		MinFreeDiskMB:              1,
		StorageDir:                 storageDir,
		AppServices:                []string{"espocrm", "espocrm-daemon", "espocrm-websocket"},
		DBService:                  "db",
		DBUser:                     "espocrm",
		DBPassword:                 "db-secret",
		DBRootPassword:             "root-secret",
		DBName:                     "espocrm",
		RuntimeUID:                 os.Getuid(),
		RuntimeGID:                 os.Getgid(),
		RuntimeOwnershipConfigured: true,
	}
}

type fakeBackupRuntime struct {
	dbDump           []byte
	validateErr      error
	stopErr          error
	startErr         error
	dumpErr          error
	cancelOnStop     context.CancelFunc
	calls            []string
	lastTarget       runtime.Target
	lastServices     []string
	startContextErrs []error
}

func (f *fakeBackupRuntime) Validate(_ context.Context, target runtime.Target) error {
	f.calls = append(f.calls, "validate")
	f.lastTarget = target
	return f.validateErr
}

func (f *fakeBackupRuntime) StopServices(_ context.Context, target runtime.Target, services []string) error {
	f.calls = append(f.calls, "stop_services")
	f.lastTarget = target
	f.lastServices = append([]string(nil), services...)
	if f.cancelOnStop != nil {
		f.cancelOnStop()
	}
	return f.stopErr
}

func (f *fakeBackupRuntime) StartServices(ctx context.Context, target runtime.Target, services []string) error {
	f.calls = append(f.calls, "start_services")
	f.lastTarget = target
	f.lastServices = append([]string(nil), services...)
	f.startContextErrs = append(f.startContextErrs, ctx.Err())
	return f.startErr
}

func (f *fakeBackupRuntime) DumpDatabase(ctx context.Context, target runtime.Target, destPath string) error {
	f.calls = append(f.calls, "dump_database")
	f.lastTarget = target
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.dumpErr != nil {
		return f.dumpErr
	}
	return os.WriteFile(destPath, append([]byte(nil), f.dbDump...), 0o644)
}

func (f *fakeBackupRuntime) requireCalls(want ...string) error {
	if strings.Join(f.calls, ",") != strings.Join(want, ",") {
		return errf("unexpected call order: got %v want %v", f.calls, want)
	}
	return nil
}

func assertBackupSetRemoved(t *testing.T, result BackupResult) {
	t.Helper()

	for _, path := range []string{
		result.Manifest,
		result.DBBackup,
		result.DBBackup + ".sha256",
		result.FilesBackup,
		result.FilesBackup + ".sha256",
	} {
		if path == "" {
			continue
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expected cleanup for %s, got %v", path, statErr)
		}
	}
}

func assertBackupSetPresent(t *testing.T, result BackupResult) {
	t.Helper()

	for _, path := range []string{
		result.Manifest,
		result.DBBackup,
		result.DBBackup + ".sha256",
		result.FilesBackup,
		result.FilesBackup + ".sha256",
	} {
		if path == "" {
			t.Fatalf("expected backup path, got empty result: %#v", result)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected backup artifact %s: %v", path, err)
		}
	}
}

func restoreBackupDiskFreeBytes(t *testing.T, fn func(string) (uint64, error)) {
	t.Helper()

	old := backupDiskFreeBytes
	backupDiskFreeBytes = fn
	t.Cleanup(func() {
		backupDiskFreeBytes = old
	})
}

func restoreBackupRemovePath(t *testing.T, fn func(string) error) {
	t.Helper()

	old := backupRemovePath
	backupRemovePath = fn
	t.Cleanup(func() {
		backupRemovePath = old
	})
}

func gzipBytes(t *testing.T, body string) []byte {
	t.Helper()

	path := filepath.Join(t.TempDir(), "db.sql.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type backupTestError string

func (e backupTestError) Error() string { return string(e) }

func errf(format string, args ...any) error {
	return backupTestError(strings.TrimSpace(fmt.Sprintf(format, args...)))
}
