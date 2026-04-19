package cli

import (
	"path/filepath"
	"testing"
	"time"
)

func TestGolden_BackupHealth_JSON(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-bh-golden"),
	}

	writeCLICatalogBackupSet(t, backupRoot, "espocrm-dev", "2026-04-15_09-00-00", "dev")
	writeCLICatalogDBOnly(t, backupRoot, "espocrm-dev", "2026-04-15_10-00-00")

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"backup-health",
		"--backup-root", backupRoot,
		"--max-age-hours", "48",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	assertGoldenJSON(t, normalizeBackupHealthJSON(t, []byte(out)), filepath.Join("testdata", "backup_health_ok.golden.json"))
}
