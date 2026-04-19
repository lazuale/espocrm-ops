package cli

import (
	"path/filepath"
	"testing"
)

func TestRestoreDrill_Validation_RejectsEqualPorts(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-drill",
		"--scope", "dev",
		"--project-dir", tmp,
		"--app-port", "28080",
		"--ws-port", "28080",
	)

	assertUsageErrorOutput(t, outcome, "APP and WS ports for restore-drill must differ")
}
