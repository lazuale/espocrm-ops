package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func normalizeStatusReportJSON(t *testing.T, raw []byte, fixture supportBundleFixture) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	replacements := map[string]string{
		fixture.projectDir: "REPLACE_PROJECT_DIR",
		filepath.Join(fixture.projectDir, "compose.yaml"):           "REPLACE_COMPOSE_FILE",
		filepath.Join(fixture.projectDir, ".env.dev"):               "REPLACE_ENV_FILE",
		filepath.Join(fixture.projectDir, ".env.prod"):              "REPLACE_ENV_FILE",
		filepath.Join(fixture.projectDir, "runtime", "dev", "db"):   "REPLACE_DB_STORAGE_DIR",
		filepath.Join(fixture.projectDir, "runtime", "dev", "espo"): "REPLACE_ESPO_STORAGE_DIR",
		fixture.backupRoot:                           "REPLACE_BACKUP_ROOT",
		filepath.Join(fixture.backupRoot, "reports"): "REPLACE_REPORTS_DIR",
		filepath.Join(fixture.backupRoot, "support"): "REPLACE_SUPPORT_DIR",
		fixture.backupSet.DBBackup:                   "REPLACE_DB_BACKUP",
		fixture.backupSet.DBBackup + ".sha256":       "REPLACE_DB_CHECKSUM",
		fixture.backupSet.FilesBackup:                "REPLACE_FILES_BACKUP",
		fixture.backupSet.FilesBackup + ".sha256":    "REPLACE_FILES_CHECKSUM",
		fixture.backupSet.ManifestJSON:               "REPLACE_MANIFEST_JSON",
		fixture.backupSet.ManifestTXT:                "REPLACE_MANIFEST_TXT",
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

	obj = normalizeStatusReportValue(obj, replacements).(map[string]any)

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func normalizeStatusReportValue(value any, replacements map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			switch key {
			case "modified_at":
				typed[key] = "REPLACE_MODIFIED_AT"
				continue
			case "age_hours":
				typed[key] = float64(0)
				continue
			}
			typed[key] = normalizeStatusReportValue(item, replacements)
		}
		return typed
	case []any:
		for idx, item := range typed {
			typed[idx] = normalizeStatusReportValue(item, replacements)
		}
		return typed
	case string:
		return replaceKnownPaths(typed, replacements)
	default:
		return value
	}
}
