package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestSchema_BackupAudit_JSON_FailureDetails(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-ba-1"),
	}

	writeCLICatalogBackupSet(t, backupRoot, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeGzipFile(t, filepath.Join(backupRoot, "db", "espocrm-dev_2026-04-07_02-00-00.sql.gz"), []byte("select 2;"))

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"backup-audit",
		"--backup-root", backupRoot,
		"--max-db-age-hours", "999",
		"--max-files-age-hours", "999",
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
	requireJSONPath(t, obj, "details", "success")
	requireJSONPath(t, obj, "details", "selected_stamp")
	requireJSONPath(t, obj, "details", "failure_findings")
	requireJSONPath(t, obj, "artifacts", "db_backup", "status")
	requireJSONPath(t, obj, "artifacts", "files_backup", "status")
	requireJSONPath(t, obj, "items")

	if obj["command"] != "backup-audit" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true for structured audit result, got %v", obj["ok"])
	}
	if success, _ := requireJSONPath(t, obj, "details", "success").(bool); success {
		t.Fatalf("expected details.success=false")
	}
	items, ok := obj["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected audit findings, got %#v", obj["items"])
	}
}
