package cli

import (
	"path/filepath"
	"testing"
)

func TestDoctorValidationRejectsEnvFileOverrideForAll(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"doctor",
		"--scope", "all",
		"--project-dir", tmp,
		"--env-file", filepath.Join(tmp, ".env.dev"),
	)

	assertUsageErrorOutput(t, outcome, "--env-file cannot be used with --scope all")
	assertNoJournalFiles(t, journalDir)
}
