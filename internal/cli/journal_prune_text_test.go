package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJournalPrune_TextOutputShowsRetentionDecisions(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "old.json", map[string]any{
		"operation_id": "op-old-text",
		"command":      "verify-backup",
		"started_at":   "2026-04-15T10:00:00Z",
		"finished_at":  "2026-04-15T10:00:01Z",
		"ok":           true,
	})
	writeJournalEntryFile(t, journalDir, "new.json", map[string]any{
		"operation_id": "op-new-text",
		"command":      "verify-backup",
		"started_at":   "2026-04-15T11:00:00Z",
		"finished_at":  "2026-04-15T11:00:01Z",
		"ok":           true,
	})

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"journal-prune",
		"--keep-last", "1",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, want := range []string{
		"journal prune dry-run:",
		"latest_operation_id=op-new-text",
		"PROTECT",
		"REMOVE",
		"id=op-new-text",
		"id=op-old-text",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}
