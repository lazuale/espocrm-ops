package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_UpdatePlan_JSON_Success(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	stateDir := filepath.Join(tmp, "docker-state")

	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "espo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "backups", "prod", "locks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDoctorEnvFile(t, projectDir, "prod", nil)
	prependFakeDockerForUpdatePlanCLITest(t)
	t.Setenv("DOCKER_MOCK_PLAN_STATE_DIR", stateDir)

	out, err := runRootCommand(
		t,
		"--journal-dir", journalDir,
		"--json",
		"update-plan",
		"--scope", "prod",
		"--project-dir", projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "dry_run")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "scope")
	requireJSONPath(t, obj, "details", "ready")
	requireJSONPath(t, obj, "details", "steps")
	requireJSONPath(t, obj, "details", "would_run")
	requireJSONPath(t, obj, "details", "blocked")
	requireJSONPath(t, obj, "artifacts", "project_dir")
	requireJSONPath(t, obj, "artifacts", "compose_file")
	requireJSONPath(t, obj, "artifacts", "env_file")
	requireJSONPath(t, obj, "artifacts", "backup_root")
	requireJSONPath(t, obj, "items")

	if obj["command"] != "update-plan" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if dryRun, _ := obj["dry_run"].(bool); !dryRun {
		t.Fatalf("expected dry_run=true, got %v", obj["dry_run"])
	}
	if message := obj["message"]; message != "update dry-run plan completed" {
		t.Fatalf("unexpected message: %v", message)
	}
	if wouldRun, _ := requireJSONPath(t, obj, "details", "would_run").(float64); int(wouldRun) != 4 {
		t.Fatalf("expected would_run=4, got %v", wouldRun)
	}

	items, ok := requireJSONPath(t, obj, "items").([]any)
	if !ok || len(items) != 4 {
		t.Fatalf("expected four plan items, got %#v", obj["items"])
	}
	expected := []struct {
		code   string
		status string
	}{
		{code: "doctor", status: "would_run"},
		{code: "backup_recovery_point", status: "would_run"},
		{code: "runtime_apply", status: "would_run"},
		{code: "runtime_readiness", status: "would_run"},
	}
	for idx, want := range expected {
		item, ok := items[idx].(map[string]any)
		if !ok {
			t.Fatalf("item %d is not an object: %#v", idx, items[idx])
		}
		if item["code"] != want.code || item["status"] != want.status {
			t.Fatalf("unexpected item %d: %#v", idx, item)
		}
	}

	assertNoJournalFiles(t, journalDir)
}

func TestSchema_UpdatePlan_JSON_Failure_BlockingRuntimePrecondition(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	stateDir := filepath.Join(tmp, "docker-state")

	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "espo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "backups", "prod", "locks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDoctorEnvFile(t, projectDir, "prod", map[string]string{
		"SITE_URL": "",
	})
	prependFakeDockerForUpdatePlanCLITest(t)
	t.Setenv("DOCKER_MOCK_PLAN_STATE_DIR", stateDir)

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"update-plan",
		"--scope", "prod",
		"--project-dir", projectDir,
		"--skip-doctor",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "update_plan_blocked", "update dry-run plan found blocking conditions")

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	items, ok := requireJSONPath(t, obj, "items").([]any)
	if !ok {
		t.Fatalf("expected plan items, got %#v", obj["items"])
	}

	foundBackupWouldRun := false
	foundRuntimeBlocked := false
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		code, _ := item["code"].(string)
		status, _ := item["status"].(string)
		switch code {
		case "backup_recovery_point":
			foundBackupWouldRun = status == "would_run"
		case "runtime_apply":
			foundRuntimeBlocked = status == "blocked" && strings.Contains(item["details"].(string), "SITE_URL is required")
		}
	}
	if !foundBackupWouldRun {
		t.Fatalf("expected backup_recovery_point to remain would_run, got %#v", items)
	}
	if !foundRuntimeBlocked {
		t.Fatalf("expected runtime_apply to be blocked by SITE_URL, got %#v", items)
	}

	assertNoJournalFiles(t, journalDir)
}

func TestSchema_UpdatePlan_JSON_SkipDoctorWarnsForUnhealthyRuntime(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	stateDir := filepath.Join(tmp, "docker-state")

	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "espo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "backups", "prod", "locks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDoctorEnvFile(t, projectDir, "prod", nil)
	if err := os.WriteFile(filepath.Join(stateDir, "running-services"), []byte("espocrm\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeUpdateRuntimeStatusFile(t, stateDir, "espocrm", "unhealthy")

	prependFakeDockerForUpdatePlanCLITest(t)
	t.Setenv("DOCKER_MOCK_PLAN_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_PLAN_HEALTH_MESSAGE", "container reports unhealthy")

	out, err := runRootCommand(
		t,
		"--journal-dir", journalDir,
		"--json",
		"update-plan",
		"--scope", "prod",
		"--project-dir", projectDir,
		"--skip-doctor",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}

	warnings, ok := requireJSONPath(t, obj, "warnings").([]any)
	if !ok || len(warnings) == 0 {
		t.Fatalf("expected warnings, got %#v", obj["warnings"])
	}
	foundDoctorWarning := false
	for _, rawWarning := range warnings {
		warning, _ := rawWarning.(string)
		if strings.Contains(warning, "Doctor would fail if it ran") && strings.Contains(warning, "A running service is unhealthy") {
			foundDoctorWarning = true
			break
		}
	}
	if !foundDoctorWarning {
		t.Fatalf("expected doctor warning in %#v", warnings)
	}

	items, ok := requireJSONPath(t, obj, "items").([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected items, got %#v", obj["items"])
	}
	if first, ok := items[0].(map[string]any); !ok || first["code"] != "doctor" || first["status"] != "skipped" {
		t.Fatalf("expected first item to skip doctor, got %#v", items[0])
	}

	assertNoJournalFiles(t, journalDir)
}
