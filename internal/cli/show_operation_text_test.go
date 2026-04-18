package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestText_ShowOperation_Report(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "update.json", map[string]any{
		"operation_id":  "op-show-text-1",
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
		"show-operation",
		"--id", "op-show-text-1",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, part := range []string{
		"EspoCRM update report",
		"Outcome:",
		"Failure:",
		"[FAILED] Doctor stopped the update",
		"[BLOCKED] Runtime apply did not run because doctor failed",
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("expected output to contain %q\n%s", part, out)
		}
	}
}
