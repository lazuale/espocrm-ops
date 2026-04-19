package cli

import (
	"path/filepath"
	"testing"
)

func TestMigrateBackup_Validation_RequiresForce(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate-backup",
		"--from", "dev",
		"--to", "prod",
	)

	assertUsageErrorOutput(t, outcome, "migrate-backup requires an explicit --force flag")
}

func TestMigrateBackup_Validation_RequiresProdConfirmation(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate-backup",
		"--from", "dev",
		"--to", "prod",
		"--force",
	)

	assertUsageErrorOutput(t, outcome, "prod backup migration also requires --confirm-prod prod")
}
