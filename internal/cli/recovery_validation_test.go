package cli

import (
	"path/filepath"
	"testing"
)

func TestSchema_Update_JSON_RecoveryRejectsOverrides(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"update",
		"--recover-operation", "op-1",
		"--scope", "prod",
	)

	assertUsageErrorOutput(t, outcome, "--scope cannot be used with --recover-operation")
}

func TestSchema_Rollback_JSON_RecoveryRequiresForce(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"rollback",
		"--recover-operation", "op-1",
	)

	assertUsageErrorOutput(t, outcome, "rollback recovery requires an explicit --force flag")
}
