package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	manifestpkg "github.com/lazuale/espocrm-ops/internal/manifest"
	"github.com/lazuale/espocrm-ops/internal/ops"
)

func TestRestoreCLIJSONSuccess(t *testing.T) {
	manifestPath, wantSQL := writeVerifiedRestoreBackupSet(t)

	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"ESPOCRM_IMAGE=espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"MARIADB_IMAGE=mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=" + currentRuntimeUIDString(),
		"ESPO_RUNTIME_GID=" + currentRuntimeGIDString(),
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	stdinLogPath := filepath.Join(t.TempDir(), "restore-db.sql")
	fakeDockerRoot := prependRestoreFakeDocker(t)
	writeCLIFakeDockerControl(t, fakeDockerRoot, "restore-stdin-log", stdinLogPath)

	oldNow := restoreNow
	restoreNow = func() time.Time {
		return time.Date(2026, 4, 24, 18, 0, 0, 0, time.UTC)
	}
	defer func() { restoreNow = oldNow }()

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "restore" {
		t.Fatalf("unexpected command: %s", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=true")
	}
	if message := requireJSONString(t, obj, "message"); message != "restore completed" {
		t.Fatalf("unexpected message: %s", message)
	}
	if gotManifest := requireJSONString(t, obj, "result", "manifest"); gotManifest != manifestPath {
		t.Fatalf("unexpected manifest: %s", gotManifest)
	}
	snapshotManifest := requireJSONString(t, obj, "result", "snapshot_manifest")
	if _, err := ops.VerifyBackup(context.Background(), snapshotManifest); err != nil {
		t.Fatalf("snapshot manifest did not verify: %v", err)
	}
	raw, err := os.ReadFile(stdinLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if body := string(raw); body != wantSQL {
		t.Fatalf("unexpected restore db body: %q", body)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(storageDir, "restored.txt")); err != nil {
		t.Fatalf("expected restored file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(storageDir, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old file removed, got %v", err)
	}
}

func TestRestoreCLIJSONFailureForInvalidManifest(t *testing.T) {
	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"ESPOCRM_IMAGE=espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"MARIADB_IMAGE=mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=" + currentRuntimeUIDString(),
		"ESPO_RUNTIME_GID=" + currentRuntimeGIDString(),
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	missingManifest := filepath.Join(projectDir, "missing.manifest.json")
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", missingManifest}, stdout, stderr)
	if exitCode != exitManifest {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitManifest, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "restore" {
		t.Fatalf("unexpected command: %s", command)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if message := requireJSONString(t, obj, "message"); message != "restore failed" {
		t.Fatalf("unexpected message: %s", message)
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "manifest" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if gotManifest := requireJSONString(t, obj, "result", "manifest"); gotManifest != missingManifest {
		t.Fatalf("unexpected manifest: %s", gotManifest)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRestoreCLIJSONRejectsManifestFromDifferentScope(t *testing.T) {
	manifestPath, _ := writeVerifiedScopedBackupSet(t, "dev")

	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"ESPOCRM_IMAGE=espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"MARIADB_IMAGE=mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=" + currentRuntimeUIDString(),
		"ESPO_RUNTIME_GID=" + currentRuntimeGIDString(),
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != exitUsage {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitUsage, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "usage" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if errMessage := requireJSONString(t, obj, "error", "message"); !strings.Contains(errMessage, "manifest scope is invalid for requested operation") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRestoreCLIJSONHealthFailureIsRuntimeFailure(t *testing.T) {
	manifestPath, _ := writeVerifiedRestoreBackupSet(t)

	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"ESPOCRM_IMAGE=espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"MARIADB_IMAGE=mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=" + currentRuntimeUIDString(),
		"ESPO_RUNTIME_GID=" + currentRuntimeGIDString(),
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	fakeDockerRoot := prependRestoreFakeDocker(t)
	writeCLIFakeDockerControl(t, fakeDockerRoot, "restore-ps-output", `[{"Service":"db","State":"running","Health":"healthy"},{"Service":"espocrm","State":"running","Health":"healthy"},{"Service":"espocrm-daemon","State":"exited","Health":"unhealthy"},{"Service":"espocrm-websocket","State":"running","Health":"healthy"}]`)

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != exitRuntime {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitRuntime, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "runtime" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if errMessage := requireJSONString(t, obj, "error", "message"); !strings.Contains(errMessage, "restore post-check failed") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if strings.Contains(requireJSONString(t, obj, "error", "message"), "root-secret") {
		t.Fatal("health failure must not leak secrets")
	}
	requireJSONSnapshotManifest(t, obj)
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRestoreCLIJSONResetFailureRedactsRootPassword(t *testing.T) {
	manifestPath, _ := writeVerifiedRestoreBackupSet(t)

	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"ESPOCRM_IMAGE=espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"MARIADB_IMAGE=mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=" + currentRuntimeUIDString(),
		"ESPO_RUNTIME_GID=" + currentRuntimeGIDString(),
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	fakeDockerRoot := prependRestoreFakeDocker(t)
	writeCLIFakeDockerControl(t, fakeDockerRoot, "restore-fail-reset", "1")
	writeCLIFakeDockerControl(t, fakeDockerRoot, "restore-fail-message", "reset failed with MYSQL_PWD=root-secret and root-secret")

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != exitRuntime {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitRuntime, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	errMessage := requireJSONString(t, obj, "error", "message")
	if strings.Contains(errMessage, "root-secret") {
		t.Fatalf("json error leaked db root password: %s", errMessage)
	}
	if !strings.Contains(errMessage, "MYSQL_PWD=<redacted>") {
		t.Fatalf("expected redacted json error message, got %s", errMessage)
	}
	requireJSONSnapshotManifest(t, obj)
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRestoreCLIJSONImportFailureIncludesSnapshotManifest(t *testing.T) {
	manifestPath, _ := writeVerifiedRestoreBackupSet(t)
	projectDir, _ := writeRestoreCLIProject(t)

	fakeDockerRoot := prependRestoreFakeDocker(t)
	writeCLIFakeDockerControl(t, fakeDockerRoot, "restore-fail-import", "1")

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != exitRuntime {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitRuntime, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "runtime" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	errMessage := requireJSONString(t, obj, "error", "message")
	if !strings.Contains(errMessage, "database restore failed") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	requireJSONSnapshotManifest(t, obj)
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRestoreCLIJSONFileRestoreFailureIncludesSnapshotManifest(t *testing.T) {
	manifestPath, wantSQL := writeVerifiedRestoreBackupSetWithFiles(t, map[string]string{
		strings.Repeat("a", 300) + ".txt": "restored\n",
	})
	projectDir, storageDir := writeRestoreCLIProject(t)

	stdinLogPath := filepath.Join(t.TempDir(), "restore-db.sql")
	fakeDockerRoot := prependRestoreFakeDocker(t)
	writeCLIFakeDockerControl(t, fakeDockerRoot, "restore-stdin-log", stdinLogPath)

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != exitIO {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitIO, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "io" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	errMessage := requireJSONString(t, obj, "error", "message")
	if !strings.Contains(errMessage, "files staging extraction failed") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	requireJSONSnapshotManifest(t, obj)
	raw, err := os.ReadFile(stdinLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if body := string(raw); body != wantSQL {
		t.Fatalf("unexpected restore db body: %q", body)
	}
	if _, err := os.Stat(filepath.Join(storageDir, "old.txt")); err != nil {
		t.Fatalf("expected old file to remain after staging failure: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRestoreCLIJSONStorageSwitchFailureIncludesSnapshotManifest(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("storage switch cleanup failure test requires non-root operator")
	}

	manifestPath, _ := writeVerifiedRestoreBackupSet(t)
	projectDir, storageDir := writeRestoreCLIProject(t)
	nestedDir := filepath.Join(storageDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "old-child.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer cleanupRestoreRollbackDirs(t, storageDir)

	fakeDockerRoot := prependRestoreFakeDocker(t)
	writeCLIFakeDockerControl(t, fakeDockerRoot, "restore-chmod-old-nested-after-import", "1")

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != exitIO {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitIO, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "io" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	errMessage := requireJSONString(t, obj, "error", "message")
	if !strings.Contains(errMessage, "files switch cleanup failed") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	requireJSONSnapshotManifest(t, obj)
	if _, err := os.Stat(filepath.Join(storageDir, "restored.txt")); err != nil {
		t.Fatalf("expected restored storage to be active after switch cleanup failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(storageDir, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old file removed from active storage, got %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRestoreCLIJSONStartFailureIncludesSnapshotManifest(t *testing.T) {
	manifestPath, _ := writeVerifiedRestoreBackupSet(t)
	projectDir, storageDir := writeRestoreCLIProject(t)

	fakeDockerRoot := prependRestoreFakeDocker(t)
	writeCLIFakeDockerControl(t, fakeDockerRoot, "restore-fail-start-on-return", "1")

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != exitRuntime {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitRuntime, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "runtime" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	errMessage := requireJSONString(t, obj, "error", "message")
	if !strings.Contains(errMessage, "failed to return app services") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	requireJSONSnapshotManifest(t, obj)
	if _, err := os.Stat(filepath.Join(storageDir, "restored.txt")); err != nil {
		t.Fatalf("expected files restored before start failure: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRestoreCLIJSONUsageFailureWhenRuntimeOwnershipMissing(t *testing.T) {
	manifestPath, _ := writeVerifiedRestoreBackupSet(t)

	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"ESPOCRM_IMAGE=espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"MARIADB_IMAGE=mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != exitUsage {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitUsage, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "usage" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if errMessage := requireJSONString(t, obj, "error", "message"); !strings.Contains(errMessage, "ESPO_RUNTIME_UID and ESPO_RUNTIME_GID are required") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRestoreCLIJSONOwnershipFailureDoesNotRevealSecrets(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("ownership failure test requires non-root operator")
	}

	manifestPath, _ := writeVerifiedRestoreBackupSet(t)

	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"ESPOCRM_IMAGE=espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"MARIADB_IMAGE=mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=" + strconv.Itoa(os.Geteuid()+1),
		"ESPO_RUNTIME_GID=" + strconv.Itoa(os.Getegid()+1),
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	prependRestoreFakeDocker(t)
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != exitIO {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitIO, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "io" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	errMessage := requireJSONString(t, obj, "error", "message")
	if !strings.Contains(errMessage, "files ownership restore failed") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if strings.Contains(errMessage, "root-secret") || strings.Contains(errMessage, "db-secret") {
		t.Fatalf("ownership error leaked secret: %s", errMessage)
	}
	requireJSONSnapshotManifest(t, obj)
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRestoreCLIJSONVersionOneManifestFailsClosed(t *testing.T) {
	manifestPath, _ := writeVersionOneRestoreBackupSet(t)

	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"ESPOCRM_IMAGE=espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"MARIADB_IMAGE=mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=" + currentRuntimeUIDString(),
		"ESPO_RUNTIME_GID=" + currentRuntimeGIDString(),
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	prependRestoreFakeDocker(t)
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"restore", "--scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != exitManifest {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitManifest, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "manifest" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	errMessage := requireJSONString(t, obj, "error", "message")
	if !strings.Contains(errMessage, "manifest version 1") || !strings.Contains(errMessage, "required") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func writeRestoreCLIProject(t *testing.T) (projectDir, storageDir string) {
	t.Helper()

	projectDir = t.TempDir()
	storageDir = filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"ESPOCRM_IMAGE=espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"MARIADB_IMAGE=mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=" + currentRuntimeUIDString(),
		"ESPO_RUNTIME_GID=" + currentRuntimeGIDString(),
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	return projectDir, storageDir
}

func requireJSONSnapshotManifest(t *testing.T, obj map[string]any) string {
	t.Helper()

	snapshotManifest := requireJSONString(t, obj, "result", "snapshot_manifest")
	if _, err := ops.VerifyBackup(context.Background(), snapshotManifest); err != nil {
		t.Fatalf("snapshot manifest did not verify: %v", err)
	}
	return snapshotManifest
}

func cleanupRestoreRollbackDirs(t *testing.T, storageDir string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(storageDir), "espops-restore-rollback-*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range matches {
		_ = filepath.WalkDir(match, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
		_ = os.RemoveAll(match)
	}
}

func writeVerifiedRestoreBackupSet(t *testing.T) (manifestPath, dbSQL string) {
	t.Helper()

	return writeVerifiedRestoreBackupSetWithFiles(t, map[string]string{"restored.txt": "restored\n"})
}

func writeVerifiedRestoreBackupSetWithFiles(t *testing.T, files map[string]string) (manifestPath, dbSQL string) {
	t.Helper()

	root := t.TempDir()
	dbPath := filepath.Join(root, "db", "restore-source.sql.gz")
	filesPath := filepath.Join(root, "files", "restore-source.tar.gz")
	manifestPath = filepath.Join(root, "manifests", "restore-source.manifest.json")

	for _, dir := range []string{filepath.Dir(dbPath), filepath.Dir(filesPath), filepath.Dir(manifestPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dbSQL = "create table restored(id int);\n"
	writeGzipFile(t, dbPath, []byte(dbSQL))
	writeTarGzFile(t, filesPath, files)
	rewriteSidecar(t, dbPath)
	rewriteSidecar(t, filesPath)

	raw, err := json.MarshalIndent(map[string]any{
		"version":    2,
		"scope":      "prod",
		"created_at": "2026-04-24T18:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(dbPath),
			"files_backup": filepath.Base(filesPath),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
		"runtime": map[string]any{
			"espo_crm_image":     "espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"mariadb_image":      "mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"db_name":            "espocrm",
			"db_service":         "db",
			"app_services":       []string{"espocrm", "espocrm-daemon", "espocrm-websocket"},
			"backup_name_prefix": "test-backup",
			"storage_contract":   manifestpkg.StorageContractEspoCRMFullStorageV1,
		},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	return manifestPath, dbSQL
}

func writeVersionOneRestoreBackupSet(t *testing.T) (manifestPath, dbSQL string) {
	t.Helper()

	root := t.TempDir()
	dbPath := filepath.Join(root, "db", "restore-source.sql.gz")
	filesPath := filepath.Join(root, "files", "restore-source.tar.gz")
	manifestPath = filepath.Join(root, "manifests", "restore-source.manifest.json")

	for _, dir := range []string{filepath.Dir(dbPath), filepath.Dir(filesPath), filepath.Dir(manifestPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dbSQL = "create table restored(id int);\n"
	writeGzipFile(t, dbPath, []byte(dbSQL))
	writeTarGzFile(t, filesPath, map[string]string{"restored.txt": "restored\n"})
	rewriteSidecar(t, dbPath)
	rewriteSidecar(t, filesPath)

	raw, err := json.MarshalIndent(map[string]any{
		"version":    1,
		"scope":      "prod",
		"created_at": "2026-04-24T18:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(dbPath),
			"files_backup": filepath.Base(filesPath),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	return manifestPath, dbSQL
}

func prependRestoreFakeDocker(t *testing.T) string {
	t.Helper()

	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(scriptPath, []byte(restoreFakeDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return rootDir
}

func currentRuntimeUIDString() string {
	return strconv.Itoa(os.Getuid())
}

func currentRuntimeGIDString() string {
	return strconv.Itoa(os.Getgid())
}

const restoreFakeDockerScript = `#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
fake_root="$(cd -- "$script_dir/.." && pwd)"
project_dir="$(pwd)"

default_ps='[{"Service":"db","State":"running","Health":"healthy"},{"Service":"espocrm","State":"running","Health":"healthy"},{"Service":"espocrm-daemon","State":"running","Health":"healthy"},{"Service":"espocrm-websocket","State":"running","Health":"healthy"}]'
stopped_ps='[{"Service":"db","State":"running","Health":"healthy"}]'

if [[ "${1:-}" != "compose" ]]; then
  printf 'unexpected docker invocation: %s\n' "$*" >&2
  exit 1
fi
shift

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project-directory)
      project_dir="$2"
      shift 2
      ;;
    -f|--env-file)
      shift 2
      ;;
    *)
      break
      ;;
  esac
done

case "${1:-}" in
  config)
    exit 0
    ;;
  ps)
    ps_count=0
    if [[ -f "$fake_root/restore-ps-count" ]]; then
      ps_count="$(cat "$fake_root/restore-ps-count")"
    fi
    ps_count="$((ps_count + 1))"
    printf '%s' "$ps_count" >"$fake_root/restore-ps-count"
    start_count=0
    if [[ -f "$fake_root/start-count" ]]; then
      start_count="$(cat "$fake_root/start-count")"
    fi
    if [[ -f "$fake_root/app-stopped" ]]; then
      printf '%s' "$stopped_ps"
    elif [[ -f "$fake_root/restore-ps-output" && "$start_count" -ge 2 ]]; then
      cat "$fake_root/restore-ps-output"
    else
      printf '%s' "$default_ps"
    fi
    exit 0
    ;;
  stop)
    printf '1' >"$fake_root/app-stopped"
    exit 0
    ;;
  start)
    rm -f "$fake_root/app-stopped"
    start_count=0
    if [[ -f "$fake_root/start-count" ]]; then
      start_count="$(cat "$fake_root/start-count")"
    fi
    if [[ -f "$fake_root/restore-fail-start-on-return" && "$start_count" -ge 1 ]]; then
      printf 'start failed\n' >&2
      exit 1
    fi
    printf '%s' "$((start_count + 1))" >"$fake_root/start-count"
    exit 0
    ;;
  up)
    [[ "${2:-}" == "-d" ]] || exit 1
    exit 0
    ;;
  exec)
    shift
    [[ "${1:-}" == "-T" ]] || exit 1
    shift
    [[ "${1:-}" == "-e" ]] || exit 1
    shift
    [[ "${1:-}" == "MYSQL_PWD" ]] || exit 1
    shift
    [[ "${1:-}" == "db" ]] || exit 1
    shift
    case "${1:-}" in
      mariadb-dump)
        [[ "${MYSQL_PWD:-}" == "db-secret" ]] || exit 1
        printf 'create table snapshot(id int);\n'
        exit 0
        ;;
      mariadb)
        shift
        [[ "${1:-}" == "-u" ]] || exit 1
        shift
        case "${1:-}" in
          root)
            [[ "${MYSQL_PWD:-}" == "root-secret" ]] || exit 1
            shift
            [[ "${1:-}" == "-e" ]] || exit 1
            shift
            if [[ -f "$fake_root/restore-fail-reset" ]]; then
              if [[ -f "$fake_root/restore-fail-message" ]]; then
                cat "$fake_root/restore-fail-message" >&2
                printf '\n' >&2
              else
                printf 'reset failed\n' >&2
              fi
              exit 1
            fi
            [[ "${1:-}" == DROP\ DATABASE\ IF\ EXISTS*CREATE\ DATABASE*CHARACTER\ SET\ utf8mb4\ COLLATE\ utf8mb4_unicode_ci\; ]] || exit 1
            exit 0
            ;;
          espocrm)
            [[ "${MYSQL_PWD:-}" == "db-secret" ]] || exit 1
            shift
            for arg in "$@"; do
              if [[ "$arg" == "-e" ]]; then
                exit 0
              fi
            done
            if [[ -f "$fake_root/restore-stdin-log" ]]; then
              cat >"$(cat "$fake_root/restore-stdin-log")"
            else
              cat >/dev/null
            fi
            if [[ -f "$fake_root/restore-chmod-old-nested-after-import" ]]; then
              [[ -n "$project_dir" ]] || exit 1
              chmod 500 "$project_dir/runtime/prod/espo/nested"
            fi
            if [[ -f "$fake_root/restore-fail-import" ]]; then
              printf 'import failed\n' >&2
              exit 1
            fi
            exit 0
            ;;
        esac
        ;;
    esac
    ;;
esac

printf 'unexpected docker invocation: %s\n' "$*" >&2
exit 1
`
