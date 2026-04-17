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
		"command":      "verify-backup",
		"started_at":   "2026-04-15T10:00:00Z",
		"finished_at":  "2026-04-15T10:00:01Z",
		"ok":           true,
	})
	writeJournalEntryFile(t, journalDir, "2026-04-15-b.json", map[string]any{
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
		"history",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	assertGoldenJSON(t, []byte(out), filepath.Join("testdata", "history_ok.golden.json"))
}
