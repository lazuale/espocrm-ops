package cli

import (
	"os"
	"path/filepath"
	"strconv"
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
	if err := os.WriteFile(filepath.Join(fixture.stateDir, "running-services"), []byte("db\nespocrm\nespocrm-daemon\nespocrm-websocket\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRuntimeStatusFile(t, fixture.stateDir, "db", "healthy")

	prependFakeDockerForRecoveryCLITest(t)
	t.Setenv("DOCKER_MOCK_RECOVERY_STATE_DIR", fixture.stateDir)
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_GID", strconv.Itoa(os.Getgid()))

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
