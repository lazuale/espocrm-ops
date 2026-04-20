package cli

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeDoctorEnvFile(t *testing.T, projectDir, scope string, overrides map[string]string) string {
	t.Helper()

	values := map[string]string{
		"ESPO_CONTOUR":               scope,
		"COMPOSE_PROJECT_NAME":       "espocrm-" + scope,
		"ESPOCRM_IMAGE":              "espocrm/espocrm:9.3.4-apache",
		"MARIADB_TAG":                "10.11",
		"DB_STORAGE_DIR":             "./runtime/" + scope + "/db",
		"ESPO_STORAGE_DIR":           "./runtime/" + scope + "/espo",
		"BACKUP_ROOT":                "./backups/" + scope,
		"BACKUP_NAME_PREFIX":         "espocrm-" + scope,
		"BACKUP_RETENTION_DAYS":      "7",
		"BACKUP_MAX_DB_AGE_HOURS":    "48",
		"BACKUP_MAX_FILES_AGE_HOURS": "48",
		"REPORT_RETENTION_DAYS":      "30",
		"SUPPORT_RETENTION_DAYS":     "14",
		"MIN_FREE_DISK_MB":           "1",
		"DOCKER_LOG_MAX_SIZE":        "10m",
		"DOCKER_LOG_MAX_FILE":        "5",
		"DB_MEM_LIMIT":               "512m",
		"DB_CPUS":                    "1.00",
		"DB_PIDS_LIMIT":              "256",
		"ESPO_MEM_LIMIT":             "512m",
		"ESPO_CPUS":                  "1.00",
		"ESPO_PIDS_LIMIT":            "256",
		"DAEMON_MEM_LIMIT":           "256m",
		"DAEMON_CPUS":                "0.50",
		"DAEMON_PIDS_LIMIT":          "128",
		"WS_MEM_LIMIT":               "256m",
		"WS_CPUS":                    "0.50",
		"WS_PIDS_LIMIT":              "128",
		"APP_PORT":                   "18080",
		"WS_PORT":                    "18081",
		"SITE_URL":                   "http://127.0.0.1:18080",
		"WS_PUBLIC_URL":              "ws://127.0.0.1:18081",
		"DB_ROOT_PASSWORD":           "root-secret",
		"DB_NAME":                    "espocrm",
		"DB_USER":                    "espocrm",
		"DB_PASSWORD":                "db-secret",
		"ADMIN_USERNAME":             "admin",
		"ADMIN_PASSWORD":             "admin-secret",
		"ESPO_DEFAULT_LANGUAGE":      "ru_RU",
		"ESPO_TIME_ZONE":             "Europe/Moscow",
		"ESPO_LOGGER_LEVEL":          "INFO",
	}
	maps.Copy(values, overrides)

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+values[key])
	}

	path := filepath.Join(projectDir, ".env."+scope)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func prependDoctorFakeDocker(t *testing.T) {
	t.Helper()

	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	script := `#!/usr/bin/env bash
set -Eeuo pipefail

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
        ;;
      config)
        exit 0
        ;;
      -q)
        shift
        ;;
      ps)
        shift
        while [[ $# -gt 0 ]]; do
          case "$1" in
            --status)
              shift 2
              ;;
            --services|-q)
              exit 0
              ;;
            *)
              shift
              ;;
          esac
        done
        exit 0
        ;;
      *)
        shift
        ;;
    esac
  done
fi

echo "unexpected docker args: $*" >&2
exit 97
`

	path := filepath.Join(binDir, "docker")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func normalizeDoctorJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		if _, ok := artifacts["project_dir"]; ok {
			artifacts["project_dir"] = "REPLACE_PROJECT_DIR"
		}
		if _, ok := artifacts["compose_file"]; ok {
			artifacts["compose_file"] = "REPLACE_COMPOSE_FILE"
		}
		if scopes, ok := artifacts["scopes"].([]any); ok {
			for _, rawScope := range scopes {
				scope, ok := rawScope.(map[string]any)
				if !ok {
					continue
				}
				if _, ok := scope["env_file"]; ok {
					scope["env_file"] = "REPLACE_ENV_FILE"
				}
				if _, ok := scope["backup_root"]; ok {
					scope["backup_root"] = "REPLACE_BACKUP_ROOT"
				}
			}
		}
	}

	if items, ok := obj["items"].([]any); ok {
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			code, _ := item["code"].(string)
			switch code {
			case "compose_file":
				item["details"] = "REPLACE_COMPOSE_FILE"
			case "shared_operation_lock":
				item["details"] = "REPLACE_SHARED_LOCK"
			case "env_resolution", "env_contract":
				item["details"] = "REPLACE_ENV_FILE"
			case "db_storage_dir":
				item["details"] = "REPLACE_DB_STORAGE_DIR"
			case "espo_storage_dir":
				item["details"] = "REPLACE_ESPO_STORAGE_DIR"
			case "backup_root":
				item["details"] = "REPLACE_BACKUP_ROOT"
			case "contour_operation_lock":
				item["details"] = "REPLACE_MAINTENANCE_LOCK"
			case "compose_config":
				item["details"] = "compose file REPLACE_COMPOSE_FILE with env REPLACE_ENV_FILE"
			}
		}
	}

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return out
}
