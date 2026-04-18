package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestContract_ExportOperation_JSON_WritesBundle(t *testing.T) {
	useJournalClockForTest(t, time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))

	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	outputDir := filepath.Join(tmp, "exports")
	outputPath := filepath.Join(outputDir, "op-export-1.bundle.json")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "rollback.json", map[string]any{
		"operation_id":  "op-export-1",
		"command":       "rollback",
		"started_at":    "2026-04-19T10:00:00Z",
		"finished_at":   "2026-04-19T10:00:04Z",
		"ok":            false,
		"message":       "rollback failed",
		"error_code":    "rollback_failed",
		"error_message": "rollback target selection failed",
		"warnings":      []string{"Rollback skipped the final HTTP probe."},
		"details": map[string]any{
			"scope":          "prod",
			"selection_mode": "auto_latest_valid",
		},
		"artifacts": map[string]any{
			"selected_prefix": "espocrm-prod",
			"selected_stamp":  "2026-04-19_09-00-00",
			"manifest_json":   "/backups/manifest.json",
			"db_backup":       "/backups/db.sql.gz",
			"files_backup":    "/backups/files.tar.gz",
		},
		"items": []map[string]any{
			{
				"code":    "target_selection",
				"status":  "failed",
				"summary": "Rollback target selection failed",
				"action":  "Resolve the target selection issue before rerunning rollback.",
			},
		},
	})
	if err := os.WriteFile(filepath.Join(journalDir, "corrupt.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"export-operation",
		"--id", "op-export-1",
		"--output", outputPath,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	if obj["command"] != "export-operation" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}

	details := requireJSONPath(t, obj, "details").(map[string]any)
	if details["id"] != "op-export-1" {
		t.Fatalf("unexpected details.id: %v", details["id"])
	}
	if details["bundle_kind"] != "operation_incident_bundle" {
		t.Fatalf("unexpected bundle kind: %v", details["bundle_kind"])
	}
	if int(details["bundle_version"].(float64)) != 1 {
		t.Fatalf("unexpected bundle version: %v", details["bundle_version"])
	}
	if details["exported_at"] != "2026-04-19T12:00:00Z" {
		t.Fatalf("unexpected exported_at: %v", details["exported_at"])
	}
	if int(details["skipped_corrupt"].(float64)) != 1 {
		t.Fatalf("unexpected skipped_corrupt: %v", details["skipped_corrupt"])
	}
	if requireJSONPath(t, obj, "artifacts", "bundle_path") != outputPath {
		t.Fatalf("unexpected bundle path artifact: %#v", obj["artifacts"])
	}

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) != 1 {
		t.Fatalf("expected one summary item, got %#v", items)
	}
	summary := items[0].(map[string]any)
	if summary["operation_id"] != "op-export-1" || summary["status"] != "failed" {
		t.Fatalf("unexpected summary item: %#v", summary)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read bundle file: %v", err)
	}

	var bundle map[string]any
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatalf("invalid bundle json: %v", err)
	}
	if bundle["bundle_kind"] != "operation_incident_bundle" {
		t.Fatalf("unexpected bundle kind in file: %v", bundle["bundle_kind"])
	}
	if requireJSONPath(t, bundle, "summary", "operation_id") != "op-export-1" {
		t.Fatalf("unexpected bundle summary: %#v", bundle["summary"])
	}
	if requireJSONPath(t, bundle, "report", "failure", "step_code") != "target_selection" {
		t.Fatalf("unexpected bundle failure attribution: %#v", bundle["report"])
	}
	if requireJSONPath(t, bundle, "report", "recovery", "decision") != "retry_from_start" {
		t.Fatalf("unexpected bundle recovery: %#v", bundle["report"])
	}
}
