package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/ops"
)

func TestBackupCLIJSONSuccess(t *testing.T) {
	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo", "data")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
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
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	prependFakeDocker(t)
	oldNow := backupNow
	backupNow = func() time.Time {
		return time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	}
	defer func() { backupNow = oldNow }()

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"backup", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "backup" {
		t.Fatalf("unexpected command: %s", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=true")
	}
	if message := requireJSONString(t, obj, "message"); message != "backup completed" {
		t.Fatalf("unexpected message: %s", message)
	}
	manifestPath := requireJSONString(t, obj, "result", "manifest")
	if got := filepath.Base(manifestPath); !strings.HasPrefix(got, "test-backup_") {
		t.Fatalf("expected prefix-based manifest name, got %s", got)
	}
	if _, err := ops.VerifyBackup(context.Background(), manifestPath); err != nil {
		t.Fatalf("produced backup did not verify: %v", err)
	}
	if requireJSONString(t, obj, "result", "db_backup") == "" {
		t.Fatal("expected db_backup in result")
	}
	if requireJSONString(t, obj, "result", "files_backup") == "" {
		t.Fatal("expected files_backup in result")
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestBackupCLIJSONSuccessWarnsWhenRetentionSkipped(t *testing.T) {
	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo", "data")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
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
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestDir := filepath.Join(projectDir, "backups", "prod", "manifests")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldManifest := filepath.Join(manifestDir, "test-backup_2026-04-10_12-00-00.manifest.json")
	if err := os.WriteFile(oldManifest, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prependFakeDocker(t)
	oldNow := backupNow
	backupNow = func() time.Time {
		return time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	}
	defer func() { backupNow = oldNow }()

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"backup", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=true")
	}
	if errorValue := requireJSONPath(t, obj, "error"); errorValue != nil {
		t.Fatalf("expected error=null, got %#v", errorValue)
	}
	warnings := requireJSONArray(t, obj, "warnings")
	if len(warnings) != 1 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	warning, ok := warnings[0].(string)
	if !ok {
		t.Fatalf("expected string warning, got %#v", warnings[0])
	}
	if !strings.Contains(warning, "retention_skipped: backup retention cleanup blocked") {
		t.Fatalf("unexpected warning: %s", warning)
	}
	manifestPath := requireJSONString(t, obj, "result", "manifest")
	if _, err := ops.VerifyBackup(context.Background(), manifestPath); err != nil {
		t.Fatalf("produced backup did not verify: %v", err)
	}
	if _, err := os.Stat(oldManifest); err != nil {
		t.Fatalf("expected suspicious manifest to remain for operator review: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestBackupCLIJSONFailureForMissingEnv(t *testing.T) {
	projectDir := t.TempDir()
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
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"backup", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
	if exitCode != exitUsage {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitUsage, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "backup" {
		t.Fatalf("unexpected command: %s", command)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if message := requireJSONString(t, obj, "message"); message != "backup failed" {
		t.Fatalf("unexpected message: %s", message)
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "usage" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if errMessage := requireJSONString(t, obj, "error", "message"); !strings.Contains(errMessage, "DB_NAME is required") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestBackupCLIJSONFailureWhenLockBusy(t *testing.T) {
	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo", "data")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
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
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	lock := holdCLIScopeLock(t, projectDir, "prod")
	defer lock.Release(t)

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"backup", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
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
	if !strings.Contains(errMessage, "backup lock failed") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if !strings.Contains(errMessage, cliScopeLockMessage("prod")) {
		t.Fatalf("missing scope lock detail: %s", errMessage)
	}
	if strings.Contains(errMessage, "db-secret") {
		t.Fatalf("lock error leaked secret: %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestBackupCLIJSONFailureWhenBackupNamePrefixMissing(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"ESPOCRM_IMAGE=espocrm/espocrm@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"MARIADB_IMAGE=mariadb@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"backup", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
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
	if errMessage := requireJSONString(t, obj, "error", "message"); !strings.Contains(errMessage, "BACKUP_NAME_PREFIX is required") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
}

func TestBackupCLIJSONFailureWhenDBServiceMissing(t *testing.T) {
	projectDir := t.TempDir()
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
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"backup", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
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
	if errMessage := requireJSONString(t, obj, "error", "message"); !strings.Contains(errMessage, "DB_SERVICE is required") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestBackupCLIJSONFailureWhenAppServicesMissing(t *testing.T) {
	projectDir := t.TempDir()
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
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"backup", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
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
	if errMessage := requireJSONString(t, obj, "error", "message"); !strings.Contains(errMessage, "APP_SERVICES is required") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestBackupCLIJSONLowDiskFailsClosed(t *testing.T) {
	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo", "data")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
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
		"MIN_FREE_DISK_MB=2147483647",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	prependFakeDocker(t)

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"backup", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
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
	if message := requireJSONString(t, obj, "message"); message == "backup completed" {
		t.Fatalf("backup result pretended success: %s", message)
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "io" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if errMessage := requireJSONString(t, obj, "error", "message"); !strings.Contains(errMessage, "backup free disk preflight failed") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}

	manifestPath := requireJSONString(t, obj, "result", "manifest")
	if manifestPath == "" {
		t.Fatal("expected manifest path in failed result")
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("expected no manifest to be created, got %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestBackupCLIJSONRuntimeErrorRedactsPassword(t *testing.T) {
	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo", "data")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
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
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	fakeDockerRoot := prependFakeDocker(t)
	writeCLIFakeDockerControl(t, fakeDockerRoot, "backup-fail-dump", "1")
	writeCLIFakeDockerControl(t, fakeDockerRoot, "backup-fail-message", "dump failed with MYSQL_PWD=db-secret")

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"backup", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
	if exitCode != exitRuntime {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitRuntime, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	errMessage := requireJSONString(t, obj, "error", "message")
	if strings.Contains(errMessage, "db-secret") {
		t.Fatalf("json error leaked db password: %s", errMessage)
	}
	if !strings.Contains(errMessage, "MYSQL_PWD=<redacted>") {
		t.Fatalf("expected redacted json error message, got %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func prependFakeDocker(t *testing.T) string {
	t.Helper()

	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(scriptPath, []byte(fakeDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return rootDir
}

func writeCLIFakeDockerControl(t *testing.T, rootDir, name, value string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(rootDir, name), []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

const fakeDockerScript = `#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
fake_root="$(cd -- "$script_dir/.." && pwd)"
default_ps='[{"Service":"db","State":"running","Health":"healthy"},{"Service":"espocrm","State":"running","Health":"healthy"},{"Service":"espocrm-daemon","State":"running","Health":"healthy"},{"Service":"espocrm-websocket","State":"running","Health":"healthy"}]'
stopped_ps='[{"Service":"db","State":"running","Health":"healthy"}]'

if [[ "${1:-}" != "compose" ]]; then
  printf 'unexpected docker invocation: %s\n' "$*" >&2
  exit 1
fi
shift

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project-directory|-f|--env-file)
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
  stop)
    printf '1' >"$fake_root/app-stopped"
    exit 0
    ;;
  start)
    rm -f "$fake_root/app-stopped"
    exit 0
    ;;
  ps)
    [[ "${2:-}" == "--format" ]] || exit 1
    [[ "${3:-}" == "json" ]] || exit 1
    if [[ -f "$fake_root/app-stopped" ]]; then
      printf '%s' "$stopped_ps"
    elif [[ -f "$fake_root/backup-ps-output" ]]; then
      cat "$fake_root/backup-ps-output"
    else
      printf '%s' "$default_ps"
    fi
    exit 0
    ;;
  exec)
    shift
    if [[ "${1:-}" != "-T" ]]; then
      printf 'expected -T, got %s\n' "${1:-}" >&2
      exit 1
    fi
    shift
    if [[ "${1:-}" != "-e" ]]; then
      printf 'expected -e, got %s\n' "${1:-}" >&2
      exit 1
    fi
    shift
    if [[ "${1:-}" != "MYSQL_PWD" ]]; then
      printf 'unexpected exec env: %s\n' "${1:-}" >&2
      exit 1
    fi
    if [[ "${MYSQL_PWD:-}" != "db-secret" ]]; then
      printf 'unexpected MYSQL_PWD environment\n' >&2
      exit 1
    fi
    shift
    if [[ "${1:-}" != "db" ]]; then
      printf 'unexpected service: %s\n' "${1:-}" >&2
      exit 1
    fi
    shift
    if [[ "${1:-}" != "mariadb-dump" ]]; then
      printf 'unexpected command: %s\n' "${1:-}" >&2
      exit 1
    fi
    if [[ -f "$fake_root/backup-fail-dump" ]]; then
      if [[ -f "$fake_root/backup-fail-message" ]]; then
        cat "$fake_root/backup-fail-message" >&2
        printf '\n' >&2
      else
        printf 'dump failed\n' >&2
      fi
      exit 1
    fi
    printf 'create table test(id int);\n'
    exit 0
    ;;
esac

printf 'unexpected docker invocation: %s\n' "$*" >&2
exit 1
`
