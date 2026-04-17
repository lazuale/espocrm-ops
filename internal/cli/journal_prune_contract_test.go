package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestContract_JournalPrune_JSON_DryRun(t *testing.T) {
	tmp := t.TempDir()
	rootDir := filepath.Join(tmp, "journal")
	journalDir := filepath.Join(rootDir, "2026-04-15")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldFile := filepath.Join(journalDir, "old.json")
	newFile := filepath.Join(journalDir, "new.json")

	writeJournalEntryFile(t, journalDir, "old.json", map[string]any{
		"operation_id": "op-old",
		"command":      "verify-backup",
		"started_at":   "2026-03-01T10:00:00Z",
		"finished_at":  "2026-03-01T10:00:01Z",
		"ok":           true,
	})
	writeJournalEntryFile(t, journalDir, "new.json", map[string]any{
		"operation_id": "op-new",
		"command":      "verify-backup",
		"started_at":   "2026-04-15T10:00:00Z",
		"finished_at":  "2026-04-15T10:00:01Z",
		"ok":           true,
	})

	out, err := runRootCommand(t,
		"--journal-dir", rootDir,
		"--json",
		"journal-prune",
		"--keep", "1",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	if obj["command"] != "journal-prune" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}

	details := obj["details"].(map[string]any)
	if int(details["total_files_seen"].(float64)) != 2 {
		t.Fatalf("expected total_files_seen=2, got %v", details["total_files_seen"])
	}
	if int(details["loaded_entries"].(float64)) != 2 {
		t.Fatalf("expected loaded_entries=2, got %v", details["loaded_entries"])
	}
	if int(details["deleted"].(float64)) != 1 {
		t.Fatalf("expected deleted=1, got %v", details["deleted"])
	}
	if int(details["removed_dirs"].(float64)) != 0 {
		t.Fatalf("expected removed_dirs=0 in dry-run, got %v", details["removed_dirs"])
	}
	if dryRun, _ := details["dry_run"].(bool); !dryRun {
		t.Fatalf("expected details.dry_run=true, got %v", details["dry_run"])
	}

	items := obj["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 planned deletion, got %d", len(items))
	}

	item := items[0].(map[string]any)
	if item["type"] != "file" {
		t.Fatalf("expected item type=file, got %v", item["type"])
	}
	if item["path"] != oldFile {
		t.Fatalf("expected planned deletion for %s, got %v", oldFile, item["path"])
	}

	if _, err := os.Stat(oldFile); err != nil {
		t.Fatalf("old file should still exist in dry-run: %v", err)
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatalf("new file should still exist in dry-run: %v", err)
	}
}
