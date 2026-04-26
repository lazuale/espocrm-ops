package ops

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	manifestpkg "github.com/lazuale/espocrm-ops/internal/manifest"
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

func TestRestoreBusyLockFailsBeforeMutation(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
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

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "restore lock failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	if result.SnapshotManifest != "" {
		t.Fatalf("unexpected snapshot manifest: %s", result.SnapshotManifest)
	}
}

func TestRestoreRejectsDifferentManifestScopeBeforeMutation(t *testing.T) {
	sourceManifest, _, _ := writeScopedRestoreSourceBackupSet(t, "dev")
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindUsage)
	if !strings.Contains(err.Error(), "manifest scope is invalid for requested operation") {
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

func TestRestoreVersionOneManifestFailsBeforeMutation(t *testing.T) {
	sourceManifest, _, _ := writeVersionOneRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
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

func TestRestoreRuntimeMismatchFailsBeforeMutation(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	loadedManifest, err := manifestpkg.Load(sourceManifest)
	if err != nil {
		t.Fatalf("Load manifest failed: %v", err)
	}
	loadedManifest.Runtime.MariaDBImage = "mariadb:12.0"
	writeManifest(t, sourceManifest, loadedManifest)

	result, restoreErr := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, restoreErr, ErrorKindManifest)
	if !strings.Contains(restoreErr.Error(), "runtime.mariadb_image") {
		t.Fatalf("unexpected error: %v", restoreErr)
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services"); err != nil {
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services"); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestRestoreDBServiceFailureAfterSnapshotAttemptsStart(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		upErrors: map[int]error{
			1: errf("db up failed"),
		},
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "failed to ensure db service") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "start_services"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != "" {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestRestoreDBFailureAttemptsStart(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		restoreDBErr:   errf("restore db failed"),
	}
	oldRename := restoreRenamePath
	renameCount := 0
	restoreRenamePath = func(oldPath, newPath string) error {
		renameCount++
		return oldRename(oldPath, newPath)
	}
	defer func() {
		restoreRenamePath = oldRename
	}()

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "database restore failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	if renameCount != 0 {
		t.Fatalf("storage switch should not start after database restore failure; got %d renames", renameCount)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
}

func TestRestoreDatabaseBackupRejectsDBGzipOverExpandedLimit(t *testing.T) {
	restoreDBBackupMaxExpandedBytes(t, 4)
	backupPath := filepath.Join(t.TempDir(), "db.sql.gz")
	writeGzipFile(t, backupPath, []byte("12345"))
	rt := &fakeRestoreRuntime{}

	err := restoreDatabaseBackup(context.Background(), backupPath, rt, runtime.Target{})
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if !strings.Contains(err.Error(), "db backup expanded size exceeds limit") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.requireCalls("restore_database"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != "" {
		t.Fatalf("expected restore body to be rejected, got %q", rt.restoreDBBody)
	}
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != "" {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestRestoreFreeDiskPreflightFailsBeforeDBReset(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	storageParent := filepath.Clean(filepath.Dir(storageDir))
	restorePreflightChecked := false
	restoreBackupDiskFreeBytes(t, func(path string) (uint64, error) {
		if filepath.Clean(path) == storageParent {
			restorePreflightChecked = true
			return bytesPerMiB, nil
		}
		return 64 * bytesPerMiB, nil
	})

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "restore free disk preflight failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "restore staging requires") ||
		!strings.Contains(err.Error(), "files backup expands") ||
		!strings.Contains(err.Error(), "MIN_FREE_DISK_MB=1") {
		t.Fatalf("error is not operator-clear enough: %v", err)
	}
	if !restorePreflightChecked {
		t.Fatal("expected restore staging free-space preflight")
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest before destructive restore mutation")
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != "" {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
}

func TestRestoreFreeDiskPreflightAllowsSufficientSpace(t *testing.T) {
	sourceManifest, wantSQL, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	storageParent := filepath.Clean(filepath.Dir(storageDir))
	restorePreflightChecked := false
	restoreBackupDiskFreeBytes(t, func(path string) (uint64, error) {
		if filepath.Clean(path) == storageParent {
			restorePreflightChecked = true
		}
		return 2 * bytesPerMiB, nil
	})

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if !restorePreflightChecked {
		t.Fatal("expected restore staging free-space preflight")
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services", "service_health", "db_ping"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != wantSQL {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func TestRestoreArchiveValidationFailureBeforeSnapshotOrMutation(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	replaceRestoreSourceFilesArchive(t, sourceManifest, []archiveTestEntry{
		{name: "../escape.txt", body: "bad\n"},
	})
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if !strings.Contains(err.Error(), "escapes archive root") {
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

func TestRestoreStagingValidationFailureBeforeDestructiveRestoreMutation(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}
	oldValidate := restoreValidateStorageTree
	restoreValidateStorageTree = func(path string) error {
		if path != storageDir {
			return errf("simulated staging validation failure")
		}
		return oldValidate(path)
	}
	defer func() {
		restoreValidateStorageTree = oldValidate
	}()

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if !strings.Contains(err.Error(), "files restore staging is invalid") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != "" {
		t.Fatalf("unexpected restore db body before prepared files succeeded: %q", rt.restoreDBBody)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
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

func TestRestoreFailureReleasesLock(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, _ := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		restoreDBErr:   errf("restore db failed"),
	}

	_, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	assertScopeOperationLockAvailable(t, cfg.ProjectDir, cfg.Scope)
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services"); err != nil {
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "start_services"); err != nil {
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "start_services"); err != nil {
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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services", "service_health", "db_ping"); err != nil {
		t.Fatal(err)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func TestRestoreServiceHealthFailureFailsBeforeDBPing(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		healthErrors: map[int]error{
			2: errf(`service "espocrm" health is "unhealthy" (want "healthy")`),
		},
	}

	oldSleep := serviceHealthSleep
	serviceHealthSleep = func(context.Context, time.Duration) error {
		return context.DeadlineExceeded
	}
	defer func() {
		serviceHealthSleep = oldSleep
	}()

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "restore post-check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), `service "espocrm" health is "unhealthy"`) {
		t.Fatalf("expected service health detail, got %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func TestRestoreHealthWaitCancellationFailsWithoutHang(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
		healthErrors: map[int]error{
			2: &runtime.ServiceHealthError{
				Service:   "espocrm",
				State:     "running",
				Health:    "starting",
				Message:   `service "espocrm" health is "starting" (want "healthy")`,
				Retryable: true,
			},
		},
	}

	oldSleep := serviceHealthSleep
	serviceHealthSleep = func(waitCtx context.Context, _ time.Duration) error {
		cancel()
		<-waitCtx.Done()
		return waitCtx.Err()
	}
	defer func() {
		serviceHealthSleep = oldSleep
	}()

	result, err := Restore(ctx, cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation in error chain, got %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	if len(rt.healthContextErrs) != 2 || rt.healthContextErrs[0] != nil || rt.healthContextErrs[1] != nil {
		t.Fatalf("expected active health context, got %v", rt.healthContextErrs)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func TestRestoreStagingExtractionFailureBeforeDestructiveRestoreMutation(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)

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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != "" {
		t.Fatalf("unexpected restore db body before prepared files succeeded: %q", rt.restoreDBBody)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
}

func TestRestoreOwnershipFailureBeforeDestructiveRestoreMutation(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}
	oldApply := restoreApplyOwnership
	oldRename := restoreRenamePath
	restoreApplyOwnership = func(root string, uid, gid int) error {
		if root == storageDir {
			t.Fatalf("unexpected ownership root: %s", root)
		}
		if filepath.Dir(root) != filepath.Dir(storageDir) {
			t.Fatalf("expected staging next to storage: staging=%s storage=%s", root, storageDir)
		}
		if _, err := os.Stat(filepath.Join(root, "restored.txt")); err != nil {
			t.Fatalf("expected staged file before ownership phase: %v", err)
		}
		if _, err := os.Stat(filepath.Join(storageDir, "old.txt")); err != nil {
			t.Fatalf("expected old target before ownership phase: %v", err)
		}
		return errf("simulated ownership failure")
	}
	restoreRenamePath = func(oldPath, newPath string) error {
		t.Fatalf("storage switch should not start after ownership failure: %s -> %s", oldPath, newPath)
		return nil
	}
	defer func() {
		restoreApplyOwnership = oldApply
		restoreRenamePath = oldRename
	}()

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "files ownership restore failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != "" {
		t.Fatalf("unexpected restore db body before prepared files succeeded: %q", rt.restoreDBBody)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
}

func TestRestoreFilesPostCheckFailureAfterStorageSwitchStillReturnsSnapshotManifest(t *testing.T) {
	sourceManifest, wantSQL, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)

	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}
	oldValidate := restoreValidateStorageTree
	restoreValidateStorageTree = func(path string) error {
		if path == storageDir {
			return errf("simulated final files post-check failure")
		}
		return oldValidate(path)
	}
	defer func() {
		restoreValidateStorageTree = oldValidate
	}()

	result, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime())
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "files restore post-check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("expected snapshot manifest")
	}
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != wantSQL {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
	assertNoRollbackDirs(t, storageDir)
}

func TestRestoreSuccessCreatesSnapshotBeforeMutation(t *testing.T) {
	sourceManifest, wantSQL, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}
	oldRename := restoreRenamePath
	var firstSwitchRenameChecked bool
	restoreRenamePath = func(oldPath, newPath string) error {
		if !firstSwitchRenameChecked {
			firstSwitchRenameChecked = true
			if rt.restoreDBBody != wantSQL {
				t.Fatalf("storage switch started before successful database import: restore body %q", rt.restoreDBBody)
			}
		}
		return oldRename(oldPath, newPath)
	}
	defer func() {
		restoreRenamePath = oldRename
	}()

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
	if err := rt.requireCalls("compose_config", "stop_services", "service_stopped", "dump_database", "start_services", "service_health", "stop_services", "up_service", "reset_database", "restore_database", "start_services", "service_health", "db_ping"); err != nil {
		t.Fatal(err)
	}
	if rt.restoreDBBody != wantSQL {
		t.Fatalf("unexpected restore db body: %q", rt.restoreDBBody)
	}
	if !firstSwitchRenameChecked {
		t.Fatal("expected storage switch after database import")
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func TestRestoreSnapshotSkipsRetention(t *testing.T) {
	sourceManifest, _, _ := writeRestoreSourceBackupSet(t)
	cfg, storageDir := restoreTargetConfig(t)
	oldResult, err := Backup(context.Background(), cfg, &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table old_snapshot(id int);\n"),
	}, time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("setup target backup failed: %v", err)
	}
	rt := &fakeRestoreRuntime{
		snapshotDBDump: gzipBytes(t, "create table snapshot(id int);\n"),
	}

	if _, err := Restore(context.Background(), cfg, sourceManifest, rt, restoreTestTime()); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	assertBackupSetPresent(t, oldResult)
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
}

func TestRestoreFilesBackupUnsafeArchiveDoesNotClearStorage(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchiveEntries(t, archivePath, []restoreArchiveEntry{
		{name: "restored.txt", body: "restored\n"},
		{name: "../escape.txt", body: "bad\n"},
	})

	_, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
}

func TestRestoreFilesBackupRejectsMaliciousArchiveBeforeStaging(t *testing.T) {
	tests := []struct {
		name      string
		entries   []archiveTestEntry
		configure func(t *testing.T)
		want      string
	}{
		{
			name:    "duplicated entry",
			entries: []archiveTestEntry{{name: "restored.txt", body: "first\n"}, {name: "restored.txt", body: "second\n"}},
			want:    "duplicated",
		},
		{
			name:    "directory file collision",
			entries: []archiveTestEntry{{name: "nested/child.txt", body: "child\n"}, {name: "nested", body: "file\n"}},
			want:    "collides with directory",
		},
		{
			name:    "world writable mode",
			entries: []archiveTestEntry{{name: "restored.txt", body: "restored\n", mode: 0o777}},
			want:    "world-writable",
		},
		{
			name: "expanded size limit",
			entries: []archiveTestEntry{{
				name: "huge.txt",
				body: "12345",
			}},
			configure: func(t *testing.T) {
				restoreFilesArchiveLimits(t, defaultFilesArchiveMaxEntries, 4)
			},
			want: "expanded size exceeds limit",
		},
		{
			name:    "empty archive",
			entries: nil,
			want:    "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.configure != nil {
				tt.configure(t)
			}
			cfg, storageDir := restoreTargetConfig(t)
			probe := useRestoreStagingProbe(t, storageDir)
			archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
			writeTarGzArchiveEntries(t, archivePath, tt.entries)

			_, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
			assertVerifyErrorKind(t, err, ErrorKindArchive)
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q in error, got %v", tt.want, err)
			}
			if len(probe.created) != 0 {
				t.Fatalf("staging should not be created before archive validation, got %v", probe.created)
			}
			assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
			assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
			probe.assertClean(t)
		})
	}
}

func TestRestoreFilesBackupBrokenArchiveDoesNotClearStorage(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeBrokenRestoreFilesArchive(t, archivePath, "restored.txt", "restored\n")

	_, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
}

func TestRestoreFilesBackupTargetSymlinkIsNotFollowedBySwitch(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
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

	prepared, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	if err != nil {
		t.Fatalf("prepareRestoreFilesBackup failed: %v", err)
	}
	if err := commitRestoreFilesBackup(context.Background(), prepared, storageDir); err != nil {
		t.Fatalf("commitRestoreFilesBackup failed: %v", err)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatalf("cleanup prepared staging: %v", err)
	}
	assertFileContains(t, linkTarget, "outside\n")
	assertNoFile(t, linkPath)
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
	probe.assertClean(t)
}

func TestRestoreFilesBackupStagingExtractionFailureDoesNotClearStorage(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
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

	_, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "files staging extraction failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	probe.assertClean(t)
}

func TestDefaultRestoreStagingDirCreatedNextToTarget(t *testing.T) {
	_, storageDir := restoreTargetConfig(t)

	stagingDir, err := defaultCreateRestoreStagingDir(storageDir)
	if err != nil {
		t.Fatalf("defaultCreateRestoreStagingDir failed: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	if filepath.Dir(stagingDir) != filepath.Dir(storageDir) {
		t.Fatalf("expected staging next to storage: staging=%s storage=%s", stagingDir, storageDir)
	}
	if stagingDir == storageDir {
		t.Fatal("staging must differ from storage")
	}
}

func TestPrepareRestoreFilesBackupDoesNotSwitchStorage(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchive(t, archivePath, map[string]string{"restored.txt": "restored\n"})

	prepared, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	if err != nil {
		t.Fatalf("prepareRestoreFilesBackup failed: %v", err)
	}
	if prepared == nil || prepared.committed {
		t.Fatalf("unexpected prepared handle: %#v", prepared)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	assertFileContains(t, filepath.Join(prepared.stagingDir, "restored.txt"), "restored\n")

	if err := prepared.Cleanup(); err != nil {
		t.Fatalf("cleanup prepared staging: %v", err)
	}
	probe.assertClean(t)
}

func TestRestoreFilesBackupValidArchiveStillWorks(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchive(t, archivePath, map[string]string{
		"restored.txt":     "restored\n",
		"nested/child.txt": "child\n",
	})
	oldRemoveAll := restoreRemoveAll
	restoreRemoveAll = func(path string) error {
		if path == storageDir {
			t.Fatalf("staging cleanup must not remove active restored storage: %s", path)
		}
		return oldRemoveAll(path)
	}
	defer func() {
		restoreRemoveAll = oldRemoveAll
	}()

	prepared, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	if err != nil {
		t.Fatalf("prepareRestoreFilesBackup failed: %v", err)
	}
	if err := commitRestoreFilesBackup(context.Background(), prepared, storageDir); err != nil {
		t.Fatalf("commitRestoreFilesBackup failed: %v", err)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatalf("cleanup prepared staging: %v", err)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
	assertFileContains(t, filepath.Join(storageDir, "nested", "child.txt"), "child\n")
	probe.assertClean(t)
}

func TestRestoreFilesBackupTargetMoveFailureLeavesOldStorage(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchive(t, archivePath, map[string]string{"restored.txt": "restored\n"})

	oldRename := restoreRenamePath
	renameCount := 0
	restoreRenamePath = func(oldPath, newPath string) error {
		renameCount++
		if renameCount == 1 {
			return errf("simulated target move failure")
		}
		return oldRename(oldPath, newPath)
	}
	defer func() {
		restoreRenamePath = oldRename
	}()

	prepared, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	if err != nil {
		t.Fatalf("prepareRestoreFilesBackup failed: %v", err)
	}
	err = commitRestoreFilesBackup(context.Background(), prepared, storageDir)
	if cleanupErr := prepared.Cleanup(); cleanupErr != nil {
		t.Fatalf("cleanup prepared staging: %v", cleanupErr)
	}
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "files switch failed before target switch") {
		t.Fatalf("unexpected error: %v", err)
	}
	if renameCount != 1 {
		t.Fatalf("expected only the target move to run; got %d renames", renameCount)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	assertNoRollbackDirs(t, storageDir)
	probe.assertClean(t)
}

func TestRestoreFilesBackupSwitchFailureRollsBackOldStorage(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchive(t, archivePath, map[string]string{"restored.txt": "restored\n"})

	oldRename := restoreRenamePath
	renameCount := 0
	restoreRenamePath = func(oldPath, newPath string) error {
		renameCount++
		if renameCount == 2 {
			return errf("simulated target switch failure")
		}
		return oldRename(oldPath, newPath)
	}
	defer func() {
		restoreRenamePath = oldRename
	}()

	prepared, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	if err != nil {
		t.Fatalf("prepareRestoreFilesBackup failed: %v", err)
	}
	err = commitRestoreFilesBackup(context.Background(), prepared, storageDir)
	if cleanupErr := prepared.Cleanup(); cleanupErr != nil {
		t.Fatalf("cleanup prepared staging: %v", cleanupErr)
	}
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "files switch failed during target switch") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "rolled back current storage") {
		t.Fatalf("expected rollback detail, got %v", err)
	}
	if renameCount != 3 {
		t.Fatalf("expected target move, failed switch, and rollback rename; got %d renames", renameCount)
	}
	assertFileContains(t, filepath.Join(storageDir, "old.txt"), "old\n")
	assertNoFile(t, filepath.Join(storageDir, "restored.txt"))
	assertNoRollbackDirs(t, storageDir)
	probe.assertClean(t)
}

func TestRestoreFilesBackupSwitchCleanupFailureLeavesRestoredStorageAndRollback(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchive(t, archivePath, map[string]string{"restored.txt": "restored\n"})

	oldRemoveAll := restoreRemoveAll
	var rollbackDir string
	restoreRemoveAll = func(path string) error {
		if strings.HasPrefix(filepath.Base(path), strings.TrimSuffix(restoreRollbackDirPattern, "*")) {
			rollbackDir = path
			return errf("simulated rollback cleanup failure")
		}
		return oldRemoveAll(path)
	}
	defer func() {
		restoreRemoveAll = oldRemoveAll
		if rollbackDir != "" {
			_ = os.RemoveAll(rollbackDir)
		}
	}()

	prepared, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	if err != nil {
		t.Fatalf("prepareRestoreFilesBackup failed: %v", err)
	}
	err = commitRestoreFilesBackup(context.Background(), prepared, storageDir)
	if cleanupErr := prepared.Cleanup(); cleanupErr != nil {
		t.Fatalf("cleanup prepared staging: %v", cleanupErr)
	}
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "files switch cleanup failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "rollback remains") {
		t.Fatalf("expected rollback detail, got %v", err)
	}
	assertNoFile(t, filepath.Join(storageDir, "old.txt"))
	assertFileContains(t, filepath.Join(storageDir, "restored.txt"), "restored\n")
	if rollbackDir == "" {
		t.Fatal("expected rollback cleanup failure")
	}
	assertFileContains(t, filepath.Join(rollbackDir, "old.txt"), "old\n")
	probe.assertClean(t)
}

func TestRestoreFilesBackupAppliesOwnershipBeforeSwitchAndFinalPostCheck(t *testing.T) {
	cfg, storageDir := restoreTargetConfig(t)
	probe := useRestoreStagingProbe(t, storageDir)
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	writeRestoreFilesArchive(t, archivePath, map[string]string{"restored.txt": "restored\n"})

	oldValidate := restoreValidateStorageTree
	oldApply := restoreApplyOwnership
	steps := []string{}
	restoreValidateStorageTree = func(path string) error {
		switch path {
		case storageDir:
			steps = append(steps, "validate_target")
		default:
			steps = append(steps, "validate_staging")
		}
		return oldValidate(path)
	}
	restoreApplyOwnership = func(root string, uid, gid int) error {
		steps = append(steps, "apply_ownership")
		if root == storageDir {
			t.Fatalf("unexpected ownership root: %s", root)
		}
		if filepath.Dir(root) != filepath.Dir(storageDir) {
			t.Fatalf("expected staging next to storage: staging=%s storage=%s", root, storageDir)
		}
		if uid != cfg.RuntimeUID || gid != cfg.RuntimeGID {
			t.Fatalf("unexpected ownership target: %d:%d", uid, gid)
		}
		if _, err := os.Stat(filepath.Join(root, "restored.txt")); err != nil {
			t.Fatalf("expected staged file before ownership phase: %v", err)
		}
		if _, err := os.Stat(filepath.Join(storageDir, "old.txt")); err != nil {
			t.Fatalf("expected old target before ownership phase: %v", err)
		}
		if _, err := os.Stat(filepath.Join(storageDir, "restored.txt")); !os.IsNotExist(err) {
			t.Fatalf("expected restored file to stay out of target before switch, got %v", err)
		}
		return nil
	}
	defer func() {
		restoreValidateStorageTree = oldValidate
		restoreApplyOwnership = oldApply
	}()

	prepared, err := prepareRestoreFilesBackup(context.Background(), archivePath, storageDir, cfg.RuntimeUID, cfg.RuntimeGID)
	if err != nil {
		t.Fatalf("prepareRestoreFilesBackup failed: %v", err)
	}
	if err := commitRestoreFilesBackup(context.Background(), prepared, storageDir); err != nil {
		t.Fatalf("commitRestoreFilesBackup failed: %v", err)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatalf("cleanup prepared staging: %v", err)
	}
	if strings.Join(steps, ",") != "validate_staging,apply_ownership,validate_target" {
		t.Fatalf("unexpected restore steps: %v", steps)
	}
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

func TestApplyRestoreOwnershipAppliesToDirectoriesAndRegularFiles(t *testing.T) {
	root := t.TempDir()
	nestedDir := filepath.Join(root, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(root, "root.txt"):       "root\n",
		filepath.Join(nestedDir, "child.txt"): "child\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ops := restoreOwnershipOps{
		lstat: func(path string) (os.FileInfo, error) {
			info, err := os.Lstat(path)
			if err != nil {
				return nil, err
			}
			return ownershipFileInfo{
				FileInfo: info,
				sys: &syscall.Stat_t{
					Uid: 123,
					Gid: 456,
				},
			}, nil
		},
	}
	var paths []string
	ops.lchown = func(path string, uid, gid int) error {
		paths = append(paths, fmt.Sprintf("%s:%d:%d", filepath.ToSlash(path), uid, gid))
		return nil
	}

	if err := applyRestoreOwnershipWithOps(root, 33, 44, ops); err != nil {
		t.Fatalf("applyRestoreOwnershipWithOps failed: %v", err)
	}
	want := []string{
		filepath.ToSlash(root) + ":33:44",
		filepath.ToSlash(filepath.Join(root, "nested")) + ":33:44",
		filepath.ToSlash(filepath.Join(root, "nested", "child.txt")) + ":33:44",
		filepath.ToSlash(filepath.Join(root, "root.txt")) + ":33:44",
	}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected chown calls: got %v want %v", paths, want)
	}
}

func TestApplyRestoreOwnershipRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	linkTarget := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(linkTarget, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkTarget, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	err := applyRestoreOwnershipWithOps(root, 33, 33, restoreOwnershipOps{
		lstat:  os.Lstat,
		lchown: func(string, int, int) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink ownership error, got %v", err)
	}
}

func TestApplyRestoreOwnershipChownFailureIncludesPath(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "restored.txt")
	if err := os.WriteFile(filePath, []byte("restored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := applyRestoreOwnershipWithOps(root, 33, 44, restoreOwnershipOps{
		lstat: func(path string) (os.FileInfo, error) {
			info, statErr := os.Lstat(path)
			if statErr != nil {
				return nil, statErr
			}
			return ownershipFileInfo{
				FileInfo: info,
				sys: &syscall.Stat_t{
					Uid: 1,
					Gid: 2,
				},
			}, nil
		},
		lchown: func(path string, uid, gid int) error {
			if path == filePath {
				return errf("operation not permitted")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), filepath.ToSlash(filePath)) {
		t.Fatalf("expected path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "uid=33 gid=44") {
		t.Fatalf("expected target ownership in error, got %v", err)
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

func replaceRestoreSourceFilesArchive(t *testing.T, manifestPath string, entries []archiveTestEntry) {
	t.Helper()

	loadedManifest, err := manifestpkg.Load(manifestPath)
	if err != nil {
		t.Fatalf("Load manifest failed: %v", err)
	}
	paths, err := manifestpkg.ResolveArtifacts(manifestPath, loadedManifest)
	if err != nil {
		t.Fatalf("ResolveArtifacts failed: %v", err)
	}
	writeTarGzArchiveEntries(t, paths.FilesPath, entries)
	rewriteSidecar(t, paths.FilesPath)
	loadedManifest.Checksums.FilesBackup = sha256OfFile(t, paths.FilesPath)
	writeManifest(t, manifestPath, loadedManifest)
}

func writeVersionOneRestoreSourceBackupSet(t *testing.T) (manifestPath, dbSQL, storageDir string) {
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
		"scope":      "prod",
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
	healthErrors      map[int]error
	restoreDBErr      error
	resetDBErr        error
	dbPingErr         error
	cancel            context.CancelFunc
	cancelOnStopCount int
	calls             []string
	stopCount         int
	startCount        int
	upCount           int
	healthCount       int
	restoreDBBody     string
	startContextErrs  []error
	healthContextErrs []error
}

func (f *fakeRestoreRuntime) ComposeConfig(_ context.Context, _ runtime.Target) error {
	f.calls = append(f.calls, "compose_config")
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

func (f *fakeRestoreRuntime) RequireStoppedServices(ctx context.Context, _ runtime.Target, _ []string) error {
	f.calls = append(f.calls, "service_stopped")
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
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

func (f *fakeRestoreRuntime) RequireHealthyServices(ctx context.Context, _ runtime.Target, _ []string) error {
	f.calls = append(f.calls, "service_health")
	f.healthContextErrs = append(f.healthContextErrs, ctx.Err())
	f.healthCount++
	return indexedError(f.healthErrors, f.healthCount)
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

func assertNoRollbackDirs(t *testing.T, storageDir string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(storageDir), restoreRollbackDirPattern))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("unexpected rollback dirs left behind: %v", matches)
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

type ownershipFileInfo struct {
	os.FileInfo
	sys any
}

func (i ownershipFileInfo) Sys() any {
	return i.sys
}

func useRestoreStagingProbe(t *testing.T, storageDir string) *restoreStagingProbe {
	t.Helper()

	probe := &restoreStagingProbe{root: filepath.Dir(storageDir)}
	oldCreate := createRestoreStagingDir
	createRestoreStagingDir = func(gotStorageDir string) (string, error) {
		if gotStorageDir != storageDir {
			t.Fatalf("unexpected storage dir for staging: got %s want %s", gotStorageDir, storageDir)
		}
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
