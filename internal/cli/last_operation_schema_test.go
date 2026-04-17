package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSchema_LastOperation_JSON(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "a.json", map[string]any{
		"operation_id": "op-1",
		"command":      "verify-backup",
		"started_at":   "2026-04-15T10:00:00Z",
		"finished_at":  "2026-04-15T10:00:01Z",
		"ok":           true,
	})
	writeJournalEntryFile(t, journalDir, "b.json", map[string]any{
		"operation_id":  "op-2",
		"command":       "restore-db",
		"started_at":    "2026-04-15T11:00:00Z",
		"finished_at":   "2026-04-15T11:00:01Z",
		"ok":            false,
		"error_code":    "restore_db_failed",
		"error_message": "boom",
	})
	if err := os.WriteFile(filepath.Join(journalDir, "corrupt.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"last-operation",
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
	requireJSONPath(t, obj, "details", "total_files_seen")
	requireJSONPath(t, obj, "details", "loaded_entries")
	requireJSONPath(t, obj, "details", "skipped_corrupt")

	itemsAny := requireJSONPath(t, obj, "items")
	items, ok := itemsAny.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one item, got %#v", itemsAny)
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected item object, got %T", items[0])
	}

	requireJSONPath(t, item, "operation_id")
	requireJSONPath(t, item, "command")
	requireJSONPath(t, item, "started_at")

	if item["operation_id"] != "op-2" {
		t.Fatalf("expected latest op-2, got %v", item["operation_id"])
	}
}

func TestSchema_LastOperation_JSON_TrimsCommandFilter(t *testing.T) {
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

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"last-operation",
		"--command", " verify-backup ",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	if command := requireJSONPath(t, obj, "details", "command"); command != "verify-backup" {
		t.Fatalf("expected trimmed command filter, got %v", command)
	}
}
