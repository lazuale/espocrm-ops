package cli

import (
	"encoding/json"
	"testing"
)

func normalizeMigrateJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	replacements := map[string]string{}
	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		for key, placeholder := range map[string]string{
			"project_dir":            "REPLACE_PROJECT_DIR",
			"compose_file":           "REPLACE_COMPOSE_FILE",
			"source_env_file":        "REPLACE_SOURCE_ENV_FILE",
			"target_env_file":        "REPLACE_TARGET_ENV_FILE",
			"source_backup_root":     "REPLACE_SOURCE_BACKUP_ROOT",
			"target_backup_root":     "REPLACE_TARGET_BACKUP_ROOT",
			"manifest_txt":           "REPLACE_MANIFEST_TXT",
			"manifest_json":          "REPLACE_MANIFEST_JSON",
			"db_backup":              "REPLACE_DB_BACKUP",
			"files_backup":           "REPLACE_FILES_BACKUP",
			"requested_db_backup":    "REPLACE_REQUESTED_DB_BACKUP",
			"requested_files_backup": "REPLACE_REQUESTED_FILES_BACKUP",
		} {
			if value, ok := artifacts[key].(string); ok && value != "" {
				replacements[value] = placeholder
				artifacts[key] = placeholder
			}
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
