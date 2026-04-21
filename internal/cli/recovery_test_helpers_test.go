package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	cliTestDockerStateDirEnv      = "DOCKER_TEST_STATE_DIR"
	cliTestDockerLogEnv           = "DOCKER_TEST_LOG"
	cliTestDockerConfigErrorEnv   = "DOCKER_TEST_CONFIG_ERROR"
	cliTestDockerHealthMessageEnv = "DOCKER_TEST_HEALTH_MESSAGE"
	cliTestDockerDumpStdoutEnv    = "DOCKER_TEST_DUMP_STDOUT"
	cliTestDockerRuntimeUIDEnv    = "DOCKER_TEST_RUNTIME_UID"
	cliTestDockerRuntimeGIDEnv    = "DOCKER_TEST_RUNTIME_GID"
	cliTestDockerFailOnCallEnv    = "DOCKER_TEST_FAIL_ON_CALL"
)

type dockerHarness struct {
	stateDir string
	logPath  string
}

type backupSetFixture struct {
	DBBackup     string
	FilesBackup  string
	ManifestTXT  string
	ManifestJSON string
}

func newDockerHarness(t *testing.T) *dockerHarness {
	t.Helper()

	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	stateDir := filepath.Join(rootDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte(cliTestDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(cliTestDockerStateDirEnv, stateDir)
	t.Setenv(cliTestDockerRuntimeUIDEnv, strconv.Itoa(os.Getuid()))
	t.Setenv(cliTestDockerRuntimeGIDEnv, strconv.Itoa(os.Getgid()))

	return &dockerHarness{
		stateDir: stateDir,
		logPath:  filepath.Join(rootDir, "docker.log"),
	}
}

func (h *dockerHarness) SetFailOnAnyCall(t *testing.T) {
	t.Helper()
	t.Setenv(cliTestDockerFailOnCallEnv, "unexpected docker invocation")
}

func (h *dockerHarness) EnableLog(t *testing.T) string {
	t.Helper()
	t.Setenv(cliTestDockerLogEnv, h.logPath)
	return h.logPath
}

func (h *dockerHarness) ReadLog(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(h.logPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (h *dockerHarness) SetComposeConfigError(t *testing.T, message string) {
	t.Helper()
	t.Setenv(cliTestDockerConfigErrorEnv, message)
}

func (h *dockerHarness) SetHealthFailureMessage(t *testing.T, message string) {
	t.Helper()
	t.Setenv(cliTestDockerHealthMessageEnv, message)
}

func (h *dockerHarness) SetDumpStdout(t *testing.T, dump string) {
	t.Helper()
	t.Setenv(cliTestDockerDumpStdoutEnv, dump)
}

func (h *dockerHarness) SetRuntimeIdentity(t *testing.T, uid, gid int) {
	t.Helper()
	t.Setenv(cliTestDockerRuntimeUIDEnv, strconv.Itoa(uid))
	t.Setenv(cliTestDockerRuntimeGIDEnv, strconv.Itoa(gid))
}

func (h *dockerHarness) SetRunningServices(t *testing.T, services ...string) {
	t.Helper()

	var body string
	if len(services) != 0 {
		body = strings.Join(services, "\n") + "\n"
	}
	if err := os.WriteFile(filepath.Join(h.stateDir, "running-services"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (h *dockerHarness) SetServiceHealth(t *testing.T, service string, statuses ...string) {
	t.Helper()

	body := strings.Join(statuses, "\n")
	if body != "" {
		body += "\n"
	}
	if err := os.WriteFile(filepath.Join(h.stateDir, service+".statuses"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func isolateRecoveryLocks(t *testing.T) testAppOption {
	t.Helper()

	return withRestoreLockDir(t.TempDir())
}

func writeBackupSet(t *testing.T, backupRoot, prefix, stamp, scope string, files map[string]string) backupSetFixture {
	t.Helper()

	if len(files) == 0 {
		files = map[string]string{
			"espo/test.txt": "hello",
		}
	}

	set := backupSetFixture{
		DBBackup:     filepath.Join(backupRoot, "db", prefix+"_"+stamp+".sql.gz"),
		FilesBackup:  filepath.Join(backupRoot, "files", prefix+"_files_"+stamp+".tar.gz"),
		ManifestTXT:  filepath.Join(backupRoot, "manifests", prefix+"_"+stamp+".manifest.txt"),
		ManifestJSON: filepath.Join(backupRoot, "manifests", prefix+"_"+stamp+".manifest.json"),
	}

	if err := os.MkdirAll(filepath.Dir(set.DBBackup), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(set.FilesBackup), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(set.ManifestJSON), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, set.DBBackup, []byte("select 1;"))
	writeTarGzFile(t, set.FilesBackup, files)
	if err := os.WriteFile(set.DBBackup+".sha256", []byte(sha256OfFile(t, set.DBBackup)+"  "+filepath.Base(set.DBBackup)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set.FilesBackup+".sha256", []byte(sha256OfFile(t, set.FilesBackup)+"  "+filepath.Base(set.FilesBackup)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set.ManifestTXT, []byte("created_at=2026-04-18T10:00:00Z\ncontour="+scope+"\ncompose_project="+prefix+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, set.ManifestJSON, map[string]any{
		"version":    1,
		"scope":      scope,
		"created_at": "2026-04-18T10:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(set.DBBackup),
			"files_backup": filepath.Base(set.FilesBackup),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, set.DBBackup),
			"files_backup": sha256OfFile(t, set.FilesBackup),
		},
	})

	return set
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

const cliTestDockerScript = `#!/usr/bin/env bash
set -Eeuo pipefail

state_dir="${DOCKER_TEST_STATE_DIR:-}"
fail_on_call="${DOCKER_TEST_FAIL_ON_CALL:-}"

log_call() {
  if [[ -n "${DOCKER_TEST_LOG:-}" ]]; then
    printf '%s\n' "$*" >> "${DOCKER_TEST_LOG}"
  fi
}

read_running_services() {
  if [[ -f "$state_dir/running-services" ]]; then
    cat "$state_dir/running-services"
  fi
}

write_running_services() {
  mkdir -p "$state_dir"
  : > "$state_dir/running-services"
  for service in "$@"; do
    [[ -n "$service" ]] || continue
    printf '%s\n' "$service" >> "$state_dir/running-services"
  done
}

service_is_running() {
  local service="$1"
  grep -qx "$service" "$state_dir/running-services" 2>/dev/null
}

set_running_services() {
  local mode="$1"
  shift

  local canonical=("db" "espocrm" "espocrm-daemon" "espocrm-websocket")
  local requested=("$@")
  local current=()
  local result=()
  local service

  if [[ -f "$state_dir/running-services" ]]; then
    mapfile -t current < "$state_dir/running-services"
  fi

  case "$mode" in
    all)
      write_running_services "${canonical[@]}"
      return 0
      ;;
    add)
      for service in "${canonical[@]}"; do
        if printf '%s\n' "${current[@]}" "${requested[@]}" | grep -qx "$service"; then
          result+=("$service")
        fi
      done
      ;;
    remove)
      for service in "${canonical[@]}"; do
        if printf '%s\n' "${requested[@]}" | grep -qx "$service"; then
          continue
        fi
        if printf '%s\n' "${current[@]}" | grep -qx "$service"; then
          result+=("$service")
        fi
      done
      ;;
  esac

  write_running_services "${result[@]}"
}

if [[ -n "$fail_on_call" ]]; then
  echo "$fail_on_call: $*" >&2
  exit 97
fi

log_call "$*"

if [[ "${1:-}" == "version" && "${2:-}" == "--format" && "${3:-}" == "{{.Client.Version}}" ]]; then
  echo "25.0.2"
  exit 0
fi

if [[ "${1:-}" == "version" && "${2:-}" == "--format" && "${3:-}" == "{{.Server.Version}}" ]]; then
  echo "25.0.2"
  exit 0
fi

if [[ "${1:-}" == "compose" && "${2:-}" == "version" && "${3:-}" == "--short" ]]; then
  echo "2.24.1"
  exit 0
fi

if [[ "${1:-}" == "compose" ]]; then
  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --project-directory|-f|--env-file)
        shift 2
        continue
        ;;
      config)
        if [[ -n "${DOCKER_TEST_CONFIG_ERROR:-}" ]]; then
          echo "${DOCKER_TEST_CONFIG_ERROR}" >&2
          exit 23
        fi
        exit 0
        ;;
      down)
        shift
        write_running_services
        exit 0
        ;;
      up)
        shift
        if [[ "${1:-}" == "-d" ]]; then
          shift
        fi
        if [[ $# -eq 0 ]]; then
          set_running_services all
        else
          set_running_services add "$@"
        fi
        exit 0
        ;;
      stop)
        shift
        set_running_services remove "$@"
        exit 0
        ;;
      ps)
        shift
        if [[ "${1:-}" == "--status" && "${2:-}" == "running" && "${3:-}" == "--services" ]]; then
          read_running_services
          exit 0
        fi
        if [[ "${1:-}" == "-q" ]]; then
          service="${2:-}"
          if service_is_running "$service"; then
            echo "mock-${service}"
          fi
          exit 0
        fi
        ;;
    esac
    shift
  done
fi

if [[ "${1:-}" == "image" && "${2:-}" == "inspect" ]]; then
  exit 0
fi

if [[ "${1:-}" == "inspect" && "${2:-}" == "--format" && "${3:-}" == "{{.State.Running}}" ]]; then
  container="${4:-}"
  service="${container#mock-}"
  if service_is_running "$service"; then
    echo "true"
  else
    echo "false"
  fi
  exit 0
fi

if [[ "${1:-}" == "inspect" ]]; then
  if [[ "$*" == *".State.Health.Log"* ]]; then
    printf '%s\n' "${DOCKER_TEST_HEALTH_MESSAGE:-mock health failure}"
    exit 0
  fi

  container="${*: -1}"
  service="${container#mock-}"
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
  elif ! service_is_running "$service"; then
    status=""
  fi

  printf '%s\n' "$status"
  exit 0
fi

if [[ "${1:-}" == "exec" ]]; then
  args=" $* "

  if [[ "$args" == *" mariadb-dump --version "* || "$args" == *" mysqldump --version "* ]]; then
    echo "mariadb-dump from 11.0.0"
    exit 0
  fi

  if [[ "$args" == *" mariadb --version "* || "$args" == *" mysql --version "* ]]; then
    echo "mariadb from 11.0.0"
    exit 0
  fi

  if [[ "$args" == *" mariadb-dump "* || "$args" == *" mysqldump "* ]]; then
    printf '%s' "${DOCKER_TEST_DUMP_STDOUT:-select 1;}"
    exit 0
  fi

  if [[ "$args" == *" mariadb "* || "$args" == *" mysql "* ]]; then
    cat >/dev/null
    exit 0
  fi
fi

if [[ "${1:-}" == "run" ]]; then
  storage_host=""
  for arg in "$@"; do
    if [[ "$arg" == *":/espo-storage" ]]; then
      storage_host="${arg%%:/espo-storage}"
      break
    fi
  done

  if [[ -n "$storage_host" ]]; then
    chown -R "${ESPO_RUNTIME_UID}:${ESPO_RUNTIME_GID}" "$storage_host"
    find "$storage_host" -type d -exec chmod 0755 {} +

    for relative in data custom client/custom upload; do
      path="$storage_host/$relative"
      if [[ -d "$path" ]]; then
        find "$path" -type d -exec chmod 0775 {} +
        find "$path" -type f -exec chmod 0664 {} +
      fi
    done
    exit 0
  fi

  printf '%s:%s\n' "${DOCKER_TEST_RUNTIME_UID:-$(id -u)}" "${DOCKER_TEST_RUNTIME_GID:-$(id -g)}"
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 98
`
