package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchema_ShowBackup_JSON(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-sb-1"),
	}

	writeCLICatalogBackupSet(t, backupRoot, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeJournalEntryFile(t, journalDir, "backup.json", map[string]any{
		"operation_id": "op-backup-1",
		"command":      "backup",
		"started_at":   "2026-04-07T01:00:00Z",
		"finished_at":  "2026-04-07T01:01:00Z",
		"ok":           true,
		"artifacts": map[string]any{
			"manifest": filepath.Join(backupRoot, "manifests", "espocrm-dev_2026-04-07_01-00-00.manifest.json"),
		},
	})

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"show-backup",
		"--backup-root", backupRoot,
		"--id", "espocrm-dev_2026-04-07_01-00-00",
		"--verify-checksum",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "backup_root")
	requireJSONPath(t, obj, "details", "id")
	requireJSONPath(t, obj, "details", "verify_checksum")
	requireJSONPath(t, obj, "items")

	items, ok := obj["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one item, got %#v", obj["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object item, got %#v", items[0])
	}
	if item["id"] != "espocrm-dev_2026-04-07_01-00-00" {
		t.Fatalf("unexpected id: %v", item["id"])
	}
	if item["restore_readiness"] != "ready_verified" {
		t.Fatalf("unexpected readiness: %v", item["restore_readiness"])
	}
	origin, ok := item["origin"].(map[string]any)
	if !ok {
		t.Fatalf("expected origin object, got %#v", item["origin"])
	}
	if origin["kind"] != "normal_backup" {
		t.Fatalf("unexpected origin: %#v", origin)
	}
}
