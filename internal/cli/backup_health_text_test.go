package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestText_BackupHealth_Degraded(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-bh-text-1"),
	}

	writeCLICatalogBackupSet(t, backupRoot, "espocrm-dev", "2026-04-15_09-00-00", "dev")
	writeCLICatalogDBOnly(t, backupRoot, "espocrm-dev", "2026-04-15_10-00-00")

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"backup-health",
		"--backup-root", backupRoot,
		"--max-age-hours", "48",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, needle := range []string{
		"EspoCRM backup health",
		"Verdict:                  degraded",
		"Latest restore-ready backup:",
		"Alerts:",
		"[WARNING] Latest observed backup set is not restore-ready",
		"Next action:",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected output to contain %q, got:\n%s", needle, out)
		}
	}
}
