package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func prependFakeDockerForRollbackCLITest(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/usr/bin/env bash
set -Eeuo pipefail

state_dir="${DOCKER_MOCK_ROLLBACK_STATE_DIR:-}"

log_call() {
  if [[ -n "${DOCKER_MOCK_ROLLBACK_LOG:-}" ]]; then
    printf '%s\n' "$*" >> "${DOCKER_MOCK_ROLLBACK_LOG}"
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
			if [[ -n "${DOCKER_MOCK_ROLLBACK_CONFIG_ERROR:-}" ]]; then
				echo "${DOCKER_MOCK_ROLLBACK_CONFIG_ERROR}" >&2
				exit 23
			fi
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
    printf '%s\n' "${DOCKER_MOCK_ROLLBACK_HEALTH_MESSAGE:-mock health failure}"
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
    printf '%s' "${DOCKER_MOCK_ROLLBACK_DUMP_STDOUT:-select 1;}"
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

func normalizeRollbackJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	replacements := map[string]string{}
	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		for key, placeholder := range map[string]string{
			"project_dir":             "REPLACE_PROJECT_DIR",
			"compose_file":            "REPLACE_COMPOSE_FILE",
			"env_file":                "REPLACE_ENV_FILE",
			"backup_root":             "REPLACE_BACKUP_ROOT",
			"site_url":                "REPLACE_SITE_URL",
			"manifest_txt":            "REPLACE_MANIFEST_TXT",
			"manifest_json":           "REPLACE_MANIFEST_JSON",
			"db_backup":               "REPLACE_DB_BACKUP",
			"files_backup":            "REPLACE_FILES_BACKUP",
			"snapshot_manifest_txt":   "REPLACE_SNAPSHOT_MANIFEST_TXT",
			"snapshot_manifest_json":  "REPLACE_SNAPSHOT_MANIFEST_JSON",
			"snapshot_db_backup":      "REPLACE_SNAPSHOT_DB_BACKUP",
			"snapshot_files_backup":   "REPLACE_SNAPSHOT_FILES_BACKUP",
			"snapshot_db_checksum":    "REPLACE_SNAPSHOT_DB_CHECKSUM",
			"snapshot_files_checksum": "REPLACE_SNAPSHOT_FILES_CHECKSUM",
		} {
			value, ok := artifacts[key].(string)
			if !ok || value == "" {
				continue
			}
			replacements[value] = placeholder
			artifacts[key] = placeholder
		}
	}

	if warnings, ok := obj["warnings"].([]any); ok {
		for idx, rawWarning := range warnings {
			warning, ok := rawWarning.(string)
			if !ok {
				continue
			}
			warnings[idx] = replaceKnownPaths(warning, replacements)
		}
	}

	if items, ok := obj["items"].([]any); ok {
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			if value, ok := item["details"].(string); ok {
				item["details"] = replaceKnownPaths(value, replacements)
			}
			if value, ok := item["action"].(string); ok {
				item["action"] = replaceKnownPaths(value, replacements)
			}
		}
	}

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return out
}
