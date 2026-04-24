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

func TestSmokeCLIJSONSuccess(t *testing.T) {
	projectDir := smokeProjectDir(t, true)

	prependSmokeFakeDocker(t)
	t.Setenv("TEST_SMOKE_DOCKER_PS_OUTPUT", smokeDockerPSOutput())

	oldNow := smokeNow
	smokeNow = func() time.Time {
		return time.Date(2026, 4, 24, 19, 0, 0, 0, time.UTC)
	}
	defer func() { smokeNow = oldNow }()

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{
		"smoke",
		"--from-scope", "dev",
		"--to-scope", "prod",
		"--project-dir", projectDir,
	}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "smoke" {
		t.Fatalf("unexpected command: %s", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=true")
	}
	if message := requireJSONString(t, obj, "message"); message != "smoke completed" {
		t.Fatalf("unexpected message: %s", message)
	}
	if got := requireJSONString(t, obj, "result", "from_scope"); got != "dev" {
		t.Fatalf("unexpected from_scope: %s", got)
	}
	if got := requireJSONString(t, obj, "result", "to_scope"); got != "prod" {
		t.Fatalf("unexpected to_scope: %s", got)
	}

	manifestPath := requireJSONString(t, obj, "result", "manifest")
	restoreSnapshot := requireJSONString(t, obj, "result", "restore_snapshot_manifest")
	migrateSnapshot := requireJSONString(t, obj, "result", "migrate_snapshot_manifest")
	if manifestPath == restoreSnapshot || manifestPath == migrateSnapshot || restoreSnapshot == migrateSnapshot {
		t.Fatalf("expected distinct manifest paths, got manifest=%s restore=%s migrate=%s", manifestPath, restoreSnapshot, migrateSnapshot)
	}

	for _, path := range []string{manifestPath, restoreSnapshot, migrateSnapshot} {
		if _, err := ops.VerifyBackup(context.Background(), path); err != nil {
			t.Fatalf("manifest did not verify %s: %v", path, err)
		}
	}

	steps := requireJSONObjectArray(t, obj, "result", "steps")
	wantSteps := []string{
		"doctor source",
		"doctor target",
		"backup",
		"backup verify",
		"restore",
		"migrate",
	}
	if len(steps) != len(wantSteps) {
		t.Fatalf("unexpected steps: %#v", steps)
	}
	for i, name := range wantSteps {
		if got := requireJSONStringFromObject(t, steps[i], "name"); got != name {
			t.Fatalf("unexpected step[%d] name: %s", i, got)
		}
		if !requireJSONBoolFromObject(t, steps[i], "ok") {
			t.Fatalf("expected step %s to pass", name)
		}
	}

	assertFileBody(t, filepath.Join(projectDir, "runtime", "prod", "espo", "source.txt"), "source\n")
	if _, err := os.Stat(filepath.Join(projectDir, "runtime", "prod", "espo", "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old target file removed, got %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestSmokeCLIJSONUsageFailure(t *testing.T) {
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{
		"smoke",
		"--to-scope", "prod",
		"--project-dir", t.TempDir(),
	}, stdout, stderr)
	if exitCode != exitUsage {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitUsage, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "smoke" {
		t.Fatalf("unexpected command: %s", command)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if message := requireJSONString(t, obj, "message"); message != "smoke failed" {
		t.Fatalf("unexpected message: %s", message)
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "usage" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if errMessage := requireJSONString(t, obj, "error", "message"); errMessage != "--from-scope is required" {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestSmokeCLIJSONFailsClosedAtTargetDoctor(t *testing.T) {
	projectDir := smokeProjectDir(t, false)

	prependSmokeFakeDocker(t)
	t.Setenv("TEST_SMOKE_DOCKER_PS_OUTPUT", smokeDockerPSOutput())

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{
		"smoke",
		"--from-scope", "dev",
		"--to-scope", "prod",
		"--project-dir", projectDir,
	}, stdout, stderr)
	if exitCode != exitIO {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitIO, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "smoke" {
		t.Fatalf("unexpected command: %s", command)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if message := requireJSONString(t, obj, "message"); message != "smoke failed" {
		t.Fatalf("unexpected message: %s", message)
	}
	if step := requireJSONString(t, obj, "result", "failed_step"); step != "doctor target" {
		t.Fatalf("unexpected failed_step: %s", step)
	}
	steps := requireJSONObjectArray(t, obj, "result", "steps")
	if len(steps) != 2 {
		t.Fatalf("unexpected steps: %#v", steps)
	}
	if got := requireJSONStringFromObject(t, steps[0], "name"); got != "doctor source" {
		t.Fatalf("unexpected first step: %s", got)
	}
	if !requireJSONBoolFromObject(t, steps[0], "ok") {
		t.Fatal("expected doctor source to pass")
	}
	if got := requireJSONStringFromObject(t, steps[1], "name"); got != "doctor target" {
		t.Fatalf("unexpected second step: %s", got)
	}
	if requireJSONBoolFromObject(t, steps[1], "ok") {
		t.Fatal("expected doctor target to fail")
	}

	sourceEntries, err := os.ReadDir(filepath.Join(projectDir, "backups", "dev"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceEntries) != 0 {
		t.Fatalf("expected no backup artifacts after target doctor failure, got %v", len(sourceEntries))
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestSmokeCLIJSONFailsClosedAtBackup(t *testing.T) {
	projectDir := smokeProjectDir(t, true)

	prependSmokeFakeDocker(t)
	t.Setenv("TEST_SMOKE_DOCKER_PS_OUTPUT", smokeDockerPSOutput())
	t.Setenv("TEST_SMOKE_DOCKER_FAIL_DUMP", "1")

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{
		"smoke",
		"--from-scope", "dev",
		"--to-scope", "prod",
		"--project-dir", projectDir,
	}, stdout, stderr)
	if exitCode != exitRuntime {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitRuntime, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if step := requireJSONString(t, obj, "result", "failed_step"); step != "backup" {
		t.Fatalf("unexpected failed_step: %s", step)
	}
	steps := requireJSONObjectArray(t, obj, "result", "steps")
	if len(steps) != 3 {
		t.Fatalf("unexpected steps: %#v", steps)
	}
	if got := requireJSONStringFromObject(t, steps[2], "name"); got != "backup" {
		t.Fatalf("unexpected third step: %s", got)
	}
	if requireJSONBoolFromObject(t, steps[2], "ok") {
		t.Fatal("expected backup step to fail")
	}

	devManifestDir := filepath.Join(projectDir, "backups", "dev", "manifests")
	entries, err := os.ReadDir(devManifestDir)
	if err == nil && len(entries) > 0 {
		t.Fatalf("expected no completed backup manifests after backup failure, got %d", len(entries))
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func smokeProjectDir(t *testing.T, createTargetBackupRoot bool) string {
	t.Helper()

	projectDir := t.TempDir()
	for _, dir := range []string{
		filepath.Join(projectDir, "backups", "dev"),
		filepath.Join(projectDir, "runtime", "dev", "espo"),
		filepath.Join(projectDir, "runtime", "prod", "espo"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if createTargetBackupRoot {
		if err := os.MkdirAll(filepath.Join(projectDir, "backups", "prod"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(projectDir, "runtime", "dev", "espo", "source.txt"), []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "runtime", "prod", "espo", "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	devEnv := []string{
		"ESPO_CONTOUR=dev",
		"BACKUP_ROOT=./backups/dev",
		"ESPO_STORAGE_DIR=./runtime/dev/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm_dev",
		"",
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.dev"), []byte(strings.Join(devEnv, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	prodEnv := []string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm_prod",
		"",
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join(prodEnv, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	return projectDir
}

func prependSmokeFakeDocker(t *testing.T) {
	t.Helper()

	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(scriptPath, []byte(smokeFakeDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func smokeDockerPSOutput() string {
	return strings.Join([]string{
		`{"Service":"db"}`,
		`{"Service":"espocrm"}`,
		`{"Service":"espocrm-daemon"}`,
		`{"Service":"espocrm-websocket"}`,
		"",
	}, "\n")
}

func assertFileBody(t *testing.T, path, want string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if got := string(raw); got != want {
		t.Fatalf("unexpected file body in %s: got %q want %q", path, got, want)
	}
}

const smokeFakeDockerScript = `#!/usr/bin/env bash
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
  ps)
    printf '%s' "${TEST_SMOKE_DOCKER_PS_OUTPUT:-[]}"
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
        if [[ "${TEST_SMOKE_DOCKER_FAIL_DUMP:-}" == "1" ]]; then
          printf 'dump failed\n' >&2
          exit 1
        fi
        printf 'create table smoke(id int);\n'
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
            cat >"${TEST_SMOKE_DOCKER_STDIN_LOG:-/dev/null}"
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
