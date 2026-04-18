package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGolden_ShowOperation_JSON(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "rollback.json", map[string]any{
		"operation_id": "op-show-golden-1",
		"command":      "rollback",
		"started_at":   "2026-04-18T13:00:00Z",
		"finished_at":  "2026-04-18T13:00:03Z",
		"ok":           true,
		"message":      "rollback completed",
		"warnings": []string{
			"Rollback will leave the contour stopped because of --no-start.",
		},
		"details": map[string]any{
			"scope":          "prod",
			"selection_mode": "auto_latest_valid",
		},
		"artifacts": map[string]any{
			"project_dir":            "REPLACE_PROJECT_DIR",
			"compose_file":           "REPLACE_COMPOSE_FILE",
			"env_file":               "REPLACE_ENV_FILE",
			"backup_root":            "REPLACE_BACKUP_ROOT",
			"site_url":               "REPLACE_SITE_URL",
			"selected_prefix":        "espocrm-prod",
			"selected_stamp":         "2026-04-18_10-00-00",
			"manifest_json":          "REPLACE_MANIFEST_JSON",
			"db_backup":              "REPLACE_DB_BACKUP",
			"files_backup":           "REPLACE_FILES_BACKUP",
			"snapshot_manifest_json": "REPLACE_SNAPSHOT_MANIFEST_JSON",
		},
		"items": []map[string]any{
			{
				"code":    "target_selection",
				"status":  "completed",
				"summary": "Automatic rollback target selection completed",
				"details": "Selected prefix espocrm-prod at 2026-04-18_10-00-00 with manifest REPLACE_MANIFEST_JSON.",
			},
			{
				"code":    "snapshot_recovery_point",
				"status":  "completed",
				"summary": "Emergency recovery-point creation completed",
				"details": "Created the emergency recovery point at REPLACE_SNAPSHOT_MANIFEST_JSON.",
			},
			{
				"code":    "runtime_return",
				"status":  "skipped",
				"summary": "Contour return skipped",
				"details": "The contour was left stopped because of --no-start.",
			},
		},
	})

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"show-operation",
		"--id", "op-show-golden-1",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	assertGoldenJSON(t, []byte(out), filepath.Join("testdata", "show_operation_ok.golden.json"))
}
