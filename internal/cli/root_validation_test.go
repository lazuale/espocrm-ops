package cli

import (
	"path/filepath"
	"testing"
)

func TestSchema_Root_JSON_Error_UnknownCommand_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"totally-unknown-command",
	)

	assertUsageErrorOutput(t, outcome, `unknown command "totally-unknown-command" for "espops"`)
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_Root_JSON_Error_UnknownFlag_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"history",
		"--totally-unknown-flag",
	)

	assertUsageErrorOutput(t, outcome, "unknown flag: --totally-unknown-flag")
	assertNoJournalFiles(t, journalDir)
}
