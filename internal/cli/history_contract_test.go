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
		"message":      "restore failed",
		"items": []any{
			map[string]any{
				"code":    "restore",
				"status":  "failed",
				"summary": "Database restore failed",
			},
		},
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
	if returned := int(details["returned"].(float64)); returned != 1 {
		t.Fatalf("expected returned=1, got %v", details["returned"])
	}
	if recentFirst, _ := details["recent_first"].(bool); !recentFirst {
		t.Fatalf("expected recent_first=true, got %v", details["recent_first"])
	}

	items := obj["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 failed item, got %d", len(items))
	}

	item := items[0].(map[string]any)
	if item["operation_id"] != "op-2" {
		t.Fatalf("expected op-2, got %v", item["operation_id"])
	}
	if item["status"] != "failed" {
		t.Fatalf("expected failed status, got %v", item["status"])
	}
	if item["summary"] != "Database restore failed" {
		t.Fatalf("expected summary, got %v", item["summary"])
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
	if returned := requireJSONPath(t, obj, "details", "returned"); returned != float64(1) {
		t.Fatalf("expected returned=1, got %v", returned)
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

func TestContract_History_JSON_FiltersByStatusScopeAndRecovery(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "a.json", map[string]any{
		"operation_id": "op-1",
		"command":      "rollback",
		"started_at":   "2026-04-15T12:00:00Z",
		"finished_at":  "2026-04-15T12:00:05Z",
		"ok":           true,
		"message":      "rollback recovery completed",
		"details": map[string]any{
			"scope":          "prod",
			"selection_mode": "auto_latest_valid",
			"recovery": map[string]any{
				"source_operation_id": "op-source",
				"requested_mode":      "auto",
				"applied_mode":        "resume",
				"decision":            "resume_from_checkpoint",
				"resume_step":         "runtime_prepare",
			},
		},
		"artifacts": map[string]any{
			"selected_prefix": "espocrm-prod",
			"selected_stamp":  "2026-04-15_11-00-00",
		},
		"items": []any{
			map[string]any{
				"code":    "runtime_return",
				"status":  "completed",
				"summary": "Contour return completed",
			},
		},
	})
	writeJournalEntryFile(t, journalDir, "b.json", map[string]any{
		"operation_id": "op-2",
		"command":      "update",
		"started_at":   "2026-04-15T11:00:00Z",
		"finished_at":  "2026-04-15T11:00:01Z",
		"ok":           false,
		"details": map[string]any{
			"scope": "stage",
		},
		"items": []any{
			map[string]any{
				"code":    "runtime_apply",
				"status":  "failed",
				"summary": "Runtime apply failed",
			},
		},
		"error_code": "update_runtime_failed",
	})

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"history",
		"--status", "completed",
		"--scope", "prod",
		"--recovery-only",
		"--target-prefix", "espocrm-prod",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	if requireJSONPath(t, obj, "details", "status") != "completed" {
		t.Fatalf("expected status filter in details, got %#v", obj["details"])
	}
	if requireJSONPath(t, obj, "details", "scope") != "prod" {
		t.Fatalf("expected scope filter in details, got %#v", obj["details"])
	}
	if requireJSONPath(t, obj, "details", "target_prefix") != "espocrm-prod" {
		t.Fatalf("expected target_prefix filter in details, got %#v", obj["details"])
	}
	if requireJSONPath(t, obj, "details", "recovery_only") != true {
		t.Fatalf("expected recovery_only=true, got %#v", obj["details"])
	}

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(items))
	}
	item := items[0].(map[string]any)
	if item["operation_id"] != "op-1" {
		t.Fatalf("expected op-1, got %v", item["operation_id"])
	}
	if item["recovery_run"] != true {
		t.Fatalf("expected recovery_run=true, got %v", item["recovery_run"])
	}
	target := item["target"].(map[string]any)
	if target["prefix"] != "espocrm-prod" {
		t.Fatalf("expected target prefix, got %v", target["prefix"])
	}
	recovery := item["recovery_attempt"].(map[string]any)
	if recovery["source_operation_id"] != "op-source" {
		t.Fatalf("expected recovery source, got %v", recovery["source_operation_id"])
	}
}
