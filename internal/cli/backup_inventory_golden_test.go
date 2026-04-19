package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGolden_BackupCatalog_JSON(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-bc-golden"),
	}

	writeCLICatalogBackupSet(t, backupRoot, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeJournalEntryFile(t, journalDir, "update.json", map[string]any{
		"operation_id": "op-update-1",
		"command":      "update",
		"started_at":   "2026-04-07T01:05:00Z",
		"finished_at":  "2026-04-07T01:07:00Z",
		"ok":           true,
		"artifacts": map[string]any{
			"manifest_json": filepath.Join(backupRoot, "manifests", "espocrm-dev_2026-04-07_01-00-00.manifest.json"),
		},
	})

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"backup-catalog",
		"--backup-root", backupRoot,
		"--verify-checksum",
		"--latest-only",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	assertGoldenJSON(t, normalizeBackupInventoryJSON(t, []byte(out)), filepath.Join("testdata", "backup_catalog_ok.golden.json"))
}

func TestGolden_ShowBackup_JSON(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-sb-golden"),
	}

	writeCLICatalogBackupSet(t, backupRoot, "espocrm-prod", "2026-04-07_01-00-00", "prod")
	writeJournalEntryFile(t, journalDir, "rollback.json", map[string]any{
		"operation_id": "op-rollback-1",
		"command":      "rollback",
		"started_at":   "2026-04-07T01:10:00Z",
		"finished_at":  "2026-04-07T01:12:00Z",
		"ok":           true,
		"artifacts": map[string]any{
			"snapshot_manifest_json": filepath.Join(backupRoot, "manifests", "espocrm-prod_2026-04-07_01-00-00.manifest.json"),
		},
	})

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"show-backup",
		"--backup-root", backupRoot,
		"--id", "espocrm-prod_2026-04-07_01-00-00",
		"--verify-checksum",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	assertGoldenJSON(t, normalizeBackupInventoryJSON(t, []byte(out)), filepath.Join("testdata", "show_backup_ok.golden.json"))
}

func normalizeBackupInventoryJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	if details, ok := obj["details"].(map[string]any); ok {
		if _, exists := details["backup_root"]; exists {
			details["backup_root"] = "REPLACE_BACKUP_ROOT"
		}
	}

	if items, ok := obj["items"].([]any); ok {
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			normalizeBackupInventoryItem(item)
		}
	}

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func normalizeBackupInventoryItem(item map[string]any) {
	if db, ok := item["db"].(map[string]any); ok {
		db["file"] = "REPLACE_DB_BACKUP"
		if _, exists := db["sidecar"]; exists {
			db["sidecar"] = "REPLACE_DB_CHECKSUM"
		}
	}
	if files, ok := item["files"].(map[string]any); ok {
		files["file"] = "REPLACE_FILES_BACKUP"
		if _, exists := files["sidecar"]; exists {
			files["sidecar"] = "REPLACE_FILES_CHECKSUM"
		}
	}
	if manifestTXT, ok := item["manifest_txt"].(map[string]any); ok {
		manifestTXT["file"] = "REPLACE_MANIFEST_TXT"
	}
	if manifestJSON, ok := item["manifest_json"].(map[string]any); ok {
		manifestJSON["file"] = "REPLACE_MANIFEST_JSON"
	}
}

func normalizeBackupHealthJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	if details, ok := obj["details"].(map[string]any); ok {
		if _, exists := details["backup_root"]; exists {
			details["backup_root"] = "REPLACE_BACKUP_ROOT"
		}
	}

	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		if latestSet, ok := artifacts["latest_set"].(map[string]any); ok {
			normalizeBackupInventoryItem(latestSet)
		}
		if latestReadySet, ok := artifacts["latest_ready_set"].(map[string]any); ok {
			normalizeBackupInventoryItem(latestReadySet)
		}
	}

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return out
}
