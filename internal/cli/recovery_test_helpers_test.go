package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func prependFakeDockerForRecoveryCLITest(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/usr/bin/env bash
set -Eeuo pipefail

state_dir="${DOCKER_MOCK_RECOVERY_STATE_DIR:-}"

log_call() {
  if [[ -n "${DOCKER_MOCK_RECOVERY_LOG:-}" ]]; then
    printf '%s\n' "$*" >> "${DOCKER_MOCK_RECOVERY_LOG}"
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
			if [[ -n "${DOCKER_MOCK_RECOVERY_CONFIG_ERROR:-}" ]]; then
				echo "${DOCKER_MOCK_RECOVERY_CONFIG_ERROR}" >&2
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
    printf '%s\n' "${DOCKER_MOCK_RECOVERY_HEALTH_MESSAGE:-mock health failure}"
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
    printf '%s' "${DOCKER_MOCK_RECOVERY_DUMP_STDOUT:-select 1;}"
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

  printf '%s:%s\n' "${DOCKER_MOCK_RESTORE_RUNTIME_UID:-$(id -u)}" "${DOCKER_MOCK_RESTORE_RUNTIME_GID:-$(id -g)}"
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

func isolateRecoveryLocks(t *testing.T) testAppOption {
	t.Helper()

	return withRestoreLockDir(t.TempDir())
}

func writeBackupSet(t *testing.T, backupRoot, prefix, stamp, scope string) {
	t.Helper()

	dbPath := filepath.Join(backupRoot, "db", prefix+"_"+stamp+".sql.gz")
	filesPath := filepath.Join(backupRoot, "files", prefix+"_files_"+stamp+".tar.gz")
	manifestTXT := filepath.Join(backupRoot, "manifests", prefix+"_"+stamp+".manifest.txt")
	manifestJSON := filepath.Join(backupRoot, "manifests", prefix+"_"+stamp+".manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestJSON), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGzFile(t, filesPath, map[string]string{
		"espo/test.txt": "hello",
	})
	if err := os.WriteFile(dbPath+".sha256", []byte(sha256OfFile(t, dbPath)+"  "+filepath.Base(dbPath)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filesPath+".sha256", []byte(sha256OfFile(t, filesPath)+"  "+filepath.Base(filesPath)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestTXT, []byte("created_at=2026-04-18T10:00:00Z\ncontour="+scope+"\ncompose_project="+prefix+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, manifestJSON, map[string]any{
		"version":    1,
		"scope":      scope,
		"created_at": "2026-04-18T10:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(dbPath),
			"files_backup": filepath.Base(filesPath),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})
}

func writeRuntimeStatusFile(t *testing.T, stateDir, service string, statuses ...string) {
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
