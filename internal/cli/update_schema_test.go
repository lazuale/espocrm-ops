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

func TestSchema_Update_JSON_Success(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	stateDir := filepath.Join(tmp, "docker-state")
	logPath := filepath.Join(tmp, "docker.log")
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	composeFile := filepath.Join(projectDir, "compose.yaml")
	fixedNow := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
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
	if err := os.WriteFile(filepath.Join(storageDir, "file.txt"), []byte("update-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
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

	envFile := writeDoctorEnvFile(t, projectDir, "prod", map[string]string{
		"APP_PORT":      fmt.Sprintf("%d", appPort),
		"WS_PORT":       fmt.Sprintf("%d", wsPort),
		"SITE_URL":      "http://" + listener.Addr().String(),
		"WS_PUBLIC_URL": fmt.Sprintf("ws://127.0.0.1:%d", wsPort),
	})

	prependFakeDockerForUpdateCLITest(t)
	t.Setenv("DOCKER_MOCK_UPDATE_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_UPDATE_LOG", logPath)
	t.Setenv("DOCKER_MOCK_UPDATE_DUMP_STDOUT", "create table test(id int);\n")

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixedNow, "op-update-1")},
		"--journal-dir", journalDir,
		"--json",
		"update",
		"--scope", "prod",
		"--project-dir", projectDir,
		"--timeout", "10",
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
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "scope")
	requireJSONPath(t, obj, "details", "ready")
	requireJSONPath(t, obj, "details", "steps")
	requireJSONPath(t, obj, "details", "completed")
	requireJSONPath(t, obj, "details", "failed")
	requireJSONPath(t, obj, "artifacts", "project_dir")
	requireJSONPath(t, obj, "artifacts", "compose_file")
	requireJSONPath(t, obj, "artifacts", "env_file")
	requireJSONPath(t, obj, "artifacts", "manifest_json")
	requireJSONPath(t, obj, "timing", "started_at")
	requireJSONPath(t, obj, "timing", "finished_at")
	requireJSONPath(t, obj, "items")

	if obj["command"] != "update" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if message := obj["message"]; message != "update completed" {
		t.Fatalf("unexpected message: %v", message)
	}
	if ready, _ := requireJSONPath(t, obj, "details", "ready").(bool); !ready {
		t.Fatalf("expected ready=true, got %v", requireJSONPath(t, obj, "details", "ready"))
	}
	if completed, _ := requireJSONPath(t, obj, "details", "completed").(float64); int(completed) != 5 {
		t.Fatalf("unexpected completed count: %v", completed)
	}

	for _, key := range []string{"manifest_txt", "manifest_json", "db_backup", "files_backup", "db_checksum", "files_checksum"} {
		path, _ := requireJSONPath(t, obj, "artifacts", key).(string)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s at %s: %v", key, path, err)
		}
	}

	items, ok := obj["items"].([]any)
	if !ok || len(items) != 5 {
		t.Fatalf("expected five update items, got %#v", obj["items"])
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if !containsAll(log,
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" config",
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" stop espocrm espocrm-daemon espocrm-websocket",
		"exec -i -e MYSQL_PWD mock-db mariadb-dump -u espocrm espocrm --single-transaction --quick --routines --triggers --events",
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" pull",
		"compose --project-directory "+projectDir+" -f "+composeFile+" --env-file "+envFile+" up -d",
	) {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestSchema_Update_DryRun_JSON_Success(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")
	stateDir := filepath.Join(tmp, "docker-state")
	appPort := freeTCPPort(t)
	wsPort := freeTCPPort(t)

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
		"APP_PORT":      fmt.Sprintf("%d", appPort),
		"WS_PORT":       fmt.Sprintf("%d", wsPort),
		"SITE_URL":      fmt.Sprintf("http://127.0.0.1:%d", appPort),
		"WS_PUBLIC_URL": fmt.Sprintf("ws://127.0.0.1:%d", wsPort),
	})
	prependFakeDockerForUpdatePlanCLITest(t)
	t.Setenv("DOCKER_MOCK_PLAN_STATE_DIR", stateDir)

	out, err := runRootCommand(
		t,
		"--journal-dir", journalDir,
		"--json",
		"update",
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

	if obj["command"] != "update" {
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
}

func TestSchema_Update_JSON_FailureIncludesDetailedResult(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDoctorEnvFile(t, projectDir, "prod", map[string]string{
		"APP_PORT": "18080",
		"WS_PORT":  "18080",
	})
	prependDoctorFakeDocker(t)

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"update",
		"--scope", "prod",
		"--project-dir", projectDir,
	)

	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code, got %d stdout=%s stderr=%s", outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}
	if outcome.Stderr != "" {
		t.Fatalf("expected empty stderr, got %q", outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if command := requireJSONPath(t, obj, "command"); command != "update" {
		t.Fatalf("unexpected command: %v", command)
	}
	if ok, _ := requireJSONPath(t, obj, "ok").(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "update_failed" {
		t.Fatalf("unexpected error code: %v", code)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "validation" {
		t.Fatalf("unexpected error kind: %v", kind)
	}

	items, ok := obj["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected non-empty update items, got %#v", obj["items"])
	}

	foundDoctorFailure := false
	foundRuntimeNotRun := false
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		if item["code"] == "doctor" && item["status"] == "failed" {
			foundDoctorFailure = true
		}
		if item["code"] == "runtime_apply" && item["status"] == "not_run" {
			foundRuntimeNotRun = true
		}
	}
	if !foundDoctorFailure {
		t.Fatalf("expected doctor failure in %#v", items)
	}
	if !foundRuntimeNotRun {
		t.Fatalf("expected runtime_apply not_run in %#v", items)
	}
}
