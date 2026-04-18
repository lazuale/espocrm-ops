package cli

import "testing"

func TestRollbackPlan_Validation_RejectsUnsupportedScope(t *testing.T) {
	outcome := executeCLI(
		"--journal-dir", t.TempDir(),
		"--json",
		"rollback-plan",
		"--scope", "all",
	)

	assertUsageErrorOutput(t, outcome, "--scope must be dev or prod")
}

func TestRollbackPlan_Validation_RequiresExplicitPairTogether(t *testing.T) {
	outcome := executeCLI(
		"--journal-dir", t.TempDir(),
		"--json",
		"rollback-plan",
		"--scope", "prod",
		"--db-backup", "/tmp/db.sql.gz",
	)

	assertUsageErrorOutput(t, outcome, "pass both --db-backup and --files-backup together")
}
