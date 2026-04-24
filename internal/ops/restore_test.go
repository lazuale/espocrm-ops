package ops

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
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

func TestRestoreRejectsDifferentManifestScopeBeforeMutation(t *testing.T) {
	sourceManifest, _, _ := writeScopedRestoreSourceBackupSet(t, "dev")
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindUsage)
	if !strings.Contains(err.Error(), "restore source scope is invalid") {
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
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "reset_database", "restore_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestRestoreResetDBFailureAttemptsStartWithoutImportOrFileMutation(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		resetDBErr:     errf("reset db failed"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database reset failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "reset_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != "" {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestRestoreUnclearableStorageFailsBeforeMutation(t *testing.T) {
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
	if !strings.Contains(err.Error(), "restore storage target is not clearable") {
		t.Fatalf("unexpected error: %v", err)
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

func TestRestoreNestedUnclearableStorageFailsBeforeMutation(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	nestedDir := filepath.Join(storageDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(nestedDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chmod(nestedDir, 0o755)
	}()

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "restore storage target is not clearable") {
		t.Fatalf("unexpected error: %v", err)
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
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "reset_database", "restore_database", "start_services"); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreCancellationAfterStopStillAttemptsStart(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	rt := &fakeRestoreRuntime{
		snapshotDBDump:    gzipBytes(t, "create table snapshot(id int);\n"),
		cancel:            cancel,
		cancelOnStopCount: 2,
	}

	result, err := Restore(ctx, cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "restore interrupted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation in error chain: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "start_services"); err != nil {
		t.Fatal(err)
	}
	if len(rt.startContextErrs) != 2 || rt.startContextErrs[1] != nil {
		t.Fatalf("expected uncanceled restore return context, got %v", rt.startContextErrs)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestRestoreCancellationAfterStopAndStartFailureIncludesBothErrors(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	rt := &fakeRestoreRuntime{
		snapshotDBDump:    gzipBytes(t, "create table snapshot(id int);\n"),
		cancel:            cancel,
		cancelOnStopCount: 2,
		startErrors: map[int]error{
			2: errf("start failed"),
		},
	}

	result, err := Restore(ctx, cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "restore interrupted") {
		t.Fatalf("original error was lost: %v", err)
	}
	if !strings.Contains(err.Error(), "return app services failed") {
		t.Fatalf("service return error missing: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation in error chain: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "start_services"); err != nil {
		t.Fatal(err)
	}
	if len(rt.startContextErrs) != 2 || rt.startContextErrs[1] != nil {
		t.Fatalf("expected uncanceled restore return context, got %v", rt.startContextErrs)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
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
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "reset_database", "restore_database", "start_services", "db_ping"); err != nil {
		t.Fatal(err)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func TestRestoreFilePhaseFailureAfterDatabaseImportStillReturnsServices(t *testing.T) {
	sourceManifest, wantSQL, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}
	oldExtract := restoreExtractTarEntry
	restoreExtractTarEntry = func(root string, header *tar.Header, reader io.Reader) error {
		if header.Name == "restored.txt" {
			return errf("simulated staging write failure")
		}
		return oldExtract(root, header, reader)
	}
	defer func() {
		restoreExtractTarEntry = oldExtract
	}()

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "files staging extraction failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "reset_database", "restore_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != wantSQL {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
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
	if err := rt.requireCalls("validate", "stop_services", "dump_database", "start_services", "stop_services", "up_service", "reset_database", "restore_database", "start_services", "db_ping"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != wantSQL {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func TestRestoreFilesBackupUnsafeArchiveDoesNotClearStorage(t *testing.T) {
	_, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchiveEntries(t, archivePath, []restoreArchiveEntry{
		{name: "restored.txt", body: "restored\n"},
		{name: "../escape.txt", body: "bad\n"},
	})

	err := restoreFilesBackup(context.Background(), archivePath, storageDir)
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
}

func TestRestoreFilesBackupBrokenArchiveDoesNotClearStorage(t *testing.T) {
	_, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeBrokenRestoreFilesArchive(t, archivePath, "restored.txt", "restored\n")

	err := restoreFilesBackup(context.Background(), archivePath, storageDir)
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
}

func TestRestoreFilesBackupTargetSymlinkFailsAfterSuccessfulStaging(t *testing.T) {
	_, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t)
	linkTarget := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(linkTarget, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(storageDir, "linked")
	if err := os.Symlink(linkTarget, linkPath); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchive(t, archivePath, map[string]string{"restored.txt": "restored\n"})

	err := restoreFilesBackup(context.Background(), archivePath, storageDir)
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "files replace failed before target clear") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("expected symlink to remain: %v", err)
	}
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
}

func TestRestoreFilesBackupStagingExtractionFailureDoesNotClearStorage(t *testing.T) {
	_, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchive(t, archivePath, map[string]string{"restored.txt": "restored\n"})

	oldExtract := restoreExtractTarEntry
	restoreExtractTarEntry = func(root string, header *tar.Header, reader io.Reader) error {
		if header.Name == "restored.txt" {
			return errf("simulated staging write failure")
		}
		return oldExtract(root, header, reader)
	}
	defer func() {
		restoreExtractTarEntry = oldExtract
	}()

	err := restoreFilesBackup(context.Background(), archivePath, storageDir)
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "files staging extraction failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
}

func TestRestoreFilesBackupValidArchiveStillWorks(t *testing.T) {
	_, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchive(t, archivePath, map[string]string{
		"restored.txt":     "restored\n",
		"nested/child.txt": "child\n",
	})

	if err := restoreFilesBackup(context.Background(), archivePath, storageDir); err != nil {
		t.Fatalf("restoreFilesBackup failed: %v", err)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
	assertFileContains(t, filepath.Join(storageDir, "nested", "child.txt"), "child\n")
	probe.assertClean(t)
}

func TestValidateRestoredStorageTreeRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	linkTarget := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(linkTarget, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkTarget, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	err := validateRestoredStorageTree(root)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink validation error, got %v", err)
	}
}

func TestValidateRestoredStorageTreeRejectsEmptyDirectory(t *testing.T) {
	root := t.TempDir()

	err := validateRestoredStorageTree(root)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty validation error, got %v", err)
	}
}

func restoreTargetConfig(t *testing.T) (config.BackupConfig, string) {
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
	snapshotDBDump    []byte
	validateErr       error
	dumpErr           error
	stopErrors        map[int]error
	startErrors       map[int]error
	upErrors          map[int]error
	restoreDBErr      error
	resetDBErr        error
	dbPingErr         error
	cancel            context.CancelFunc
	cancelOnStopCount int
	calls             []string
	stopCount         int
	startCount        int
	upCount           int
	restoreDBBody     string
	startContextErrs  []error
}

func (f *fakeRestoreRuntime) Validate(_ context.Context, _ runtime.Target) error {
	f.calls = append(f.calls, "validate")
	return f.validateErr
}

func (f *fakeRestoreRuntime) StopServices(_ context.Context, _ runtime.Target, _ []string) error {
	f.calls = append(f.calls, "stop_services")
	f.stopCount++
	if f.cancel != nil && f.stopCount == f.cancelOnStopCount {
		f.cancel()
	}
	return indexedError(f.stopErrors, f.stopCount)
}

func (f *fakeRestoreRuntime) StartServices(ctx context.Context, _ runtime.Target, _ []string) error {
	f.calls = append(f.calls, "start_services")
	f.startContextErrs = append(f.startContextErrs, ctx.Err())
	f.startCount++
	return indexedError(f.startErrors, f.startCount)
}

func (f *fakeRestoreRuntime) DumpDatabase(_ context.Context, _ runtime.Target, destPath string) error {
	f.calls = append(f.calls, "dump_database")
	if f.dumpErr != nil {
		return f.dumpErr
	}
	return os.WriteFile(destPath, append([]byte(nil), f.snapshotDBDump...), 0o644)
}

func (f *fakeRestoreRuntime) UpService(_ context.Context, _ runtime.Target, _ string) error {
	f.calls = append(f.calls, "up_service")
	f.upCount++
	return indexedError(f.upErrors, f.upCount)
}

func (f *fakeRestoreRuntime) ResetDatabase(_ context.Context, _ runtime.Target) error {
	f.calls = append(f.calls, "reset_database")
	return f.resetDBErr
}

func (f *fakeRestoreRuntime) RestoreDatabase(_ context.Context, _ runtime.Target, reader io.Reader) error {
	f.calls = append(f.calls, "restore_database")
	raw, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.restoreDBBody = string(raw)
	return f.restoreDBErr
}

func (f *fakeRestoreRuntime) DBPing(_ context.Context, _ runtime.Target) error {
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

type restoreArchiveEntry struct {
	name string
	body string
}

func writeRestoreFilesArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()

	entries := make([]restoreArchiveEntry, 0, len(files))
	for name, body := range files {
		entries = append(entries, restoreArchiveEntry{name: name, body: body})
	}
	writeRestoreFilesArchiveEntries(t, path, entries)
}

func writeRestoreFilesArchiveEntries(t *testing.T, path string, entries []restoreArchiveEntry) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestResource(t, file)

	gzipWriter := gzip.NewWriter(file)
	defer closeTestResource(t, gzipWriter)

	tarWriter := tar.NewWriter(gzipWriter)
	defer closeTestResource(t, tarWriter)

	for _, entry := range entries {
		header := &tar.Header{
			Name: entry.name,
			Mode: 0o644,
			Size: int64(len(entry.body)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
}

func writeBrokenRestoreFilesArchive(t *testing.T, path, name, body string) {
	t.Helper()

	var tarBody bytes.Buffer
	tarWriter := tar.NewWriter(&tarBody)
	header := &tar.Header{
		Name: name,
		Mode: 0o644,
		Size: int64(len(body)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}

	raw := tarBody.Bytes()
	if len(raw) < 1024 {
		t.Fatalf("unexpected tar size: %d", len(raw))
	}
	broken := append([]byte(nil), raw[:len(raw)-1024]...)
	broken = append(broken, []byte("broken")...)

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestResource(t, file)

	gzipWriter := gzip.NewWriter(file)
	if _, err := gzipWriter.Write(broken); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
}

type restoreStagingProbe struct {
	root    string
	created []string
}

func useRestoreStagingProbe(t *testing.T) *restoreStagingProbe {
	t.Helper()

	probe := &restoreStagingProbe{root: t.TempDir()}
	oldCreate := createRestoreStagingDir
	createRestoreStagingDir = func(_ string) (string, error) {
		dir, err := os.MkdirTemp(probe.root, restoreStagingDirPattern)
		if err != nil {
			return "", err
		}
		probe.created = append(probe.created, dir)
		return dir, nil
	}
	t.Cleanup(func() {
		createRestoreStagingDir = oldCreate
	})
	return probe
}

func (p *restoreStagingProbe) assertClean(t *testing.T) {
	t.Helper()

	for _, dir := range p.created {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("expected staging dir %s to be removed, got %v", dir, err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(p.root, restoreStagingDirPattern))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("unexpected staging dirs left behind: %v", matches)
	}
}
