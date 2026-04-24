package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "restore_database", "start_services"); err != nil {
		t.Fatal(err)
	}
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
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "restore_database", "start_services", "db_ping"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != wantSQL {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
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
