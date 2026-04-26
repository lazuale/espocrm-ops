package ops

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	manifestpkg "github.com/lazuale/espocrm-ops/internal/manifest"
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rt.lastServices, ",") != strings.Join(cfg.AppServices, ",") {
		t.Fatalf("unexpected app services: %v", rt.lastServices)
	}
	wantHealthServices := runtimeContractServices(cfg.DBService, cfg.AppServices)
	if strings.Join(rt.lastHealthServices, ",") != strings.Join(wantHealthServices, ",") {
		t.Fatalf("unexpected health services: got %v want %v", rt.lastHealthServices, wantHealthServices)
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
	loadedManifest, err := manifestpkg.Load(result.Manifest)
	if err != nil {
		t.Fatalf("Load manifest failed: %v", err)
	}
	if loadedManifest.Version != manifestpkg.VersionCurrent {
		t.Fatalf("unexpected manifest version: %d", loadedManifest.Version)
	}
	wantRuntime := backupManifestRuntime(cfg)
	if loadedManifest.Runtime.EspoCRMImage != wantRuntime.EspoCRMImage ||
		loadedManifest.Runtime.MariaDBImage != wantRuntime.MariaDBImage ||
		loadedManifest.Runtime.DBName != wantRuntime.DBName ||
		loadedManifest.Runtime.DBService != wantRuntime.DBService ||
		loadedManifest.Runtime.BackupNamePrefix != wantRuntime.BackupNamePrefix ||
		loadedManifest.Runtime.StorageContract != wantRuntime.StorageContract ||
		!slices.Equal(loadedManifest.Runtime.AppServices, wantRuntime.AppServices) {
		t.Fatalf("unexpected manifest runtime: %#v", loadedManifest.Runtime)
	}

	matches, err := filepath.Glob(filepath.Join(cfg.BackupRoot, "*", "*.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("unexpected temporary files after success: %v", matches)
	}
}

func TestBackupSuccessRequiresHealthCheck(t *testing.T) {
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

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	if len(rt.healthContextErrs) != 1 || rt.healthContextErrs[0] != nil {
		t.Fatalf("expected active health context, got %v", rt.healthContextErrs)
	}
	assertBackupSetPresent(t, result)
}

func TestBackupBusyLockFailsBeforeSideEffects(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}

	lock := mustAcquireScopeOperationLock(t, root, "prod")
	defer func() {
		if err := lock.Release(); err != nil {
			t.Fatalf("release lock: %v", err)
		}
	}()

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "backup lock failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupSuccessReleasesLock(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "data.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := backupTestConfig(root, storageDir)

	if _, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	assertScopeOperationLockAvailable(t, root, "prod")
}

func TestBackupFailureAfterStopReleasesLock(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "data.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dumpErr: errf("dump failed"),
	}
	cfg := backupTestConfig(root, storageDir)

	_, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	assertScopeOperationLockAvailable(t, root, "prod")
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

