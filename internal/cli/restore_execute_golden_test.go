package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGolden_Restore_JSON(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareRestoreCommandFixture(t, "prod", map[string]string{
		"espo/data/nested/file.txt":      "hello",
		"espo/custom/modules/module.txt": "custom",
		"espo/client/custom/app.js":      "client",
		"espo/upload/blob.txt":           "upload",
	})
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-restore-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore",
		"--scope", "prod",
		"--project-dir", fixture.projectDir,
		"--manifest", fixture.backupSet.ManifestJSON,
		"--force",
		"--confirm-prod", "prod",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	normalized := normalizeRestoreJSON(t, []byte(outcome.Stdout))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "restore_ok.golden.json"))
}

func TestGolden_Restore_DryRun_JSON(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareRestoreCommandFixture(t, "prod", map[string]string{
		"espo/data/nested/file.txt":      "hello",
		"espo/custom/modules/module.txt": "custom",
		"espo/client/custom/app.js":      "client",
		"espo/upload/blob.txt":           "upload",
	})
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-restore-dry-run-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore",
		"--scope", "prod",
		"--project-dir", fixture.projectDir,
		"--manifest", fixture.backupSet.ManifestJSON,
		"--dry-run",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}
	if _, err := os.Stat(filepath.Join(fixture.storageDir, "before.txt")); err != nil {
		t.Fatalf("expected dry-run to avoid mutating storage: %v", err)
	}

	normalized := normalizeRestoreJSON(t, []byte(outcome.Stdout))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "restore_dry_run.golden.json"))
}
