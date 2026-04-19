package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_MigrateBackup_JSON_Success(t *testing.T) {
	isolateRollbackPlanLocks(t)

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")
	stateDir := filepath.Join(tmp, "docker-state")
	logPath := filepath.Join(tmp, "docker.log")
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	fixedNow := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)

	useJournalClockForTest(t, fixedNow)

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
	writeUpdateRuntimeStatusFile(t, stateDir, "db", "healthy")

	writeDoctorEnvFile(t, projectDir, "dev", nil)
	writeDoctorEnvFile(t, projectDir, "prod", nil)
	writeRollbackBackupSet(t, filepath.Join(projectDir, "backups", "dev"), "espocrm-dev", "2026-04-19_08-00-00", "dev")

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_ROLLBACK_LOG", logPath)

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixedNow, "op-migrate-1")},
		"--journal-dir", journalDir,
		"--json",
		"migrate-backup",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", projectDir,
		"--force",
		"--confirm-prod", "prod",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "source_scope")
	requireJSONPath(t, obj, "details", "target_scope")
	requireJSONPath(t, obj, "details", "selection_mode")
	requireJSONPath(t, obj, "details", "completed")
	requireJSONPath(t, obj, "details", "started_db_temporarily")
	requireJSONPath(t, obj, "artifacts", "source_env_file")
	requireJSONPath(t, obj, "artifacts", "target_env_file")
	requireJSONPath(t, obj, "artifacts", "manifest_json")
	requireJSONPath(t, obj, "artifacts", "db_backup")
	requireJSONPath(t, obj, "artifacts", "files_backup")
	requireJSONPath(t, obj, "items")

	if obj["command"] != "migrate-backup" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if message := obj["message"]; message != "backup migration completed" {
		t.Fatalf("unexpected message: %v", message)
	}
	if sourceScope := requireJSONPath(t, obj, "details", "source_scope"); sourceScope != "dev" {
		t.Fatalf("expected source_scope=dev, got %v", sourceScope)
	}
	if targetScope := requireJSONPath(t, obj, "details", "target_scope"); targetScope != "prod" {
		t.Fatalf("expected target_scope=prod, got %v", targetScope)
	}
	if selectionMode := requireJSONPath(t, obj, "details", "selection_mode"); selectionMode != "auto_latest_complete" {
		t.Fatalf("expected auto_latest_complete selection, got %v", selectionMode)
	}
	if completed, _ := requireJSONPath(t, obj, "details", "completed").(float64); int(completed) != 8 {
		t.Fatalf("unexpected completed count: %v", completed)
	}
	if startedTemporarily, _ := requireJSONPath(t, obj, "details", "started_db_temporarily").(bool); !startedTemporarily {
		t.Fatalf("expected started_db_temporarily=true")
	}

	for _, key := range []string{"manifest_json", "db_backup", "files_backup"} {
		path, _ := requireJSONPath(t, obj, "artifacts", key).(string)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s at %s: %v", key, path, err)
		}
	}

	items, ok := obj["items"].([]any)
	if !ok || len(items) != 8 {
		t.Fatalf("expected eight migrate-backup items, got %#v", obj["items"])
	}

	restoredContent, err := os.ReadFile(filepath.Join(storageDir, "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restoredContent) != "hello" {
		t.Fatalf("unexpected restored file content: %q", string(restoredContent))
	}
	if _, err := os.Stat(filepath.Join(storageDir, "before.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old storage tree to be replaced, stat err=%v", err)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if !containsAll(log,
		"compose --project-directory "+projectDir+" -f "+filepath.Join(projectDir, "compose.yaml")+" --env-file "+filepath.Join(projectDir, ".env.prod")+" up -d db",
		"compose --project-directory "+projectDir+" -f "+filepath.Join(projectDir, "compose.yaml")+" --env-file "+filepath.Join(projectDir, ".env.prod")+" stop espocrm espocrm-daemon espocrm-websocket",
		"exec mock-db mariadb --version",
		"exec -i -e MYSQL_PWD mock-db mariadb -u root",
		"compose --project-directory "+projectDir+" -f "+filepath.Join(projectDir, "compose.yaml")+" --env-file "+filepath.Join(projectDir, ".env.prod")+" up -d",
	) {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestSchema_MigrateBackup_JSON_Success_SkipDBNoStart(t *testing.T) {
	isolateRollbackPlanLocks(t)

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")
	stateDir := filepath.Join(tmp, "docker-state")
	logPath := filepath.Join(tmp, "docker.log")
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")

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
	writeUpdateRuntimeStatusFile(t, stateDir, "db", "healthy")

	writeDoctorEnvFile(t, projectDir, "dev", nil)
	writeDoctorEnvFile(t, projectDir, "prod", nil)
	writeRollbackBackupSet(t, filepath.Join(projectDir, "backups", "dev"), "espocrm-dev", "2026-04-19_08-00-00", "dev")

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_ROLLBACK_LOG", logPath)

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate-backup",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", projectDir,
		"--force",
		"--confirm-prod", "prod",
		"--skip-db",
		"--no-start",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if selectionMode := requireJSONPath(t, obj, "details", "selection_mode"); selectionMode != "auto_latest_files" {
		t.Fatalf("expected auto_latest_files selection, got %v", selectionMode)
	}
	if skipped, _ := requireJSONPath(t, obj, "details", "skipped").(float64); int(skipped) != 2 {
		t.Fatalf("expected two skipped steps, got %v", skipped)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if strings.Contains(log, "exec -i -e MYSQL_PWD mock-db mariadb -u root") {
		t.Fatalf("did not expect database restore commands in docker log:\n%s", log)
	}
	if strings.Contains(log, "compose --project-directory "+projectDir+" -f "+filepath.Join(projectDir, "compose.yaml")+" --env-file "+filepath.Join(projectDir, ".env.prod")+" up -d\n") {
		t.Fatalf("did not expect final contour start in docker log:\n%s", log)
	}
}

func TestSchema_MigrateBackup_JSON_Failure_CompatibilityDrift(t *testing.T) {
	isolateRollbackPlanLocks(t)

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeDoctorEnvFile(t, projectDir, "dev", map[string]string{
		"ESPOCRM_IMAGE": "espocrm/espocrm:9.3.4-apache",
	})
	writeDoctorEnvFile(t, projectDir, "prod", map[string]string{
		"ESPOCRM_IMAGE": "espocrm/espocrm:9.4.0-apache",
	})
	writeRollbackBackupSet(t, filepath.Join(projectDir, "backups", "dev"), "espocrm-dev", "2026-04-19_08-00-00", "dev")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"migrate-backup",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", projectDir,
		"--force",
		"--confirm-prod", "prod",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "migrate_backup_failed", "conflict with the migration compatibility contract")

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if message := obj["message"]; message != "backup migration failed" {
		t.Fatalf("unexpected message: %v", message)
	}
}
