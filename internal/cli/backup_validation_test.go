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
		"--project-dir", tmp,
	)

	assertUsageErrorOutput(t, outcome, "--scope must be dev or prod")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_Backup_JSON_Error_RejectsEmptySelection_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"--scope", "dev",
		"--project-dir", tmp,
		"--skip-db",
		"--skip-files",
	)

	assertUsageErrorOutput(t, outcome, "nothing to back up")
	assertNoJournalFiles(t, journalDir)
}
