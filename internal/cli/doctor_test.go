package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorCLIJSONSuccess(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "backups", "prod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "espo"), 0o755); err != nil {
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
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	prependDoctorFakeDocker(t)
	t.Setenv("TEST_DOCTOR_DOCKER_PS_OUTPUT", `[{"Service":"db"},{"Service":"espocrm"},{"Service":"espocrm-daemon"},{"Service":"espocrm-websocket"}]`)

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"doctor", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "doctor" {
		t.Fatalf("unexpected command: %s", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=true")
	}
	if message := requireJSONString(t, obj, "message"); message != "doctor passed" {
		t.Fatalf("unexpected message: %s", message)
	}
	checks := requireJSONObjectArray(t, obj, "result", "checks")
	if len(checks) != 6 {
		t.Fatalf("unexpected checks: %#v", checks)
	}
	for i, name := range []string{"config", "backup_root", "storage_dir", "compose_config", "services", "db_ping"} {
		if got := requireJSONStringFromObject(t, checks[i], "name"); got != name {
			t.Fatalf("unexpected check name at %d: %s", i, got)
		}
		if !requireJSONBoolFromObject(t, checks[i], "ok") {
			t.Fatalf("expected check %s to pass", name)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestDoctorCLIJSONFailureForMissingEnv(t *testing.T) {
	projectDir := t.TempDir()
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
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"doctor", "--scope", "prod", "--project-dir", projectDir}, stdout, stderr)
	if exitCode != exitUsage {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitUsage, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "doctor" {
		t.Fatalf("unexpected command: %s", command)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if message := requireJSONString(t, obj, "message"); message != "doctor failed" {
		t.Fatalf("unexpected message: %s", message)
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "usage" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if errMessage := requireJSONString(t, obj, "error", "message"); !strings.Contains(errMessage, "DB_NAME is required") {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	checks := requireJSONObjectArray(t, obj, "result", "checks")
	if len(checks) != 1 {
		t.Fatalf("unexpected checks: %#v", checks)
	}
	if name := requireJSONStringFromObject(t, checks[0], "name"); name != "config" {
		t.Fatalf("unexpected failed check: %s", name)
	}
	if requireJSONBoolFromObject(t, checks[0], "ok") {
		t.Fatal("expected config check to fail")
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func prependDoctorFakeDocker(t *testing.T) {
	t.Helper()

	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(scriptPath, []byte(doctorFakeDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func requireJSONObjectArray(t *testing.T, obj map[string]any, path ...string) []map[string]any {
	t.Helper()

	raw := requireJSONPath(t, obj, path...)
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected array at %v, got %#v", path, raw)
	}

	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		value, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("expected object item at %v, got %#v", path, item)
		}
		out = append(out, value)
	}
	return out
}

func requireJSONStringFromObject(t *testing.T, obj map[string]any, path ...string) string {
	t.Helper()

	return requireJSONString(t, obj, path...)
}

func requireJSONBoolFromObject(t *testing.T, obj map[string]any, path ...string) bool {
	t.Helper()

	return requireJSONBool(t, obj, path...)
}

const doctorFakeDockerScript = `#!/usr/bin/env bash
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
    if [[ "${TEST_DOCTOR_DOCKER_FAIL_CONFIG:-}" == "1" ]]; then
      printf 'compose config failed\n' >&2
      exit 1
    fi
    exit 0
    ;;
  ps)
    printf '%s' "${TEST_DOCTOR_DOCKER_PS_OUTPUT:-[]}"
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
    case "${1:-}" in
      mariadb)
        if [[ "${TEST_DOCTOR_DOCKER_FAIL_DBPING:-}" == "1" ]]; then
          printf 'db ping failed\n' >&2
          exit 1
        fi
        exit 0
        ;;
    esac
    ;;
esac

printf 'unexpected docker invocation: %s\n' "$*" >&2
exit 1
`
