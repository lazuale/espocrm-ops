package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestContract_ShowOperation_JSON(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "ok.json", map[string]any{
		"operation_id": "op-show-1",
		"command":      "restore-files",
		"started_at":   "2026-04-15T12:00:00Z",
		"finished_at":  "2026-04-15T12:00:01Z",
		"ok":           true,
		"dry_run":      true,
	})
	if err := os.WriteFile(filepath.Join(journalDir, "broken.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"show-operation",
		"--id", "op-show-1",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	if obj["command"] != "show-operation" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}

	details := obj["details"].(map[string]any)
	if int(details["total_files_seen"].(float64)) != 2 {
		t.Fatalf("expected total_files_seen=2, got %v", details["total_files_seen"])
	}
	if int(details["loaded_entries"].(float64)) != 1 {
		t.Fatalf("expected loaded_entries=1, got %v", details["loaded_entries"])
	}
	if int(details["skipped_corrupt"].(float64)) != 1 {
		t.Fatalf("expected skipped_corrupt=1, got %v", details["skipped_corrupt"])
	}
	if details["id"] != "op-show-1" {
		t.Fatalf("expected details.id=op-show-1, got %v", details["id"])
	}

	items := obj["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0].(map[string]any)
	if item["operation_id"] != "op-show-1" {
		t.Fatalf("unexpected operation_id: %v", item["operation_id"])
	}
	if item["command"] != "restore-files" {
		t.Fatalf("unexpected command: %v", item["command"])
	}
	if dryRun, _ := item["dry_run"].(bool); !dryRun {
		t.Fatalf("expected dry_run=true, got %v", item["dry_run"])
	}
}
