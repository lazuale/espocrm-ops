package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestText_ExportOperation_Report(t *testing.T) {
	useJournalClockForTest(t, time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))

	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	outputDir := filepath.Join(tmp, "exports")
	outputPath := filepath.Join(outputDir, "op-export-text.bundle.json")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "update.json", map[string]any{
		"operation_id":  "op-export-text",
		"command":       "update",
		"started_at":    "2026-04-19T10:00:00Z",
		"finished_at":   "2026-04-19T10:00:05Z",
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
				"action":  "Resolve the doctor failures before retrying update.",
			},
		},
	})

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"export-operation",
		"--id", "op-export-text",
		"--output", outputPath,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, part := range []string{
		"operation bundle exported:",
		outputPath,
		"bundle: operation_incident_bundle v1",
		"operation:",
		"id=op-export-text",
		"included:",
		"omitted:",
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("expected output to contain %q\n%s", part, out)
		}
	}
}