func TestBackupSameSecondCollisionFailsBeforeStoppingServices(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	cfg := backupTestConfig(root, storageDir)
	cfg.BackupRetentionDays = 0
	if _, err := Backup(context.Background(), cfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table first_snapshot(id int);\n"),
	}, now); err != nil {
		t.Fatalf("setup backup failed: %v", err)
	}

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table second_snapshot(id int);\n"),
	}
	result, err := Backup(context.Background(), cfg, rt, now)
	assertVerifyErrorKind(t, err, ErrorKindArtifact)
	if !strings.Contains(err.Error(), "backup target already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.requireCalls("compose_config"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetPresent(t, BackupResult{
		Manifest:    result.Manifest,
		DBBackup:    result.DBBackup,
		FilesBackup: result.FilesBackup,
	})
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupFailsIfReturnedServicesAreNotHealthy(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump:    gzipBytes(t, "create table test(id int);\n"),
		healthErr: errf(`service "espocrm" health is "unhealthy" (want "healthy")`),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "backup post-check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), `service "espocrm" health is "unhealthy"`) {
		t.Fatalf("expected health detail, got %v", err)
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	wantHealthServices := runtimeContractServices(cfg.DBService, cfg.AppServices)
	if strings.Join(rt.lastHealthServices, ",") != strings.Join(wantHealthServices, ",") {
		t.Fatalf("unexpected health services: got %v want %v", rt.lastHealthServices, wantHealthServices)
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
	if err := rt.requireCalls("compose_config"); err != nil {
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
	if err := rt.requireCalls("compose_config", "stop_services", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupStopFailureAndStartFailureIncludesBothErrors(t *testing.T) {
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
		stopErr:  errf("stop failed after partial stop"),
		startErr: errf("start failed"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "failed to stop app services") {
		t.Fatalf("original stop error was lost: %v", err)
	}
	if !strings.Contains(err.Error(), "return app services failed") {
		t.Fatalf("service return error missing: %v", err)
	}
	if err := rt.requireCalls("compose_config", "stop_services", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupFailsIfAppServicesStillRunningAfterStop(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump:       gzipBytes(t, "create table test(id int);\n"),
		stopCheckErr: errf(`service "espocrm" state is "running" (want stopped)`),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "app service stop check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "start_services"); err != nil {
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestBackupDumpFailureCleansPartialTempArtifacts(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dumpErr:     errf("dump failed"),
		dumpPartial: []byte("partial dump"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database backup failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertBackupSetRemoved(t, result)
	assertNoBackupTempFiles(t, cfg)
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services"); err != nil {
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services"); err != nil {
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
	if !strings.Contains(err.Error(), "app service stop check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation in error chain: %v", err)
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "start_services"); err != nil {
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
	if !strings.Contains(err.Error(), "app service stop check failed") {
		t.Fatalf("original error was lost: %v", err)
	}
	if !strings.Contains(err.Error(), "return app services failed") {
		t.Fatalf("service return error missing: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation in error chain: %v", err)
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "start_services"); err != nil {
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertBackupSetRemoved(t, result)
}

func TestArchiveStorageDirRejectsRootSymlink(t *testing.T) {
	root := t.TempDir()
	tarLog := installFailingTar(t)
	realStorage := filepath.Join(root, "real-storage")
	if err := os.MkdirAll(realStorage, 0o755); err != nil {
		t.Fatal(err)
	}
	storageLink := filepath.Join(root, "storage-link")
	if err := os.Symlink(realStorage, storageLink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := archiveStorageDir(context.Background(), storageLink, filepath.Join(root, "files.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "root is a symlink") {
		t.Fatalf("expected root symlink rejection, got %v", err)
	}
	assertNoTarCall(t, tarLog)
}

func TestArchiveStorageDirRejectsSymlinkEntryBeforeTar(t *testing.T) {
	root := t.TempDir()
	tarLog := installFailingTar(t)
	storageDir := filepath.Join(root, "storage")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	linkTarget := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(linkTarget, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkTarget, filepath.Join(storageDir, "linked.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := archiveStorageDir(context.Background(), storageDir, filepath.Join(root, "files.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "is a symlink") {
		t.Fatalf("expected symlink entry rejection, got %v", err)
	}
	assertNoTarCall(t, tarLog)
}

func TestArchiveStorageDirRejectsHardlinkedFile(t *testing.T) {
	root := t.TempDir()
	tarLog := installFailingTar(t)
	storageDir := filepath.Join(root, "storage")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(realFile, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(realFile, filepath.Join(storageDir, "hardlinked.txt")); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}

	err := archiveStorageDir(context.Background(), storageDir, filepath.Join(root, "files.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "multiple hardlinks") {
		t.Fatalf("expected hardlink rejection, got %v", err)
	}
	assertNoTarCall(t, tarLog)
}

func TestArchiveStorageDirRejectsUnsupportedTypeBeforeTar(t *testing.T) {
	root := t.TempDir()
	tarLog := installFailingTar(t)
	storageDir := filepath.Join(root, "storage")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(storageDir, "queue"), 0o644); err != nil {
		t.Skipf("fifo unavailable: %v", err)
	}

	err := archiveStorageDir(context.Background(), storageDir, filepath.Join(root, "files.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type rejection, got %v", err)
	}
	assertNoTarCall(t, tarLog)
}

func TestArchiveStorageDirRejectsUnsafeMode(t *testing.T) {
	root := t.TempDir()
	tarLog := installFailingTar(t)
	storageDir := filepath.Join(root, "storage")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	openFile := filepath.Join(storageDir, "open.txt")
	if err := os.WriteFile(openFile, []byte("open\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(openFile, 0o777); err != nil {
		t.Fatal(err)
	}

	err := archiveStorageDir(context.Background(), storageDir, filepath.Join(root, "files.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "world-writable") {
		t.Fatalf("expected unsafe mode rejection, got %v", err)
	}
	assertNoTarCall(t, tarLog)
}

func TestArchiveStorageDirRejectsTooManyEntriesBeforeTar(t *testing.T) {
	restoreFilesArchiveLimits(t, 1, defaultFilesArchiveMaxExpandedBytes)
	root := t.TempDir()
	tarLog := installFailingTar(t)
	storageDir := filepath.Join(root, "storage")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.txt", "two.txt"} {
		if err := os.WriteFile(filepath.Join(storageDir, name), []byte("ok\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := archiveStorageDir(context.Background(), storageDir, filepath.Join(root, "files.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "too many entries") {
		t.Fatalf("expected too many entries rejection, got %v", err)
	}
	assertNoTarCall(t, tarLog)
}

func TestArchiveStorageDirRejectsTooLargeSourceTreeBeforeTar(t *testing.T) {
	restoreFilesArchiveLimits(t, defaultFilesArchiveMaxEntries, 4)
	root := t.TempDir()
	tarLog := installFailingTar(t)
	storageDir := filepath.Join(root, "storage")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "large.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := archiveStorageDir(context.Background(), storageDir, filepath.Join(root, "files.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "regular file size exceeds limit") {
		t.Fatalf("expected source size rejection, got %v", err)
	}
	assertNoTarCall(t, tarLog)
}

func TestBackupNativeTarArchiveRejectedBySelfVerifyCleansIncompleteSet(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tarLog := installCorruptTar(t)

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(tarLog); statErr != nil {
		t.Fatalf("expected native tar call log: %v", statErr)
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

func TestSnapshotBackupSkipsRetentionAfterSelfVerify(t *testing.T) {
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
	result, err := snapshotBackup(context.Background(), cfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table fresh_snapshot(id int);\n"),
	}, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("snapshotBackup failed: %v", err)
	}

	assertBackupSetPresent(t, oldResult)
	assertBackupSetPresent(t, result)
	if len(result.Warnings) != 0 {
		t.Fatalf("snapshot backup should not report retention warnings: %#v", result.Warnings)
	}
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

func TestBackupRetentionWarnsForIncompleteSamePrefixSet(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	assertBackupWarningContains(t, result, "retention_skipped: backup retention cleanup blocked")
	assertBackupSetPresent(t, result)
	if _, statErr := os.Stat(oldResult.Manifest); statErr != nil {
		t.Fatalf("expected incomplete manifest to remain for operator review: %v", statErr)
	}
}

func TestBackupRetentionCleanupErrorWarnsVisibly(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	assertBackupWarningContains(t, result, "retention_skipped: backup retention cleanup failed")
	assertBackupSetPresent(t, result)
	assertBackupSetPresent(t, oldResult)
}

func TestPromoteTempFileRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	tempPath := filepath.Join(dir, ".artifact.tmp-1")
	finalPath := filepath.Join(dir, "artifact.sql.gz")
	if err := os.WriteFile(tempPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := promoteTempFile(tempPath, finalPath)
	if err == nil {
		t.Fatal("expected overwrite refusal")
	}
	if !strings.Contains(err.Error(), "target already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "old" {
		t.Fatalf("final artifact was overwritten: %q", string(raw))
	}
	if _, err := os.Stat(tempPath); err != nil {
		t.Fatalf("expected temp artifact to remain for cleanup: %v", err)
	}
}

func backupTestConfig(root, storageDir string) config.BackupConfig {
	return config.BackupConfig{
		Scope:                      "prod",
		ProjectDir:                 root,
		ComposeFile:                filepath.Join(root, "compose.yaml"),
		EnvFile:                    filepath.Join(root, ".env.prod"),
		EspoCRMImage:               "espocrm/espocrm:9.3.4-apache",
		MariaDBImage:               "mariadb:11.4",
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
	dbDump             []byte
	validateErr        error
	stopErr            error
	stopCheckErr       error
	startErr           error
	dumpErr            error
	dumpPartial        []byte
	healthErr          error
	cancelOnStop       context.CancelFunc
	calls              []string
	lastTarget         runtime.Target
	lastServices       []string
	lastStopServices   []string
	lastHealthServices []string
	startContextErrs   []error
	stopContextErrs    []error
	healthContextErrs  []error
}

func (f *fakeBackupRuntime) ComposeConfig(_ context.Context, target runtime.Target) error {
	f.calls = append(f.calls, "compose_config")
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

func (f *fakeBackupRuntime) RequireStoppedServices(ctx context.Context, target runtime.Target, services []string) error {
	f.calls = append(f.calls, "service_stopped")
	f.lastTarget = target
	f.lastStopServices = append([]string(nil), services...)
	f.stopContextErrs = append(f.stopContextErrs, ctx.Err())
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.stopCheckErr
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
	if len(f.dumpPartial) > 0 {
		if err := os.WriteFile(destPath, append([]byte(nil), f.dumpPartial...), 0o644); err != nil {
			return err
		}
	}
	if f.dumpErr != nil {
		return f.dumpErr
	}
	return os.WriteFile(destPath, append([]byte(nil), f.dbDump...), 0o644)
}

func (f *fakeBackupRuntime) RequireHealthyServices(ctx context.Context, target runtime.Target, services []string) error {
	f.calls = append(f.calls, "service_health")
	f.lastTarget = target
	f.lastHealthServices = append([]string(nil), services...)
	f.healthContextErrs = append(f.healthContextErrs, ctx.Err())
	return f.healthErr
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

func assertBackupWarningContains(t *testing.T, result BackupResult, want string) {
	t.Helper()

	if len(result.Warnings) != 1 {
		t.Fatalf("expected one warning, got %#v", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], want) {
		t.Fatalf("unexpected warning: %s", result.Warnings[0])
	}
}

func assertNoBackupTempFiles(t *testing.T, cfg config.BackupConfig) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(cfg.BackupRoot, "*", "*.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("unexpected temporary files: %v", matches)
	}
}

func installFailingTar(t *testing.T) string {
	t.Helper()
	return installBackupTestTar(t, `#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
fake_root="$(cd -- "$script_dir/.." && pwd)"
printf '%s\n' "$*" >>"$fake_root/tar.log"
printf 'tar should not run\n' >&2
exit 99
`)
}

func installCorruptTar(t *testing.T) string {
	t.Helper()
	return installBackupTestTar(t, `#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
fake_root="$(cd -- "$script_dir/.." && pwd)"
printf '%s\n' "$*" >>"$fake_root/tar.log"
dest=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-czf" ]]; then
    shift
    dest="${1:-}"
    break
  fi
  shift
done
[[ -n "$dest" ]] || exit 1
printf 'not a tar.gz archive\n' >"$dest"
`)
}

func installBackupTestTar(t *testing.T, script string) string {
	t.Helper()

	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "tar"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return filepath.Join(rootDir, "tar.log")
}

func assertNoTarCall(t *testing.T, tarLog string) {
	t.Helper()
	if _, err := os.Stat(tarLog); !os.IsNotExist(err) {
		t.Fatalf("expected tar not to run, log stat=%v", err)
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
