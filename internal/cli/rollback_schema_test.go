package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_Rollback_JSON_Success(t *testing.T) {
	isolateRollbackPlanLocks(t)

	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	stateDir := filepath.Join(tmp, "docker-state")
	logPath := filepath.Join(tmp, "docker.log")
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	fixedNow := time.Date(2026, 4, 18, 13, 0, 0, 0, time.UTC)
	appPort := freeTCPPort(t)
	wsPort := freeTCPPort(t)

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
	writeUpdateRuntimeStatusFile(t, stateDir, "espocrm", "healthy")
	writeUpdateRuntimeStatusFile(t, stateDir, "espocrm-daemon", "healthy")
	writeUpdateRuntimeStatusFile(t, stateDir, "espocrm-websocket", "healthy")

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", appPort))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()

	writeDoctorEnvFile(t, projectDir, "prod", map[string]string{
		"APP_PORT":      fmt.Sprintf("%d", appPort),
		"WS_PORT":       fmt.Sprintf("%d", wsPort),
		"SITE_URL":      "http://" + listener.Addr().String(),
		"WS_PUBLIC_URL": fmt.Sprintf("ws://127.0.0.1:%d", wsPort),
	})
	writeRollbackBackupSet(t, filepath.Join(projectDir, "backups", "prod"), "espocrm-prod", "2026-04-18_10-00-00", "prod")

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_ROLLBACK_LOG", logPath)
	t.Setenv("DOCKER_MOCK_ROLLBACK_DUMP_STDOUT", "create table test(id int);\n")

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixedNow, "op-rollback-1")},
		"--journal-dir", journalDir,
		"--json",
		"rollback",
		"--scope", "prod",
		"--project-dir", projectDir,
		"--force",
		"--confirm-prod", "prod",
		"--timeout", "10",
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
	requireJSONPath(t, obj, "details", "scope")
	requireJSONPath(t, obj, "details", "ready")
	requireJSONPath(t, obj, "details", "selection_mode")
	requireJSONPath(t, obj, "details", "completed")
	requireJSONPath(t, obj, "details", "started_db_temporarily")
	requireJSONPath(t, obj, "artifacts", "manifest_json")
	requireJSONPath(t, obj, "artifacts", "snapshot_manifest_json")
	requireJSONPath(t, obj, "items")

	if obj["command"] != "rollback" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if message := obj["message"]; message != "rollback completed" {
		t.Fatalf("unexpected message: %v", message)
	}
	if ready, _ := requireJSONPath(t, obj, "details", "ready").(bool); !ready {
		t.Fatalf("expected ready=true, got %v", requireJSONPath(t, obj, "details", "ready"))
	}
	if selectionMode := requireJSONPath(t, obj, "details", "selection_mode"); selectionMode != "auto_latest_valid" {
		t.Fatalf("expected auto_latest_valid selection, got %v", selectionMode)
	}
	if completed, _ := requireJSONPath(t, obj, "details", "completed").(float64); int(completed) != 8 {
		t.Fatalf("unexpected completed count: %v", completed)
	}
	if startedTemporarily, _ := requireJSONPath(t, obj, "details", "started_db_temporarily").(bool); !startedTemporarily {
		t.Fatalf("expected started_db_temporarily=true")
	}

	for _, key := range []string{
		"manifest_json",
		"db_backup",
		"files_backup",
		"snapshot_manifest_json",
		"snapshot_db_backup",
		"snapshot_files_backup",
		"snapshot_db_checksum",
		"snapshot_files_checksum",
	} {
		path, _ := requireJSONPath(t, obj, "artifacts", key).(string)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s at %s: %v", key, path, err)
		}
	}

	items, ok := obj["items"].([]any)
	if !ok || len(items) != 8 {
		t.Fatalf("expected eight rollback items, got %#v", obj["items"])
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
		"exec -i -e MYSQL_PWD mock-db mariadb-dump -u espocrm espocrm --single-transaction --quick --routines --triggers --events",
		"exec mock-db mariadb --version",
		"exec -i -e MYSQL_PWD mock-db mariadb -u root",
		"compose --project-directory "+projectDir+" -f "+filepath.Join(projectDir, "compose.yaml")+" --env-file "+filepath.Join(projectDir, ".env.prod")+" up -d",
	) {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestSchema_Rollback_DryRun_JSON_Success(t *testing.T) {
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
		"rollback",
		"--scope", "prod",
		"--project-dir", projectDir,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	if obj["command"] != "rollback" {
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
}

func TestSchema_Rollback_JSON_Failure_NoValidBackupSet(t *testing.T) {
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

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", stateDir)

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"rollback",
		"--scope", "prod",
		"--project-dir", projectDir,
		"--force",
		"--confirm-prod", "prod",
	)

	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code, got %d stdout=%s stderr=%s", outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if code := requireJSONPath(t, obj, "error", "code"); code != "rollback_failed" {
		t.Fatalf("expected rollback_failed error code, got %v", code)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "validation" {
		t.Fatalf("expected validation error kind, got %v", kind)
	}

	items, ok := requireJSONPath(t, obj, "items").([]any)
	if !ok || len(items) != 8 {
		t.Fatalf("expected eight rollback items, got %#v", obj["items"])
	}

	item := items[2].(map[string]any)
	if item["code"] != "target_selection" || item["status"] != "failed" {
		t.Fatalf("unexpected target selection item: %#v", item)
	}
}
