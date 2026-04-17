package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegration_JournalPrune_KeepDays(t *testing.T) {
	useJournalClockForTest(t, time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC))

	tmp := t.TempDir()
	root := filepath.Join(tmp, "journal")

	oldDir := filepath.Join(root, "2026-03-01")
	newDir := filepath.Join(root, "2026-04-15")

	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldFile := filepath.Join(oldDir, "old.json")
	newFile := filepath.Join(newDir, "new.json")

	writeJournalEntryFile(t, oldDir, "old.json", map[string]any{
		"operation_id": "op-old",
		"command":      "verify-backup",
		"started_at":   "2026-03-01T10:00:00Z",
		"finished_at":  "2026-03-01T10:00:01Z",
		"ok":           true,
	})

	writeJournalEntryFile(t, newDir, "new.json", map[string]any{
		"operation_id": "op-new",
		"command":      "verify-backup",
		"started_at":   "2026-04-15T10:00:00Z",
		"finished_at":  "2026-04-15T10:00:01Z",
		"ok":           true,
	})

	out, err := runRootCommand(t,
		"--journal-dir", root,
		"--json",
		"journal-prune",
		"--keep-days", "30",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected old file deleted, got: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("expected old dir deleted, got: %v", err)
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatalf("expected new file to remain: %v", err)
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("expected new dir to remain: %v", err)
	}
}
