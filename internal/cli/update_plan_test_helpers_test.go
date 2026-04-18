package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func prependFakeDockerForUpdatePlanCLITest(t *testing.T) {
	t.Helper()

	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	script := `#!/usr/bin/env bash
set -Eeuo pipefail

state_dir="${DOCKER_MOCK_PLAN_STATE_DIR:-}"

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
        if [[ -n "${DOCKER_MOCK_PLAN_CONFIG_ERROR:-}" ]]; then
          echo "${DOCKER_MOCK_PLAN_CONFIG_ERROR}" >&2
          exit 23
        fi
        exit 0
        ;;
      ps)
        shift
        if [[ "${1:-}" == "--status" && "${2:-}" == "running" && "${3:-}" == "--services" ]]; then
          if [[ -f "$state_dir/running-services" ]]; then
            cat "$state_dir/running-services"
          fi
          exit 0
        fi
        if [[ "${1:-}" == "-q" ]]; then
          service="${2:-}"
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
        continue
        ;;
    esac
    shift
  done
fi

if [[ "${1:-}" == "inspect" ]]; then
  if [[ "$*" == *".State.Health.Log"* ]]; then
    printf '%s\n' "${DOCKER_MOCK_PLAN_HEALTH_MESSAGE:-mock health failure}"
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

echo "unexpected docker args: $*" >&2
exit 98
`

	path := filepath.Join(binDir, "docker")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func normalizeUpdatePlanJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	replacements := map[string]string{}
	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		if value, ok := artifacts["project_dir"].(string); ok && value != "" {
			replacements[value] = "REPLACE_PROJECT_DIR"
			artifacts["project_dir"] = "REPLACE_PROJECT_DIR"
		}
		if value, ok := artifacts["compose_file"].(string); ok && value != "" {
			replacements[value] = "REPLACE_COMPOSE_FILE"
			artifacts["compose_file"] = "REPLACE_COMPOSE_FILE"
		}
		if value, ok := artifacts["env_file"].(string); ok && value != "" {
			replacements[value] = "REPLACE_ENV_FILE"
			artifacts["env_file"] = "REPLACE_ENV_FILE"
		}
		if value, ok := artifacts["backup_root"].(string); ok && value != "" {
			replacements[value] = "REPLACE_BACKUP_ROOT"
			artifacts["backup_root"] = "REPLACE_BACKUP_ROOT"
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

func replaceKnownPaths(text string, replacements map[string]string) string {
	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

	out := text
	for _, key := range keys {
		out = strings.ReplaceAll(out, key, replacements[key])
	}

	return out
}
