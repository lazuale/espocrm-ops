package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	manifestpkg "github.com/lazuale/espocrm-ops/internal/manifest"
)

func TestMigrateFromEqualsToFails(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Migrate(context.Background(), "prod", cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindUsage)
	if !strings.Contains(err.Error(), "--from-scope and --to-scope must differ") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Manifest != "" {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	if result.SnapshotManifest != "" {
		t.Fatalf("unexpected snapshot manifest: %s", result.SnapshotManifest)
	}
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestMigrateInvalidSourceManifestFailsBeforeRestoreMutation(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}
	missingManifest := filepath.Join(t.TempDir(), "missing.manifest.json")

	result, err := Migrate(context.Background(), "dev", cfg, missingManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindManifest)
	if result.Manifest != missingManifest {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	if result.SnapshotManifest != "" {
		t.Fatalf("unexpected snapshot manifest: %s", result.SnapshotManifest)
	}
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestMigrateVersionOneManifestFailsBeforeRestoreMutation(t *testing.T) {
	sourceManifest, _, _ := writeVersionOneScopedRestoreSourceBackupSet(t, "dev")
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Migrate(context.Background(), "dev", cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindManifest)
	if !strings.Contains(err.Error(), "manifest version 1") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Manifest != sourceManifest {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	if result.SnapshotManifest != "" {
		t.Fatalf("unexpected snapshot manifest: %s", result.SnapshotManifest)
	}
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestMigrateRuntimeMismatchFailsBeforeRestoreMutation(t *testing.T) {
	sourceManifest, _, _ := writeScopedRestoreSourceBackupSet(t, "dev")
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	loadedManifest, err := manifestpkg.Load(sourceManifest)
	if err != nil {
		t.Fatalf("Load manifest failed: %v", err)
	}
	loadedManifest.Runtime.EspoCRMImage = "espocrm/espocrm:10.0.0-apache"
	writeManifest(t, sourceManifest, loadedManifest)

	result, migrateErr := Migrate(context.Background(), "dev", cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, migrateErr, ErrorKindManifest)
	if !strings.Contains(migrateErr.Error(), "runtime.espo_crm_image") {
		t.Fatalf("unexpected error: %v", migrateErr)
	}
	if result.Manifest != sourceManifest {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	if result.SnapshotManifest != "" {
		t.Fatalf("unexpected snapshot manifest: %s", result.SnapshotManifest)
	}
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestMigrateRestoreFailureFails(t *testing.T) {
	sourceManifest, _, _ := writeScopedRestoreSourceBackupSet(t, "dev")
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		restoreDBErr:   errf("restore db failed"),
	}

	result, err := Migrate(context.Background(), "dev", cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database restore failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Manifest != sourceManifest {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("validate", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestMigrateFreeDiskPreflightFailsBeforeTargetDBReset(t *testing.T) {
	sourceManifest, _, _ := writeScopedRestoreSourceBackupSet(t, "dev")
	cfg, storageDir := restoreTargetConfig(t)
	storageParent := filepath.Clean(filepath.Dir(storageDir))
	restoreBackupDiskFreeBytes(t, func(path string) (uint64, error) {
		if filepath.Clean(path) == storageParent {
			return bytesPerMiB, nil
		}
		return 64 * bytesPerMiB, nil
	})

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Migrate(context.Background(), "dev", cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "restore free disk preflight failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Manifest != sourceManifest {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest before destructive migrate mutation")
	}
	if err := rt.requireCalls("validate", "stop_services", "service_stopped", "dump_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestMigrateBusyTargetLockFailsBeforeMutation(t *testing.T) {
	sourceManifest, _, _ := writeScopedRestoreSourceBackupSet(t, "dev")
	cfg, storageDir := restoreTargetConfig(t)
	lock := mustAcquireScopeOperationLock(t, cfg.ProjectDir, cfg.Scope)
	defer func() {
		if err := lock.Release(); err != nil {
			t.Fatalf("release lock: %v", err)
		}
	}()

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Migrate(context.Background(), "dev", cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "migrate lock failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	if result.SnapshotManifest != "" {
		t.Fatalf("unexpected snapshot manifest: %s", result.SnapshotManifest)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestMigrateBusySourceLockFailsBeforeTargetMutation(t *testing.T) {
	sourceManifest, _, _ := writeScopedRestoreSourceBackupSet(t, "dev")
	cfg, storageDir := restoreTargetConfig(t)
	lock := mustAcquireScopeOperationLock(t, cfg.ProjectDir, "dev")
	defer func() {
		if err := lock.Release(); err != nil {
			t.Fatalf("release lock: %v", err)
		}
	}()

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Migrate(context.Background(), "dev", cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "migrate lock failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Manifest != "" {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	if result.SnapshotManifest != "" {
		t.Fatalf("unexpected snapshot manifest: %s", result.SnapshotManifest)
	}
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestMigrateBusyKnownTargetLockFailsBeforeTargetMutation(t *testing.T) {
	sourceManifest, _, cfg, storageDir := writeKnownSourceMigrateBackupSet(t)
	lock := mustAcquireScopeOperationLock(t, cfg.ProjectDir, cfg.Scope)
	defer func() {
		if err := lock.Release(); err != nil {
			t.Fatalf("release lock: %v", err)
		}
	}()

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Migrate(context.Background(), "dev", cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "migrate lock failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Manifest != "" {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	if result.SnapshotManifest != "" {
		t.Fatalf("unexpected snapshot manifest: %s", result.SnapshotManifest)
	}
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	assertScopeOperationLockAvailable(t, cfg.ProjectDir, "dev")
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestMigrateSuccessReturnsManifestAndSnapshotManifest(t *testing.T) {
	sourceManifest, wantSQL, _ := writeScopedRestoreSourceBackupSet(t, "dev")
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Migrate(context.Background(), "dev", cfg, sourceManifest, rt, restoreTestTime())
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if result.Manifest != sourceManifest {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if _, err := os.Stat(result.SnapshotManifest); err != nil {
		t.Fatalf("expected snapshot manifest: %v", err)
	}
	if err := rt.requireCalls("validate", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services", "service_health", "db_ping"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != wantSQL {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func TestMigrateServiceHealthFailureFails(t *testing.T) {
	sourceManifest, _, _ := writeScopedRestoreSourceBackupSet(t, "dev")
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		healthErrors: map[int]error{
			2: errf(`service "espocrm-websocket" health is "unhealthy" (want "healthy")`),
		},
	}

	oldSleep := serviceHealthSleep
	serviceHealthSleep = func(context.Context, time.Duration) error {
		return context.DeadlineExceeded
	}
	defer func() {
		serviceHealthSleep = oldSleep
	}()

	result, err := Migrate(context.Background(), "dev", cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "restore post-check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), `service "espocrm-websocket"`) {
		t.Fatalf("expected health detail, got %v", err)
	}
	if result.Manifest != sourceManifest {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("validate", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func writeScopedRestoreSourceBackupSet(t *testing.T, scope string) (manifestPath, dbSQL, storageDir string) {
	t.Helper()

	root := t.TempDir()
	storageDir = filepath.Join(root, "runtime", scope, "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "restored.txt"), []byte("restored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := backupTestConfig(root, storageDir)
	cfg.Scope = scope
	cfg.EnvFile = filepath.Join(root, ".env."+scope)
	cfg.BackupRoot = filepath.Join(root, "backups", scope)

	dbSQL = "create table restored(id int);\n"
	result, err := Backup(context.Background(), cfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, dbSQL),
	}, restoreTestTime())
	if err != nil {
		t.Fatalf("write source backup set: %v", err)
	}
	return result.Manifest, dbSQL, storageDir
}

func writeKnownSourceMigrateBackupSet(t *testing.T) (manifestPath, dbSQL string, targetCfg config.BackupConfig, targetStorageDir string) {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sourceStorageDir := filepath.Join(root, "runtime", "dev", "espo")
	if err := os.MkdirAll(sourceStorageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceStorageDir, "restored.txt"), []byte("restored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceCfg := backupTestConfig(root, sourceStorageDir)
	sourceCfg.Scope = "dev"
	sourceCfg.EnvFile = filepath.Join(root, ".env.dev")
	sourceCfg.BackupRoot = filepath.Join(root, "backups", "dev")
	sourceCfg.BackupNamePrefix = "espocrm-dev"
	dbSQL = "create table restored(id int);\n"
	result, err := Backup(context.Background(), sourceCfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, dbSQL),
	}, restoreTestTime())
	if err != nil {
		t.Fatalf("write source backup set: %v", err)
	}

	targetStorageDir = filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(targetStorageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetStorageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	targetCfg = backupTestConfig(root, targetStorageDir)
	return result.Manifest, dbSQL, targetCfg, targetStorageDir
}

func writeVersionOneScopedRestoreSourceBackupSet(t *testing.T, scope string) (manifestPath, dbSQL, storageDir string) {
	t.Helper()

	root := t.TempDir()
	storageDir = filepath.Join(root, "runtime", scope, "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "restored.txt"), []byte("restored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbSQL = "create table restored(id int);\n"
	dbPath := filepath.Join(root, "db", "restore-source.sql.gz")
	filesPath := filepath.Join(root, "files", "restore-source.tar.gz")
	manifestPath = filepath.Join(root, "manifests", "restore-source.manifest.json")
	for _, dir := range []string{filepath.Dir(dbPath), filepath.Dir(filesPath), filepath.Dir(manifestPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeGzipFile(t, dbPath, []byte(dbSQL))
	writeTarGzFile(t, filesPath, map[string]string{"restored.txt": "restored\n"})
	rewriteSidecar(t, dbPath)
	rewriteSidecar(t, filesPath)
	writeManifest(t, manifestPath, map[string]any{
		"version":    1,
		"scope":      scope,
		"created_at": restoreTestTime().Format(time.RFC3339),
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(dbPath),
			"files_backup": filepath.Base(filesPath),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})

	return manifestPath, dbSQL, storageDir
}
