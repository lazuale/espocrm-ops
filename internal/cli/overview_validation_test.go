package cli

import (
	"path/filepath"
	"testing"
)

func TestOverviewRequiresExplicitScope(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"overview",
	)

	assertUsageErrorOutput(t, outcome, "--scope must be dev or prod")
	assertNoJournalFiles(t, journalDir)
}
