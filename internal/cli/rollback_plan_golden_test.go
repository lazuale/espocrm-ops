package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGolden_RollbackPlan_JSON(t *testing.T) {
	isolateRollbackPlanLocks(t)

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")
	stateDir := filepath.Join(tmp, "docker-state")

	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "espo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDoctorEnvFile(t, projectDir, "prod", nil)
	writeRollbackBackupSet(t, filepath.Join(projectDir, "backups", "prod"), "espocrm-prod", "2026-04-18_10-00-00", "prod")
	prependFakeDockerForRollbackPlanCLITest(t)
	t.Setenv("DOCKER_MOCK_PLAN_STATE_DIR", stateDir)

	out, err := runRootCommand(
		t,
		"--journal-dir", journalDir,
		"--json",
		"rollback-plan",
		"--scope", "prod",
		"--project-dir", projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	normalized := normalizeRollbackPlanJSON(t, []byte(out))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "rollback_plan_ok.golden.json"))
}
