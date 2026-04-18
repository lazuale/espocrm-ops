package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_RollbackPlan_JSON_Success_AutoSelection(t *testing.T) {
	isolateRollbackPlanLocks(t)

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

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "dry_run")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "scope")
	requireJSONPath(t, obj, "details", "selection_mode")
	requireJSONPath(t, obj, "details", "ready")
	requireJSONPath(t, obj, "details", "steps")
	requireJSONPath(t, obj, "artifacts", "project_dir")
	requireJSONPath(t, obj, "artifacts", "compose_file")
	requireJSONPath(t, obj, "artifacts", "env_file")
	requireJSONPath(t, obj, "artifacts", "backup_root")
	requireJSONPath(t, obj, "artifacts", "manifest_json")
	requireJSONPath(t, obj, "artifacts", "db_backup")
	requireJSONPath(t, obj, "artifacts", "files_backup")
	requireJSONPath(t, obj, "items")

	if obj["command"] != "rollback-plan" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if dryRun, _ := obj["dry_run"].(bool); !dryRun {
		t.Fatalf("expected dry_run=true, got %v", obj["dry_run"])
	}
	if message := obj["message"]; message != "rollback dry-run plan completed" {
		t.Fatalf("unexpected message: %v", message)
	}
	if selectionMode := requireJSONPath(t, obj, "details", "selection_mode"); selectionMode != "auto_latest_valid" {
		t.Fatalf("expected auto_latest_valid selection, got %v", selectionMode)
	}
	if wouldRun, _ := requireJSONPath(t, obj, "details", "would_run").(float64); int(wouldRun) != 6 {
		t.Fatalf("expected would_run=6, got %v", wouldRun)
	}

	items, ok := requireJSONPath(t, obj, "items").([]any)
	if !ok || len(items) != 6 {
		t.Fatalf("expected six plan items, got %#v", obj["items"])
	}
	expected := []struct {
		code   string
		status string
	}{
		{code: "target_selection", status: "would_run"},
		{code: "runtime_prepare", status: "would_run"},
		{code: "snapshot_recovery_point", status: "would_run"},
		{code: "db_restore", status: "would_run"},
		{code: "files_restore", status: "would_run"},
		{code: "runtime_return", status: "would_run"},
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

func TestSchema_RollbackPlan_JSON_Success_ExplicitSelectionAndSkippedSteps(t *testing.T) {
	isolateRollbackPlanLocks(t)

	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	backupRoot := filepath.Join(projectDir, "backups", "prod")
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
	writeRollbackBackupSet(t, backupRoot, "espocrm-prod", "2026-04-18_11-00-00", "prod")
	prependFakeDockerForRollbackPlanCLITest(t)
	t.Setenv("DOCKER_MOCK_PLAN_STATE_DIR", stateDir)

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"rollback-plan",
		"--scope", "prod",
		"--project-dir", projectDir,
		"--db-backup", filepath.Join(backupRoot, "db", "espocrm-prod_2026-04-18_11-00-00.sql.gz"),
		"--files-backup", filepath.Join(backupRoot, "files", "espocrm-prod_files_2026-04-18_11-00-00.tar.gz"),
		"--no-snapshot",
		"--no-start",
		"--skip-http-probe",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if selectionMode := requireJSONPath(t, obj, "details", "selection_mode"); selectionMode != "explicit" {
		t.Fatalf("expected explicit selection, got %v", selectionMode)
	}
	if skipped, _ := requireJSONPath(t, obj, "details", "skipped").(float64); int(skipped) != 2 {
		t.Fatalf("expected skipped=2, got %v", skipped)
	}

	warnings, ok := requireJSONPath(t, obj, "warnings").([]any)
	if !ok || len(warnings) < 2 {
		t.Fatalf("expected warnings, got %#v", obj["warnings"])
	}
	foundSnapshotWarning := false
	foundNoStartWarning := false
	for _, rawWarning := range warnings {
		warning, _ := rawWarning.(string)
		if strings.Contains(warning, "--no-snapshot") {
			foundSnapshotWarning = true
		}
		if strings.Contains(warning, "--no-start") {
			foundNoStartWarning = true
		}
	}
	if !foundSnapshotWarning || !foundNoStartWarning {
		t.Fatalf("expected snapshot and no-start warnings, got %#v", warnings)
	}

	items, ok := requireJSONPath(t, obj, "items").([]any)
	if !ok {
		t.Fatalf("expected plan items, got %#v", obj["items"])
	}

	foundSnapshotSkipped := false
	foundReturnSkipped := false
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		switch item["code"] {
		case "snapshot_recovery_point":
			foundSnapshotSkipped = item["status"] == "skipped"
		case "runtime_return":
			foundReturnSkipped = item["status"] == "skipped"
		}
	}
	if !foundSnapshotSkipped || !foundReturnSkipped {
		t.Fatalf("expected skipped snapshot/runtime_return steps, got %#v", items)
	}

	assertNoJournalFiles(t, journalDir)
}

func TestSchema_RollbackPlan_JSON_Failure_NoValidBackupSet(t *testing.T) {
	isolateRollbackPlanLocks(t)

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
	if err := os.MkdirAll(filepath.Join(projectDir, "backups", "prod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDoctorEnvFile(t, projectDir, "prod", nil)
	prependFakeDockerForRollbackPlanCLITest(t)
	t.Setenv("DOCKER_MOCK_PLAN_STATE_DIR", stateDir)

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"rollback-plan",
		"--scope", "prod",
		"--project-dir", projectDir,
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "rollback_plan_blocked", "rollback dry-run plan found blocking conditions")

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	items, ok := requireJSONPath(t, obj, "items").([]any)
	if !ok {
		t.Fatalf("expected plan items, got %#v", obj["items"])
	}

	foundTargetBlocked := false
	foundDBBlocked := false
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		code, _ := item["code"].(string)
		status, _ := item["status"].(string)
		switch code {
		case "target_selection":
			foundTargetBlocked = status == "blocked" && strings.Contains(item["details"].(string), "could not find a valid backup set")
		case "db_restore":
			foundDBBlocked = status == "blocked"
		}
	}
	if !foundTargetBlocked || !foundDBBlocked {
		t.Fatalf("expected blocked target selection and db restore, got %#v", items)
	}

	assertNoJournalFiles(t, journalDir)
}
