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

func TestSchema_LastOperation_JSON_Report(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "update.json", map[string]any{
		"operation_id":  "op-update-3",
		"command":       "update",
		"started_at":    "2026-04-18T12:00:00Z",
		"finished_at":   "2026-04-18T12:00:05Z",
		"ok":            false,
		"message":       "update failed",
		"error_code":    "update_failed",
		"error_message": "doctor found readiness failures",
		"details": map[string]any{
			"scope": "prod",
		},
		"artifacts": map[string]any{
			"project_dir":  "/srv/espocrm",
			"compose_file": "/srv/espocrm/compose.yaml",
			"env_file":     "/srv/espocrm/.env.prod",
		},
		"items": []map[string]any{
			{
				"code":    "doctor",
				"status":  "failed",
				"summary": "Doctor stopped the update",
				"details": "db is unhealthy",
				"action":  "Resolve the reported doctor failures before rerunning update.",
			},
			{
				"code":    "runtime_apply",
				"status":  "not_run",
				"summary": "Runtime apply did not run because doctor failed",
			},
		},
	})

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

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) != 1 {
		t.Fatalf("expected one item, got %#v", items)
	}

	report := items[0].(map[string]any)
	counts := requireJSONPath(t, report, "counts").(map[string]any)
	if int(counts["failed"].(float64)) != 1 {
		t.Fatalf("unexpected failed count: %v", counts["failed"])
	}
	if int(counts["blocked"].(float64)) != 1 {
		t.Fatalf("unexpected blocked count: %v", counts["blocked"])
	}
	failure := requireJSONPath(t, report, "failure").(map[string]any)
	if failure["step_code"] != "doctor" {
		t.Fatalf("unexpected failure attribution: %#v", failure)
	}
	recovery := requireJSONPath(t, report, "recovery").(map[string]any)
	if recovery["decision"] != "retry_from_start" {
		t.Fatalf("unexpected recovery decision: %#v", recovery)
	}
	steps := requireJSONPath(t, report, "steps").([]any)
	if second := steps[1].(map[string]any); second["status"] != "blocked" {
		t.Fatalf("expected blocked downstream step, got %#v", second)
	}
}
