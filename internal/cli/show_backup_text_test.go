package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestText_ShowBackup(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-sb-text"),
	}

	writeCLICatalogBackupSet(t, backupRoot, "espocrm-prod", "2026-04-07_01-00-00", "prod")
	writeJournalEntryFile(t, journalDir, "rollback.json", map[string]any{
		"operation_id": "op-rollback-1",
		"command":      "rollback",
		"started_at":   "2026-04-07T01:10:00Z",
		"finished_at":  "2026-04-07T01:12:00Z",
		"ok":           true,
		"artifacts": map[string]any{
			"snapshot_manifest_json": filepath.Join(backupRoot, "manifests", "espocrm-prod_2026-04-07_01-00-00.manifest.json"),
		},
	})

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"show-backup",
		"--backup-root", backupRoot,
		"--id", "espocrm-prod_2026-04-07_01-00-00",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, needle := range []string{
		"EspoCRM backup inspection",
		"Backup id:               espocrm-prod_2026-04-07_01-00-00",
		"Source:                  rollback protection snapshot",
		"Artifacts:",
		"JSON manifest:",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected output to contain %q, got:\n%s", needle, out)
		}
	}
}
