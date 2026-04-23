package cli

import (
	"path/filepath"
	"testing"
)

func TestGolden_Migrate_JSON(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)
	fixture := prepareMigrateCommandFixture(t)
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm-daemon", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm-websocket", "healthy")

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-migrate-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
		"--force",
		"--confirm-prod", "prod",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	normalized := normalizeMigrateJSON(t, []byte(outcome.Stdout))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "migrate_ok.golden.json"))
}
