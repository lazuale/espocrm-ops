package cli

import (
	"path/filepath"
	"testing"
)

func TestStatusReportRequiresExplicitScope(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"status-report",
	)

	assertUsageErrorOutput(t, outcome, "--scope must be dev or prod")
	assertNoJournalFiles(t, journalDir)
}
