package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGolden_History_JSON(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "2026-04-15-a.json", map[string]any{
		"operation_id": "op-1",
		"command":      "rollback",
		"started_at":   "2026-04-15T10:00:00Z",
		"finished_at":  "2026-04-15T10:00:04Z",
		"ok":           true,
		"message":      "rollback recovery completed",
		"warnings":     []string{"final probe retried once"},
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
			"selected_stamp":  "2026-04-15_09-00-00",
		},
		"items": []any{
			map[string]any{
				"code":    "runtime_return",
				"status":  "completed",
				"summary": "Contour return completed",
			},
		},
	})
	writeJournalEntryFile(t, journalDir, "2026-04-15-b.json", map[string]any{
		"operation_id":  "op-2",
		"command":       "update",
		"started_at":    "2026-04-15T11:00:00Z",
		"finished_at":   "2026-04-15T11:00:01Z",
		"ok":            false,
		"dry_run":       true,
		"message":       "update plan blocked",
		"error_code":    "update_plan_blocked",
		"error_message": "doctor blocked",
		"details": map[string]any{
			"scope": "stage",
		},
		"items": []any{
			map[string]any{
				"code":    "doctor",
				"status":  "blocked",
				"summary": "Doctor is blocked",
				"action":  "Resolve the blocking prerequisites before running update.",
			},
		},
	})
	if err := os.WriteFile(filepath.Join(journalDir, "corrupt.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"history",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	assertGoldenJSON(t, []byte(out), filepath.Join("testdata", "history_ok.golden.json"))
}
