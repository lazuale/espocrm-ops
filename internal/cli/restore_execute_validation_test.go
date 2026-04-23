package cli

import (
	"path/filepath"
	"testing"
)

func TestRestore_Validation_RequiresForce(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore",
		"--scope", "dev",
		"--db-backup", filepath.Join(tmp, "db.sql.gz"),
		"--skip-files",
	)

	assertUsageErrorOutput(t, outcome, "restore requires an explicit --force flag")
}

func TestRestore_Validation_RequiresProdConfirmation(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore",
		"--scope", "prod",
		"--db-backup", filepath.Join(tmp, "db.sql.gz"),
		"--skip-files",
		"--force",
	)

	assertUsageErrorOutput(t, outcome, "prod restore also requires --confirm-prod prod")
}

func TestRestore_Validation_RejectsPartialManifestPath(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore",
		"--scope", "dev",
		"--manifest", filepath.Join(tmp, "backup.manifest.json"),
		"--skip-files",
	)

	assertUsageErrorOutput(t, outcome, "--manifest нельзя комбинировать с --skip-db или --skip-files")
}
