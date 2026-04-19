package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestShowBackupValidation_RequiresID(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"show-backup",
		"--backup-root", filepath.Join(tmp, "backups"),
	)

	assertUsageErrorOutput(t, outcome, "--id is required")
	assertNoJournalFiles(t, journalDir)
}

func TestShowBackupValidation_NotFoundIsStructuredError(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeCLICatalogBackupSet(t, backupRoot, "espocrm-dev", "2026-04-07_01-00-00", "dev")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"show-backup",
		"--backup-root", backupRoot,
		"--id", "missing",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_not_found", "backup not found: missing")
}
