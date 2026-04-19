package cli

import (
	"encoding/json"
	"testing"
)

func normalizeOverviewJSON(t *testing.T, raw []byte, fixture supportBundleFixture) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	replacements := map[string]string{
		fixture.projectDir:                   "REPLACE_PROJECT_DIR",
		fixture.projectDir + "/compose.yaml": "REPLACE_COMPOSE_FILE",
		fixture.projectDir + "/.env.dev":     "REPLACE_ENV_FILE",
		fixture.projectDir + "/.env.prod":    "REPLACE_ENV_FILE",
		fixture.backupRoot:                   "REPLACE_BACKUP_ROOT",
	}

	if items, ok := obj["items"].([]any); ok {
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok || item["code"] != "runtime" {
				continue
			}
			runtimeData, ok := item["runtime"].(map[string]any)
			if !ok {
				continue
			}
			if lock, ok := runtimeData["shared_operation_lock"].(map[string]any); ok {
				if metadata, ok := lock["metadata_path"].(string); ok && metadata != "" {
					replacements[metadata] = "REPLACE_SHARED_OPERATION_LOCK"
				}
			}
			if lock, ok := runtimeData["maintenance_lock"].(map[string]any); ok {
				if metadata, ok := lock["metadata_path"].(string); ok && metadata != "" {
					replacements[metadata] = "REPLACE_MAINTENANCE_LOCK"
				}
			}
		}
	}

	obj = normalizeOverviewValue(obj, replacements).(map[string]any)

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func normalizeOverviewValue(value any, replacements map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if key == "age_hours" {
				typed[key] = float64(0)
				continue
			}
			typed[key] = normalizeOverviewValue(item, replacements)
		}
		return typed
	case []any:
		for idx, item := range typed {
			typed[idx] = normalizeOverviewValue(item, replacements)
		}
		return typed
	case string:
		return replaceKnownPaths(typed, replacements)
	default:
		return value
	}
}
