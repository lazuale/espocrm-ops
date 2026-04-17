package cli

import (
	"path/filepath"
	"testing"
)

func TestSchema_Backup_JSON_Error_MissingScope_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"--db-backup", filepath.Join(tmp, "db.sql.gz"),
		"--files-backup", filepath.Join(tmp, "files.tar.gz"),
		"--manifest", filepath.Join(tmp, "manifest.json"),
	)

	assertUsageErrorOutput(t, outcome, "--scope is required")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_Backup_JSON_Error_InvalidCreatedAt_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"--scope", "dev",
		"--created-at", "not-a-time",
		"--db-backup", filepath.Join(tmp, "db.sql.gz"),
		"--files-backup", filepath.Join(tmp, "files.tar.gz"),
		"--manifest", filepath.Join(tmp, "manifest.json"),
	)

	assertUsageErrorOutput(t, outcome, "--created-at must be RFC3339")
	assertNoJournalFiles(t, journalDir)
}
