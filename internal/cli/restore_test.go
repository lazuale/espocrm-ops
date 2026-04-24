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
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	stdinLogPath := filepath.Join(t.TempDir(), "restore-db.sql")
	prependRestoreFakeDocker(t)
	t.Setenv("TEST_RESTORE_DOCKER_STDIN_LOG", stdinLogPath)

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
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
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
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
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
	if errMessage := requireJSONString(t, obj, "error", "message"); !strings.Contains(errMessage, "restore source scope is invalid") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
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
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	prependRestoreFakeDocker(t)
	t.Setenv("TEST_RESTORE_DOCKER_FAIL_RESET", "1")
	t.Setenv("TEST_RESTORE_DOCKER_FAIL_MESSAGE", "reset failed with MYSQL_PWD=root-secret and root-secret")

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
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func writeVerifiedRestoreBackupSet(t *testing.T) (manifestPath, dbSQL string) {
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

func prependRestoreFakeDocker(t *testing.T) {
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
}

const restoreFakeDockerScript = `#!/usr/bin/env bash
set -Eeuo pipefail

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
  stop|start)
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
            if [[ "${TEST_RESTORE_DOCKER_FAIL_RESET:-}" == "1" ]]; then
              printf '%s\n' "${TEST_RESTORE_DOCKER_FAIL_MESSAGE:-reset failed}" >&2
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
            cat >"${TEST_RESTORE_DOCKER_STDIN_LOG:-/dev/null}"
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
