package docker

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type fakeDockerOptions struct {
	logPath              string
	stdinPath            string
	envLogPath           string
	mariaDBAvailable     bool
	mysqlAvailable       bool
	dumpStdout           string
	inspectRunning       string
	execStderr           string
	execExitCode         int
	probeStderr          string
	probeExitCode        int
	availableImages      []string
	imageInspectStderr   string
	imageInspectExitCode int
	runningServices      []string
	composeRunningOutput string
	composeConfigError   string
	serviceStates        map[string]string
	healthMessage        string
}

func prependFakeDocker(t *testing.T, opts fakeDockerOptions) {
	t.Helper()

	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	stateDir := filepath.Join(rootDir, "state")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if len(opts.runningServices) != 0 {
		writeFakeDockerStateList(t, filepath.Join(stateDir, "running-services"), opts.runningServices)
	}
	if len(opts.availableImages) != 0 {
		writeFakeDockerStateList(t, filepath.Join(stateDir, "available-images"), opts.availableImages)
	}
	for service, status := range opts.serviceStates {
		if err := os.WriteFile(filepath.Join(stateDir, "status-"+fakeDockerServiceKey(service)), []byte(status+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte(fakeDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DOCKER_TEST_STATE_DIR", stateDir)
	if opts.logPath != "" {
		t.Setenv("DOCKER_TEST_LOG", opts.logPath)
	}
	if opts.stdinPath != "" {
		t.Setenv("DOCKER_TEST_STDIN_LOG", opts.stdinPath)
	}
	if opts.envLogPath != "" {
		t.Setenv("DOCKER_TEST_ENV_LOG", opts.envLogPath)
	}
	if opts.dumpStdout != "" {
		t.Setenv("DOCKER_TEST_DUMP_STDOUT", opts.dumpStdout)
	}
	if opts.inspectRunning != "" {
		t.Setenv("DOCKER_TEST_INSPECT_RUNNING", opts.inspectRunning)
	}
	if opts.execStderr != "" {
		t.Setenv("DOCKER_TEST_EXEC_STDERR", opts.execStderr)
	}
	if opts.execExitCode != 0 {
		t.Setenv("DOCKER_TEST_EXEC_EXIT_CODE", strconv.Itoa(opts.execExitCode))
	}
	if opts.probeStderr != "" {
		t.Setenv("DOCKER_TEST_PROBE_STDERR", opts.probeStderr)
	}
	if opts.probeExitCode != 0 {
		t.Setenv("DOCKER_TEST_PROBE_EXIT_CODE", strconv.Itoa(opts.probeExitCode))
	}
	if opts.imageInspectStderr != "" {
		t.Setenv("DOCKER_TEST_IMAGE_INSPECT_STDERR", opts.imageInspectStderr)
	}
	if opts.imageInspectExitCode != 0 {
		t.Setenv("DOCKER_TEST_IMAGE_INSPECT_EXIT_CODE", strconv.Itoa(opts.imageInspectExitCode))
	}
	if opts.composeRunningOutput != "" {
		t.Setenv("DOCKER_TEST_COMPOSE_RUNNING_OUTPUT", opts.composeRunningOutput)
	}
	if opts.composeConfigError != "" {
		t.Setenv("DOCKER_TEST_COMPOSE_CONFIG_ERROR", opts.composeConfigError)
	}
	if opts.healthMessage != "" {
		t.Setenv("DOCKER_TEST_HEALTH_MESSAGE", opts.healthMessage)
	}
	t.Setenv("DOCKER_TEST_MARIADB_AVAILABLE", strconv.FormatBool(opts.mariaDBAvailable))
	t.Setenv("DOCKER_TEST_MYSQL_AVAILABLE", strconv.FormatBool(opts.mysqlAvailable))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeFakeDockerStateList(t *testing.T, path string, values []string) {
	t.Helper()

	body := strings.Join(values, "\n")
	if body != "" {
		body += "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fakeDockerServiceKey(service string) string {
	replacer := strings.NewReplacer("-", "_", ".", "_", "/", "_")
	return replacer.Replace(service)
}

func writeTestGzipFile(t *testing.T, path string, body []byte) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			t.Fatalf("close test gzip file: %v", closeErr)
		}
	}()

	gz := gzip.NewWriter(f)
	if _, err := gz.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

const fakeDockerScript = `#!/usr/bin/env bash
set -Eeuo pipefail

state_dir="${DOCKER_TEST_STATE_DIR:-}"
running_services_file="$state_dir/running-services"
available_images_file="$state_dir/available-images"

log_call() {
  if [[ -n "${DOCKER_TEST_LOG:-}" ]]; then
    printf '%s\n' "$*" >> "${DOCKER_TEST_LOG}"
  fi
}

record_env() {
  if [[ -n "${DOCKER_TEST_ENV_LOG:-}" ]]; then
    env | sort > "${DOCKER_TEST_ENV_LOG}"
  fi
}

service_key() {
  printf '%s' "$1" | tr './-' '___'
}

read_running_services() {
  if [[ -f "$running_services_file" ]]; then
    cat "$running_services_file"
  fi
}

write_running_services() {
  mkdir -p "$state_dir"
  : > "$running_services_file"
  for service in "$@"; do
    [[ -n "$service" ]] || continue
    printf '%s\n' "$service" >> "$running_services_file"
  done
}

service_is_running() {
  local service="$1"
  grep -qx "$service" "$running_services_file" 2>/dev/null
}

set_running_services() {
  local mode="$1"
  shift

  local canonical=("db" "espocrm" "espocrm-daemon" "espocrm-websocket")
  local requested=("$@")
  local current=()
  local result=()
  local service

  if [[ -f "$running_services_file" ]]; then
    mapfile -t current < "$running_services_file"
  fi

  case "$mode" in
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

service_status() {
  local service="$1"
  local key
  key="$(service_key "$service")"
  local path="$state_dir/status-$key"
  if [[ -f "$path" ]]; then
    head -n 1 "$path"
    return 0
  fi
  if service_is_running "$service"; then
    printf '%s\n' "running"
  fi
}

image_is_available() {
  local image="$1"
  grep -qx "$image" "$available_images_file" 2>/dev/null
}

log_call "$*"
record_env

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
        if [[ -n "${DOCKER_TEST_COMPOSE_CONFIG_ERROR:-}" ]]; then
          echo "${DOCKER_TEST_COMPOSE_CONFIG_ERROR}" >&2
          exit 23
        fi
        exit 0
        ;;
      up)
        shift
        if [[ "${1:-}" == "-d" ]]; then
          shift
        fi
        set_running_services add "$@"
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
          if [[ -n "${DOCKER_TEST_COMPOSE_RUNNING_OUTPUT:-}" ]]; then
            printf '%s' "${DOCKER_TEST_COMPOSE_RUNNING_OUTPUT}"
          else
            read_running_services
          fi
          exit 0
        fi
        if [[ "${1:-}" == "-q" ]]; then
          service="${2:-}"
          if service_is_running "$service" || [[ -n "$(service_status "$service")" ]]; then
            echo "mock-${service}"
          fi
          exit 0
        fi
        ;;
    esac
    shift
  done
fi

if [[ "${1:-}" == "inspect" && "${2:-}" == "--format" && "${3:-}" == "{{.State.Running}}" ]]; then
  if [[ -n "${DOCKER_TEST_INSPECT_RUNNING:-}" ]]; then
    echo "${DOCKER_TEST_INSPECT_RUNNING}"
    exit 0
  fi
  container="${4:-}"
  service="${container#mock-}"
  if service_is_running "$service"; then
    echo "true"
  else
    echo "false"
  fi
  exit 0
fi

if [[ "${1:-}" == "inspect" && "${2:-}" == "--format" ]]; then
  container="${4:-}"
  service="${container#mock-}"
  if [[ "${3:-}" == *".State.Health.Status"* || "${3:-}" == *".State.Status"* ]]; then
    printf '%s\n' "$(service_status "$service")"
    exit 0
  fi
  if [[ "${3:-}" == *".State.Health.Log"* ]]; then
    printf '%s\n' "${DOCKER_TEST_HEALTH_MESSAGE:-mock health failure}"
    exit 0
  fi
fi

if [[ "${1:-}" == "image" && "${2:-}" == "inspect" ]]; then
  if [[ -n "${DOCKER_TEST_IMAGE_INSPECT_STDERR:-}" ]]; then
    printf '%s\n' "${DOCKER_TEST_IMAGE_INSPECT_STDERR}" >&2
    exit "${DOCKER_TEST_IMAGE_INSPECT_EXIT_CODE:-23}"
  fi
  if image_is_available "${3:-}"; then
    exit 0
  fi
  printf 'Error response from daemon: No such image: %s\n' "${3:-}" >&2
  exit 1
fi

if [[ "${1:-}" == "exec" ]]; then
  args=" $* "

  if [[ "$args" == *" --version "* && -n "${DOCKER_TEST_PROBE_STDERR:-}" ]]; then
    printf '%s\n' "${DOCKER_TEST_PROBE_STDERR}" >&2
    exit "${DOCKER_TEST_PROBE_EXIT_CODE:-23}"
  fi

  if [[ "$args" == *" mariadb --version "* ]]; then
    if [[ "${DOCKER_TEST_MARIADB_AVAILABLE:-false}" == "true" ]]; then
      echo "mariadb from 11.0.0"
      exit 0
    fi
    echo 'exec: "mariadb": executable file not found in $PATH' >&2
    exit 127
  fi

  if [[ "$args" == *" mysql --version "* ]]; then
    if [[ "${DOCKER_TEST_MYSQL_AVAILABLE:-false}" == "true" ]]; then
      echo "mysql from 8.0.0"
      exit 0
    fi
    echo 'exec: "mysql": executable file not found in $PATH' >&2
    exit 127
  fi

  if [[ "$args" == *" mariadb-dump --version "* ]]; then
    if [[ "${DOCKER_TEST_MARIADB_AVAILABLE:-false}" == "true" ]]; then
      echo "mariadb-dump from 11.0.0"
      exit 0
    fi
    echo 'exec: "mariadb-dump": executable file not found in $PATH' >&2
    exit 127
  fi

  if [[ "$args" == *" mysqldump --version "* ]]; then
    if [[ "${DOCKER_TEST_MYSQL_AVAILABLE:-false}" == "true" ]]; then
      echo "mysqldump from 8.0.0"
      exit 0
    fi
    echo 'exec: "mysqldump": executable file not found in $PATH' >&2
    exit 127
  fi

  if [[ "${2:-}" == "-i" ]]; then
    if [[ "$args" == *" mariadb-dump "* || "$args" == *" mysqldump "* ]]; then
      printf '%s' "${DOCKER_TEST_DUMP_STDOUT:-}"
      exit 0
    fi
    if [[ -n "${DOCKER_TEST_STDIN_LOG:-}" ]]; then
      cat >> "${DOCKER_TEST_STDIN_LOG}"
    else
      cat >/dev/null
    fi
    if [[ -n "${DOCKER_TEST_EXEC_STDERR:-}" ]]; then
      printf '%s\n' "${DOCKER_TEST_EXEC_STDERR}" >&2
    fi
    if [[ -n "${DOCKER_TEST_EXEC_EXIT_CODE:-}" ]]; then
      exit "${DOCKER_TEST_EXEC_EXIT_CODE}"
    fi
    exit 0
  fi
fi

if [[ "${1:-}" == "run" ]]; then
  shift
  entrypoint=""
  source_host=""
  target_host=""
  storage_host=""
  args=()
  image=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --pull=never|--rm|--user)
        if [[ "$1" == "--user" ]]; then
          shift 2
        else
          shift
        fi
        ;;
      --entrypoint)
        entrypoint="${2:-}"
        shift 2
        ;;
      -v)
        mount="${2:-}"
        case "$mount" in
          *:/archive-source:ro)
            source_host="${mount%%:/archive-source:ro}"
            ;;
          *:/archive-target)
            target_host="${mount%%:/archive-target}"
            ;;
          *:/espo-storage)
            storage_host="${mount%%:/espo-storage}"
            ;;
        esac
        shift 2
        ;;
      -e)
        shift 2
        ;;
      *)
        image="$1"
        shift
        args=("$@")
        break
        ;;
    esac
  done

  if [[ "$entrypoint" == "tar" ]]; then
    archive_arg=""
    source_base=""
    for ((i=0; i<${#args[@]}; i++)); do
      if [[ "${args[$i]}" == "-czf" ]]; then
        archive_arg="${args[$((i+1))]:-}"
      fi
    done
    source_base="${args[$((${#args[@]}-1))]:-}"
    archive_base="${archive_arg#/archive-target/}"
    tar -C "$source_host" -czf "$target_host/$archive_base" "$source_base"
    exit 0
  fi

  if [[ -n "$storage_host" ]]; then
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

  echo "unexpected docker run invocation: $image ${args[*]:-}" >&2
  exit 99
fi

echo "unexpected docker invocation: $*" >&2
exit 98
`
