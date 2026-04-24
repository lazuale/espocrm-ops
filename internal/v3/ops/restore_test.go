package ops

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v3config "github.com/lazuale/espocrm-ops/internal/v3/config"
	v3runtime "github.com/lazuale/espocrm-ops/internal/v3/runtime"
)

func TestRestoreSourceManifestInvalidFailsBeforeMutation(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Restore(context.Background(), cfg, filepath.Join(t.TempDir(), "missing.manifest.json"), rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindManifest)
	if result.SnapshotManifest != "" {
		t.Fatalf("unexpected snapshot manifest: %s", result.SnapshotManifest)
	}
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
}

func TestRestoreSnapshotFailureFailsBeforeMutation(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		dumpErr: errf("snapshot dump failed"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database backup failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest != "" {
		t.Fatalf("unexpected snapshot manifest: %s", result.SnapshotManifest)
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestRestoreStopFailureFailsBeforeMutation(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		stopErrors: map[int]error{
			2: errf("stop failed"),
		},
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "failed to stop app services") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services"); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestRestoreDBFailureAttemptsStart(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		restoreDBErr:   errf("restore db failed"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database restore failed") {
		t.Fatalf("unexpected error: %v", err)
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

func TestRestoreFilesFailureAttemptsStart(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	if err := os.Chmod(storageDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chmod(storageDir, 0o755)
	}()

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "files restore failed") {
		t.Fatalf("unexpected error: %v", err)
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

func TestRestoreFailureAndStartFailureIncludesBothErrors(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, _ := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		restoreDBErr:   errf("restore db failed"),
		startErrors: map[int]error{
			2: errf("start failed"),
		},
	}

	_, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database restore failed") {
		t.Fatalf("original error was lost: %v", err)
	}
	if !strings.Contains(err.Error(), "return app services failed") {
		t.Fatalf("service return error missing: %v", err)
	}
}

func TestRestoreStartFailureFails(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, _ := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		startErrors: map[int]error{
			2: errf("start failed"),
		},
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "failed to return app services") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "restore_database", "start_services"); err != nil {
		t.Fatal(err)
	}
}

func TestRestorePostCheckFailureFails(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		dbPingErr:      errf("db ping failed"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "restore post-check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "restore_database", "start_services", "db_ping"); err != nil {
		t.Fatal(err)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func TestRestoreSuccessCreatesSnapshotBeforeMutation(t *testing.T) {
	sourceManifest, wantSQL, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
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

func restoreTargetConfig(t *testing.T) (v3config.BackupConfig, string) {
	t.Helper()

	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return backupTestConfig(root, storageDir), storageDir
}

func writeRestoreSourceBackupSet(t *testing.T) (manifestPath, dbSQL, storageDir string) {
	t.Helper()

	root := t.TempDir()
	storageDir = filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "restored.txt"), []byte("restored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbSQL = "create table restored(id int);\n"
	result, err := Backup(context.Background(), backupTestConfig(root, storageDir), &fakeBackupRuntime{
		dbDump: gzipBytes(t, dbSQL),
	}, restoreTestTime())
	if err != nil {
		t.Fatalf("write source backup set: %v", err)
	}
	return result.Manifest, dbSQL, storageDir
}

func restoreTestTime() time.Time {
	return time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
}

type fakeRestoreRuntime struct {
	snapshotDBDump []byte
	validateErr    error
	dumpErr        error
	stopErrors     map[int]error
	startErrors    map[int]error
	upErrors       map[int]error
	restoreDBErr   error
	dbPingErr      error
	calls          []string
	stopCount      int
	startCount     int
	upCount        int
	restoreDBBody  string
}

func (f *fakeRestoreRuntime) Validate(_ context.Context, _ v3runtime.Target) error {
	f.calls = append(f.calls, "validate")
	return f.validateErr
}

func (f *fakeRestoreRuntime) StopServices(_ context.Context, _ v3runtime.Target, _ []string) error {
	f.calls = append(f.calls, "stop_services")
	f.stopCount++
	return indexedError(f.stopErrors, f.stopCount)
}

func (f *fakeRestoreRuntime) StartServices(_ context.Context, _ v3runtime.Target, _ []string) error {
	f.calls = append(f.calls, "start_services")
	f.startCount++
	return indexedError(f.startErrors, f.startCount)
}

func (f *fakeRestoreRuntime) DumpDatabase(_ context.Context, _ v3runtime.Target, destPath string) error {
	f.calls = append(f.calls, "dump_database")
	if f.dumpErr != nil {
		return f.dumpErr
	}
	return os.WriteFile(destPath, append([]byte(nil), f.snapshotDBDump...), 0o644)
}

func (f *fakeRestoreRuntime) UpService(_ context.Context, _ v3runtime.Target, _ string) error {
	f.calls = append(f.calls, "up_service")
	f.upCount++
	return indexedError(f.upErrors, f.upCount)
}

func (f *fakeRestoreRuntime) RestoreDatabase(_ context.Context, _ v3runtime.Target, reader io.Reader) error {
	f.calls = append(f.calls, "restore_database")
	raw, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.restoreDBBody = string(raw)
	return f.restoreDBErr
}

func (f *fakeRestoreRuntime) DBPing(_ context.Context, _ v3runtime.Target) error {
	f.calls = append(f.calls, "db_ping")
	return f.dbPingErr
}

func (f *fakeRestoreRuntime) requireCalls(want ...string) error {
	if strings.Join(f.calls, ",") != strings.Join(want, ",") {
		return errf("unexpected call order: got %v want %v", f.calls, want)
	}
	return nil
}

func indexedError(items map[int]error, index int) error {
	if items == nil {
		return nil
	}
	return items[index]
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(raw) != want {
		t.Fatalf("unexpected file body for %s: got %q want %q", path, string(raw), want)
	}
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no file %s, got %v", path, err)
	}
}
