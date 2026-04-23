package cli

import (
	"path/filepath"
	"testing"
)

func TestMigrate_Validation_RequiresForce(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
	)

	assertUsageErrorOutput(t, outcome, "migrate requires an explicit --force flag")
}

func TestMigrate_Validation_RequiresProdConfirmation(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--force",
	)

	assertUsageErrorOutput(t, outcome, "prod migration also requires --confirm-prod prod")
}

func TestMigrate_Validation_RejectsSameSourceAndTarget(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate",
		"--from", "prod",
		"--to", "prod",
		"--force",
		"--confirm-prod", "prod",
	)

	assertUsageErrorOutput(t, outcome, "source and target contours must differ")
}

func TestMigrate_Validation_RejectsSkipDBAndSkipFilesTogether(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--skip-db",
		"--skip-files",
		"--force",
		"--confirm-prod", "prod",
	)

	assertUsageErrorOutput(t, outcome, "nothing to migrate")
}

func TestMigrate_Validation_RejectsFilesOnlyWithoutExplicitFilesArtifact(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--skip-db",
		"--force",
		"--confirm-prod", "prod",
	)

	assertUsageErrorOutput(t, outcome, "files-only migrate требует явный --files-backup")
}

func TestMigrate_Validation_RejectsDBOnlyWithoutExplicitDBArtifact(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--skip-files",
		"--force",
		"--confirm-prod", "prod",
	)

	assertUsageErrorOutput(t, outcome, "db-only migrate требует явный --db-backup")
}

func TestMigrate_Validation_RejectsImplicitPairingFromSingleDBArtifact(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--db-backup", filepath.Join(tmp, "db.sql.gz"),
		"--force",
		"--confirm-prod", "prod",
	)

	assertUsageErrorOutput(t, outcome, "полный migrate требует либо latest complete backup-set, либо explicit direct pair")
}

func TestMigrate_Validation_RejectsImplicitPairingFromSingleFilesArtifact(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--files-backup", filepath.Join(tmp, "files.tar.gz"),
		"--force",
		"--confirm-prod", "prod",
	)

	assertUsageErrorOutput(t, outcome, "полный migrate требует либо latest complete backup-set, либо explicit direct pair")
}
