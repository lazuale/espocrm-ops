package cli

import (
	"path/filepath"
	"testing"
)

func TestSchema_LastOperation_JSON_Error_BlankCommandFilter(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"last-operation",
		"--command", "   ",
	)

	assertUsageErrorOutput(t, outcome, "--command must not be blank")
}
