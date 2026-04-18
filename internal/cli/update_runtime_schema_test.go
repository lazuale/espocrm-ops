package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_UpdateRuntime_JSON_Success(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	composeFile := filepath.Join(projectDir, "compose.yaml")
	envFile := filepath.Join(projectDir, ".env.dev")
	stateDir := filepath.Join(tmp, "docker-state")
	logPath := filepath.Join(tmp, "docker.log")

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
	writeUpdateRuntimeStatusFile(t, stateDir, "espocrm", "healthy")
	writeUpdateRuntimeStatusFile(t, stateDir, "espocrm-daemon", "healthy")
	writeUpdateRuntimeStatusFile(t, stateDir, "espocrm-websocket", "healthy")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	prependFakeDockerForUpdateRuntimeCLITest(t)
	t.Setenv("DOCKER_MOCK_UPDATE_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_UPDATE_LOG", logPath)

	out, err := runRootCommand(
		t,
		"--journal-dir", journalDir,
		"--json",
		"update-runtime",
		"--project-dir", projectDir,
		"--compose-file", composeFile,
		"--env-file", envFile,
		"--site-url", server.URL,
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
	requireJSONPath(t, obj, "details", "skip_pull")
	requireJSONPath(t, obj, "details", "skip_http_probe")
	requireJSONPath(t, obj, "details", "services_ready")
	requireJSONPath(t, obj, "artifacts", "project_dir")
	requireJSONPath(t, obj, "artifacts", "compose_file")
	requireJSONPath(t, obj, "artifacts", "env_file")
	requireJSONPath(t, obj, "artifacts", "site_url")

	if obj["command"] != "update-runtime" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if message := obj["message"]; message != "update runtime completed" {
		t.Fatalf("unexpected message: %v", message)
	}

	services, ok := requireJSONPath(t, obj, "details", "services_ready").([]any)
	if !ok || len(services) != 4 {
		t.Fatalf("expected four ready services, got %v", requireJSONPath(t, obj, "details", "services_ready"))
	}
	if timeout, _ := requireJSONPath(t, obj, "details", "timeout_seconds").(float64); int(timeout) != 10 {
		t.Fatalf("unexpected timeout seconds: %v", timeout)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if !containsAll(log,
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" pull",
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" up -d",
	) {
		t.Fatalf("unexpected docker log:\n%s", log)
	}

	assertNoJournalFiles(t, journalDir)
}

func TestSchema_UpdateRuntime_JSON_Error_Timeout(t *testing.T) {
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
	writeUpdateRuntimeStatusFile(t, stateDir, "db", "starting")

	prependFakeDockerForUpdateRuntimeCLITest(t)
	t.Setenv("DOCKER_MOCK_UPDATE_STATE_DIR", stateDir)

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"update-runtime",
		"--project-dir", projectDir,
		"--compose-file", composeFile,
		"--env-file", envFile,
		"--skip-http-probe",
		"--timeout", "1",
	)

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "update_runtime_failed", "timed out while waiting for service readiness 'db'")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_UpdateRuntime_JSON_Error_MissingSiteURL_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"update-runtime",
		"--project-dir", tmp,
		"--compose-file", filepath.Join(tmp, "compose.yaml"),
		"--env-file", filepath.Join(tmp, ".env"),
	)

	assertUsageErrorOutput(t, outcome, "--site-url is required")
	assertNoJournalFiles(t, journalDir)
}

func prependFakeDockerForUpdateRuntimeCLITest(t *testing.T) {
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

if [[ "${1:-}" == "compose" && "$args" == *" pull "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" up -d "* ]]; then
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

echo "unexpected docker invocation: $*" >&2
exit 98
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeUpdateRuntimeStatusFile(t *testing.T, stateDir, service string, statuses ...string) {
	t.Helper()

	body := strings.Join(statuses, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(stateDir, service+".statuses"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
