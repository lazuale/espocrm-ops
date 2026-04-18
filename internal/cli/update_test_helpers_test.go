package cli

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func prependFakeDockerForUpdateCLITest(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/usr/bin/env bash
set -Eeuo pipefail

args=" $* "
state_dir="${DOCKER_MOCK_UPDATE_STATE_DIR:-}"

if [[ -n "${DOCKER_MOCK_UPDATE_LOG:-}" ]]; then
  printf '%s\n' "$*" >> "${DOCKER_MOCK_UPDATE_LOG}"
fi

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

if [[ "${1:-}" == "compose" && "$args" == *" config "* ]]; then
  if [[ -n "${DOCKER_MOCK_UPDATE_CONFIG_ERROR:-}" ]]; then
    echo "${DOCKER_MOCK_UPDATE_CONFIG_ERROR}" >&2
    exit 23
  fi
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" pull "* ]]; then
  exit 0
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
  if [[ -f "$state_dir/${service}.statuses" ]]; then
    echo "mock-${service}"
    exit 0
  fi
  if [[ -f "$state_dir/running-services" ]] && grep -qx "$service" "$state_dir/running-services"; then
    echo "mock-${service}"
    exit 0
  fi
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

func normalizeUpdateJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	replacements := map[string]string{}
	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		for key, placeholder := range map[string]string{
			"project_dir":    "REPLACE_PROJECT_DIR",
			"compose_file":   "REPLACE_COMPOSE_FILE",
			"env_file":       "REPLACE_ENV_FILE",
			"backup_root":    "REPLACE_BACKUP_ROOT",
			"site_url":       "REPLACE_SITE_URL",
			"manifest_txt":   "REPLACE_MANIFEST_TXT",
			"manifest_json":  "REPLACE_MANIFEST_JSON",
			"db_backup":      "REPLACE_DB_BACKUP",
			"files_backup":   "REPLACE_FILES_BACKUP",
			"db_checksum":    "REPLACE_DB_CHECKSUM",
			"files_checksum": "REPLACE_FILES_CHECKSUM",
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

func freeTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = listener.Close()
	}()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address type %T", listener.Addr())
	}

	return addr.Port
}
