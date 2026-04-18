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

func TestContract_ShowOperation_JSON_Report(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "rollback.json", map[string]any{
		"operation_id":  "op-show-report-1",
		"command":       "rollback",
		"started_at":    "2026-04-18T13:00:00Z",
		"finished_at":   "2026-04-18T13:00:03Z",
		"ok":            false,
		"message":       "rollback failed",
		"error_code":    "rollback_failed",
		"error_message": "rollback target selection failed",
		"warnings": []string{
			"Rollback will skip the final HTTP probe because of --skip-http-probe.",
		},
		"details": map[string]any{
			"scope":          "prod",
			"selection_mode": "auto_latest_valid",
		},
		"artifacts": map[string]any{
			"project_dir":     "/srv/espocrm",
			"compose_file":    "/srv/espocrm/compose.yaml",
			"env_file":        "/srv/espocrm/.env.prod",
			"selected_prefix": "espocrm-prod",
			"selected_stamp":  "2026-04-18_10-00-00",
			"manifest_json":   "/backups/manifest.json",
			"db_backup":       "/backups/db.sql.gz",
			"files_backup":    "/backups/files.tar.gz",
		},
		"items": []map[string]any{
			{
				"code":    "target_selection",
				"status":  "failed",
				"summary": "Rollback target selection failed",
				"details": "could not find a valid backup set",
				"action":  "Resolve the rollback target selection error before rerunning rollback.",
			},
			{
				"code":    "runtime_prepare",
				"status":  "not_run",
				"summary": "Runtime preparation did not run because rollback target selection failed",
			},
		},
	})

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"show-operation",
		"--id", "op-show-report-1",
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
		t.Fatalf("expected one report item, got %#v", items)
	}

	report := items[0].(map[string]any)
	if report["scope"] != "prod" {
		t.Fatalf("unexpected scope: %v", report["scope"])
	}
	counts := requireJSONPath(t, report, "counts").(map[string]any)
	if int(counts["failed"].(float64)) != 1 {
		t.Fatalf("unexpected failed count: %v", counts["failed"])
	}
	if int(counts["blocked"].(float64)) != 1 {
		t.Fatalf("unexpected blocked count: %v", counts["blocked"])
	}
	target := requireJSONPath(t, report, "target").(map[string]any)
	if target["prefix"] != "espocrm-prod" {
		t.Fatalf("unexpected target prefix: %v", target["prefix"])
	}
	steps := requireJSONPath(t, report, "steps").([]any)
	if len(steps) != 2 {
		t.Fatalf("unexpected steps: %#v", steps)
	}
	if second := steps[1].(map[string]any); second["status"] != "blocked" {
		t.Fatalf("expected blocked downstream step, got %#v", second)
	}
	failure := requireJSONPath(t, report, "failure").(map[string]any)
	if failure["step_code"] != "target_selection" {
		t.Fatalf("unexpected failure attribution: %#v", failure)
	}
}
