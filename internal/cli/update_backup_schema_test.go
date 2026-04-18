package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_UpdateBackup_JSON_Success_StartsDBAndCreatesRecoveryPoint(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	composeFile := filepath.Join(projectDir, "compose.yaml")
	envFile := filepath.Join(projectDir, ".env.dev")
	stateDir := filepath.Join(tmp, "docker-state")
	logPath := filepath.Join(tmp, "docker.log")
	backupRoot := filepath.Join(tmp, "backups")
	storageDir := filepath.Join(tmp, "storage", "espo")

	useJournalClockForTest(t, time.Date(2026, 4, 15, 11, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "file.txt"), []byte("backup-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte("COMPOSE_PROJECT_NAME=espocrm-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	writeUpdateRuntimeStatusFile(t, stateDir, "db", "starting", "healthy")
	if err := os.WriteFile(filepath.Join(stateDir, "running-services"), []byte("espocrm\nespocrm-daemon\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prependFakeDockerForUpdateBackupCLITest(t)
	t.Setenv("DOCKER_MOCK_UPDATE_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_UPDATE_LOG", logPath)
	t.Setenv("COMPOSE_PROJECT_NAME", "espocrm-test")
	t.Setenv("BACKUP_ROOT", backupRoot)
	t.Setenv("ESPO_STORAGE_DIR", storageDir)
	t.Setenv("BACKUP_NAME_PREFIX", "espocrm-test-dev")
	t.Setenv("BACKUP_RETENTION_DAYS", "7")
	t.Setenv("DB_USER", "espocrm")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "espocrm")
	t.Setenv("DOCKER_MOCK_UPDATE_DUMP_STDOUT", "create table test(id int);\n")

	out, err := runRootCommand(
		t,
		"--journal-dir", journalDir,
		"--json",
		"update-backup",
		"--scope", "dev",
		"--project-dir", projectDir,
		"--compose-file", composeFile,
		"--env-file", envFile,
		"--timeout", "10",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "timeout_seconds")
	requireJSONPath(t, obj, "details", "started_db_temporarily")
	requireJSONPath(t, obj, "details", "created_at")
	requireJSONPath(t, obj, "details", "consistent_snapshot")
	requireJSONPath(t, obj, "details", "app_services_were_running")
	requireJSONPath(t, obj, "artifacts", "scope")
	requireJSONPath(t, obj, "artifacts", "manifest_txt")
	requireJSONPath(t, obj, "artifacts", "manifest_json")
	requireJSONPath(t, obj, "artifacts", "db_backup")
	requireJSONPath(t, obj, "artifacts", "files_backup")
	requireJSONPath(t, obj, "artifacts", "db_checksum")
	requireJSONPath(t, obj, "artifacts", "files_checksum")

	if obj["command"] != "update-backup" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if message := obj["message"]; message != "update backup completed" {
		t.Fatalf("unexpected message: %v", message)
	}
	if started, _ := requireJSONPath(t, obj, "details", "started_db_temporarily").(bool); !started {
		t.Fatalf("expected started_db_temporarily=true, got %v", requireJSONPath(t, obj, "details", "started_db_temporarily"))
	}
	if createdAt := requireJSONPath(t, obj, "details", "created_at"); createdAt != "2026-04-15T11:00:00Z" {
		t.Fatalf("unexpected created_at: %v", createdAt)
	}

	for _, key := range []string{"manifest_txt", "manifest_json", "db_backup", "files_backup", "db_checksum", "files_checksum"} {
		path, _ := requireJSONPath(t, obj, "artifacts", key).(string)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s at %s: %v", key, path, err)
		}
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(string(rawLog),
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" up -d db",
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" ps -q db",
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" ps --status running --services",
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" stop espocrm espocrm-daemon espocrm-websocket",
		"exec -i -e MYSQL_PWD mock-db mariadb-dump -u espocrm espocrm --single-transaction --quick --routines --triggers --events",
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" up -d espocrm espocrm-daemon espocrm-websocket",
	) {
		t.Fatalf("unexpected docker log:\n%s", string(rawLog))
	}

	assertNoJournalFiles(t, journalDir)
}

func TestSchema_UpdateBackup_JSON_Error_DumpFailure(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	composeFile := filepath.Join(projectDir, "compose.yaml")
	envFile := filepath.Join(projectDir, ".env.dev")
	stateDir := filepath.Join(tmp, "docker-state")

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte("COMPOSE_PROJECT_NAME=espocrm-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeUpdateRuntimeStatusFile(t, stateDir, "db", "healthy")

	prependFakeDockerForUpdateBackupCLITest(t)
	t.Setenv("DOCKER_MOCK_UPDATE_STATE_DIR", stateDir)
	t.Setenv("COMPOSE_PROJECT_NAME", "espocrm-test")
	t.Setenv("BACKUP_ROOT", filepath.Join(tmp, "backups"))
	t.Setenv("ESPO_STORAGE_DIR", filepath.Join(tmp, "storage", "espo"))
	t.Setenv("BACKUP_NAME_PREFIX", "espocrm-test-dev")
	t.Setenv("BACKUP_RETENTION_DAYS", "7")
	t.Setenv("DB_USER", "espocrm")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "espocrm")
	t.Setenv("DOCKER_MOCK_UPDATE_EXEC_STDERR", "mock backup failure")
	t.Setenv("DOCKER_MOCK_UPDATE_EXEC_EXIT_CODE", "23")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"update-backup",
		"--scope", "dev",
		"--project-dir", projectDir,
		"--compose-file", composeFile,
		"--env-file", envFile,
		"--timeout", "10",
	)

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "update_backup_failed", "mock backup failure")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_UpdateBackup_JSON_Error_MissingBackupRoot_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	t.Setenv("COMPOSE_PROJECT_NAME", "espocrm-test")
	t.Setenv("ESPO_STORAGE_DIR", filepath.Join(tmp, "storage", "espo"))
	t.Setenv("DB_USER", "espocrm")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "espocrm")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"update-backup",
		"--scope", "dev",
		"--project-dir", tmp,
		"--compose-file", filepath.Join(tmp, "compose.yaml"),
		"--env-file", filepath.Join(tmp, ".env"),
		"--timeout", "10",
	)

	assertUsageErrorOutput(t, outcome, "BACKUP_ROOT is required")
	assertNoJournalFiles(t, journalDir)
}

