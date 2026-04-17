package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestContract_History_JSON_WithCorruptEntry(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "a.json", map[string]any{
		"operation_id": "op-1",
		"command":      "restore-db",
		"started_at":   "2026-04-15T10:00:00Z",
		"finished_at":  "2026-04-15T10:00:01Z",
		"ok":           true,
	})
	writeJournalEntryFile(t, journalDir, "b.json", map[string]any{
		"operation_id": "op-2",
		"command":      "restore-db",
		"started_at":   "2026-04-15T11:00:00Z",
		"finished_at":  "2026-04-15T11:00:01Z",
		"ok":           false,
		"error_code":   "restore_db_failed",
	})
	if err := os.WriteFile(filepath.Join(journalDir, "corrupt.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"history",
		"--failed-only",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}

	details := obj["details"].(map[string]any)
	if int(details["total_files_seen"].(float64)) != 3 {
		t.Fatalf("expected total_files_seen=3, got %v", details["total_files_seen"])
	}
	if int(details["loaded_entries"].(float64)) != 2 {
		t.Fatalf("expected loaded_entries=2, got %v", details["loaded_entries"])
	}
	if int(details["skipped_corrupt"].(float64)) != 1 {
		t.Fatalf("expected skipped_corrupt=1, got %v", details["skipped_corrupt"])
	}

	items := obj["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 failed item, got %d", len(items))
	}

	item := items[0].(map[string]any)
	if item["operation_id"] != "op-2" {
		t.Fatalf("expected op-2, got %v", item["operation_id"])
	}
}

func TestContract_History_JSON_TrimsCommandFilter(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "a.json", map[string]any{
		"operation_id": "op-1",
		"command":      "verify-backup",
		"started_at":   "2026-04-15T10:00:00Z",
		"ok":           true,
	})
	writeJournalEntryFile(t, journalDir, "b.json", map[string]any{
		"operation_id": "op-2",
		"command":      "restore-db",
		"started_at":   "2026-04-15T11:00:00Z",
		"ok":           true,
	})

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"history",
		"--command", " restore-db ",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	if command := requireJSONPath(t, obj, "details", "command"); command != "restore-db" {
		t.Fatalf("expected trimmed command filter, got %v", command)
	}

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(items))
	}
	item := items[0].(map[string]any)
	if item["operation_id"] != "op-2" {
		t.Fatalf("expected op-2, got %v", item["operation_id"])
	}
}
