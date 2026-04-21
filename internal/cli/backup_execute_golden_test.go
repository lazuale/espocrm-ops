package cli

import (
	"path/filepath"
	"testing"
)

func TestGolden_Backup_JSON(t *testing.T) {
	fixture := prepareBackupCommandFixture(t)
	fixture.docker.SetFailOnAnyCall(t)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{
			withFixedTestRuntime(fixture.fixedNow, "op-backup-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"backup",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--compose-file", fixture.composeFile,
		"--env-file", fixture.envFile,
		"--skip-db",
		"--no-stop",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	assertGoldenJSON(t, normalizeBackupJSON(t, []byte(out)), filepath.Join("testdata", "backup_ok.golden.json"))
}
