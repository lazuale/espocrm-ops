package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type restoreDrillFixture struct {
	restoreCommandFixture
	incompleteDBBackup string
}

func prepareRestoreDrillFixture(t *testing.T, scope string, files map[string]string) restoreDrillFixture {
	t.Helper()

	base := prepareRestoreCommandFixture(t, scope, files)
	incompleteStamp := "2026-04-19_08-30-00"
	incompleteDBBackup := filepath.Join(base.backupRoot, "db", "espocrm-"+scope+"_"+incompleteStamp+".sql.gz")
	writeGzipFile(t, incompleteDBBackup, []byte("select 2;"))
	if err := os.WriteFile(incompleteDBBackup+".sha256", []byte(sha256OfFile(t, incompleteDBBackup)+"  "+filepath.Base(incompleteDBBackup)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return restoreDrillFixture{
		restoreCommandFixture: base,
		incompleteDBBackup:    incompleteDBBackup,
	}
}

func normalizeRestoreDrillJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	replacements := map[string]string{}
	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		for key, placeholder := range map[string]string{
			"project_dir":        "REPLACE_PROJECT_DIR",
			"compose_file":       "REPLACE_COMPOSE_FILE",
			"source_env_file":    "REPLACE_SOURCE_ENV_FILE",
			"source_backup_root": "REPLACE_SOURCE_BACKUP_ROOT",
			"manifest_txt":       "REPLACE_MANIFEST_TXT",
			"manifest_json":      "REPLACE_MANIFEST_JSON",
			"db_backup":          "REPLACE_DB_BACKUP",
			"files_backup":       "REPLACE_FILES_BACKUP",
			"drill_env_file":     "REPLACE_DRILL_ENV_FILE",
			"drill_backup_root":  "REPLACE_DRILL_BACKUP_ROOT",
			"drill_db_storage":   "REPLACE_DRILL_DB_STORAGE",
			"drill_espo_storage": "REPLACE_DRILL_ESPO_STORAGE",
			"report_txt":         "REPLACE_REPORT_TXT",
			"report_json":        "REPLACE_REPORT_JSON",
			"site_url":           "REPLACE_SITE_URL",
			"ws_public_url":      "REPLACE_WS_PUBLIC_URL",
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
