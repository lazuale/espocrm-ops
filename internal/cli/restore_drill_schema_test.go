package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_RestoreDrill_JSON_Success_AutoSelection(t *testing.T) {
	fixture := prepareRestoreDrillFixture(t, "dev", map[string]string{
		"espo/data/drill.txt": "restore-drill",
	})
	useJournalClockForTest(t, fixture.fixedNow)

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", fixture.stateDir)
	t.Setenv("DOCKER_MOCK_ROLLBACK_LOG", fixture.logPath)
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

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "scope")
	requireJSONPath(t, obj, "details", "selection_mode")
	requireJSONPath(t, obj, "details", "timeout_seconds")
	requireJSONPath(t, obj, "details", "drill_app_port")
	requireJSONPath(t, obj, "details", "drill_ws_port")
	requireJSONPath(t, obj, "details", "services_ready")
	requireJSONPath(t, obj, "artifacts", "source_env_file")
	requireJSONPath(t, obj, "artifacts", "db_backup")
	requireJSONPath(t, obj, "artifacts", "files_backup")
	requireJSONPath(t, obj, "artifacts", "drill_env_file")
	requireJSONPath(t, obj, "artifacts", "report_txt")
	requireJSONPath(t, obj, "artifacts", "report_json")
	requireJSONPath(t, obj, "items")

	if obj["command"] != "restore-drill" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if selectionMode := requireJSONPath(t, obj, "details", "selection_mode"); selectionMode != "auto_latest_valid" {
		t.Fatalf("expected auto_latest_valid selection, got %v", selectionMode)
	}
	if drillAppPort, _ := requireJSONPath(t, obj, "details", "drill_app_port").(float64); int(drillAppPort) != 38080 {
		t.Fatalf("unexpected drill_app_port: %v", drillAppPort)
	}
	if drillWSPort, _ := requireJSONPath(t, obj, "details", "drill_ws_port").(float64); int(drillWSPort) != 38081 {
		t.Fatalf("unexpected drill_ws_port: %v", drillWSPort)
	}
	if selectedStamp := requireJSONPath(t, obj, "artifacts", "selected_stamp"); selectedStamp != "2026-04-19_08-00-00" {
		t.Fatalf("expected older complete set to be selected, got %v", selectedStamp)
	}

	warnings, ok := obj["warnings"].([]any)
	if !ok || len(warnings) < 3 {
		t.Fatalf("expected keep-artifacts warnings, got %#v", obj["warnings"])
	}

	reportTXT, _ := requireJSONPath(t, obj, "artifacts", "report_txt").(string)
	reportJSON, _ := requireJSONPath(t, obj, "artifacts", "report_json").(string)
	drillEnvFile, _ := requireJSONPath(t, obj, "artifacts", "drill_env_file").(string)
	for _, path := range []string{reportTXT, reportJSON, drillEnvFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact at %s: %v", path, err)
		}
	}

	for _, dir := range []string{
		filepath.Join(fixture.projectDir, "storage", "restore-drill", "dev", "db"),
		filepath.Join(fixture.projectDir, "storage", "restore-drill", "dev", "espo"),
		filepath.Join(fixture.projectDir, "backups", "restore-drill", "dev"),
	} {
		if stat, err := os.Stat(dir); err != nil || !stat.IsDir() {
			t.Fatalf("expected preserved restore-drill dir %s: stat=%v err=%v", dir, stat, err)
		}
	}

	if got, err := os.ReadFile(filepath.Join(fixture.projectDir, "storage", "restore-drill", "dev", "espo", "data", "drill.txt")); err != nil {
		t.Fatal(err)
	} else if string(got) != "restore-drill" {
		t.Fatalf("unexpected restored file content: %q", string(got))
	}

	rawLog, err := os.ReadFile(fixture.logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if !containsAll(log,
		"compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+drillEnvFile+" down",
		"compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+drillEnvFile+" up -d db",
		"exec -i -e MYSQL_PWD mock-db mariadb -u root",
		"image inspect espocrm/espocrm:9.3.4-apache",
		"compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+drillEnvFile+" up -d",
	) {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestSchema_RestoreDrill_JSON_Failure_HTTPProbe(t *testing.T) {
	fixture := prepareRestoreDrillFixture(t, "dev", map[string]string{
		"espo/data/probe.txt": "probe",
	})
	useJournalClockForTest(t, fixture.fixedNow)

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", fixture.stateDir)
	t.Setenv("DOCKER_MOCK_ROLLBACK_LOG", fixture.logPath)
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_GID", strconv.Itoa(os.Getgid()))

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-restore-drill-fail-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore-drill",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--timeout", "10",
	)
	if outcome.ExitCode != exitcode.RestoreError {
		t.Fatalf("expected restore exit code %d, got %d\nstdout=%s\nstderr=%s", exitcode.RestoreError, outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if ok, _ := obj["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %v", obj["ok"])
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "restore_drill_failed" {
		t.Fatalf("expected restore_drill_failed error code, got %v", code)
	}
	if message := obj["message"]; message != "restore drill failed" {
		t.Fatalf("unexpected message: %v", message)
	}

	items, ok := obj["items"].([]any)
	if !ok || len(items) != 7 {
		t.Fatalf("expected seven restore-drill items, got %#v", obj["items"])
	}
	last, ok := items[len(items)-1].(map[string]any)
	if !ok || last["code"] != "runtime_return" || last["status"] != "failed" {
		t.Fatalf("expected runtime_return failure, got %#v", items[len(items)-1])
	}
	if details, _ := last["details"].(string); !strings.Contains(details, "http probe failed") {
		t.Fatalf("expected HTTP probe failure details, got %q", details)
	}

	reportTXT, _ := requireJSONPath(t, obj, "artifacts", "report_txt").(string)
	reportJSON, _ := requireJSONPath(t, obj, "artifacts", "report_json").(string)
	for _, path := range []string{reportTXT, reportJSON} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected failure report at %s: %v", path, err)
		}
	}

	drillEnvFile, _ := requireJSONPath(t, obj, "artifacts", "drill_env_file").(string)
	if _, err := os.Stat(drillEnvFile); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove drill env file %s, stat err=%v", drillEnvFile, err)
	}
}
