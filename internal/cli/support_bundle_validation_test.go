package cli

import (
	"path/filepath"
	"testing"
)

func TestSupportBundle_Validation_RejectsNonPositiveTail(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"support-bundle",
		"--scope", "dev",
		"--project-dir", tmp,
		"--tail", "0",
	)

	assertUsageErrorOutput(t, outcome, "--tail must be a positive integer")
}
