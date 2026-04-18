package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGolden_ExportOperation_JSON(t *testing.T) {
	useJournalClockForTest(t, time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))

	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	outputDir := filepath.Join(tmp, "exports")
	outputPath := filepath.Join(outputDir, "op-export-golden.bundle.json")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "rollback.json", map[string]any{
		"operation_id":  "op-export-golden-1",
		"command":       "rollback",
		"started_at":    "2026-04-19T10:00:00Z",
		"finished_at":   "2026-04-19T10:00:03Z",
		"ok":            false,
		"message":       "rollback failed",
		"error_code":    "rollback_failed",
		"error_message": "rollback target selection failed",
		"warnings":      []string{"Rollback will skip the final HTTP probe because of --skip-http-probe."},
		"details": map[string]any{
			"scope":          "prod",
			"selection_mode": "auto_latest_valid",
		},
		"artifacts": map[string]any{
			"project_dir":     "REPLACE_PROJECT_DIR",
			"compose_file":    "REPLACE_COMPOSE_FILE",
			"env_file":        "REPLACE_ENV_FILE",
			"selected_prefix": "espocrm-prod",
			"selected_stamp":  "2026-04-19_09-00-00",
			"manifest_json":   "REPLACE_MANIFEST_JSON",
			"db_backup":       "REPLACE_DB_BACKUP",
			"files_backup":    "REPLACE_FILES_BACKUP",
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
		"export-operation",
		"--id", "op-export-golden-1",
		"--output", outputPath,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	normalized := normalizeExportOperationJSON(t, []byte(out))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "export_operation_ok.golden.json"))

	bundleRaw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read bundle file: %v", err)
	}
	assertGoldenJSON(t, bundleRaw, filepath.Join("testdata", "export_operation_bundle_ok.golden.json"))
}

func normalizeExportOperationJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	artifacts, _ := payload["artifacts"].(map[string]any)
	if artifacts != nil {
		artifacts["bundle_path"] = "REPLACE_BUNDLE_PATH"
	}
	if message, _ := payload["message"].(string); message != "" {
		payload["message"] = "operation bundle exported to REPLACE_BUNDLE_PATH"
	}

	normalized, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	return normalized
}
