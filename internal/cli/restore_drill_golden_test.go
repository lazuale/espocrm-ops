package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestGolden_RestoreDrill_JSON(t *testing.T) {
	fixture := prepareRestoreDrillFixture(t, "dev", map[string]string{
		"espo/data/golden.txt": "golden",
	})
	useJournalClockForTest(t, fixture.fixedNow)

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", fixture.stateDir)
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_GID", strconv.Itoa(os.Getgid()))

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-restore-drill-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore-drill",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--timeout", "123",
		"--skip-http-probe",
		"--keep-artifacts",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	normalized := normalizeRestoreDrillJSON(t, []byte(outcome.Stdout))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "restore_drill_ok.golden.json"))
}
