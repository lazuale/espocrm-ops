package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryTextRendersCompactOperationSummaries(t *testing.T) {
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
		"warnings":     []string{"final probe retried once"},
		"details": map[string]any{
			"scope":          "prod",
			"selection_mode": "auto_latest_valid",
			"recovery": map[string]any{
				"source_operation_id": "op-source",
				"applied_mode":        "resume",
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

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"history",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, part := range []string{
		"2026-04-15T12:00:00Z",
		"rollback",
		"COMPLETED",
		"scope=prod",
		"target=espocrm-prod@2026-04-15_11-00-00",
		"recovery=resume:op-source@runtime_prepare",
		"warnings=1",
		`summary="rollback recovery completed"`,
		"id=op-1",
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("expected %q in output:\n%s", part, out)
		}
	}
}

func TestHistoryTextShowsEmptyState(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"history",
		"--status", "failed",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}
	if strings.TrimSpace(out) != "no operations found" {
		t.Fatalf("unexpected empty output: %q", out)
	}
}
