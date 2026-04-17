package cli

import (
	"path/filepath"
	"testing"
)

func TestSchema_ShowOperation_JSON_Error_BlankID(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"show-operation",
		"--id", "   ",
	)

	assertUsageErrorOutput(t, outcome, "--id is required")
}
