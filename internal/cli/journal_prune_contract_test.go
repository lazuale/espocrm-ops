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
	middleFile := filepath.Join(journalDir, "middle.json")
	newFile := filepath.Join(journalDir, "new.json")

	writeJournalEntryFile(t, journalDir, "old.json", map[string]any{
		"operation_id": "op-old",
		"command":      "verify-backup",
		"started_at":   "2026-03-01T10:00:00Z",
		"finished_at":  "2026-03-01T10:00:01Z",
		"ok":           true,
	})
	writeJournalEntryFile(t, journalDir, "middle.json", map[string]any{
		"operation_id": "op-middle",
		"command":      "verify-backup",
		"started_at":   "2026-04-10T10:00:00Z",
		"finished_at":  "2026-04-10T10:00:01Z",
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
		"--keep-last", "2",
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
	if int(details["total_files_seen"].(float64)) != 3 {
		t.Fatalf("expected total_files_seen=3, got %v", details["total_files_seen"])
	}
	if int(details["loaded_entries"].(float64)) != 3 {
		t.Fatalf("expected loaded_entries=3, got %v", details["loaded_entries"])
	}
	if int(details["checked"].(float64)) != 3 {
		t.Fatalf("expected checked=3, got %v", details["checked"])
	}
	if int(details["retained"].(float64)) != 2 {
		t.Fatalf("expected retained=2, got %v", details["retained"])
	}
	if int(details["protected"].(float64)) != 1 {
		t.Fatalf("expected protected=1, got %v", details["protected"])
	}
	if int(details["deleted"].(float64)) != 1 {
		t.Fatalf("expected deleted=1, got %v", details["deleted"])
	}
	if int(details["removed_dirs"].(float64)) != 0 {
		t.Fatalf("expected removed_dirs=0 in dry-run, got %v", details["removed_dirs"])
	}
	if int(details["keep_last"].(float64)) != 2 {
		t.Fatalf("expected keep_last=2, got %v", details["keep_last"])
	}
	if details["latest_operation_id"] != "op-new" {
		t.Fatalf("expected latest_operation_id=op-new, got %v", details["latest_operation_id"])
	}
	if dryRun, _ := details["dry_run"].(bool); !dryRun {
		t.Fatalf("expected details.dry_run=true, got %v", details["dry_run"])
	}

	items := obj["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 operation decisions, got %d", len(items))
	}

	protected := items[0].(map[string]any)
	if protected["kind"] != "operation" {
		t.Fatalf("expected protected item kind=operation, got %v", protected["kind"])
	}
	if protected["decision"] != "protect" {
		t.Fatalf("expected protected item decision=protect, got %v", protected["decision"])
	}
	if protected["path"] != newFile {
		t.Fatalf("expected protected latest operation path %s, got %v", newFile, protected["path"])
	}
	protectedOperation := protected["operation"].(map[string]any)
	if protectedOperation["operation_id"] != "op-new" {
		t.Fatalf("expected protected latest operation id op-new, got %v", protectedOperation["operation_id"])
	}

	kept := items[1].(map[string]any)
	if kept["decision"] != "keep" {
		t.Fatalf("expected middle item decision=keep, got %v", kept["decision"])
	}
	if kept["path"] != middleFile {
		t.Fatalf("expected kept middle operation path %s, got %v", middleFile, kept["path"])
	}

	removed := items[2].(map[string]any)
	if removed["decision"] != "remove" {
		t.Fatalf("expected oldest item decision=remove, got %v", removed["decision"])
	}
	if removed["path"] != oldFile {
		t.Fatalf("expected planned deletion for %s, got %v", oldFile, removed["path"])
	}
	removedReasons := removed["reasons"].([]any)
	if len(removedReasons) != 1 || removedReasons[0] != "outside_keep_last" {
		t.Fatalf("expected outside_keep_last reason, got %v", removed["reasons"])
	}

	if _, err := os.Stat(oldFile); err != nil {
		t.Fatalf("old file should still exist in dry-run: %v", err)
	}
	if _, err := os.Stat(middleFile); err != nil {
		t.Fatalf("middle file should still exist in dry-run: %v", err)
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatalf("new file should still exist in dry-run: %v", err)
	}
}
