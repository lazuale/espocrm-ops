package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGolden_Migrate_JSON(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")
	stateDir := filepath.Join(tmp, "docker-state")
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	fixedNow := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "before.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "running-services"), []byte("espocrm\nespocrm-daemon\nespocrm-websocket\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRuntimeStatusFile(t, stateDir, "db", "healthy")

	writeDoctorEnvFile(t, projectDir, "dev", nil)
	writeDoctorEnvFile(t, projectDir, "prod", nil)
	writeBackupSet(t, filepath.Join(projectDir, "backups", "dev"), "espocrm-dev", "2026-04-19_08-00-00", "dev")

	prependFakeDockerForRecoveryCLITest(t)
	t.Setenv("DOCKER_MOCK_RECOVERY_STATE_DIR", stateDir)

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixedNow, "op-migrate-1"),
		},
		"--journal-dir", journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", projectDir,
		"--force",
		"--confirm-prod", "prod",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	normalized := normalizeMigrateJSON(t, []byte(outcome.Stdout))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "migrate_ok.golden.json"))
}