func prependFakeDockerForUpdateBackupCLITest(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/usr/bin/env bash
set -euo pipefail

args=" $* "
state_dir="${DOCKER_MOCK_UPDATE_STATE_DIR:-}"

if [[ -n "${DOCKER_MOCK_UPDATE_LOG:-}" ]]; then
  printf '%s\n' "$*" >> "${DOCKER_MOCK_UPDATE_LOG}"
fi

if [[ "${1:-}" == "compose" && "$args" == *" up -d "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" stop "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps --status running --services"* ]]; then
  if [[ -f "$state_dir/running-services" ]]; then
    cat "$state_dir/running-services"
  fi
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps "* && "$args" == *" -q "* ]]; then
  service="${*: -1}"
  echo "mock-${service}"
  exit 0
fi

if [[ "${1:-}" == "inspect" ]]; then
  if [[ "$*" == *".State.Health.Log"* ]]; then
    printf '%s\n' "${DOCKER_MOCK_UPDATE_HEALTH_MESSAGE:-mock health failure}"
    exit 0
  fi

  container_id="${*: -1}"
  service="${container_id#mock-}"
  status_file="$state_dir/${service}.statuses"
  index_file="$state_dir/${service}.index"
  status="healthy"
  index=0

  if [[ -f "$status_file" ]]; then
    mapfile -t statuses < "$status_file"

    if [[ -f "$index_file" ]]; then
      index="$(cat "$index_file")"
    fi

    if (( ${#statuses[@]} > 0 )); then
      if (( index >= ${#statuses[@]} )); then
        index=$((${#statuses[@]} - 1))
      fi

      status="${statuses[$index]}"
      if (( index < ${#statuses[@]} - 1 )); then
        printf '%s\n' "$((index + 1))" > "$index_file"
      fi
    fi
  fi

  printf '%s\n' "$status"
  exit 0
fi

if [[ "${1:-}" == "exec" && "${3:-}" == "mariadb-dump" && "${4:-}" == "--version" ]]; then
  echo "mariadb-dump from 11.0.0"
  exit 0
fi

if [[ "${1:-}" == "exec" && "${2:-}" == "-i" && " $* " == *" mariadb-dump "* ]]; then
  if [[ -n "${DOCKER_MOCK_UPDATE_EXEC_STDERR:-}" ]]; then
    printf '%s\n' "${DOCKER_MOCK_UPDATE_EXEC_STDERR}" >&2
  fi
  if [[ -n "${DOCKER_MOCK_UPDATE_EXEC_EXIT_CODE:-}" ]]; then
    exit "${DOCKER_MOCK_UPDATE_EXEC_EXIT_CODE}"
  fi
  printf '%s' "${DOCKER_MOCK_UPDATE_DUMP_STDOUT:-select 1;}"
  exit 0
fi

if [[ "${1:-}" == "image" && "${2:-}" == "inspect" ]]; then
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 98
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
